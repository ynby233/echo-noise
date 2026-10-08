package music

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
)

type fixtureTools struct {
	mu     sync.Mutex
	probes int
}

func (f *fixtureTools) Run(ctx context.Context, executable string, args []string, input ToolInput, out io.Writer) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.Contains(executable, "ffprobe") {
		f.mu.Lock()
		f.probes++
		f.mu.Unlock()
		_, err := io.WriteString(out, `{"format":{"duration":"6.0","tags":{"title":"Format title","artist":"Fixture artist","lyrics":"[00:01.00]First\n[00:04.00]Second"}},"streams":[{"index":0,"codec_type":"audio","codec_name":"mp3","duration":"6.0","tags":{"title":"<img onerror=unsafe>"}}]}`)
		return err
	}
	_, err := io.WriteString(out, "fixture MP3 representation")
	return err
}
func musicFixture(t *testing.T) (*Service, *fixtureTools) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "music.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sql, _ := db.DB()
	sql.SetMaxOpenConns(1)
	t.Cleanup(func() { sql.Close() })
	if err = db.AutoMigrate(&models.SiteConfig{}, &models.User{}, &models.AdminCapabilityGrant{}, &models.AdminAuditLog{}, &models.AdminAuditConfig{}, &models.MusicConfig{}, &models.MusicTrack{}, &models.MusicPlaylistItem{}, &models.MusicScanState{}); err != nil {
		t.Fatal(err)
	}
	if err = db.Create(&models.User{ID: 1, Username: "owner", IsAdmin: true}).Error; err != nil {
		t.Fatal(err)
	}
	site := defaultSite()
	site.MusicEnabled = true
	if err = db.Create(&site).Error; err != nil {
		t.Fatal(err)
	}
	if err = EnsureDefaults(db); err != nil {
		t.Fatal(err)
	}
	tools := &fixtureTools{}
	s := NewService(db, Options{RootDir: t.TempDir(), CacheDir: filepath.Join(t.TempDir(), "cache"), Tools: tools})
	s.toolsReady = true
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = s.Wait(ctx)
	})
	return s, tools
}
func fixtureFile(t *testing.T, s *Service, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(s.opts.RootDir, name), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}
func persistScan(t *testing.T, s *Service, run string) ([]models.MusicTrack, ScanStatus) {
	t.Helper()
	var old []models.MusicTrack
	if err := s.db.Find(&old).Error; err != nil {
		t.Fatal(err)
	}
	tracks, status, err := s.collectScan(context.Background(), run, old)
	if err != nil {
		t.Fatal(err)
	}
	for _, track := range tracks {
		if err = s.db.Save(&track).Error; err != nil {
			t.Fatal(err)
		}
	}
	return tracks, status
}
func selectTracks(t *testing.T, s *Service, ids ...string) {
	t.Helper()
	var cfg models.MusicConfig
	if err := s.db.First(&cfg, 1).Error; err != nil {
		t.Fatal(err)
	}
	var site models.SiteConfig
	s.db.First(&site, 1)
	_, err := s.SaveConfig(context.Background(), 1, SaveConfigRequest{Version: cfg.Version, FrontendSettings: frontendSettings(site, "local", false), ScanIntervalMinutes: 60, TrackIDs: ids})
	if err != nil {
		t.Fatal(err)
	}
}
func TestScanIncrementalCUEAndPublicAvailability(t *testing.T) {
	s, tools := musicFixture(t)
	fixtureFile(t, s, "整轨.mp3", "original bytes")
	fixtureFile(t, s, "整轨.cue", "TITLE \"Album\"\nFILE \"整轨.mp3\" MP3\n TRACK 01 AUDIO\n TITLE \"First\"\n INDEX 01 00:00:00\n TRACK 02 AUDIO\n TITLE \"Second\"\n INDEX 01 00:03:00\n")
	tracks, status := persistScan(t, s, "first")
	if status.Available != 3 {
		t.Fatalf("want original + two segments: %+v", status)
	}
	var first, second, original models.MusicTrack
	for _, track := range tracks {
		switch track.CueTrackNumber {
		case 1:
			first = track
		case 2:
			second = track
		case 0:
			original = track
		}
	}
	if first.StartFrames != 0 || first.EndFrames != 225 || first.DurationMS != 3000 || second.StartFrames != 225 || second.EndFrames != 450 {
		t.Fatalf("invalid segments: %+v %+v", first, second)
	}
	before := tools.probes
	again, _ := persistScan(t, s, "again")
	if tools.probes != before {
		t.Fatal("unchanged refresh must reuse metadata")
	}
	if again[0].ID != tracks[0].ID {
		t.Fatal("stable identity lost")
	}
	selectTracks(t, s, first.ID)
	public, err := s.GetPublic(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(public.Tracks) != 1 || public.Tracks[0].TrackID != first.ID || public.Tracks[0].MIMEType != "audio/mpeg" {
		t.Fatalf("public=%+v", public)
	}
	if _, err = s.OpenMedia(context.Background(), second.ID, "stream", false, false); !errors.Is(err, ErrNotFound) {
		t.Fatal("unselected CUE must not stream")
	}
	if _, err = s.OpenMedia(context.Background(), original.ID, "stream", false, false); !errors.Is(err, ErrNotFound) {
		t.Fatal("original must not bypass selected segment")
	}
	lyrics, err := s.GetLyrics(context.Background(), first.ID)
	if err != nil || len(lyrics.Lines) != 1 || lyrics.Lines[0].Text != "First" {
		t.Fatalf("segment lyrics=%+v err=%v", lyrics, err)
	}
	rep := ResolveRepresentation(first, false)
	if !rep.Derived {
		t.Fatal("CUE must be bounded representation")
	}
	args, err := BuildMediaArgs("/proc/self/fd/3", "mp3", first, "pipe:1")
	if err != nil || !strings.Contains(strings.Join(args, " "), "atrim=duration=3.000000000") {
		t.Fatalf("trim args=%v error=%v", args, err)
	}
	encoded, _ := json.Marshal(public)
	if strings.Contains(string(encoded), s.opts.RootDir) || strings.Contains(string(encoded), "relativePath") {
		t.Fatal("filesystem path leaked")
	}
	if err = os.Remove(filepath.Join(s.opts.RootDir, "整轨.mp3")); err != nil {
		t.Fatal(err)
	}
	missing, err := s.GetPublic(context.Background())
	if err != nil || len(missing.Tracks) != 0 || missing.Revision == public.Revision {
		t.Fatalf("deleted file public=%+v err=%v", missing, err)
	}
}
func TestOriginalMediaRangeHeadAndRevocation(t *testing.T) {
	s, _ := musicFixture(t)
	fixtureFile(t, s, "single.mp3", "0123456789")
	tracks, _ := persistScan(t, s, "one")
	id := tracks[0].ID
	selectTracks(t, s, id)
	serve := func(method, rangeValue string) *httptest.ResponseRecorder {
		t.Helper()
		media, err := s.OpenMedia(context.Background(), id, "stream", false, false)
		if err != nil {
			t.Fatal(err)
		}
		defer media.Close()
		req := httptest.NewRequest(method, "/api/music/stream/"+id, nil)
		if rangeValue != "" {
			req.Header.Set("Range", rangeValue)
		}
		res := httptest.NewRecorder()
		http.ServeContent(res, req, media.Name, media.Info.ModTime(), media.File)
		return res
	}
	rangeRes := serve("GET", "bytes=0-3")
	if rangeRes.Code != 206 || rangeRes.Body.String() != "0123" || rangeRes.Header().Get("Content-Range") != "bytes 0-3/10" {
		t.Fatal("range semantics broken", rangeRes.Code, rangeRes.Body.String())
	}
	if r := serve("HEAD", ""); r.Code != 200 || r.Body.Len() != 0 || r.Header().Get("Content-Length") != "10" {
		t.Fatal("HEAD semantics broken")
	}
	if r := serve("GET", "bytes=99-100"); r.Code != 416 {
		t.Fatal("out of range must be 416")
	}
	if err := s.db.Model(&models.MusicConfig{}).Where("id = ?", 1).Update("source", "netease").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := s.OpenMedia(context.Background(), id, "stream", false, false); !errors.Is(err, ErrNotFound) {
		t.Fatal("source switch must revoke old media")
	}
}
func TestConfigCASAndInvalidPlaylistRollback(t *testing.T) {
	s, _ := musicFixture(t)
	site := defaultSite()
	fields := frontendSettings(site, "local", false)
	first, err := s.SaveConfig(context.Background(), 1, SaveConfigRequest{Version: 1, FrontendSettings: fields, ScanIntervalMinutes: 15, TrackIDs: []string{}})
	if err != nil || first.Version != 2 {
		t.Fatalf("first config %+v %v", first, err)
	}
	_, err = s.SaveConfig(context.Background(), 1, SaveConfigRequest{Version: 1, FrontendSettings: fields, ScanIntervalMinutes: 60, TrackIDs: []string{}})
	if !errors.Is(err, ErrConflict) {
		t.Fatal("stale version accepted", err)
	}
	_, err = s.SaveConfig(context.Background(), 1, SaveConfigRequest{Version: 2, FrontendSettings: fields, ScanIntervalMinutes: 60, TrackIDs: []string{"00000000-0000-0000-0000-000000000000"}})
	if !errors.Is(err, ErrUnprocessable) {
		t.Fatal("missing track accepted", err)
	}
	cfg, err := s.GetAdminConfig(context.Background())
	if err != nil || cfg.Version != 2 || cfg.ScanIntervalMinutes != 15 {
		t.Fatalf("rollback lost %+v %v", cfg, err)
	}
}
func TestManualRefreshSingleRunAndMissingRootKeepsIndex(t *testing.T) {
	s, _ := musicFixture(t)
	fixtureFile(t, s, "song.mp3", "music")
	tracks, _ := persistScan(t, s, "old")
	s.mu.Lock()
	s.running = true
	s.status = ScanStatus{RunID: "in-progress", State: "running"}
	s.mu.Unlock()
	state, started, err := s.RequestScan(context.Background(), 1)
	if err != nil || started || state.RunID != "in-progress" {
		t.Fatalf("duplicate scan=%+v %v %v", state, started, err)
	}
	s.mu.Lock()
	s.running = false
	s.mu.Unlock()
	s.opts.RootDir = filepath.Join(t.TempDir(), "missing")
	_, _, err = s.collectScan(context.Background(), "failed", tracks)
	if scanCode(err) != "root_missing" {
		t.Fatal("missing root failure wrong", err)
	}
	var count int64
	s.db.Model(&models.MusicTrack{}).Count(&count)
	if count != int64(len(tracks)) {
		t.Fatal("failed scan replaced index")
	}
}
