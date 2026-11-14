package controllers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/rcy1314/echo-noise/internal/authorization"
	backupservice "github.com/rcy1314/echo-noise/internal/backup"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
)

func managedControllerDatabase(t *testing.T) *gorm.DB {
	t.Helper()
	t.Setenv("NOISE_PLAIN_PASSWORD_STORE", filepath.Join(t.TempDir(), "passwords.db"))
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "test.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.AdminCapabilityGrant{}); err != nil {
		t.Fatal(err)
	}
	for _, user := range []models.User{{ID: 1, Username: "owner", IsAdmin: true}, {ID: 2, Username: "delegate", IsAdmin: true}, {ID: 3, Username: "ordinary"}} {
		if err := db.Create(&user).Error; err != nil {
			t.Fatal(err)
		}
	}
	previous := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = previous; sqlDB, _ := db.DB(); _ = sqlDB.Close() })
	return db
}

func TestManagedBackupGenerationDoesNotGrantArchiveReadOrRestore(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := managedControllerDatabase(t)
	for _, action := range []string{"view", "create", "plan", "history", "download", "restore"} {
		for _, kind := range []string{"data", "attachments", "server-config"} {
			if err := db.Create(&models.AdminCapabilityGrant{UserID: 2, Capability: string(managedBackupCapability(kind, action)), GrantedByUserID: 1}).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, kind := range []string{"data", "attachments"} {
		for _, action := range []string{"view", "create", "plan", "history"} {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Set("user_id", uint(2))
			if !managedBackupAllowed(c, kind, action) {
				t.Fatalf("delegate unexpectedly denied %s.%s", kind, action)
			}
		}
	}
	for _, kind := range []string{"data", "attachments", "server-config"} {
		for _, action := range []string{"download", "restore"} {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Set("user_id", uint(2))
			if managedBackupAllowed(c, kind, action) {
				t.Fatalf("delegate can obtain or replace complete protected archive: %s.%s", kind, action)
			}
			c.Set("user_id", uint(1))
			if !managedBackupAllowed(c, kind, action) {
				t.Fatalf("owner denied %s.%s", kind, action)
			}
		}
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("user_id", uint(2))
	if managedBackupAllowed(c, "server-config", "create") {
		t.Fatal("server config unexpectedly delegable")
	}
	if err := db.Where("user_id = ? AND capability = ?", 2, string(managedBackupCapability("data", "create"))).Delete(&models.AdminCapabilityGrant{}).Error; err != nil {
		t.Fatal(err)
	}
	if managedBackupAllowed(c, "data", "create") {
		t.Fatal("revoked create grant remained effective")
	}
	if !authorization.New(db).Authorize(1, authorization.CapabilityBackupDataView, nil).Allowed {
		t.Fatal("owner authorization lost")
	}
}

func TestManagedBackupRejectsUnconfirmedRestoreBeforeOpeningSource(t *testing.T) {
	gin.SetMode(gin.TestMode)
	managedControllerDatabase(t)
	r := gin.New()
	r.POST("/backups/:kind/history/:id/restore", func(c *gin.Context) { c.Set("user_id", uint(1)); HandleManagedBackupRestoreRecord(c) })
	req := httptest.NewRequest(http.MethodPost, "/backups/data/history/not-a-record/restore", bytes.NewBufferString(`{"confirm":false,"source":"local","attachmentMode":"original"}`))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	r.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var body map[string]interface{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["msg"] != "恢复前必须明确确认" {
		t.Fatalf("unexpected failure: %#v", body)
	}
}

func TestManagedBackupJSONRejectsDestinationOverridesAndTrailingObjects(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, body := range []string{`{"target":"cloud","uploadURL":"https://unexpected.invalid"}`, `{"target":"local"} {"target":"cloud"}`} {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(body))
		var input struct {
			Target string `json:"target"`
		}
		if err := managedBackupJSON(c, &input); err == nil {
			t.Fatalf("accepted unsafe payload %s", body)
		}
	}
}

func TestManagedRestoreReturnsEachWarningOnlyOnce(t *testing.T) {
	gin.SetMode(gin.TestMode)
	root := t.TempDir()
	t.Chdir(root)
	db := managedControllerDatabase(t)
	if err := db.AutoMigrate(&models.Message{}, &models.SiteConfig{}); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(root, "data.zip")
	layout := backupservice.Layout{DatabasePath: filepath.Join(root, "source.db"), PlainPasswordStorePath: filepath.Join(root, "missing-passwords.db"), ConfigDir: filepath.Join(root, "config")}
	if err := backupservice.CreateTypedArchive(context.Background(), archive, db, layout, backupservice.KindData, ""); err != nil {
		t.Fatal(err)
	}
	inspection, err := backupservice.InspectTypedArchive(archive)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("DB_PATH", filepath.Join(root, "target", "noise.db"))
	t.Setenv("ATTACHMENT_BLOB_ROOT", filepath.Join(root, "blobs"))
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/restore", nil)
	c.Set("user_id", uint(1))
	managedBackupStage(c, archive, "original", "data")
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("restore: %d %s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Data struct {
			Warnings []string `json:"warnings"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Data.Warnings) != len(inspection.Warnings) || len(response.Data.Warnings) == 0 {
		t.Fatalf("warnings were duplicated or omitted: %#v", response.Data.Warnings)
	}
	for i, warning := range response.Data.Warnings {
		if warning != inspection.Warnings[i] {
			t.Fatalf("warning %d: got %q, want %q", i, warning, inspection.Warnings[i])
		}
	}
}
