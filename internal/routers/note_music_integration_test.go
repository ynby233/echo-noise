package routers

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/music"
	"github.com/rcy1314/echo-noise/internal/repository"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type noteMusicTools struct{}

func (noteMusicTools) Run(ctx context.Context, executable string, args []string, input music.ToolInput, out io.Writer) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.Contains(executable, "ffprobe") {
		_, err := io.WriteString(out, `{"format":{"duration":"6.0","tags":{"lyrics":"[00:01.00]A lyric"}},"streams":[{"index":0,"codec_type":"audio","codec_name":"mp3"}]}`)
		return err
	}
	if strings.Contains(strings.Join(args, " "), "image2pipe") {
		return jpeg.Encode(out, image.NewRGBA(image.Rect(0, 0, 2, 2)), nil)
	}
	_, err := io.WriteString(out, "fixture representation libmp3lame")
	return err
}

type noteMusicFixture struct {
	db      *gorm.DB
	r       *gin.Engine
	manager models.User
	editor  models.User
	trackID string
}

func setupNoteMusicRoutes(t *testing.T) noteMusicFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	t.Setenv("ACCESS_LOG", "false")
	t.Setenv("SESSION_SECRET", "note-music-session-secret-32-bytes")
	t.Chdir(t.TempDir())
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err = models.MigrateDB(db); err != nil {
		t.Fatal(err)
	}
	if err = db.Create(&models.SiteConfig{Model: gorm.Model{ID: 1}, MusicEnabled: true, MusicPosition: "bottom-left", MusicTheme: "auto"}).Error; err != nil {
		t.Fatal(err)
	}
	if err = music.EnsureDefaults(db); err != nil {
		t.Fatal(err)
	}
	database.DB = db
	models.SetDB(db)
	repository.ClearUserCache()
	manager := models.User{Username: "note-music-manager", IsAdmin: true, Token: "note-music-manager-token"}
	editor := models.User{Username: "note-music-editor", Token: "note-music-editor-token"}
	for _, user := range []*models.User{&manager, &editor} {
		if err = repository.CreateUser(user); err != nil {
			t.Fatal(err)
		}
	}
	root := t.TempDir()
	path := filepath.Join(root, "unselected.mp3")
	if err = os.WriteFile(path, []byte("0123456789"), 0600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	// An indexed source fixture, using the scanner's documented stat-v1 key.
	hash := sha256.New()
	fmt.Fprintf(hash, "stat-v1\x00%s\x00%d\x00%d\x00%d\x00", "", 0, 0, 0)
	fmt.Fprintf(hash, "%s\x00%d\x00%d\x00", "unselected.mp3", info.Size(), info.ModTime().UnixNano())
	track := models.MusicTrack{ID: "4a075723-b2b6-4a36-976c-c74e0041ca44", CatalogKey: "note-music-fixture", RelativePath: "unselected.mp3", Title: "Fixture", Artist: "Artist", Format: "mp3", Codec: "mp3", FileSize: info.Size(), ModifiedNS: info.ModTime().UnixNano(), Fingerprint: hex.EncodeToString(hash.Sum(nil)), CoverKind: "embedded", LyricsKind: "embedded", Available: true}
	if err = db.Create(&track).Error; err != nil {
		t.Fatal(err)
	}
	service := music.NewService(db, music.Options{RootDir: root, CacheDir: t.TempDir(), Tools: noteMusicTools{}})
	r := SetupRouter(service)
	r.GET("/__test/note-music-session/:id", func(c *gin.Context) {
		session := sessions.Default(c)
		session.Set("user_id", c.Param("id"))
		session.Set("login_expire_at", time.Now().Add(time.Hour).Unix())
		if err := session.Save(); err != nil {
			c.Status(500)
			return
		}
		c.Status(204)
	})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = service.Wait(ctx)
		repository.ClearUserCache()
		database.DB = nil
		models.SetDB(nil)
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return noteMusicFixture{db: db, r: r, manager: manager, editor: editor, trackID: track.ID}
}

func (f noteMusicFixture) request(t *testing.T, user *models.User, token bool, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	if user != nil {
		if token {
			req.Header.Set("Authorization", "Bearer "+user.Token)
		} else {
			seed := httptest.NewRecorder()
			f.r.ServeHTTP(seed, httptest.NewRequest("GET", "/__test/note-music-session/"+strconv.FormatUint(uint64(user.ID), 10), nil))
			if seed.Code != 204 {
				t.Fatalf("session seed: %d %s", seed.Code, seed.Body.String())
			}
			for _, cookie := range seed.Result().Cookies() {
				req.AddCookie(cookie)
			}
		}
	}
	result := httptest.NewRecorder()
	f.r.ServeHTTP(result, req)
	return result
}

func noteMusicBody(content string) string {
	body, _ := json.Marshal(map[string]any{"content": content})
	return string(body)
}

