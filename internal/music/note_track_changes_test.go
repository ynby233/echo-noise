package music

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rcy1314/echo-noise/internal/models"
)

// Only probe output changes; encoder and media commands use the shared fake.
type noteTrackChangeTools struct {
	fallback *fixtureTools
	title    string
	artist   string
	album    string
	cover    bool
}

func (f *noteTrackChangeTools) Run(ctx context.Context, executable string, args []string, input ToolInput, out io.Writer) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !strings.Contains(executable, "ffprobe") {
		return f.fallback.Run(ctx, executable, args, input, out)
	}
	streams := []map[string]interface{}{{"index": 0, "codec_type": "audio", "codec_name": "mp3", "duration": "6.0"}}
	if f.cover {
		streams = append(streams, map[string]interface{}{"index": 1, "codec_type": "video", "disposition": map[string]int{"attached_pic": 1}})
	}
	return json.NewEncoder(out).Encode(map[string]interface{}{
		"format":  map[string]interface{}{"duration": "6.0", "tags": map[string]string{"title": f.title, "artist": f.artist, "album": f.album}},
		"streams": streams,
	})
}

func noteTrackChangeScan(t *testing.T, s *Service, run, relative string) models.MusicTrack {
	t.Helper()
	tracks, _ := persistScan(t, s, run)
	for _, track := range tracks {
		if track.RelativePath == relative && track.Available {
			return track
		}
	}
	t.Fatalf("scan %q: no available track at %q: %+v", run, relative, tracks)
	return models.MusicTrack{}
}

func assertNoteTrackChangeInvalid(t *testing.T, s *Service, id string) {
	t.Helper()
	projection, err := s.ResolveNoteTrack(context.Background(), id)
	if !errors.Is(err, ErrNoteTrackInvalid) {
		t.Fatalf("ResolveNoteTrack(%q) = %+v, %v; want ErrNoteTrackInvalid", id, projection, err)
	}
	if projection != (PublicTrack{}) {
		t.Fatalf("invalid track leaked playback projection: %+v", projection)
	}
}

