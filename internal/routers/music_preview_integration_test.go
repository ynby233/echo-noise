package routers

import (
	"encoding/json"
	"fmt"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/music"
	"github.com/rcy1314/echo-noise/internal/repository"
)

func previewMusicTrack(t *testing.T, f noteMusicFixture, user *models.User, token bool) music.PublicTrack {
	t.Helper()
	prefix := "/api"
	if token {
		prefix = "/api/token"
	}
	got := f.request(t, user, token, "GET", prefix+"/music/preview/"+f.trackID, "")
	var envelope struct {
		Code int               `json:"code"`
		Data music.PublicTrack `json:"data"`
	}
	if got.Code != 200 || json.Unmarshal(got.Body.Bytes(), &envelope) != nil || envelope.Code != 1 || envelope.Data.TrackID != f.trackID {
		t.Fatalf("preview token=%v: %d %s", token, got.Code, got.Body.String())
	}
	for _, field := range []string{"relativePath", "catalogKey", "fingerprint", "selected", "unselected.mp3"} {
		if strings.Contains(got.Body.String(), field) {
			t.Fatalf("preview leaked administrative field %s: %s", field, got.Body.String())
		}
	}
	if got.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("preview cache policy: %s", got.Header().Get("Cache-Control"))
	}
	return envelope.Data
}

func previewMusicURLs(track music.PublicTrack) []string {
	return []string{track.StreamURL, track.FallbackStreamURL, track.CoverURL, track.LyricsURL}
}

func TestMusicPreviewSessionTokenAndActorBoundMedia(t *testing.T) {
	f := setupNoteMusicRoutes(t)
	other := models.User{Username: "other-preview-manager", IsAdmin: true, Token: "other-preview-manager-token"}
	if err := repository.CreateUser(&other); err != nil {
		t.Fatal(err)
	}
	grant := models.AdminCapabilityGrant{UserID: other.ID, Capability: "music.manage", GrantedByUserID: f.manager.ID}
	if err := f.db.Create(&grant).Error; err != nil {
		t.Fatal(err)
	}
	for _, token := range []bool{false, true} {
		track := previewMusicTrack(t, f, &f.manager, token)
		for _, media := range previewMusicURLs(track) {
			parsed, err := url.Parse(media)
			if err != nil || media == "" {
				t.Fatalf("invalid preview URL %q: %v", media, err)
			}
			query := parsed.Query()
			deadline, err := strconv.ParseInt(query.Get("expires"), 10, 64)
			if err != nil || deadline <= time.Now().Unix() || deadline > time.Now().Add(31*time.Minute).Unix() || query.Get("previewActorID") != strconv.FormatUint(uint64(f.manager.ID), 10) || len(query.Get("signature")) != 64 || query.Has("messageID") || query.Has("token") {
				t.Fatalf("invalid preview signature contract: %s", media)
			}
			for _, mediaToken := range []bool{false, true} {
				if got := f.request(t, &f.manager, mediaToken, "GET", media, ""); got.Code != 200 {
					t.Fatalf("same actor media token=%v: %d %s", mediaToken, got.Code, got.Body.String())
				}
				if got := f.request(t, &other, mediaToken, "GET", media, ""); got.Code != 404 {
					t.Fatalf("different authorized actor token=%v: %d", mediaToken, got.Code)
				}
				if got := f.request(t, &f.editor, mediaToken, "GET", media, ""); got.Code != 404 {
					t.Fatalf("actor without manage token=%v: %d", mediaToken, got.Code)
				}
			}
			if got := f.request(t, nil, false, "GET", media, ""); got.Code != 404 {
				t.Fatalf("guest replay: %d", got.Code)
			}
			if !strings.Contains(parsed.Path, "/lyrics/") {
				if got := f.request(t, &f.manager, token, "HEAD", media, ""); got.Code != 200 {
					t.Fatalf("signed HEAD: %d", got.Code)
				}
			}
		}
		fallback, _ := url.Parse(track.FallbackStreamURL)
		if fallback.Query().Get("compat") != "1" {
			t.Fatal("preview lost compatibility stream query")
		}
		for _, change := range []struct{ key, value string }{
			{"expires", strconv.FormatInt(time.Now().Add(-time.Minute).Unix(), 10)},
			{"expires", strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10)},
			{"signature", strings.Repeat("0", 64)},
			{"previewActorID", strconv.FormatUint(uint64(other.ID), 10)},
			{"messageID", "1"},
		} {
			parsed, _ := url.Parse(track.StreamURL)
			query := parsed.Query()
			query.Set(change.key, change.value)
			parsed.RawQuery = query.Encode()
			if got := f.request(t, &f.manager, token, "GET", parsed.String(), ""); got.Code != 404 {
				t.Fatalf("tampered %s: %d", change.key, got.Code)
			}
		}
		parsed, _ := url.Parse(track.StreamURL)
		query := parsed.Query()
		query.Del("previewActorID")
		parsed.RawQuery = query.Encode()
		if got := f.request(t, &f.manager, token, "GET", parsed.String(), ""); got.Code != 404 {
			t.Fatalf("preview signature accepted as note signature: %d", got.Code)
		}
		parsed.RawQuery = ""
		if got := f.request(t, nil, false, "GET", parsed.String(), ""); got.Code != 404 {
			t.Fatalf("unsigned unpublished stream: %d", got.Code)
		}
	}
}

