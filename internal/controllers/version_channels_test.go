package controllers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/updates"
)

func TestUpdateChannelsRequirePrimaryAdministrator(t *testing.T) {
	gin.SetMode(gin.TestMode)
	original := discoverUpdates
	discoverUpdates = func(*gin.Context) updates.Report {
		return updates.Report{Channels: []updates.Channel{{Name: "edge", Digest: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Revision: targetRevisionForController}}}
	}
	t.Cleanup(func() { discoverUpdates = original })

	for _, test := range []struct {
		userID uint
		status int
	}{
		{userID: models.PrimaryAdminUserID, status: http.StatusOK},
		{userID: 2, status: http.StatusForbidden},
	} {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Set("user_id", test.userID)
		GetUpdateChannels(ctx)
		if recorder.Code != test.status {
			t.Fatalf("user %d status=%d body=%s", test.userID, recorder.Code, recorder.Body.String())
		}
	}
}

func TestPublicVersionCheckDoesNotExposeRevisionOrDigest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	original := discoverUpdates
	discoverUpdates = func(*gin.Context) updates.Report {
		return updates.Report{
			Source: updates.Source{Revision: targetRevisionForController, Status: updates.StatusSourceReady},
			Channels: []updates.Channel{
				{Name: "stable", Version: "v1.2.3", BuiltAt: "2026-09-11T08:10:00Z", Status: updates.StatusUpdateAvailable, Installable: true, HasUpdate: true, Digest: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Revision: targetRevisionForController},
				{Name: "edge", Version: targetRevisionForController[:12], Status: updates.StatusCurrent, Installable: true, Digest: "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", Revision: targetRevisionForController},
			},
		}
	}
	t.Cleanup(func() { discoverUpdates = original })

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	CheckVersion(ctx)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	for _, secret := range []string{targetRevisionForController, targetRevisionForController[:12], "sha256:bbbb", "sha256:cccc"} {
		if strings.Contains(body, secret) {
			t.Fatalf("public response leaked %q: %s", secret, body)
		}
	}
	if !strings.Contains(body, `"currentTag":"v1.2.3"`) || !strings.Contains(body, `"hasUpdate":true`) {
		t.Fatalf("public response lost safe release status: %s", body)
	}
}

func TestLegacyUpdateHandlersAreExplicitlyUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, handler := range []gin.HandlerFunc{UpdateVersion, UpdateVersionStream} {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		handler(ctx)
		if recorder.Code != http.StatusNotImplemented || !strings.Contains(recorder.Body.String(), "仅支持检查更新") {
			t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
		}
	}
}

const targetRevisionForController = "2222222222222222222222222222222222222222"
