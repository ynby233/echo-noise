package music

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/rcy1314/echo-noise/internal/models"
)

const maxWalkDepth = 64
const maxScanEntries = 100000

var errSkipDirectory = errors.New("skip directory")
var errScanLimit = errors.New("scan limit exceeded")

type scanFailure struct {
	code string
	err  error
}

func (e *scanFailure) Error() string { return e.code }
func (e *scanFailure) Unwrap() error { return e.err }
func scanCode(err error) string {
	var failure *scanFailure
	if errors.As(err, &failure) {
		return failure.code
	}
	return "io_error"
}

func secureParts(relative string) ([]string, error) {
	if relative == "" || strings.ContainsAny(relative, "\\\x00:") || strings.HasPrefix(relative, "/") || filepath.IsAbs(relative) {
		return nil, ErrNotFound
	}
	parts := strings.Split(relative, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return nil, ErrNotFound
		}
	}
	return parts, nil
}
func secureOpen(root, relative string) (*os.File, error) {
	dir, err := openRoot(root)
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	return openAt(dir, relative, false)
}
func secureRead(root, relative string, limit int64) ([]byte, error) {
	if limit < 0 || limit > 64<<20 {
		return nil, ErrInvalid
	}
	file, err := secureOpen(root, relative)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() > limit {
		return nil, ErrInvalid
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, ErrInvalid
	}
	return data, nil
}
func (s *Service) secureOpen(relative string) (*os.File, error) {
	if s.opts.BeforeOpen != nil {
		s.opts.BeforeOpen(relative)
	}
	return secureOpen(s.opts.RootDir, relative)
}
func (s *Service) secureRead(ctx context.Context, relative string, limit int64) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s.opts.BeforeOpen != nil {
		s.opts.BeforeOpen(relative)
	}
	return secureRead(s.opts.RootDir, relative, limit)
}
func (s *Service) directoryEntries(ctx context.Context, relative string) ([]os.DirEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	root, err := openRoot(s.opts.RootDir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	dir := root
	if relative != "" && relative != "." {
		dir, err = openAt(root, relative, true)
		if err != nil {
			return nil, err
		}
		defer dir.Close()
	}
	entries, err := dir.ReadDir(maxScanEntries + 1)
	if len(entries) > maxScanEntries {
		return nil, errScanLimit
	}
	if errors.Is(err, io.EOF) {
		err = nil
	}
	return entries, err
}
func (s *Service) coverCandidates(ctx context.Context, relative string) ([]string, error) {
	entries, err := s.directoryEntries(ctx, path.Dir(relative))
	if err != nil {
		return nil, err
	}
	base := strings.TrimSuffix(path.Base(relative), path.Ext(relative))
	var same, album []string
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		name := entry.Name()
		ext := strings.ToLower(path.Ext(name))
		if !oneOf(ext, ".jpg", ".jpeg", ".png", ".webp") {
			continue
		}
		stem := strings.TrimSuffix(name, path.Ext(name))
		rel := path.Join(path.Dir(relative), name)
		if strings.EqualFold(stem, base) {
			same = append(same, rel)
		} else if oneOf(strings.ToLower(stem), "cover", "folder", "front", "album") {
			album = append(album, rel)
		}
	}
	sort.Strings(same)
	sort.Strings(album)
	return append(same, album...), nil
}
func (s *Service) lyricsCandidates(ctx context.Context, relative string) ([]string, error) {
	entries, err := s.directoryEntries(ctx, path.Dir(relative))
	if err != nil {
		return nil, err
	}
	base := strings.TrimSuffix(path.Base(relative), path.Ext(relative))
	var result []string
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		name := entry.Name()
		if oneOf(strings.ToLower(path.Ext(name)), ".lrc", ".srt", ".txt") && strings.EqualFold(strings.TrimSuffix(name, path.Ext(name)), base) {
			result = append(result, path.Join(path.Dir(relative), name))
		}
	}
	rank := map[string]int{".lrc": 0, ".srt": 1, ".txt": 2}
	sort.Slice(result, func(i, j int) bool {
		a, b := rank[strings.ToLower(path.Ext(result[i]))], rank[strings.ToLower(path.Ext(result[j]))]
		if a != b {
			return a < b
		}
		return result[i] < result[j]
	})
	return result, nil
}
func (s *Service) trackFingerprint(track models.MusicTrack) (string, error) {
	covers, err := s.coverCandidates(context.Background(), track.RelativePath)
	if err != nil {
		return "", err
	}
	lyrics, err := s.lyricsCandidates(context.Background(), track.RelativePath)
	if err != nil {
		return "", err
	}
	paths := append([]string{track.RelativePath}, covers...)
	paths = append(paths, lyrics...)
	if track.CueRelativePath != "" {
		paths = append(paths, track.CueRelativePath)
	}
	h := sha256.New()
	fmt.Fprintf(h, "stat-v1\x00%s\x00%d\x00%d\x00%d\x00", track.CueRelativePath, track.CueTrackNumber, track.StartFrames, track.EndFrames)
	for _, relative := range paths {
		file, e := s.secureOpen(relative)
		if e != nil {
			return "", e
		}
		info, e := file.Stat()
		file.Close()
		if e != nil {
			return "", e
		}
		if relative == track.RelativePath && (info.Size() != track.FileSize || info.ModTime().UnixNano() != track.ModifiedNS) {
			return "", ErrNotFound
		}
		fmt.Fprintf(h, "%s\x00%d\x00%d\x00", relative, info.Size(), info.ModTime().UnixNano())
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func (s *Service) fingerprintFresh(track models.MusicTrack) bool {
	fingerprint, err := s.trackFingerprint(track)
	return err == nil && track.Fingerprint != "" && fingerprint == track.Fingerprint
}
func catalogKey(relative, cue string, number int) string {
	hash := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%d", relative, cue, number)))
	return hex.EncodeToString(hash[:])
}
func (s *Service) excluded(relative string) bool {
	candidate := filepath.Join(s.opts.RootDir, filepath.FromSlash(relative))
	roots := append([]string{s.opts.CacheDir}, s.opts.ExcludedRoots...)
	candidateAbs, err := filepath.Abs(candidate)
	if err != nil {
		return true
	}
	for _, root := range roots {
		if root == "" {
			continue
		}
		abs, e := filepath.Abs(root)
		if e == nil && cacheContains(abs, candidateAbs) {
			return true
		}
	}
	return false
}
func (s *Service) collectScan(ctx context.Context, runID string, old []models.MusicTrack) ([]models.MusicTrack, ScanStatus, error) {
	status := ScanStatus{RunID: runID, State: "running"}
	root, err := openRoot(s.opts.RootDir)
	if err != nil {
		code := "root_unreadable"
		if errors.Is(err, os.ErrNotExist) {
			code = "root_missing"
		}
		return nil, status, &scanFailure{code: code, err: err}
	}
	defer root.Close()
	if err = s.checkTools(ctx); err != nil {
		return nil, status, err
	}
	sources := map[string]os.FileInfo{}
	var cues []string
	count := 0
	err = secureWalk(ctx, root, func(relative string, info os.FileInfo) error {
		count++
		if count > maxScanEntries {
			return errScanLimit
		}
		if s.excluded(relative) {
			if info.IsDir() {
				return errSkipDirectory
			}
			return nil
		}
		if info.IsDir() {
			return nil
		}
		ext := strings.ToLower(path.Ext(relative))
		if ext == ".cue" {
			cues = append(cues, relative)
		} else if _, e := mediaDemuxer(ext); e == nil {
			sources[relative] = info
		}
		return nil
	})
	if err != nil {
		return nil, status, err
	}
	sort.Strings(cues)
	prior := map[string]models.MusicTrack{}
	for _, track := range old {
		prior[track.CatalogKey] = track
	}
	metadata := map[string]Metadata{}
	probeErrors := map[string]error{}
	getMetadata := func(relative string) (Metadata, error) {
		if value, ok := metadata[relative]; ok {
			return value, nil
		}
		if e, ok := probeErrors[relative]; ok {
			return Metadata{}, e
		}
		key := catalogKey(relative, "", 0)
		if track, ok := prior[key]; ok && track.Available && s.fingerprintFresh(track) {
			m := Metadata{Title: track.Title, Artist: track.Artist, Album: track.Album, Codec: track.Codec, Format: track.Format, MIMEType: track.MIMEType, DurationMS: track.DurationMS, CoverStreamIndex: track.CoverStreamIndex, Tags: map[string]string{}}
			if track.LyricsKind == "embedded" {
				m.Tags[track.LyricsTag] = "cached"
			}
			metadata[relative] = m
			return m, nil
		}
		m, e := s.probe(ctx, relative)
		if e != nil {
			probeErrors[relative] = e
		} else {
			metadata[relative] = m
		}
		return m, e
	}
	var tracks []models.MusicTrack
	seen := map[string]bool{}
	suppressed := map[string]bool{}
	add := func(track models.MusicTrack) {
		if seen[track.CatalogKey] {
			return
		}
		seen[track.CatalogKey] = true
		track.LastSeenRunID = runID
		if previous, ok := prior[track.CatalogKey]; ok {
			track.ID = previous.ID
			track.CreatedAt = previous.CreatedAt
		}
		if track.ID == "" {
			track.ID = uuid.NewString()
		}
		tracks = append(tracks, track)
		status.Discovered++
		status.Processed++
		if track.Available {
			status.Available++
		} else {
			status.Unavailable++
			if track.FailureCode == "invalid_metadata" {
				status.MetadataFailed++
			}
		}
		s.progress(func(st *ScanStatus) {
			st.Discovered = status.Discovered
			st.Processed = status.Processed
			st.Available = status.Available
			st.Unavailable = status.Unavailable
			st.MetadataFailed = status.MetadataFailed
			st.InvalidCUE = status.InvalidCUE
		})
	}
	build := func(relative, cue string, number int, start, end int64, m Metadata, probeErr error) models.MusicTrack {
		info := sources[relative]
		track := models.MusicTrack{CatalogKey: catalogKey(relative, cue, number), RelativePath: relative, CueRelativePath: cue, CueTrackNumber: number, StartFrames: start, EndFrames: end, Title: m.Title, Artist: m.Artist, Album: m.Album, DurationMS: m.DurationMS, Format: strings.TrimPrefix(strings.ToLower(path.Ext(relative)), "."), Codec: m.Codec, MIMEType: m.MIMEType, CoverKind: "none", CoverStreamIndex: -1, LyricsKind: "none", Available: probeErr == nil}
		if info != nil {
			track.FileSize = info.Size()
			track.ModifiedNS = info.ModTime().UnixNano()
		}
		if track.Title == "" {
			track.Title = strings.TrimSuffix(path.Base(relative), path.Ext(relative))
		}
		if probeErr != nil {
			track.FailureCode = "invalid_metadata"
		}
		if number > 0 {
			track.DurationMS = (end - start) * 1000 / 75
		}
		if m.CoverStreamIndex >= 0 && probeErr == nil {
			track.CoverKind = "embedded"
			track.CoverStreamIndex = m.CoverStreamIndex
		} else if candidates, e := s.coverCandidates(ctx, relative); e == nil && len(candidates) > 0 {
			track.CoverKind = "sidecar"
			track.CoverRelativePath = candidates[0]
		}
		keys := make([]string, 0, len(m.Tags))
		for key := range m.Tags {
			if key != "lyrics" {
				keys = append(keys, key)
			}
		}
		sort.Strings(keys)
		keys = append([]string{"lyrics"}, keys...)
		for _, key := range keys {
			if m.Tags[key] != "" && strings.Contains(key, "lyrics") {
				track.LyricsKind = "embedded"
				track.LyricsTag = key
				break
			}
		}
		if track.LyricsKind == "none" {
			if candidates, e := s.lyricsCandidates(ctx, relative); e == nil && len(candidates) > 0 {
				track.LyricsKind = "sidecar"
				track.LyricsRelativePath = candidates[0]
			}
		}
		fingerprint, e := s.trackFingerprint(track)
		if e != nil {
			track.Available = false
			track.FailureCode = "missing_source"
		}
		track.Fingerprint = fingerprint
		return track
	}
	for _, relative := range cues {
		if err := ctx.Err(); err != nil {
			return nil, status, err
		}
		data, e := s.secureRead(ctx, relative, textLimit)
		var sheet cueSheet
		if e == nil {
			sheet, e = parseCUE(data, relative)
		}
		valid := e == nil
		var logical []models.MusicTrack
		if valid {
			cached := make([]models.MusicTrack, 0, len(sheet.Tracks))
			allFresh := true
			for _, segment := range sheet.Tracks {
				track, ok := prior[catalogKey(segment.File, relative, segment.Number)]
				if !ok || !track.Available || track.StartFrames != segment.StartFrames || (segment.EndFrames >= 0 && track.EndFrames != segment.EndFrames) || !s.fingerprintFresh(track) {
					allFresh = false
					break
				}
				cached = append(cached, track)
			}
			if allFresh {
				for _, track := range cached {
					suppressed[track.RelativePath] = true
					add(track)
				}
				continue
			}
		}
		if valid {
			for _, segment := range sheet.Tracks {
				if _, exists := sources[segment.File]; !exists {
					valid = false
					break
				}
				m, e := getMetadata(segment.File)
				if e != nil {
					valid = false
					break
				}
				end := segment.EndFrames
				if end < 0 {
					end = m.DurationMS * 75 / 1000
				}
				if segment.StartFrames < 0 || end <= segment.StartFrames || end > m.DurationMS*75/1000 {
					valid = false
					break
				}
				track := build(segment.File, relative, segment.Number, segment.StartFrames, end, m, nil)
				if segment.Title != "" {
					track.Title = segment.Title
				}
				if segment.Artist != "" {
					track.Artist = segment.Artist
				}
				if sheet.Title != "" {
					track.Album = sheet.Title
				}
				logical = append(logical, track)
			}
		}
		if !valid {
			status.InvalidCUE++
			for _, track := range old {
				if track.CueRelativePath == relative {
					track.Available = false
					track.FailureCode = "invalid_cue"
					add(track)
				}
			}
			continue
		}
		for _, track := range logical {
			suppressed[track.RelativePath] = true
			add(track)
		}
	}
	ordered := make([]string, 0, len(sources))
	for relative := range sources {
		ordered = append(ordered, relative)
	}
	sort.Strings(ordered)
	for _, relative := range ordered {
		if err := ctx.Err(); err != nil {
			return nil, status, err
		}
		m, e := getMetadata(relative)
		if errors.Is(e, exec.ErrNotFound) || errors.Is(e, os.ErrNotExist) {
			return nil, status, &scanFailure{code: "tool_error", err: e}
		}
		add(build(relative, "", 0, 0, 0, m, e))
	}
	for _, track := range old {
		if seen[track.CatalogKey] {
			continue
		}
		track.Available = false
		track.FailureCode = "missing_source"
		add(track)
	}
	s.progress(func(st *ScanStatus) { st.InvalidCUE = status.InvalidCUE })
	return tracks, status, nil
}