func TestResolveNoteTrackSamePathMetadataChanges(t *testing.T) {
	s, fallback := musicFixture(t)
	tools := &noteTrackChangeTools{fallback: fallback, title: "First title", artist: "First artist", album: "First album"}
	s.opts.Tools = tools
	fixtureFile(t, s, "song.mp3", "original tagged audio")
	original := noteTrackChangeScan(t, s, "initial", "song.mp3")
	before, err := s.ResolveNoteTrack(context.Background(), original.ID)
	if err != nil {
		t.Fatal(err)
	}
	if before.Title != tools.title || before.Artist != tools.artist || before.Album != tools.album || before.CoverURL != "" {
		t.Fatalf("unexpected initial projection: %+v", before)
	}

	tools.title, tools.artist, tools.album, tools.cover = "Updated title", "Updated artist", "Updated album", true
	// A different size forces a fresh probe even on coarse timestamp filesystems.
	fixtureFile(t, s, "song.mp3", "audio with updated title artist album and embedded cover metadata")
	updated := noteTrackChangeScan(t, s, "metadata-update", "song.mp3")
	if updated.ID != original.ID {
		t.Fatalf("metadata edit changed ID: %q -> %q", original.ID, updated.ID)
	}
	after, err := s.ResolveNoteTrack(context.Background(), original.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.TrackID != original.ID || after.Title != tools.title || after.Artist != tools.artist || after.Album != tools.album || after.CoverURL != "/api/music/cover/"+original.ID {
		t.Fatalf("stale metadata projection: %+v", after)
	}
	if after.MediaVersion == before.MediaVersion {
		t.Fatal("metadata and cover update retained stale media version")
	}
	if after.StreamURL != "/api/music/stream/"+original.ID {
		t.Fatalf("unexpected playback URL: %q", after.StreamURL)
	}
}

func TestResolveNoteTrackMoveAndRestorePath(t *testing.T) {
	s, _ := musicFixture(t)
	fixtureFile(t, s, "original.mp3", "audio to move")
	original := noteTrackChangeScan(t, s, "initial", "original.mp3")
	if _, err := s.ResolveNoteTrack(context.Background(), original.ID); err != nil {
		t.Fatal(err)
	}
	originalPath := filepath.Join(s.opts.RootDir, "original.mp3")
	movedPath := filepath.Join(s.opts.RootDir, "moved.mp3")
	if err := os.Rename(originalPath, movedPath); err != nil {
		t.Fatal(err)
	}
	assertNoteTrackChangeInvalid(t, s, original.ID)
	moved := noteTrackChangeScan(t, s, "moved", "moved.mp3")
	assertNoteTrackChangeInvalid(t, s, original.ID)
	if moved.ID == original.ID {
		t.Fatal("moving to a different path retained the old ID")
	}
	if projection, err := s.ResolveNoteTrack(context.Background(), moved.ID); err != nil || projection.TrackID != moved.ID {
		t.Fatalf("moved track did not resolve: %+v, %v", projection, err)
	}
	if err := os.Rename(movedPath, originalPath); err != nil {
		t.Fatal(err)
	}
	restored := noteTrackChangeScan(t, s, "restored", "original.mp3")
	if restored.ID != original.ID {
		t.Fatalf("restoring original path did not recover ID: %q != %q", restored.ID, original.ID)
	}
	if projection, err := s.ResolveNoteTrack(context.Background(), original.ID); err != nil || projection.TrackID != original.ID {
		t.Fatalf("restored track did not resolve: %+v, %v", projection, err)
	}
	assertNoteTrackChangeInvalid(t, s, moved.ID)
}

func TestResolveNoteTrackMissingSourceBeforeAndAfterScan(t *testing.T) {
	s, _ := musicFixture(t)
	fixtureFile(t, s, "missing.mp3", "audio that will disappear")
	track := noteTrackChangeScan(t, s, "initial", "missing.mp3")
	if _, err := s.ResolveNoteTrack(context.Background(), track.ID); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(s.opts.RootDir, "missing.mp3")); err != nil {
		t.Fatal(err)
	}
	assertNoteTrackChangeInvalid(t, s, track.ID)
	tracks, _ := persistScan(t, s, "missing")
	if len(tracks) != 1 || tracks[0].ID != track.ID || tracks[0].Available || tracks[0].FailureCode != "missing_source" {
		t.Fatalf("scan did not retain an unavailable missing-source record: %+v", tracks)
	}
	assertNoteTrackChangeInvalid(t, s, track.ID)
}

func TestResolveNoteTrackSamePathContentReplacementKeepsID(t *testing.T) {
	s, _ := musicFixture(t)
	fixtureFile(t, s, "replaced.mp3", "original recording")
	original := noteTrackChangeScan(t, s, "initial", "replaced.mp3")
	before, err := s.ResolveNoteTrack(context.Background(), original.ID)
	if err != nil {
		t.Fatal(err)
	}
	fixtureFile(t, s, "replaced.mp3", "an entirely different recording with a different size")
	replacement := noteTrackChangeScan(t, s, "replacement", "replaced.mp3")
	// Identity follows the catalog path, not audio content. A note can therefore
	// resolve to a different recording after replacement and a successful scan.
	if replacement.ID != original.ID {
		t.Fatalf("same-path replacement changed catalog ID: %q -> %q", original.ID, replacement.ID)
	}
	after, err := s.ResolveNoteTrack(context.Background(), original.ID)
	if err != nil || after.TrackID != original.ID {
		t.Fatalf("replacement did not resolve through original ID: %+v, %v", after, err)
	}
	if after.MediaVersion == before.MediaVersion {
		t.Fatal("different-size replacement retained stale media version")
	}
}