func TestNoteMusicWritesRequireManageAndPreserveExistingMarkers(t *testing.T) {
	f := setupNoteMusicRoutes(t)
	marker := "[♫ Fixture — Artist](/music/notes/" + f.trackID + ")"
	for _, token := range []bool{false, true} {
		prefix := "/api"
		if token {
			prefix = "/api/token"
		}
		if got := f.request(t, &f.editor, token, "POST", prefix+"/messages", noteMusicBody(marker)); got.Code != 403 || !strings.Contains(got.Body.String(), music.ErrForbidden.Error()) {
			t.Fatalf("new marker token=%v: %d %s", token, got.Code, got.Body.String())
		}
		message := models.Message{Content: marker, UserID: f.editor.ID, Visibility: "PUBLIC"}
		if err := f.db.Create(&message).Error; err != nil {
			t.Fatal(err)
		}
		path := fmt.Sprintf("%s/messages/%d", prefix, message.ID)
		if got := f.request(t, &f.editor, token, "PUT", path, noteMusicBody("edited "+marker)); got.Code != 200 || !strings.Contains(got.Body.String(), `"code":1`) {
			t.Fatalf("keep marker token=%v: %d %s", token, got.Code, got.Body.String())
		}
		for _, content := range []string{marker + "\n" + marker, "[♫ Replacement](/music/notes/94be5ee7-1fa9-469c-9cda-31db6152264b)"} {
			if got := f.request(t, &f.editor, token, "PUT", path, noteMusicBody(content)); got.Code != 403 {
				t.Fatalf("add/replace token=%v: %d %s", token, got.Code, got.Body.String())
			}
		}
		if got := f.request(t, &f.editor, token, "PUT", path, noteMusicBody("removed")); got.Code != 200 || !strings.Contains(got.Body.String(), `"code":1`) {
			t.Fatalf("remove marker token=%v: %d %s", token, got.Code, got.Body.String())
		}
		if got := f.request(t, &f.manager, token, "POST", prefix+"/messages", noteMusicBody(marker)); got.Code != 200 || !strings.Contains(got.Body.String(), `"code":1`) {
			t.Fatalf("manager write token=%v: %d %s", token, got.Code, got.Body.String())
		}
		if got := f.request(t, &f.manager, token, "POST", prefix+"/messages", noteMusicBody("[♫ Missing](/music/notes/94be5ee7-1fa9-469c-9cda-31db6152264b)")); got.Code != 400 || !strings.Contains(got.Body.String(), music.ErrNoteTrackInvalid.Error()) {
			t.Fatalf("missing track token=%v: %d %s", token, got.Code, got.Body.String())
		}
	}
	var count int64
	f.db.Model(&models.MusicPlaylistItem{}).Count(&count)
	var config models.MusicConfig
	f.db.First(&config, 1)
	if count != 0 || config.Version != 1 || config.Source != "netease" {
		t.Fatalf("note writes changed playlist/config: count=%d config=%+v", count, config)
	}
}

func TestNoteMusicDelegatedAdministratorRequiresMusicManage(t *testing.T) {
	f := setupNoteMusicRoutes(t)
	delegated := models.User{Username: "delegated-music", IsAdmin: true, Token: "delegated-music-token"}
	if err := repository.CreateUser(&delegated); err != nil {
		t.Fatal(err)
	}
	view := models.AdminCapabilityGrant{UserID: delegated.ID, Capability: "music.view", GrantedByUserID: f.manager.ID}
	if err := f.db.Create(&view).Error; err != nil {
		t.Fatal(err)
	}
	marker := "[♫ Fixture](/music/notes/" + f.trackID + ")"
	for _, token := range []bool{false, true} {
		prefix := "/api"
		if token {
			prefix = "/api/token"
		}
		path := prefix + "/messages"
		if got := f.request(t, &delegated, token, "POST", path, noteMusicBody(marker)); got.Code != 403 {
			t.Fatalf("view-only administrator token=%v: %d %s", token, got.Code, got.Body.String())
		}
		grant := models.AdminCapabilityGrant{UserID: delegated.ID, Capability: "music.manage", GrantedByUserID: f.manager.ID}
		if err := f.db.Create(&grant).Error; err != nil {
			t.Fatal(err)
		}
		if got := f.request(t, &delegated, token, "POST", path, noteMusicBody(marker)); got.Code != 200 || !strings.Contains(got.Body.String(), `"code":1`) {
			t.Fatalf("music manager token=%v: %d %s", token, got.Code, got.Body.String())
		}
		if err := f.db.Delete(&grant).Error; err != nil {
			t.Fatal(err)
		}
		if got := f.request(t, &delegated, token, "POST", path, noteMusicBody(marker)); got.Code != 403 {
			t.Fatalf("revoked music manager token=%v: %d %s", token, got.Code, got.Body.String())
		}
	}
}