func TestMusicPreviewRequiresManageAndRevokesExistingURLs(t *testing.T) {
	f := setupNoteMusicRoutes(t)
	delegated := models.User{Username: "delegated-preview", IsAdmin: true, Token: "delegated-preview-token"}
	if err := repository.CreateUser(&delegated); err != nil {
		t.Fatal(err)
	}
	view := models.AdminCapabilityGrant{UserID: delegated.ID, Capability: "music.view", GrantedByUserID: f.manager.ID}
	if err := f.db.Create(&view).Error; err != nil {
		t.Fatal(err)
	}
	for _, token := range []bool{false, true} {
		prefix := "/api"
		if token {
			prefix = "/api/token"
		}
		path := prefix + "/music/preview/" + f.trackID
		if got := f.request(t, nil, token, "GET", path, ""); got.Code != 401 {
			t.Fatalf("guest preview token=%v: %d", token, got.Code)
		}
		for _, user := range []*models.User{&f.editor, &delegated} {
			if got := f.request(t, user, token, "GET", path, ""); got.Code != 403 {
				t.Fatalf("preview without manage token=%v: %d", token, got.Code)
			}
		}
		grant := models.AdminCapabilityGrant{UserID: delegated.ID, Capability: "music.manage", GrantedByUserID: f.manager.ID}
		if err := f.db.Create(&grant).Error; err != nil {
			t.Fatal(err)
		}
		track := previewMusicTrack(t, f, &delegated, token)
		for _, media := range previewMusicURLs(track) {
			if got := f.request(t, &delegated, token, "GET", media, ""); got.Code != 200 {
				t.Fatalf("delegated preview media: %d %s", got.Code, got.Body.String())
			}
		}
		if err := f.db.Delete(&grant).Error; err != nil {
			t.Fatal(err)
		}
		if got := f.request(t, &delegated, token, "GET", path, ""); got.Code != 403 {
			t.Fatalf("revoked preview: %d", got.Code)
		}
		for _, media := range previewMusicURLs(track) {
			if got := f.request(t, &delegated, token, "GET", media, ""); got.Code != 404 {
				t.Fatalf("revoked media token=%v: %d", token, got.Code)
			}
		}
	}
}

