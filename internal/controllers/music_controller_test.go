package controllers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/internal/music"
)

func TestMusicFailureHTTPStatusAndPublicConcealment(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name   string
		err    error
		public bool
		want   int
	}{
		{"invalid", music.ErrInvalid, false, 400},
		{"forbidden", music.ErrForbidden, false, 403},
		{"conflict", music.ErrConflict, false, 409},
		{"unprocessable", music.ErrUnprocessable, false, 422},
		{"unavailable", music.ErrUnavailable, false, 503},
		{"busy", music.ErrBusy, true, 503},
		{"public concealment", music.ErrForbidden, true, 404},
	} {
		for _, method := range []string{http.MethodGet, http.MethodHead} {
			t.Run(tc.name+method, func(t *testing.T) {
				response := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(response)
				c.Request = httptest.NewRequest(method, "/api/music/cover/id", nil)
				musicFailure(c, tc.err, tc.public)
				c.Writer.WriteHeaderNow()
				if response.Code != tc.want {
					t.Fatalf("status=%d want=%d", response.Code, tc.want)
				}
				if method == http.MethodHead && response.Body.Len() != 0 {
					t.Fatal("HEAD error response contains body")
				}
				if tc.want == 503 && response.Header().Get("Retry-After") != "2" {
					t.Fatal("busy media must provide Retry-After")
				}
			})
		}
	}
}
