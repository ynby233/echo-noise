package music

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/rcy1314/echo-noise/internal/models"
)

const mediaFormatWhitelist = "mp3,ogg,flac,aac,mov,wav,ape,asf,aiff,tak"
const mediaEncodingVersion = "mp3-192k-44100-stereo-v1"
const mediaCoverInputLimit int64 = 16 << 20

func mediaDemuxer(extension string) (string, error) {
	extension = strings.ToLower(strings.TrimPrefix(extension, "."))
	switch extension {
	case "mp3", "flac", "aac", "wav", "ape", "tak":
		return extension, nil
	case "ogg", "oga", "opus":
		return "ogg", nil
	case "m4a", "alac":
		return "mov", nil
	case "wma":
		return "asf", nil
	case "aif", "aiff":
		return "aiff", nil
	default:
		return "", ErrInvalid
	}
}

func mediaInputArgs(input, extension string) ([]string, error) {
	demuxer, err := mediaDemuxer(extension)
	if err != nil || input == "" || strings.ContainsRune(input, 0) {
		return nil, ErrInvalid
	}
	args := []string{"-f", demuxer, "-format_whitelist", mediaFormatWhitelist, "-protocol_whitelist", "file,pipe"}
	if demuxer == "mov" {
		args = append(args, "-enable_drefs", "0", "-use_absolute_path", "0")
	}
	return append(args, "-i", input), nil
}

// ResolveRepresentation describes actual response bytes. Codec checks prevent
// an ALAC-in-M4A or 24-bit WAV source from being advertised as browser playable.
func ResolveRepresentation(track models.MusicTrack, compat bool) Representation {
	format := strings.ToLower(strings.TrimPrefix(track.Format, "."))
	if format == "" {
		format = strings.ToLower(strings.TrimPrefix(filepath.Ext(track.RelativePath), "."))
	}
	codec := strings.ToLower(track.Codec)
	mime := ""
	if !compat && track.CueTrackNumber == 0 && track.CueRelativePath == "" {
		switch format {
		case "mp3":
			if codec == "mp3" {
				mime = "audio/mpeg"
			}
		case "flac":
			if codec == "flac" {
				mime = "audio/flac"
			}
		case "ogg", "oga":
			if codec == "vorbis" || codec == "opus" {
				mime = "audio/ogg"
			}
		case "opus":
			if codec == "opus" {
				mime = "audio/ogg"
			}
		case "aac":
			if codec == "aac" {
				mime = "audio/aac"
			}
		case "m4a":
			if codec == "aac" {
				mime = "audio/mp4"
			}
		case "wav":
			if codec == "pcm_s16le" {
				mime = "audio/wav"
			}
		}
	}
	derived := mime == ""
	version := "original-v1"
	if derived {
		mime, version = "audio/mpeg", mediaEncodingVersion
	}
	identity := fmt.Sprintf("%s\x00%s\x00%d\x00%d\x00%d\x00%s", track.ID, track.Fingerprint, track.CueTrackNumber, track.StartFrames, track.EndFrames, version)
	hash := sha256.Sum256([]byte(identity))
	return Representation{MIME: mime, Derived: derived, Key: hex.EncodeToString(hash[:])}
}

func mediaFramesSeconds(frames int64) string {
	return strconv.FormatFloat(float64(frames)/75, 'f', 9, 64)
}

// BuildMediaArgs forces the known container before opening a verified input.
// CUE uses an input seek followed by a zero-based duration trim; no neighbouring
// segment is copied into the returned, fully generated representation.
func BuildMediaArgs(input string, extension string, track models.MusicTrack, output string) ([]string, error) {
	if output == "" || strings.ContainsRune(output, 0) {
		return nil, ErrInvalid
	}
	inputArgs, err := mediaInputArgs(input, extension)
	if err != nil {
		return nil, err
	}
	args := []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-y"}
	cue := track.CueTrackNumber != 0 || track.CueRelativePath != ""
	if cue {
		if track.CueTrackNumber <= 0 || track.StartFrames < 0 || track.EndFrames <= track.StartFrames {
			return nil, ErrInvalid
		}
		// Limit arithmetic and reject bounds beyond known source duration when
		// DurationMS is the logical segment duration (as stored by the scanner).
		if track.EndFrames > math.MaxInt64/1000 {
			return nil, ErrInvalid
		}
		args = append(args, "-ss", mediaFramesSeconds(track.StartFrames))
	}
	args = append(args, inputArgs...)
	args = append(args, "-map", "0:a:0")
	if cue {
		duration := mediaFramesSeconds(track.EndFrames - track.StartFrames)
		args = append(args, "-af", "atrim=duration="+duration+",asetpts=PTS-STARTPTS", "-t", duration)
	}
	return append(args, "-c:a", "libmp3lame", "-b:a", "192k", "-ar", "44100", "-ac", "2", "-map_metadata", "-1", "-map_chapters", "-1", "-vn", "-sn", "-dn", "-f", "mp3", output), nil
}