func TestMusicPreviewDoesNotChangePlaylistAndPreservesNoteAuthorization(t *testing.T) {
	f := setupNoteMusicRoutes(t)
	var original models.MusicConfig
	if err := f.db.First(&original, 1).Error; err != nil {
		t.Fatal(err)
	}
	before := f.request(t, nil, false, "GET", "/api/music/public", "")
	for _, token := range []bool{false, true} {
		previewMusicTrack(t, f, &f.manager, token)
	}
	var current models.MusicConfig
	if err := f.db.First(&current, 1).Error; err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := f.db.Model(&models.MusicPlaylistItem{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	after := f.request(t, nil, false, "GET", "/api/music/public", "")
	if count != 0 || !reflect.DeepEqual(original, current) || before.Code != 200 || after.Code != 200 || before.Body.String() != after.Body.String() {
		t.Fatalf("preview mutated configuration/playlist/public output: count=%d before=%+v after=%+v", count, original, current)
	}
	if err := f.db.Model(&models.MusicConfig{}).Where("id = ?", 1).Update("source", "local").Error; err != nil {
		t.Fatal(err)
	}
	track := previewMusicTrack(t, f, &f.manager, false)
	if got := f.request(t, nil, false, "GET", "/api/music/stream/"+f.trackID, ""); got.Code != 404 {
		t.Fatalf("preview added unpublished local track to playlist: %d", got.Code)
	}
	item := models.MusicPlaylistItem{TrackID: f.trackID, Position: 0}
	if err := f.db.Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	if got := f.request(t, nil, false, "GET", "/api/music/stream/"+f.trackID, ""); got.Code != 200 {
		t.Fatalf("ordinary public playlist stream: %d %s", got.Code, got.Body.String())
	}
	if err := f.db.Delete(&item).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.db.Model(&models.MusicConfig{}).Where("id = ?", 1).Update("source", "netease").Error; err != nil {
		t.Fatal(err)
	}
	message := models.Message{Content: "[♫ Fixture](/music/notes/" + f.trackID + ")", UserID: f.editor.ID, Visibility: "PUBLIC"}
	if err := f.db.Create(&message).Error; err != nil {
		t.Fatal(err)
	}
	notePath := fmt.Sprintf("/api/music/notes/%s?messageID=%d", f.trackID, message.ID)
	got := f.request(t, nil, false, "GET", notePath, "")
	var note struct {
		Code int               `json:"code"`
		Data music.PublicTrack `json:"data"`
	}
	if got.Code != 200 || json.Unmarshal(got.Body.Bytes(), &note) != nil || note.Code != 1 {
		t.Fatalf("published note resolve: %d %s", got.Code, got.Body.String())
	}
	for _, media := range previewMusicURLs(note.Data) {
		if got := f.request(t, nil, false, "GET", media, ""); got.Code != 200 {
			t.Fatalf("published note guest media: %d", got.Code)
		}
	}
	if err := f.db.Model(&message).Updates(map[string]any{"private": true, "visibility": "PRIVATE"}).Error; err != nil {
		t.Fatal(err)
	}
	if got := f.request(t, nil, false, "GET", note.Data.StreamURL, ""); got.Code != 404 {
		t.Fatalf("private note old URL: %d", got.Code)
	}
	if got := f.request(t, &f.editor, false, "GET", note.Data.StreamURL, ""); got.Code != 200 {
		t.Fatalf("private note owner without manage: %d", got.Code)
	}
	if err := f.db.Model(&message).Update("content", "removed").Error; err != nil {
		t.Fatal(err)
	}
	if got := f.request(t, &f.editor, false, "GET", note.Data.StreamURL, ""); got.Code != 404 {
		t.Fatalf("removed note reference: %d", got.Code)
	}
	if err := f.db.Model(&models.SiteConfig{}).Where("id = ?", 1).Update("music_enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	if got := f.request(t, &f.manager, false, "GET", "/api/music/preview/"+f.trackID, ""); got.Code != 404 {
		t.Fatalf("disabled music preview: %d", got.Code)
	}
	for _, media := range previewMusicURLs(track) {
		if got := f.request(t, &f.manager, false, "GET", media, ""); got.Code != 404 {
			t.Fatalf("disabled music existing preview URL: %d", got.Code)
		}
	}
}
