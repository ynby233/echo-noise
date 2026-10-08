package routers

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/rcy1314/echo-noise/internal/authorization"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/music"
	"github.com/rcy1314/echo-noise/internal/repository"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestMusicRoutesEnforceViewManageAndStrictPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("ACCESS_LOG", "false")
	t.Setenv("SESSION_SECRET", "music-integration-session-secret-32")
	t.Chdir(t.TempDir())
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := models.MigrateDB(db); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.SiteConfig{Model: gorm.Model{ID: 1}, MusicPosition: "bottom-left", MusicTheme: "auto"}).Error; err != nil {
		t.Fatal(err)
	}
	database.DB = db
	models.SetDB(db)
	repository.ClearUserCache()
	service := music.NewService(db, music.Options{RootDir: t.TempDir(), CacheDir: t.TempDir()})
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
	primary := models.User{Username: "music-primary", IsAdmin: true, Token: "music-primary-token"}
	viewer := models.User{Username: "music-viewer", IsAdmin: true, Token: "music-viewer-token"}
	for _, user := range []*models.User{&primary, &viewer} {
		if err := repository.CreateUser(user); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Create(&models.AdminCapabilityGrant{UserID: viewer.ID, Capability: string(authorization.CapabilityMusicView), GrantedByUserID: primary.ID}).Error; err != nil {
		t.Fatal(err)
	}
	r := SetupRouter(service)
	r.GET("/__test/music-session/:id", func(c *gin.Context) {
		session := sessions.Default(c)
		session.Set("user_id", c.Param("id"))
		session.Set("login_expire_at", time.Now().Add(time.Hour).Unix())
		if err := session.Save(); err != nil {
			c.Status(500)
			return
		}
		c.Status(204)
	})
	seed := httptest.NewRecorder()
	r.ServeHTTP(seed, httptest.NewRequest("GET", "/__test/music-session/"+strconv.FormatUint(uint64(viewer.ID), 10), nil))
	if seed.Code != 204 {
		t.Fatalf("session seed: %d", seed.Code)
	}
	for _, token := range []bool{false, true} {
		prefix := "/api"
		if token {
			prefix = "/api/token"
		}
		request := func(method, path, body string) *httptest.ResponseRecorder {
			req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
			req.Header.Set("Content-Type", "application/json")
			if token {
				req.Header.Set("Authorization", "Bearer "+viewer.Token)
			} else {
				for _, cookie := range seed.Result().Cookies() {
					req.AddCookie(cookie)
				}
			}
			response := httptest.NewRecorder()
			r.ServeHTTP(response, req)
			return response
		}
		for _, path := range []string{"/music/config", "/music/library", "/music/playlist", "/music/library/refresh-status"} {
			if got := request("GET", prefix+path, ""); got.Code != 200 {
				t.Fatalf("view %s: %d %s", prefix+path, got.Code, got.Body.String())
			}
		}
		if got := request("PUT", prefix+"/music/playlist", `{"version":1,"trackIDs":[]}`); got.Code != 403 {
			t.Fatalf("view-only write: %d %s", got.Code, got.Body.String())
		}
		if got := request("HEAD", prefix+"/music/library/not-a-track/cover", ""); got.Code != 404 || got.Body.Len() != 0 {
			t.Fatalf("admin cover HEAD: %d %s", got.Code, got.Body.String())
		}
	}
	if err := db.Create(&models.AdminCapabilityGrant{UserID: viewer.ID, Capability: string(authorization.CapabilityMusicManage), GrantedByUserID: primary.ID}).Error; err != nil {
		t.Fatal(err)
	}
	for _, payload := range []string{`null`, `{}`, `{"version":1,"trackIDs":null}`, `{"version":1,"trackIDs":[],"extra":true}`, `{"version":1,"trackIDs":[]} {}`} {
		req := httptest.NewRequest("PUT", "/api/token/music/playlist", bytes.NewBufferString(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+viewer.Token)
		response := httptest.NewRecorder()
		r.ServeHTTP(response, req)
		if response.Code != 400 {
			t.Fatalf("invalid payload %s: %d %s", payload, response.Code, response.Body.String())
		}
	}
	var config models.MusicConfig
	if err := db.First(&config, 1).Error; err != nil {
		t.Fatal(err)
	}
	if config.Version != 1 {
		t.Fatalf("rejected writes changed version: %d", config.Version)
	}
	for attempt, want := range []int{http.StatusOK, http.StatusConflict} {
		req := httptest.NewRequest("PUT", "/api/token/music/playlist", bytes.NewBufferString(`{"version":1,"trackIDs":[]}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+viewer.Token)
		response := httptest.NewRecorder()
		r.ServeHTTP(response, req)
		if response.Code != want {
			t.Fatalf("playlist save attempt %d: %d %s", attempt, response.Code, response.Body.String())
		}
	}
	if err := db.First(&config, 1).Error; err != nil {
		t.Fatal(err)
	}
	if config.Version != 2 {
		t.Fatalf("conflicting save changed revision: %d", config.Version)
	}
	for _, method := range []string{"GET", "HEAD"} {
		response := httptest.NewRecorder()
		r.ServeHTTP(response, httptest.NewRequest(method, "/api/music/cover/not-a-track", nil))
		if response.Code != 404 {
			t.Fatalf("public cover %s: %d", method, response.Code)
		}
		if method == "HEAD" && response.Body.Len() != 0 {
			t.Fatal("HEAD returned a body")
		}
	}
}
