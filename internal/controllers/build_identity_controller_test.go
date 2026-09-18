package controllers

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/internal/buildinfo"
	"github.com/rcy1314/echo-noise/internal/models"
)

func TestGetBuildIdentityRequiresPrimaryAdministrator(t *testing.T) {
	gin.SetMode(gin.TestMode)
	original, originalRevision, originalBuiltAt, originalVersion := buildinfo.Identity, buildinfo.Revision, buildinfo.BuiltAt, buildinfo.Version
	buildinfo.Identity = "8a5ed7759b123200d8200bbb9ab2e2386977ad38"
	buildinfo.Revision = "8a5ed7759b123200d8200bbb9ab2e2386977ad38"
	buildinfo.BuiltAt = "2026-09-11T08:30:00Z"
	buildinfo.Version = "v4.0.6"
	t.Cleanup(func() {
		buildinfo.Identity, buildinfo.Revision, buildinfo.BuiltAt, buildinfo.Version = original, originalRevision, originalBuiltAt, originalVersion
	})

	for _, test := range []struct {
		name   string
		userID uint
		status int
	}{
		{name: "primary", userID: models.PrimaryAdminUserID, status: http.StatusOK},
		{name: "delegated", userID: 2, status: http.StatusForbidden},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := gin.New()
			r.GET("/api/version/build", func(c *gin.Context) {
				c.Set("user_id", test.userID)
				GetBuildIdentity(c)
			})
			recorder := httptest.NewRecorder()
			r.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/version/build", nil))
			if recorder.Code != test.status {
				t.Fatalf("status=%d body=%s, want %d", recorder.Code, recorder.Body.String(), test.status)
			}
			if test.status == http.StatusOK && !bytes.Contains(recorder.Body.Bytes(), []byte(`"build_identity":"8a5ed7759b12"`)) {
				t.Fatalf("body=%s", recorder.Body.String())
			}
			if test.status == http.StatusOK && !bytes.Contains(recorder.Body.Bytes(), []byte(`"revision":"8a5ed7759b123200d8200bbb9ab2e2386977ad38"`)) {
				t.Fatalf("body=%s", recorder.Body.String())
			}
		})
	}
}
