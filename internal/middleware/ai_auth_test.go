package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/services"
)

func TestAIBearerCannotUseAdminFallbackOrSessionOnlyManagement(t *testing.T) {
	r, user, admin, _ := setupSessionAuthMiddlewareTest(t)
	if err := database.DB.AutoMigrate(&models.AICredential{}); err != nil {
		t.Fatal(err)
	}
	_, aiToken, err := services.CreateAICredential(admin.ID, "admin agent")
	if err != nil {
		t.Fatal(err)
	}
	r.POST("/session-only", SessionOnlyAuthMiddleware(), func(c *gin.Context) { c.Status(http.StatusNoContent) })
	r.POST("/api/token/business", TokenAuthMiddleware(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"user_id": c.GetUint("user_id"), "auth_via": c.GetString("auth_via")})
	})
	for _, token := range []string{admin.Token, user.Token, aiToken} {
		request := httptest.NewRequest(http.MethodPost, "/session-only", nil)
		request.Header.Set("Authorization", "Bearer "+token)
		request.Header.Set("X-Auth-Via", "session")
		response := httptest.NewRecorder()
		r.ServeHTTP(response, request)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("bearer reached session-only management: status=%d", response.Code)
		}
	}
	for _, token := range []string{aiToken, user.Token} {
		request := httptest.NewRequest(http.MethodGet, "/protected", nil)
		request.Header.Set("Authorization", "Bearer "+token)
		response := httptest.NewRecorder()
		r.ServeHTTP(response, request)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("non-admin-API bearer reached session route: status=%d", response.Code)
		}
	}
	request := httptest.NewRequest(http.MethodPost, "/api/token/business", nil)
	request.Header.Set("Authorization", "bEaReR "+aiToken)
	response := httptest.NewRecorder()
	r.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"auth_via":"ai_token"`) {
		t.Fatalf("AI token business auth failed: %d %s", response.Code, response.Body.String())
	}
	seed := httptest.NewRecorder()
	r.ServeHTTP(seed, httptest.NewRequest(http.MethodGet, "/seed?admin=true", nil))
	request = httptest.NewRequest(http.MethodPost, "/session-only", nil)
	for _, cookie := range seed.Result().Cookies() {
		request.AddCookie(cookie)
	}
	response = httptest.NewRecorder()
	r.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("valid session could not manage credentials: %d", response.Code)
	}
}

func TestAIReadIdentityDoesNotAuthorizeWritesAndTracksRevocationAndRole(t *testing.T) {
	r, _, _, delegated := setupSessionAuthMiddlewareTest(t)
	if err := database.DB.AutoMigrate(&models.AICredential{}); err != nil {
		t.Fatal(err)
	}
	credential, token, err := services.CreateAICredential(delegated.ID, "reader")
	if err != nil {
		t.Fatal(err)
	}
	r.GET("/reader", func(c *gin.Context) {
		user, via, ok := ResolveReadIdentity(c)
		_, authorized := c.Get("user_id")
		c.JSON(http.StatusOK, gin.H{"authenticated": ok, "is_admin": user.IsAdmin, "auth_via": via, "authorized": authorized})
	})
	r.POST("/reader-then-write", func(c *gin.Context) {
		ResolveReadIdentity(c)
		c.Next()
	}, SessionAuthMiddleware(), func(c *gin.Context) { c.Status(http.StatusNoContent) })
	request := func(method, path string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("X-Auth-Via", "session")
		response := httptest.NewRecorder()
		r.ServeHTTP(response, req)
		return response
	}
	response := request(http.MethodGet, "/reader")
	if !containsAll(response.Body.String(), `"authenticated":true`, `"authorized":false`, `"auth_via":"ai_token"`, `"is_admin":true`) {
		t.Fatalf("read identity is not isolated: %s", response.Body.String())
	}
	if response := request(http.MethodPost, "/reader-then-write"); response.Code != http.StatusUnauthorized {
		t.Fatalf("reader authorized session write: %d", response.Code)
	}
	if err := database.DB.Model(&models.User{}).Where("id = ?", delegated.ID).Update("is_admin", false).Error; err != nil {
		t.Fatal(err)
	}
	if response := request(http.MethodGet, "/reader"); !strings.Contains(response.Body.String(), `"is_admin":false`) {
		t.Fatalf("stale administrator identity: %s", response.Body.String())
	}
	if err := services.RevokeAICredential(delegated.ID, credential.ID); err != nil {
		t.Fatal(err)
	}
	if response := request(http.MethodGet, "/reader"); !strings.Contains(response.Body.String(), `"authenticated":false`) {
		t.Fatalf("revoked reader authenticated: %s", response.Body.String())
	}
}

func TestAIAndOrdinaryBearerFollowLiveLoginExpiry(t *testing.T) {
	r, _, _, delegated := setupSessionAuthMiddlewareTest(t)
	if err := database.DB.AutoMigrate(&models.AICredential{}); err != nil {
		t.Fatal(err)
	}
	_, aiToken, err := services.CreateAICredential(delegated.ID, "expiring")
	if err != nil {
		t.Fatal(err)
	}
	if err := database.DB.Create(&models.SiteConfig{}).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.DB.Model(&models.SiteConfig{}).Where("1 = 1").Updates(map[string]interface{}{
		"delegated_admin_login_expire_days":  0,
		"delegated_admin_login_expire_hours": 1,
	}).Error; err != nil {
		t.Fatal(err)
	}
	r.GET("/api/token/expiry", TokenAuthMiddleware(), func(c *gin.Context) { c.Status(http.StatusNoContent) })
	for _, age := range []time.Duration{0, -2 * time.Hour} {
		if err := database.DB.Model(&models.User{}).Where("id = ?", delegated.ID).Update("login_issued_at", time.Now().Add(age)).Error; err != nil {
			t.Fatal(err)
		}
		for _, token := range []string{delegated.Token, aiToken} {
			request := httptest.NewRequest(http.MethodGet, "/api/token/expiry", nil)
			request.Header.Set("Authorization", "Bearer "+token)
			response := httptest.NewRecorder()
			r.ServeHTTP(response, request)
			want := http.StatusNoContent
			if age != 0 {
				want = http.StatusUnauthorized
			}
			if response.Code != want {
				t.Fatalf("age=%v status=%d want=%d", age, response.Code, want)
			}
		}
	}
}

func TestLegacyRawOrdinaryTokenCompatibilityDoesNotExpandPublicReads(t *testing.T) {
	r, user, admin, _ := setupSessionAuthMiddlewareTest(t)
	if err := database.DB.AutoMigrate(&models.AICredential{}); err != nil {
		t.Fatal(err)
	}
	_, aiToken, err := services.CreateAICredential(admin.ID, "raw rejected")
	if err != nil {
		t.Fatal(err)
	}
	r.GET("/legacy-token", TokenAuthMiddleware(), func(c *gin.Context) { c.Status(http.StatusNoContent) })
	r.GET("/public-reader", func(c *gin.Context) {
		_, _, ok := ResolveReadIdentity(c)
		c.JSON(http.StatusOK, gin.H{"authenticated": ok})
	})
	for _, test := range []struct {
		path   string
		header string
		status int
	}{
		{"/legacy-token", user.Token, http.StatusNoContent},
		{"/protected", admin.Token, http.StatusOK},
		{"/protected", user.Token, http.StatusUnauthorized},
		{"/legacy-token", aiToken, http.StatusUnauthorized},
		{"/protected", aiToken, http.StatusUnauthorized},
	} {
		request := httptest.NewRequest(http.MethodGet, test.path, nil)
		request.Header.Set("Authorization", test.header)
		response := httptest.NewRecorder()
		r.ServeHTTP(response, request)
		if response.Code != test.status {
			t.Fatalf("path=%s status=%d want=%d", test.path, response.Code, test.status)
		}
	}
	for _, token := range []string{user.Token, admin.Token, aiToken} {
		request := httptest.NewRequest(http.MethodGet, "/public-reader", nil)
		request.Header.Set("Authorization", token)
		response := httptest.NewRecorder()
		r.ServeHTTP(response, request)
		if !strings.Contains(response.Body.String(), `"authenticated":false`) {
			t.Fatalf("raw token expanded public identity: %s", response.Body.String())
		}
	}
}
