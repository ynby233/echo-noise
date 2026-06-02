package middleware

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/repository"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupSessionAuthMiddlewareTest(t *testing.T) (*gin.Engine, models.User, models.User) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.SiteConfig{}); err != nil {
		t.Fatalf("migrate test db: %v", err)
	}
	repository.ClearUserCache()
	database.DB = db
	models.SetDB(db)
	t.Cleanup(func() {
		repository.ClearUserCache()
		database.DB = nil
		models.SetDB(nil)
	})

	user := models.User{Username: "ordinary", Token: "ordinary-token", IsAdmin: false}
	admin := models.User{Username: "admin", Token: "admin-token", IsAdmin: true}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create ordinary user: %v", err)
	}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatalf("create admin user: %v", err)
	}

	r := gin.New()
	r.Use(sessions.Sessions("test", cookie.NewStore([]byte("session-auth-test-secret"))))
	r.GET("/seed", func(c *gin.Context) {
		session := sessions.Default(c)
		isAdmin := c.Query("admin") == "true"
		selected := user
		if isAdmin {
			selected = admin
		}
		session.Set("user_id", selected.ID)
		session.Set("username", selected.Username)
		session.Set("is_admin", selected.IsAdmin)
		expireAt, _ := strconv.ParseInt(c.DefaultQuery("expire_at", "0"), 10, 64)
		session.Set("login_expire_at", expireAt)
		if err := session.Save(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.Status(http.StatusNoContent)
	})
	r.GET("/protected", SessionAuthMiddleware(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"user_id":  c.GetUint("user_id"),
			"username": c.GetString("username"),
			"is_admin": c.GetBool("is_admin"),
			"auth_via": c.GetString("auth_via"),
		})
	})
	return r, user, admin
}

func TestSessionAuthMiddlewareRejectsExpiredOrdinarySessionDespiteBearerToken(t *testing.T) {
	r, _, _ := setupSessionAuthMiddlewareTest(t)
	expiredAt := time.Now().Add(-time.Hour).Unix()
	seed := httptest.NewRecorder()
	r.ServeHTTP(seed, httptest.NewRequest(http.MethodGet, "/seed?expire_at="+strconv.FormatInt(expiredAt, 10), nil))
	if seed.Code != http.StatusNoContent {
		t.Fatalf("seed session got %d: %s", seed.Code, seed.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	for _, cookie := range seed.Result().Cookies() {
		req.AddCookie(cookie)
	}
	req.Header.Set("Authorization", "Bearer ordinary-token")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expired ordinary session with bearer token should be rejected, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSessionAuthMiddlewareAllowsAdminBearerTokenFallback(t *testing.T) {
	r, _, _ := setupSessionAuthMiddlewareTest(t)
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer admin-token")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("admin bearer token should be accepted, got %d: %s", w.Code, w.Body.String())
	}
	if body := w.Body.String(); body == "" || !containsAll(body, "admin", "token") {
		t.Fatalf("admin token response missing expected context: %s", body)
	}
}

func TestSessionAuthMiddlewareDoesNotExpireAdminSession(t *testing.T) {
	r, _, _ := setupSessionAuthMiddlewareTest(t)
	expiredAt := time.Now().Add(-time.Hour).Unix()
	seed := httptest.NewRecorder()
	r.ServeHTTP(seed, httptest.NewRequest(http.MethodGet, "/seed?admin=true&expire_at="+strconv.FormatInt(expiredAt, 10), nil))
	if seed.Code != http.StatusNoContent {
		t.Fatalf("seed admin session got %d: %s", seed.Code, seed.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	for _, cookie := range seed.Result().Cookies() {
		req.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("admin session should ignore login expiry, got %d: %s", w.Code, w.Body.String())
	}
	if body := w.Body.String(); body == "" || !containsAll(body, "admin", "session") {
		t.Fatalf("admin session response missing expected context: %s", body)
	}
}

func containsAll(s string, needles ...string) bool {
	for _, needle := range needles {
		if !strings.Contains(s, needle) {
			return false
		}
	}
	return true
}