func (s *Service) runMediaTool(ctx context.Context, args []string, input ToolInput, output io.Writer) error {
	select {
	case s.mediaSlots <- struct{}{}:
		defer func() { <-s.mediaSlots }()
	case <-ctx.Done():
		return ctx.Err()
	}
	return s.toolRun(ctx, s.opts.FFmpegPath, args, input, output)
}

func mediaName(track models.MusicTrack, cover bool, derived bool) string {
	name := strings.TrimSpace(track.Title)
	name = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 || r == '/' || r == '\\' {
			return '-'
		}
		return r
	}, name)
	if name == "" {
		name = "music"
	}
	if len([]rune(name)) > 160 {
		name = string([]rune(name)[:160])
	}
	if cover {
		return name + ".jpg"
	}
	if derived {
		return name + ".mp3"
	}
	return name + strings.ToLower(filepath.Ext(track.RelativePath))
}

func (s *Service) mediaStillEligible(ctx context.Context, track models.MusicTrack, admin bool) error {
	current, err := s.checkTrack(ctx, track.ID, admin)
	if err != nil {
		return err
	}
	if current.Fingerprint != track.Fingerprint || current.RelativePath != track.RelativePath || current.CueRelativePath != track.CueRelativePath || current.StartFrames != track.StartFrames || current.EndFrames != track.EndFrames || current.CoverKind != track.CoverKind || current.CoverRelativePath != track.CoverRelativePath || current.CoverStreamIndex != track.CoverStreamIndex || !s.fingerprintFresh(current) {
		return ErrNotFound
	}
	return nil
}

// OpenMedia checks eligibility before any generation and again immediately
// before returning the descriptor used by the handler's ServeContent call.
func (s *Service) OpenMedia(ctx context.Context, trackID string, kind string, compat bool, admin bool) (*MediaFile, error) {
	if (kind != "stream" && kind != "cover") || (admin && kind != "cover") {
		return nil, ErrInvalid
	}
	track, err := s.checkTrack(ctx, trackID, admin)
	if err != nil {
		return nil, err
	}
	if !s.fingerprintFresh(track) {
		return nil, ErrNotFound
	}
	var result *MediaFile
	if kind == "cover" {
		if track.CoverKind != "embedded" && track.CoverKind != "sidecar" {
			return nil, ErrNotFound
		}
		rep := ResolveRepresentation(track, false)
		result, err = s.cache.acquire(ctx, "cover-jpeg-512-v1:"+rep.Key, true, "image/jpeg", mediaName(track, true, true), func(buildCtx context.Context, output *os.File) error {
			if err := s.mediaStillEligible(buildCtx, track, admin); err != nil {
				return err
			}
			return s.buildCover(buildCtx, track, output)
		})
	} else {
		rep := ResolveRepresentation(track, compat)
		if rep.Derived {
			result, err = s.cache.acquire(ctx, "audio:"+rep.Key, false, rep.MIME, mediaName(track, false, true), func(buildCtx context.Context, output *os.File) error {
				if err := s.mediaStillEligible(buildCtx, track, false); err != nil {
					return err
				}
				file, err := s.secureOpen(track.RelativePath)
				if err != nil {
					return ErrNotFound
				}
				defer file.Close()
				args, err := BuildMediaArgs(inputPath(file), filepath.Ext(track.RelativePath), track, "pipe:1")
				if err != nil {
					return err
				}
				writer := &boundedMediaWriter{writer: output, remaining: mediaAudioLimit}
				return s.runMediaTool(buildCtx, args, ToolInput{File: file}, writer)
			})
		} else {
			var file *os.File
			file, err = s.secureOpen(track.RelativePath)
			if err == nil {
				var info os.FileInfo
				info, err = file.Stat()
				if err == nil && info.Mode().IsRegular() && info.Size() == track.FileSize && info.ModTime().UnixNano() == track.ModifiedNS {
					result = &MediaFile{File: file, Info: info, MIME: rep.MIME, ETag: "\"" + rep.Key + "\"", Name: mediaName(track, false, false)}
				} else {
					_ = file.Close()
					err = ErrNotFound
				}
			}
		}
	}
	if err != nil {
		return nil, err
	}
	if err = s.mediaStillEligible(ctx, track, admin); err != nil {
		_ = result.Close()
		return nil, err
	}
	return result, nil
}