func TestNoteMusicResolveSignedMediaVisibilityAndSwitch(t *testing.T) {
	f := setupNoteMusicRoutes(t)
	marker := "[♫ Fixture — Artist](/music/notes/" + f.trackID + ")"
	message := models.Message{Content: marker, UserID: f.editor.ID, Visibility: "PUBLIC"}
	if err := f.db.Create(&message).Error; err != nil {
		t.Fatal(err)
	}
	path := fmt.Sprintf("/api/music/notes/%s?messageID=%d", f.trackID, message.ID)
	resolve := func(user *models.User, token bool) *httptest.ResponseRecorder {
		return f.request(t, user, token, "GET", path, "")
	}
	got := resolve(nil, false)
	var envelope struct {
		Code int               `json:"code"`
		Data music.PublicTrack `json:"data"`
	}
	if got.Code != 200 || json.Unmarshal(got.Body.Bytes(), &envelope) != nil || envelope.Code != 1 || envelope.Data.TrackID != f.trackID {
		t.Fatalf("resolve nonplaylist, netease source: %d %s", got.Code, got.Body.String())
	}
	for _, field := range []string{"relativePath", "catalogKey", "fingerprint", "selected", "unselected.mp3"} {
		if strings.Contains(got.Body.String(), field) {
			t.Fatalf("administrative field leaked: %s", got.Body.String())
		}
	}
	for _, media := range []string{envelope.Data.StreamURL, envelope.Data.FallbackStreamURL, envelope.Data.CoverURL, envelope.Data.LyricsURL} {
		if media == "" {
			t.Fatal("missing playback URL")
		}
		if response := f.request(t, nil, false, "GET", media, ""); response.Code != 200 {
			t.Fatalf("signed media %s: %d %s", media, response.Code, response.Body.String())
		}
	}
	if response := f.request(t, nil, false, "GET", "/api/music/stream/"+f.trackID, ""); response.Code != 404 {
		t.Fatalf("unsigned nonplaylist access: %d", response.Code)
	}
	tampered, _ := url.Parse(envelope.Data.StreamURL)
	query := tampered.Query()
	query.Set("expires", strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10))
	tampered.RawQuery = query.Encode()
	if response := f.request(t, nil, false, "GET", tampered.String(), ""); response.Code != 404 {
		t.Fatalf("tampered signature: %d", response.Code)
	}
	if err := f.db.Model(&message).Updates(map[string]any{"private": true, "visibility": "PRIVATE"}).Error; err != nil {
		t.Fatal(err)
	}
	if got := resolve(nil, false); got.Code != 404 {
		t.Fatalf("private note guest resolve: %d", got.Code)
	}
	if got := f.request(t, nil, false, "GET", envelope.Data.StreamURL, ""); got.Code != 404 {
		t.Fatalf("private note old URL guest: %d", got.Code)
	}
	for _, token := range []bool{false, true} {
		if got := resolve(&f.editor, token); got.Code != 200 {
			t.Fatalf("private note owner token=%v: %d %s", token, got.Code, got.Body.String())
		}
	}
	if err := f.db.Model(&message).Update("content", "reference removed").Error; err != nil {
		t.Fatal(err)
	}
	if got := f.request(t, &f.editor, true, "GET", envelope.Data.StreamURL, ""); got.Code != 404 {
		t.Fatalf("removed reference URL: %d", got.Code)
	}
	if err := f.db.Model(&message).Update("content", marker).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.db.Model(&models.SiteConfig{}).Where("id = ?", 1).Update("music_enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	if got := resolve(&f.editor, true); got.Code != 404 {
		t.Fatalf("disabled music resolve: %d", got.Code)
	}
	if got := f.request(t, &f.editor, true, "GET", envelope.Data.StreamURL, ""); got.Code != 404 {
		t.Fatalf("disabled music signed stream: %d", got.Code)
	}
	if got := f.request(t, &f.manager, true, "POST", "/api/token/messages", noteMusicBody(marker)); got.Code != 400 {
		t.Fatalf("disabled music new marker: %d %s", got.Code, got.Body.String())
	}
	if got := f.request(t, &f.editor, true, "PUT", fmt.Sprintf("/api/token/messages/%d", message.ID), noteMusicBody("preserved "+marker)); got.Code != 200 || !strings.Contains(got.Body.String(), `"code":1`) {
		t.Fatalf("disabled music preserve old marker: %d %s", got.Code, got.Body.String())
	}
	for _, noReference := range []string{"/api/music/notes/" + f.trackID, fmt.Sprintf("/api/music/notes/%s?messageID=%d", f.trackID, message.ID+1000)} {
		if got := f.request(t, nil, false, http.MethodGet, noReference, ""); got.Code != 404 {
			t.Fatalf("missing note/reference: %d", got.Code)
		}
	}
}
