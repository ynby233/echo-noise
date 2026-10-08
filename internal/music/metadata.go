package music

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/rcy1314/echo-noise/internal/models"
)

const probeOutputLimit int64 = 1 << 20

type commandRunner struct{}

func inputPath(file *os.File) string {
	if runtime.GOOS == "linux" {
		return "/proc/self/fd/3"
	}
	if runtime.GOOS == "windows" {
		return file.Name()
	}
	return "/dev/fd/3"
}
func (commandRunner) Run(ctx context.Context, executable string, args []string, input ToolInput, stdout io.Writer) error {
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.Stdout = stdout
	cmd.WaitDelay = 2 * time.Second
	if input.File != nil {
		if runtime.GOOS == "windows" {
			verified, err := secureOpen(filepath.Dir(input.File.Name()), filepath.Base(input.File.Name()))
			if err != nil {
				return err
			}
			before, e := input.File.Stat()
			after, e2 := verified.Stat()
			verified.Close()
			if e != nil || e2 != nil || !os.SameFile(before, after) {
				return ErrNotFound
			}
		} else {
			cmd.ExtraFiles = []*os.File{input.File}
		}
	} else {
		cmd.Stdin = input.Stdin
	}
	// Tool diagnostics may include source paths and are never returned to clients.
	cmd.Stderr = io.Discard
	return cmd.Run()
}
func (s *Service) toolRun(ctx context.Context, executable string, args []string, input ToolInput, stdout io.Writer) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	runner := s.opts.Tools
	if runner == nil {
		runner = commandRunner{}
	}
	return runner.Run(ctx, executable, args, input, stdout)
}
func (s *Service) checkTools(ctx context.Context) error {
	s.mu.Lock()
	ready := s.toolsReady
	s.mu.Unlock()
	if ready {
		return nil
	}
	toolCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	for _, exe := range []string{s.opts.FFprobePath, s.opts.FFmpegPath} {
		var out bytes.Buffer
		err := s.toolRun(toolCtx, exe, []string{"-version"}, ToolInput{}, &boundedMediaWriter{writer: &out, remaining: 64 << 10})
		if err != nil {
			code := "tool_error"
			if errors.Is(err, exec.ErrNotFound) || errors.Is(err, os.ErrNotExist) {
				code = "tool_missing"
			}
			s.mu.Lock()
			s.toolsReady = false
			s.toolError = code
			s.mu.Unlock()
			return &scanFailure{code: code, err: err}
		}
	}
	if err := s.checkToolCapabilities(toolCtx); err != nil {
		s.mu.Lock()
		s.toolsReady = false
		s.toolError = "tool_error"
		s.mu.Unlock()
		return &scanFailure{code: "tool_error", err: err}
	}
	s.mu.Lock()
	s.toolsReady = true
	s.toolError = ""
	s.mu.Unlock()
	return nil
}

type Metadata struct {
	Title, Artist, Album, Codec, Format, MIMEType string
	DurationMS                                    int64
	Tags                                          map[string]string
	CoverStreamIndex                              int
}
type probeStream struct {
	Index       int               `json:"index"`
	CodecName   string            `json:"codec_name"`
	CodecType   string            `json:"codec_type"`
	Duration    string            `json:"duration"`
	Tags        map[string]string `json:"tags"`
	Disposition struct {
		AttachedPic int `json:"attached_pic"`
	} `json:"disposition"`
}
type probeResult struct {
	Streams []probeStream `json:"streams"`
	Format  struct {
		Duration string            `json:"duration"`
		Tags     map[string]string `json:"tags"`
	} `json:"format"`
}

