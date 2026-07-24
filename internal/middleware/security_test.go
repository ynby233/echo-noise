package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSuspiciousPathKeepsAPIBoundaryExact(t *testing.T) {
	tests := []struct {
		path       string
		suspicious bool
	}{
		{path: "/api", suspicious: false},
		{path: "/api/login", suspicious: false},
		{path: "/api.php", suspicious: true},
		{path: "/api-backup/config.jsp", suspicious: true},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := isSuspiciousPath(tt.path); got != tt.suspicious {
				t.Fatalf("isSuspiciousPath(%q) = %v, want %v", tt.path, got, tt.suspicious)
			}
		})
	}
}

func TestPrivatePeerStillGetsSuspiciousPathProtection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(SecurityMiddleware())
	r.GET("/.env", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/.env", nil)
	req.RemoteAddr = "172.18.0.1:4321"
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("private proxy peer must not bypass suspicious path protection, got %d", w.Code)
	}
}
