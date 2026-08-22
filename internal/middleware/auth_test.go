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
	"github.com/rcy1314/echo-noise/internal/services"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupSessionAuthMiddlewareTest(t *testing.T) (*gin.Engine, models.User, models.User, models.User) {
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

	now := time.Now()
	admin := models.User{Username: "admin", Token: "admin-token", IsAdmin: true, LoginIssuedAt: &now}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatalf("create admin user: %v", err)
	}
	user := models.User{Username: "ordinary", Token: "ordinary-token", IsAdmin: false}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create ordinary user: %v", err)
	}
	delegated := models.User{Username: "delegated", Token: "delegated-token", IsAdmin: true, LoginIssuedAt: &now}
	if err := db.Create(&delegated).Error; err != nil {
		t.Fatalf("create delegated admin user: %v", err)
	}

	r := gin.New()
	r.Use(sessions.Sessions("test", cookie.NewStore([]byte("session-auth-test-secret"))))
	r.GET("/seed", func(c *gin.Context) {
		session := sessions.Default(c)
		isAdmin := c.Query("admin") == "true"
		selected := user
		if isAdmin {
			selected = admin
		} else if c.Query("role") == "delegated" {
			selected = delegated
		}
		session.Set("user_id", selected.ID)
		session.Set("username", selected.Username)
		session.Set("is_admin", selected.IsAdmin)
		expireAt, _ := strconv.ParseInt(c.DefaultQuery("expire_at", "0"), 10, 64)
		session.Set("login_expire_at", expireAt)
		issuedAt := time.Now().Unix()
		if parsed, err := strconv.ParseInt(c.Query("issued_at"), 10, 64); err == nil && parsed > 0 {
			issuedAt = parsed
		}
		if !selected.IsAdmin && expireAt > 0 {
			issuedAt = expireAt - int64(3*24*time.Hour/time.Second)
		}
		session.Set("login_issued_at", issuedAt)
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
	return r, user, admin, delegated
}

func TestSessionAuthMiddlewareRejectsExpiredOrdinarySessionDespiteBearerToken(t *testing.T) {
	r, _, _, _ := setupSessionAuthMiddlewareTest(t)
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
	r, _, _, _ := setupSessionAuthMiddlewareTest(t)
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
	r, _, _, _ := setupSessionAuthMiddlewareTest(t)
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

func TestSessionAuthMiddlewareAppliesDelegatedAdminExpiryToSessionAndBearer(t *testing.T) {
	r, _, _, delegated := setupSessionAuthMiddlewareTest(t)
	db, err := database.GetDB()
	if err != nil {
		t.Fatalf("get test db: %v", err)
	}
	if err := db.Create(&models.SiteConfig{}).Error; err != nil {
		t.Fatalf("create login policy: %v", err)
	}
	if err := db.Model(&models.SiteConfig{}).Where("1 = 1").Updates(map[string]interface{}{
		"login_expire_days":                  2,
		"login_expire_hours":                 0,
		"delegated_admin_login_expire_days":  0,
		"delegated_admin_login_expire_hours": 1,
	}).Error; err != nil {
		t.Fatalf("configure delegated login policy: %v", err)
	}

	issuedAt := time.Now().Unix()
	if err := db.Model(&models.User{}).Where("id = ?", delegated.ID).Update("login_issued_at", time.Unix(issuedAt, 0)).Error; err != nil {
		t.Fatalf("set delegated issued time: %v", err)
	}

	validBearer := httptest.NewRequest(http.MethodGet, "/protected", nil)
	validBearer.Header.Set("Authorization", "Bearer delegated-token")
	validBearerResponse := httptest.NewRecorder()
	r.ServeHTTP(validBearerResponse, validBearer)
	if validBearerResponse.Code != http.StatusOK {
		t.Fatalf("delegated bearer inside its configured lifetime got %d: %s", validBearerResponse.Code, validBearerResponse.Body.String())
	}

	validSeed := httptest.NewRecorder()
	r.ServeHTTP(validSeed, httptest.NewRequest(http.MethodGet, "/seed?role=delegated&issued_at="+strconv.FormatInt(issuedAt, 10), nil))
	validSession := httptest.NewRequest(http.MethodGet, "/protected", nil)
	for _, cookie := range validSeed.Result().Cookies() {
		validSession.AddCookie(cookie)
	}
	validSessionResponse := httptest.NewRecorder()
	r.ServeHTTP(validSessionResponse, validSession)
	if validSessionResponse.Code != http.StatusOK {
		t.Fatalf("delegated session inside its configured lifetime got %d: %s", validSessionResponse.Code, validSessionResponse.Body.String())
	}

	expiredIssuedAt := time.Now().Add(-2 * time.Hour).Unix()
	if err := db.Model(&models.User{}).Where("id = ?", delegated.ID).Update("login_issued_at", time.Unix(expiredIssuedAt, 0)).Error; err != nil {
		t.Fatalf("expire delegated bearer: %v", err)
	}
	var expiredDelegated models.User
	if err := db.First(&expiredDelegated, delegated.ID).Error; err != nil {
		t.Fatalf("reload delegated admin: %v", err)
	}
	if expiredDelegated.LoginIssuedAt == nil || !services.IsUserLoginExpired(&expiredDelegated, expiredDelegated.LoginIssuedAt.Unix(), time.Now()) {
		t.Fatalf("test setup did not create an expired delegated credential: %#v", expiredDelegated.LoginIssuedAt)
	}
	expiredBearer := httptest.NewRequest(http.MethodGet, "/protected", nil)
	expiredBearer.Header.Set("Authorization", "Bearer delegated-token")
	expiredBearerResponse := httptest.NewRecorder()
	r.ServeHTTP(expiredBearerResponse, expiredBearer)
	if expiredBearerResponse.Code != http.StatusUnauthorized {
		t.Fatalf("expired delegated bearer should be rejected, got %d: %s", expiredBearerResponse.Code, expiredBearerResponse.Body.String())
	}

	expiredSeed := httptest.NewRecorder()
	r.ServeHTTP(expiredSeed, httptest.NewRequest(http.MethodGet, "/seed?role=delegated&issued_at="+strconv.FormatInt(expiredIssuedAt, 10), nil))
	expiredSession := httptest.NewRequest(http.MethodGet, "/protected", nil)
	for _, cookie := range expiredSeed.Result().Cookies() {
		expiredSession.AddCookie(cookie)
	}
	expiredSessionResponse := httptest.NewRecorder()
	r.ServeHTTP(expiredSessionResponse, expiredSession)
	if expiredSessionResponse.Code != http.StatusUnauthorized {
		t.Fatalf("expired delegated session should be rejected, got %d: %s", expiredSessionResponse.Code, expiredSessionResponse.Body.String())
	}

	if err := db.Model(&models.SiteConfig{}).Where("1 = 1").Updates(map[string]interface{}{
		"delegated_admin_login_expire_days":  0,
		"delegated_admin_login_expire_hours": 0,
	}).Error; err != nil {
		t.Fatalf("make delegated policy permanent: %v", err)
	}
	permanentBearer := httptest.NewRequest(http.MethodGet, "/protected", nil)
	permanentBearer.Header.Set("Authorization", "Bearer delegated-token")
	permanentBearerResponse := httptest.NewRecorder()
	r.ServeHTTP(permanentBearerResponse, permanentBearer)
	if permanentBearerResponse.Code != http.StatusOK {
		t.Fatalf("delegated bearer should follow updated permanent policy, got %d: %s", permanentBearerResponse.Code, permanentBearerResponse.Body.String())
	}

	permanentSeed := httptest.NewRecorder()
	r.ServeHTTP(permanentSeed, httptest.NewRequest(http.MethodGet, "/seed?role=delegated&issued_at="+strconv.FormatInt(expiredIssuedAt, 10), nil))
	permanentSession := httptest.NewRequest(http.MethodGet, "/protected", nil)
	for _, cookie := range permanentSeed.Result().Cookies() {
		permanentSession.AddCookie(cookie)
	}
	permanentSessionResponse := httptest.NewRecorder()
	r.ServeHTTP(permanentSessionResponse, permanentSession)
	if permanentSessionResponse.Code != http.StatusOK {
		t.Fatalf("delegated session should follow updated permanent policy, got %d: %s", permanentSessionResponse.Code, permanentSessionResponse.Body.String())
	}
}

func TestSessionAuthMiddlewareUsesCurrentDatabaseRoleAndOrdinaryPermanentPolicy(t *testing.T) {
	r, _, _, delegated := setupSessionAuthMiddlewareTest(t)
	db, err := database.GetDB()
	if err != nil {
		t.Fatalf("get test db: %v", err)
	}
	if err := db.Create(&models.SiteConfig{}).Error; err != nil {
		t.Fatalf("create login policy: %v", err)
	}
	if err := db.Model(&models.SiteConfig{}).Where("1 = 1").Updates(map[string]interface{}{
		"login_expire_days":                  0,
		"login_expire_hours":                 0,
		"delegated_admin_login_expire_days":  0,
		"delegated_admin_login_expire_hours": 1,
	}).Error; err != nil {
		t.Fatalf("configure ordinary and delegated login policies: %v", err)
	}
	expiredIssuedAt := time.Now().Add(-2 * time.Hour).Unix()
	seed := httptest.NewRecorder()
	r.ServeHTTP(seed, httptest.NewRequest(http.MethodGet, "/seed?role=delegated&issued_at="+strconv.FormatInt(expiredIssuedAt, 10), nil))
	if err := db.Model(&models.User{}).Where("id = ?", delegated.ID).Update("is_admin", false).Error; err != nil {
		t.Fatalf("demote delegated user: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	for _, cookie := range seed.Result().Cookies() {
		req.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	r.ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("demoted delegated session should use ordinary permanent policy, got %d: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"is_admin":false`) {
		t.Fatalf("session must expose the current database role, got %s", response.Body.String())
	}
}

func TestSessionAuthMiddlewareKeepsPrimaryAdminBearerPermanent(t *testing.T) {
	r, _, admin, _ := setupSessionAuthMiddlewareTest(t)
	db, err := database.GetDB()
	if err != nil {
		t.Fatalf("get test db: %v", err)
	}
	if err := db.Create(&models.SiteConfig{}).Error; err != nil {
		t.Fatalf("create login policy: %v", err)
	}
	if err := db.Model(&models.SiteConfig{}).Where("1 = 1").Updates(map[string]interface{}{
		"login_expire_days":                  0,
		"login_expire_hours":                 1,
		"delegated_admin_login_expire_days":  0,
		"delegated_admin_login_expire_hours": 1,
	}).Error; err != nil {
		t.Fatalf("configure login policies: %v", err)
	}
	if err := db.Model(&models.User{}).Where("id = ?", admin.ID).Update("login_issued_at", time.Now().Add(-72*time.Hour)).Error; err != nil {
		t.Fatalf("age primary admin credential: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer admin-token")
	response := httptest.NewRecorder()
	r.ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("primary admin bearer must remain permanent, got %d: %s", response.Code, response.Body.String())
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
