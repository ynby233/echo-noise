package routers

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLegacyPasswordForgotRouteIsUnavailable(t *testing.T) {
	t.Setenv("ACCESS_LOG", "false")
	t.Setenv("SESSION_SECRET", "password-forgot-route-test-secret-32")
	t.Chdir(t.TempDir())

	r := SetupRouter()
	response := httptest.NewRecorder()
	r.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/password/forgot", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("POST /api/password/forgot status = %d, want 404: %s", response.Code, response.Body.String())
	}
}
