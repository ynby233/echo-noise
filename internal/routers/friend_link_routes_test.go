package routers

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRetiredFriendLinkRoutesAreUnreachable(t *testing.T) {
	t.Setenv("ACCESS_LOG", "false")
	t.Setenv("SESSION_SECRET", "retired-friend-link-route-test-secret-32")
	t.Chdir(t.TempDir())

	r := SetupRouter()
	for _, request := range []struct {
		name   string
		method string
		path   string
	}{
		{name: "public application", method: http.MethodPost, path: "/api/friend-links/apply"},
		{name: "administrator application list", method: http.MethodGet, path: "/api/friend-links/apply"},
		{name: "administrator clear applications", method: http.MethodDelete, path: "/api/friend-links/apply"},
		{name: "administrator delete application", method: http.MethodDelete, path: "/api/friend-links/apply/1"},
		{name: "administrator audit application", method: http.MethodPut, path: "/api/friend-links/1/audit"},
	} {
		t.Run(request.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			r.ServeHTTP(response, httptest.NewRequest(request.method, request.path, nil))
			if response.Code != http.StatusNotFound {
				t.Fatalf("%s %s status = %d, want 404: %s", request.method, request.path, response.Code, response.Body.String())
			}
		})
	}
}