func mediaImageDimensions(data []byte) (int, int, error) {
	if config, _, err := image.DecodeConfig(bytes.NewReader(data)); err == nil {
		return config.Width, config.Height, nil
	}
	// WebP is validated without loading a full image or adding a decoder
	// dependency. The restricted image2pipe tool performs the actual decode.
	if len(data) < 30 || string(data[:4]) != "RIFF" || string(data[8:12]) != "WEBP" {
		return 0, 0, ErrNotFound
	}
	if uint64(binary.LittleEndian.Uint32(data[4:8]))+8 != uint64(len(data)) {
		return 0, 0, ErrNotFound
	}
	switch string(data[12:16]) {
	case "VP8X":
		return 1 + int(data[24]) + int(data[25])<<8 + int(data[26])<<16, 1 + int(data[27]) + int(data[28])<<8 + int(data[29])<<16, nil
	case "VP8 ":
		if string(data[23:26]) != "\x9d\x01\x2a" {
			return 0, 0, ErrNotFound
		}
		return int(binary.LittleEndian.Uint16(data[26:28]) & 0x3fff), int(binary.LittleEndian.Uint16(data[28:30]) & 0x3fff), nil
	case "VP8L":
		if data[20] != 0x2f {
			return 0, 0, ErrNotFound
		}
		bits := binary.LittleEndian.Uint32(data[21:25])
		return int(bits&0x3fff) + 1, int((bits>>14)&0x3fff) + 1, nil
	}
	return 0, 0, ErrNotFound
}

func (s *Service) renderCover(ctx context.Context, data []byte, output *os.File) error {
	if int64(len(data)) > mediaCoverInputLimit {
		return ErrNotFound
	}
	width, height, err := mediaImageDimensions(data)
	if err != nil || width <= 0 || height <= 0 || width > 16384 || height > 16384 {
		return ErrNotFound
	}
	args := []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-f", "image2pipe", "-format_whitelist", "image2pipe", "-protocol_whitelist", "pipe", "-i", "pipe:0", "-map", "0:v:0", "-vf", "thumbnail,scale=w='min(512,iw)':h='min(512,ih)':force_original_aspect_ratio=decrease", "-frames:v", "1", "-c:v", "mjpeg", "-q:v", "3", "-map_metadata", "-1", "-map_chapters", "-1", "-an", "-sn", "-dn", "-f", "image2pipe", "pipe:1"}
	var encoded bytes.Buffer
	writer := &boundedMediaWriter{writer: &encoded, remaining: mediaCoverLimit}
	if err := s.runMediaTool(ctx, args, ToolInput{Stdin: bytes.NewReader(data)}, writer); err != nil {
		return ErrNotFound
	}
	config, err := jpeg.DecodeConfig(bytes.NewReader(encoded.Bytes()))
	if err != nil || config.Width <= 0 || config.Height <= 0 || config.Width > 512 || config.Height > 512 {
		return ErrNotFound
	}
	// Decode the bounded final frame and re-encode to drop all source metadata
	// and reject truncated JPEG output before publication.
	decoded, err := jpeg.Decode(bytes.NewReader(encoded.Bytes()))
	if err != nil {
		return ErrNotFound
	}
	if err := output.Truncate(0); err != nil {
		return err
	}
	if _, err := output.Seek(0, io.SeekStart); err != nil {
		return err
	}
	return jpeg.Encode(&boundedMediaWriter{writer: output, remaining: mediaCoverLimit}, decoded, &jpeg.Options{Quality: 85})
}

func (s *Service) buildCover(ctx context.Context, track models.MusicTrack, output *os.File) error {
	if track.CoverKind == "embedded" && track.CoverStreamIndex >= 0 {
		file, err := s.secureOpen(track.RelativePath)
		if err == nil {
			inputArgs, argsErr := mediaInputArgs(inputPath(file), filepath.Ext(track.RelativePath))
			if argsErr == nil {
				args := append([]string{"-hide_banner", "-loglevel", "error", "-nostdin"}, inputArgs...)
				args = append(args, "-map", "0:"+strconv.Itoa(track.CoverStreamIndex), "-frames:v", "1", "-c:v", "copy", "-an", "-sn", "-dn", "-map_metadata", "-1", "-f", "image2pipe", "pipe:1")
				var imageBytes bytes.Buffer
				err = s.runMediaTool(ctx, args, ToolInput{File: file}, &boundedMediaWriter{writer: &imageBytes, remaining: mediaCoverInputLimit})
				if err == nil {
					err = s.renderCover(ctx, imageBytes.Bytes(), output)
				}
			}
			_ = file.Close()
			if argsErr == nil && err == nil {
				return nil
			}
		}
	}
	candidates, err := s.coverCandidates(ctx, track.RelativePath)
	if err != nil {
		return ErrNotFound
	}
	for _, candidate := range candidates {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		data, err := s.secureRead(ctx, candidate, mediaCoverInputLimit)
		if err == nil && s.renderCover(ctx, data, output) == nil {
			return nil
		}
	}
	return ErrNotFound
}