func BuildProbeArgs(input, extension string) ([]string, error) {
	args, err := mediaInputArgs(input, extension)
	if err != nil {
		return nil, err
	}
	prefix := []string{"-hide_banner", "-loglevel", "error", "-probesize", "8388608", "-analyzeduration", "10000000"}
	prefix = append(prefix, args...)
	return append(prefix, "-show_entries", "format=duration:format_tags:stream=index,codec_name,codec_type,duration:stream_tags:stream_disposition=attached_pic", "-of", "json"), nil
}
func durationMS(value string) int64 {
	v, e := strconv.ParseFloat(value, 64)
	if e != nil || math.IsNaN(v) || math.IsInf(v, 0) || v <= 0 || v > float64(math.MaxInt64/75000) {
		return 0
	}
	return int64(v * 1000)
}
func formatMIME(format string) string {
	switch format {
	case "mp3":
		return "audio/mpeg"
	case "flac":
		return "audio/flac"
	case "ogg", "oga", "opus":
		return "audio/ogg"
	case "aac":
		return "audio/aac"
	case "m4a", "alac":
		return "audio/mp4"
	case "wav":
		return "audio/wav"
	case "wma":
		return "audio/x-ms-wma"
	case "aif", "aiff":
		return "audio/aiff"
	case "ape":
		return "audio/x-ape"
	default:
		return "application/octet-stream"
	}
}
func (s *Service) probe(ctx context.Context, relative string) (Metadata, error) {
	result := Metadata{Tags: map[string]string{}, CoverStreamIndex: -1, Format: strings.ToLower(strings.TrimPrefix(filepath.Ext(relative), "."))}
	result.MIMEType = formatMIME(result.Format)
	file, err := s.secureOpen(relative)
	if err != nil {
		return result, err
	}
	defer file.Close()
	args, err := BuildProbeArgs(inputPath(file), filepath.Ext(relative))
	if err != nil {
		return result, err
	}
	probeCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	var output bytes.Buffer
	if err = s.toolRun(probeCtx, s.opts.FFprobePath, args, ToolInput{File: file}, &boundedMediaWriter{writer: &output, remaining: probeOutputLimit}); err != nil {
		return result, err
	}
	var raw probeResult
	if err = json.Unmarshal(output.Bytes(), &raw); err != nil {
		return result, err
	}
	for key, value := range raw.Format.Tags {
		result.Tags[strings.ToLower(key)] = value
	}
	result.DurationMS = durationMS(raw.Format.Duration)
	audio := false
	for _, stream := range raw.Streams {
		if stream.CodecType == "audio" && !audio {
			audio = true
			result.Codec = stream.CodecName
			if result.DurationMS == 0 {
				result.DurationMS = durationMS(stream.Duration)
			}
			for key, value := range stream.Tags {
				result.Tags[strings.ToLower(key)] = value
			}
		}
		if stream.CodecType == "video" && stream.Disposition.AttachedPic == 1 && stream.Index >= 0 && result.CoverStreamIndex < 0 {
			result.CoverStreamIndex = stream.Index
		}
	}
	if !audio || result.Codec == "" || result.DurationMS <= 0 {
		return result, ErrUnprocessable
	}
	for key, value := range result.Tags {
		if !strings.Contains(key, "lyrics") {
			runes := []rune(value)
			if len(runes) > 512 {
				result.Tags[key] = string(runes[:512])
			}
		}
	}
	result.Title = result.Tags["title"]
	result.Artist = result.Tags["artist"]
	if result.Artist == "" {
		result.Artist = result.Tags["album_artist"]
	}
	result.Album = result.Tags["album"]
	return result, nil
}
func (s *Service) embeddedLyrics(ctx context.Context, track models.MusicTrack) (string, error) {
	metadata, err := s.probe(ctx, track.RelativePath)
	if err != nil {
		return "", err
	}
	keys := make([]string, 0, len(metadata.Tags))
	for key := range metadata.Tags {
		if key != "lyrics" && strings.Contains(key, "lyrics") {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	keys = append([]string{"lyrics"}, keys...)
	for _, key := range keys {
		value := metadata.Tags[key]
		if value != "" {
			if int64(len(value)) > lyricsTextLimit {
				return "", ErrInvalid
			}
			return value, nil
		}
	}
	return "", ErrNotFound
}
func (s *Service) checkToolCapabilities(ctx context.Context) error {
	var encoders bytes.Buffer
	if err := s.toolRun(ctx, s.opts.FFmpegPath, []string{"-hide_banner", "-encoders"}, ToolInput{}, &boundedMediaWriter{writer: &encoders, remaining: probeOutputLimit}); err != nil {
		return err
	}
	if !strings.Contains(encoders.String(), "libmp3lame") {
		return ErrUnavailable
	}
	wav := append([]byte{'R', 'I', 'F', 'F', 0x44, 3, 0, 0, 'W', 'A', 'V', 'E', 'f', 'm', 't', ' ', 16, 0, 0, 0, 1, 0, 1, 0, 0x40, 0x1f, 0, 0, 0x80, 0x3e, 0, 0, 2, 0, 16, 0, 'd', 'a', 't', 'a', 0x20, 3, 0, 0}, make([]byte, 800)...)
	for _, format := range []string{"wav", "mp3", "m4a"} {
		args := []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-f", "wav", "-format_whitelist", mediaFormatWhitelist, "-protocol_whitelist", "pipe", "-i", "pipe:0", "-map", "0:a:0", "-vn", "-sn", "-dn"}
		switch format {
		case "wav":
			args = append(args, "-c:a", "pcm_s16le", "-f", "wav")
		case "mp3":
			args = append(args, "-c:a", "libmp3lame", "-f", "mp3")
		case "m4a":
			args = append(args, "-c:a", "aac", "-movflags", "frag_keyframe+empty_moov", "-f", "mp4")
		}
		args = append(args, "pipe:1")
		var encoded bytes.Buffer
		if err := s.toolRun(ctx, s.opts.FFmpegPath, args, ToolInput{Stdin: bytes.NewReader(wav)}, &boundedMediaWriter{writer: &encoded, remaining: probeOutputLimit}); err != nil {
			return err
		}
		probeArgs, err := BuildProbeArgs("pipe:0", format)
		if err != nil {
			return err
		}
		var result bytes.Buffer
		if err = s.toolRun(ctx, s.opts.FFprobePath, probeArgs, ToolInput{Stdin: bytes.NewReader(encoded.Bytes())}, &boundedMediaWriter{writer: &result, remaining: probeOutputLimit}); err != nil {
			return err
		}
		var metadata probeResult
		if err = json.Unmarshal(result.Bytes(), &metadata); err != nil {
			return err
		}
		audio := false
		for _, stream := range metadata.Streams {
			if stream.CodecType == "audio" && stream.CodecName != "" {
				audio = true
			}
		}
		if !audio {
			return ErrUnavailable
		}
	}
	return nil
}
