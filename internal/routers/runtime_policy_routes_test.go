package routers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/repository"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestRuntimePolicyRoutesArePrimaryOnlyAndRedactHealthDetails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("ACCESS_LOG", "false")
	t.Setenv("SESSION_SECRET", "runtime-policy-route-test-secret-32")
	t.Chdir(t.TempDir())
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := models.MigrateDB(db); err != nil {
		t.Fatalf("migrate database: %v", err)
	}
	database.DB = db
	models.SetDB(db)
	repository.ClearUserCache()
	t.Cleanup(func() {
		repository.ClearUserCache()
		database.DB = nil
		models.SetDB(nil)
	})
	primary := models.User{Username: "primary-runtime-route", Password: models.HashPassword("primary"), IsAdmin: true}
	delegated := models.User{Username: "delegated-runtime-route", Password: models.HashPassword("delegated"), IsAdmin: true}
	if err := repository.CreateUser(&primary); err != nil {
		t.Fatalf("create primary: %v", err)
	}
	if err := repository.CreateUser(&delegated); err != nil {
		t.Fatalf("create delegated: %v", err)
	}
	if err := db.Create(&models.SiteConfig{
		RuntimeMode:                 models.RuntimeModeLocal,
		RuntimeModeMigrationVersion: models.RuntimeModeMigrationVersionCurrent,
		VoceChatLastHealthStatus:    "failed",
		VoceChatLastHealthError:     "Authorization: Bearer secret-upstream-detail",
		VoceChatLastHealthCheckAt:   func() *time.Time { value := time.Now().UTC(); return &value }(),
	}).Error; err != nil {
		t.Fatalf("create site config: %v", err)
	}

	r := SetupRouter()
	r.GET("/__test/runtime-session/:id", func(c *gin.Context) {
		session := sessions.Default(c)
		session.Set("user_id", c.Param("id"))
		session.Set("login_expire_at", time.Now().Add(time.Hour).Unix())
		if err := session.Save(); err != nil {
			c.Status(http.StatusInternalServerError)
			return
		}
		c.Status(http.StatusNoContent)
	})
	seedSession := func(userID uint) []*http.Cookie {
		t.Helper()
		response := httptest.NewRecorder()
		r.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/__test/runtime-session/"+strconv.FormatUint(uint64(userID), 10), nil))
		if response.Code != http.StatusNoContent {
			t.Fatalf("seed session status=%d body=%s", response.Code, response.Body.String())
		}
		return response.Result().Cookies()
	}
	request := func(method string, path string, body []byte, cookies []*http.Cookie) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		for _, cookie := range cookies {
			req.AddCookie(cookie)
		}
		response := httptest.NewRecorder()
		r.ServeHTTP(response, req)
		return response
	}

	delegatedCookies := seedSession(delegated.ID)
	primaryCookies := seedSession(primary.ID)
	if response := request(http.MethodGet, "/api/admin/runtime-policy", nil, delegatedCookies); response.Code != http.StatusForbidden {
		t.Fatalf("delegated diagnostics status=%d body=%s", response.Code, response.Body.String())
	}
	response := request(http.MethodGet, "/api/admin/runtime-policy", nil, primaryCookies)
	if response.Code != http.StatusOK {
		t.Fatalf("primary diagnostics status=%d body=%s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "secret-upstream-detail") || strings.Contains(strings.ToLower(response.Body.String()), "bearer") {
		t.Fatalf("runtime diagnostics leaked raw upstream detail: %s", response.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode diagnostics: %v", err)
	}
	data, _ := payload["data"].(map[string]any)
	if data["configured_mode"] != models.RuntimeModeLocal || data["runtime_state"] != models.RuntimeModeLocal {
		t.Fatalf("runtime diagnostics = %#v", data)
	}

	if response := request(http.MethodPut, "/api/admin/runtime-policy/mode", []byte(`{"mode":"vocechat"}`), delegatedCookies); response.Code != http.StatusForbidden {
		t.Fatalf("delegated switch status=%d body=%s", response.Code, response.Body.String())
	}
	if response := request(http.MethodPut, "/api/admin/runtime-policy/mode", []byte(`{"mode":"invalid"}`), primaryCookies); response.Code != http.StatusBadRequest {
		t.Fatalf("invalid switch status=%d body=%s", response.Code, response.Body.String())
	}
}
