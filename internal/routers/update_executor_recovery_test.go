package routers

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/updates"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestRotatedExecutorCanRetryOnlyItsOwnFinalReportHTTP(t *testing.T) {
	for _, final := range []string{updates.TaskSucceeded, updates.TaskFailed} {
		t.Run(final, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			t.Setenv("ACCESS_LOG", "false")
			t.Setenv("SESSION_SECRET", "update-recovery-test-secret-32-characters")
			t.Chdir(t.TempDir())
			db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
			if err != nil {
				t.Fatal(err)
			}
			sqlDB, err := db.DB()
			if err != nil {
				t.Fatal(err)
			}
			sqlDB.SetMaxOpenConns(1)
			if err := models.MigrateDB(db); err != nil {
				t.Fatal(err)
			}
			database.DB = db
			models.SetDB(db)
			t.Cleanup(func() { database.DB = nil; models.SetDB(nil); _ = sqlDB.Close() })
			service := updates.NewTaskService(db)
			old, token, err := service.CreateCredential(1, "old host")
			if err != nil {
				t.Fatal(err)
			}
			router := SetupRouter(nil)
			call := func(method, path, raw, body string) *httptest.ResponseRecorder {
				t.Helper()
				request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
				request.Header.Set("Authorization", "Bearer "+raw)
				request.Header.Set("Content-Type", "application/json")
				response := httptest.NewRecorder()
				router.ServeHTTP(response, request)
				return response
			}
			if response := call(http.MethodGet, "/api/updates/executor/runtime", token, ""); response.Code != http.StatusOK {
				t.Fatalf("initial authentication: %d %s", response.Code, response.Body.String())
			}
			target := updates.Target{Channel: "edge", Image: "ghcr.io/ynby233/echo-noise", Digest: "sha256:" + strings.Repeat("a", 64), Revision: strings.Repeat("1", 40)}
			instance, _ := service.InstanceID()
			if err := service.RecordDeploymentCheck(old.ID, updates.DeploymentCheck{InstanceID: instance, Version: updates.ExecutorVersion, Platform: "linux/amd64", Revision: target.Revision, OK: true}); err != nil {
				t.Fatal(err)
			}
			task, _, err := service.Create(1, target)
			if err != nil {
				t.Fatal(err)
			}
			if response := call(http.MethodPost, "/api/updates/executor/claim", token, ""); response.Code != http.StatusOK {
				t.Fatalf("claim: %d %s", response.Code, response.Body.String())
			}
			path := "/api/updates/executor/tasks/" + task.PublicID + "/events"
			for _, body := range []string{`{}`, `{"status":""}`, `{"status":" \t"}`, `{"status":"unknown"}`} {
				if response := call(http.MethodPost, path, token, body); response.Code != http.StatusBadRequest && response.Code != http.StatusConflict {
					t.Fatalf("invalid event %s: %d %s", body, response.Code, response.Body.String())
				}
			}
			if _, _, err := service.CreateCredential(1, "new host"); err != nil {
				t.Fatal(err)
			}
			statuses := []string{updates.TaskFailed}
			if final == updates.TaskSucceeded {
				statuses = []string{updates.TaskDownloading, updates.TaskStopping, updates.TaskBackingUp, updates.TaskReplacing, updates.TaskVerifying, updates.TaskSucceeded}
			}
			for _, status := range statuses {
				if response := call(http.MethodPost, path, token, `{"status":"`+status+`"}`); response.Code != http.StatusOK {
					t.Fatalf("report %s: %d %s", status, response.Code, response.Body.String())
				}
			}
			// The terminal write committed, but the executor lost its response.
			_, newToken, err := service.CreateCredential(1, "current host")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := service.Authenticate(newToken); err != nil {
				t.Fatal(err)
			}
			next, _ := service.CurrentCredential()
			if err := service.RecordDeploymentCheck(next.ID, updates.DeploymentCheck{InstanceID: instance, Version: updates.ExecutorVersion, Platform: "linux/amd64", Revision: target.Revision, OK: true}); err != nil {
				t.Fatal(err)
			}
			otherTask, _, err := service.Create(1, target)
			if err != nil {
				t.Fatal(err)
			}
			otherPath := "/api/updates/executor/tasks/" + otherTask.PublicID + "/events"
			if response := call(http.MethodPost, path, token, `{"status":"`+final+`"}`); response.Code != http.StatusOK {
				t.Errorf("final retry: %d %s", response.Code, response.Body.String())
			}
			for _, request := range []struct{ method, path string }{
				{http.MethodPost, "/api/updates/executor/claim"},
				{http.MethodGet, "/api/updates/executor/runtime"},
				{http.MethodGet, "/api/updates/tasks/" + task.PublicID},
				{http.MethodPost, "/api/updates/executor/tasks/not-owned/events"},
				{http.MethodPost, otherPath},
			} {
				if response := call(request.method, request.path, token, `{"status":"`+final+`"}`); response.Code != http.StatusUnauthorized {
					t.Errorf("completed old credential reached %s: %d %s", request.path, response.Code, response.Body.String())
				}
			}
			if response := call(http.MethodPost, path, token, `{"status":"downloading"}`); response.Code != http.StatusConflict {
				t.Errorf("final state could be changed: %d %s", response.Code, response.Body.String())
			}
			if current, err := service.Get(otherTask.PublicID); err != nil || current.Status != updates.TaskPending {
				t.Fatalf("old credential changed the new task: task=%#v err=%v", current, err)
			}
			past := time.Now().UTC().Add(-time.Minute)
			if err := db.Model(&old).Update("expires_at", past).Error; err != nil {
				t.Fatal(err)
			}
			if response := call(http.MethodPost, path, token, `{"status":"`+final+`"}`); response.Code != http.StatusUnauthorized {
				t.Errorf("expired credential retried final report: %d", response.Code)
			}
			if err := db.Model(&old).Update("expires_at", nil).Error; err != nil {
				t.Fatal(err)
			}
			if err := service.RevokeCredential(1); err != nil {
				t.Fatal(err)
			}
			if response := call(http.MethodPost, path, token, `{"status":"`+final+`"}`); response.Code != http.StatusUnauthorized {
				t.Errorf("revoked credential retried final report: %d", response.Code)
			}
		})
	}
}
