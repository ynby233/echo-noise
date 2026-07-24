package routers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSetupRouterDoesNotTrustForwardedIPByDefault(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("TRUSTED_PROXIES", "")
	r := SetupRouter()

	req := httptest.NewRequest(http.MethodGet, "/.env", nil)
	req.RemoteAddr = "198.51.100.40:4321"
	req.Header.Set("X-Forwarded-For", "127.0.0.1")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("spoofed forwarded IP must not bypass security middleware, got %d: %s", w.Code, w.Body.String())
	}
}

func TestConfigureTrustedProxiesUsesExplicitProxyOnly(t *testing.T) {
	t.Setenv("TRUSTED_PROXIES", "127.0.0.1/32")
	r := gin.New()
	configureTrustedProxies(r)
	r.GET("/ip", func(c *gin.Context) { c.String(http.StatusOK, c.ClientIP()) })

	req := httptest.NewRequest(http.MethodGet, "/ip", nil)
	req.RemoteAddr = "127.0.0.1:4321"
	req.Header.Set("X-Forwarded-For", "198.51.100.41")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if got := w.Body.String(); got != "198.51.100.41" {
		t.Fatalf("explicit trusted proxy client IP = %q", got)
	}
}

func TestConfigureTrustedProxiesFallsBackToDirectIPWhenInvalid(t *testing.T) {
	t.Setenv("TRUSTED_PROXIES", "not-a-proxy")
	r := gin.New()
	configureTrustedProxies(r)
	r.GET("/ip", func(c *gin.Context) { c.String(http.StatusOK, c.ClientIP()) })

	req := httptest.NewRequest(http.MethodGet, "/ip", nil)
	req.RemoteAddr = "198.51.100.42:4321"
	req.Header.Set("X-Forwarded-For", "127.0.0.1")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if got := w.Body.String(); got != "198.51.100.42" {
		t.Fatalf("invalid proxy config must fail closed, client IP = %q", got)
	}
}
