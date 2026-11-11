package routers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/middleware"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/repository"
	"github.com/rcy1314/echo-noise/internal/services"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestLoginExpiredRealRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("ACCESS_LOG", "false")
	t.Setenv("SESSION_SECRET", "login-expired-route-test-secret-32")
	t.Chdir(t.TempDir())
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := models.MigrateDB(db); err != nil {
		t.Fatal(err)
	}
	database.DB = db
	models.SetDB(db)
	repository.ClearUserCache()
	t.Cleanup(func() { repository.ClearUserCache(); database.DB = nil; models.SetDB(nil) })
	old, now := time.Now().Add(-48*time.Hour), time.Now()
	users := []models.User{
		{Username: "owner", Token: "owner-route-token", IsAdmin: true},
		{Username: "expired", Token: "expired-route-token", LoginIssuedAt: &old},
		{Username: "valid", Token: "valid-route-token", LoginIssuedAt: &now},
		{Username: "expired-admin", Token: "expired-admin-route-token", IsAdmin: true, LoginIssuedAt: &old},
	}
	for i := range users {
		if err := repository.CreateUser(&users[i]); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Create(&models.SiteConfig{LoginExpireDays: 1, DelegatedAdminLoginExpireDays: 1}).Error; err != nil {
		t.Fatal(err)
	}
	credential, aiToken, err := services.CreateAICredential(users[1].ID, "route test")
	if err != nil {
		t.Fatal(err)
	}
	r := SetupRouter(nil)
	r.GET("/__test/session/:state", func(c *gin.Context) {
		session := sessions.Default(c)
		session.Set("user_id", users[2].ID)
		issued := now.Unix()
		if c.Param("state") == "expired" {
			issued = old.Unix()
		}
		session.Set("login_issued_at", issued)
		if err := session.Save(); err != nil {
			t.Error(err)
		}
		c.Status(http.StatusNoContent)
	})
	r.GET("/__test/read-context", middleware.PublicReadAuthMiddleware(), func(c *gin.Context) {
		for _, key := range []string{"user_id", "is_admin", "auth_via", "credential_id"} {
			if _, exists := c.Get(key); exists {
				t.Errorf("read guard leaked authorization context %s", key)
			}
		}
		c.JSON(http.StatusOK, gin.H{"code": 1})
	})
	request := func(method, path, token, body string, cookies []*http.Cookie) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		for _, cookie := range cookies {
			req.AddCookie(cookie)
		}
		response := httptest.NewRecorder()
		r.ServeHTTP(response, req)
		return response
	}
	assertFailure := func(response *httptest.ResponseRecorder, status int, code string) {
		t.Helper()
		var body map[string]any
		// Unmarshal rejects a second JSON response appended by a handler.
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatalf("multiple or invalid responses: %s (%v)", response.Body.String(), err)
		}
		if response.Code != status || body["code"] != float64(0) || body["error"] != code {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
		if code == "login_expired" && body["msg"] != "登录已过期，请重新登录站点或通过登录接口续期后重试。" {
			t.Fatalf("wrong expiry message: %v", body)
		}
		if code == "invalid_credentials" && strings.Contains(body["msg"].(string), "过期") {
			t.Fatalf("invalid credential reported expired: %v", body)
		}
	}
	paths := []string{"/api/messages/search?q=test", "/api/messages/page", "/api/messages/99999", "/api/messages/calendar", "/api/frontend/config", "/api/settings", "/api/status", "/api", "/api/auth/identity", "/api/token/user/widget-preferences", "/api/user"}
	for _, token := range []string{users[1].Token, aiToken, users[3].Token} {
		for _, path := range paths {
			t.Run(path+"/"+token[:7], func(t *testing.T) {
				assertFailure(request(http.MethodGet, path, token, "", nil), http.StatusUnauthorized, "login_expired")
			})
		}
		assertFailure(request(http.MethodPost, "/api/messages/page", token, "{}", nil), http.StatusUnauthorized, "login_expired")
	}
	for _, path := range []string{"/api/messages/search?q=test", "/api/messages/page", "/api/messages/calendar", "/api/frontend/config", "/api/status"} {
		response := request(http.MethodGet, path, "", "", nil)
		if response.Code != http.StatusOK {
			t.Fatalf("guest %s: %d %s", path, response.Code, response.Body.String())
		}
	}
	cookies := request(http.MethodGet, "/__test/session/valid", "", "", nil).Result().Cookies()
	for _, path := range []string{"/api/status", "/api/messages/page", "/api/user"} {
		response := request(http.MethodGet, path, aiToken, "", cookies)
		if response.Code != http.StatusOK {
			t.Fatalf("session precedence %s: %d %s", path, response.Code, response.Body.String())
		}
	}
	assertFailure(request(http.MethodGet, "/api/auth/identity", aiToken, "", cookies), http.StatusUnauthorized, "login_expired")
	expiredCookies := request(http.MethodGet, "/__test/session/expired", "", "", nil).Result().Cookies()
	for _, path := range []string{"/api/status", "/api/user", "/api/user/ai-credentials"} {
		assertFailure(request(http.MethodGet, path, users[0].Token, "", expiredCookies), http.StatusUnauthorized, "login_expired")
	}
	// Invalid JSON proves that recovery reaches its handler despite expired credentials/session.
	for _, path := range []string{"/api/login", "/api/ai/login"} {
		response := request(http.MethodPost, path, aiToken, "{", expiredCookies)
		wantStatus := http.StatusBadRequest
		if path == "/api/login" {
			wantStatus = http.StatusOK
		}
		if response.Code != wantStatus || !strings.Contains(response.Body.String(), "参数错误") || strings.Contains(response.Body.String(), "login_expired") {
			t.Fatalf("recovery blocked %s: %d %s", path, response.Code, response.Body.String())
		}
	}
	if response := request(http.MethodGet, "/__test/read-context", users[2].Token, "", nil); response.Code != http.StatusOK {
		t.Fatal(response.Body.String())
	}
	if err := services.RevokeAICredential(users[1].ID, credential.ID); err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{aiToken, "invalid-token"} {
		for _, path := range []string{"/api/status", "/api/auth/identity", "/api/token/user/widget-preferences", "/api/user"} {
			assertFailure(request(http.MethodGet, path, token, "", nil), http.StatusUnauthorized, "invalid_credentials")
		}
	}
	if err := db.Callback().Query().Before("gorm:query").Register("test:authentication_database_failure", func(tx *gorm.DB) { tx.AddError(gorm.ErrInvalidDB) }); err != nil {
		t.Fatal(err)
	}
	defer db.Callback().Query().Remove("test:authentication_database_failure")
	for _, path := range []string{"/api/status", "/api/auth/identity", "/api/token/user/widget-preferences", "/api/user"} {
		assertFailure(request(http.MethodGet, path, users[2].Token, "", nil), http.StatusInternalServerError, "authentication_unavailable")
	}
	assertFailure(request(http.MethodGet, "/api/user", "", "", cookies), http.StatusInternalServerError, "authentication_unavailable")
}
