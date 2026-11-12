package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/rcy1314/echo-noise/internal/authorization"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/repository"
	"github.com/rcy1314/echo-noise/internal/services"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupStorageSettingsAuthorizationTest(t *testing.T) (*gorm.DB, models.User, models.User) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.Setting{}, &models.SiteConfig{}, &models.AdminCapabilityGrant{}, &models.AdminAuditLog{}, &models.AdminAuditConfig{}); err != nil {
		t.Fatal(err)
	}
	previousDB := database.DB
	database.DB = db
	repository.ClearUserCache()
	t.Cleanup(func() {
		repository.ClearUserCache()
		database.DB = previousDB
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	primary := models.User{ID: models.PrimaryAdminUserID, Username: "storage-owner", IsAdmin: true}
	delegated := models.User{Username: "storage-delegated", IsAdmin: true}
	for _, user := range []*models.User{&primary, &delegated} {
		if err := db.Create(user).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Create(&models.Setting{AllowRegistration: false}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.SiteConfig{
		SiteTitle: "original", StorageProvider: "s3", StorageBucket: "backup-bucket",
		StorageEndpoint: "https://backup.example.test", StorageAccessKey: "backup-access", StorageSecretKey: "backup-secret",
		AttachmentStorageProvider: "s3", AttachmentStorageBucket: "attachment-bucket",
		AttachmentStorageAccessKey: "attachment-access", AttachmentStorageSecretKey: "attachment-secret",
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&models.SiteConfig{}).Where("id > 0").Updates(map[string]interface{}{"feed_enabled": false, "enable_compression": false}).Error; err != nil {
		t.Fatal(err)
	}
	return db, primary, delegated
}

// These paths exercise the common handler with the identity supplied by each
// authentication mode. Router authentication itself is covered by router tests.
func storageSettingsRequest(t *testing.T, userID uint, token bool, payload string) *httptest.ResponseRecorder {
	t.Helper()
	path, via := "/api/settings", "session"
	if token {
		path, via = "/api/token/settings", "token"
	}
	r := gin.New()
	r.PUT(path, func(c *gin.Context) {
		c.Set("user_id", userID)
		c.Set("auth_via", via)
	}, UpdateSetting)
	req := httptest.NewRequest(http.MethodPut, path, strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	r.ServeHTTP(response, req)
	return response
}

func readStorageSettingsState(t *testing.T, db *gorm.DB) (models.SiteConfig, models.Setting) {
	t.Helper()
	var config models.SiteConfig
	var setting models.Setting
	if err := db.First(&config).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&setting).Error; err != nil {
		t.Fatal(err)
	}
	return config, setting
}

func TestSettingsDelegatedCloudAndConnectionWritesAreForbidden(t *testing.T) {
	db, primary, delegated := setupStorageSettingsAuthorizationTest(t)
	grants := []authorization.Capability{
		authorization.CapabilitySiteSettingsView, authorization.CapabilitySiteSettingsManage,
		authorization.CapabilityStorageView, authorization.CapabilityStorageManage,
	}
	if err := authorization.New(db).ReplaceGrants(primary.ID, delegated.ID, grants); err != nil {
		t.Fatal(err)
	}
	payloads := []string{
		`{"storageEnabled":true}`, `{"storageEnabled":false}`, `{"storageConfig":{}}`,
		`{"attachmentStorageEnabled":false}`, `{"attachmentStorageEnabled":true}`,
		`{"storageConfig":{"autoSyncEnabled":false,"syncRole":"secondary"}}`,
		`{"storageConfig":{"publicBaseURL":"https://other.example.test"}}`,
		`{"attachmentStorageConfig":{"enableCompression":true},"storageConfig":{"bucket":"other"},"allowRegistration":true}`,
	}
	for _, field := range []string{"provider", "endpoint", "region", "bucket", "accessKey", "secretKey", "publicBaseURL", "usePathStyle", "clearAccessKey", "clearSecretKey", "accessKeyConfigured", "ffmpegInstalled", "unknownField"} {
		value := `""`
		if field == "usePathStyle" || strings.HasPrefix(field, "clear") || field == "ffmpegInstalled" || field == "accessKeyConfigured" {
			value = "false"
		}
		payloads = append(payloads, `{"attachmentStorageConfig":{"enableCompression":true,"`+field+`":`+value+`}}`)
		payloads = append(payloads, `{"storageConfig":{"`+field+`":`+value+`}}`)
	}
	beforeConfig, beforeSetting := readStorageSettingsState(t, db)
	for _, token := range []bool{false, true} {
		for _, payload := range payloads {
			response := storageSettingsRequest(t, delegated.ID, token, payload)
			if response.Code != http.StatusForbidden {
				t.Fatalf("token=%v payload=%s status=%d body=%s", token, payload, response.Code, response.Body.String())
			}
			afterConfig, afterSetting := readStorageSettingsState(t, db)
			if !reflect.DeepEqual(beforeConfig, afterConfig) || !reflect.DeepEqual(beforeSetting, afterSetting) {
				t.Fatalf("denied payload changed persisted settings: %s", payload)
			}
		}
	}
}

func TestSettingsCompressionNeedsStorageManageAndMixedPayloadNeedsSiteManage(t *testing.T) {
	db, primary, delegated := setupStorageSettingsAuthorizationTest(t)
	for _, test := range []struct {
		name   string
		grants []authorization.Capability
	}{
		{name: "no grants"},
		{name: "storage view only", grants: []authorization.Capability{authorization.CapabilityStorageView}},
		{name: "broad site grant", grants: []authorization.Capability{authorization.CapabilitySiteSettingsView, authorization.CapabilitySiteSettingsManage}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := authorization.New(db).ReplaceGrants(primary.ID, delegated.ID, test.grants); err != nil {
				t.Fatal(err)
			}
			for _, token := range []bool{false, true} {
				response := storageSettingsRequest(t, delegated.ID, token, `{"attachmentStorageConfig":{"enableCompression":true}}`)
				if response.Code != http.StatusForbidden {
					t.Fatalf("compression without storage.manage: %d %s", response.Code, response.Body.String())
				}
			}
		})
	}
	if err := authorization.New(db).ReplaceGrants(primary.ID, delegated.ID, []authorization.Capability{authorization.CapabilityStorageView, authorization.CapabilityStorageManage}); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&models.Setting{}).Where("id > 0").Updates(map[string]interface{}{
		"allow_registration": false, "auto_approve_registration": true,
	}).Error; err != nil {
		t.Fatal(err)
	}
	// Keep complete but disabled connections and noncanonical legacy values: a
	// compression patch must not enable services or normalize unrelated fields.
	if err := db.Model(&models.SiteConfig{}).Where("id > 0").Updates(map[string]interface{}{
		"storage_provider": "r2", "storage_use_path_style": false,
		"attachment_storage_provider": "r2", "attachment_storage_use_path_style": false,
		"smtp_enabled": false, "smtp_host": "smtp.example.test", "smtp_port": 465,
		"smtp_user": "smtp-user", "smtp_pass": "smtp-secret", "smtp_encryption": "ssl",
		"rss_enabled": true, "rss_member_ids": "[]", "login_expire_days": 7, "login_expire_hours": 0,
		"feed_limit": -2, "feed_refresh_seconds": 0, "life_expectancy_years": -1,
	}).Error; err != nil {
		t.Fatal(err)
	}
	beforeConfig, beforeSetting := readStorageSettingsState(t, db)
	for _, payload := range []string{
		`{}`, `{"allowRegistration":true}`, `{"frontendSettings":{"siteTitle":"changed"}}`,
		`{"attachmentStorageConfig":{"enableCompression":true},"allowRegistration":true}`,
		`{"attachmentStorageConfig":{"enableCompression":true},"frontendSettings":{"siteTitle":"changed"}}`,
	} {
		for _, token := range []bool{false, true} {
			response := storageSettingsRequest(t, delegated.ID, token, payload)
			if response.Code != http.StatusForbidden {
				t.Fatalf("mixed/site payload without site_settings.manage: %d %s", response.Code, response.Body.String())
			}
		}
	}
	afterConfig, afterSetting := readStorageSettingsState(t, db)
	if !reflect.DeepEqual(beforeConfig, afterConfig) || !reflect.DeepEqual(beforeSetting, afterSetting) {
		t.Fatal("forbidden mixed payload changed persisted settings")
	}
	for _, token := range []bool{false, true} {
		for _, enabled := range []bool{true, false} {
			payload, _ := json.Marshal(map[string]interface{}{"attachmentStorageConfig": map[string]interface{}{"enableCompression": enabled}})
			response := storageSettingsRequest(t, delegated.ID, token, string(payload))
			var body struct {
				Code int `json:"code"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if response.Code != http.StatusOK || body.Code != 1 {
				t.Fatalf("compression-only denied: %d %s", response.Code, response.Body.String())
			}
			config, setting := readStorageSettingsState(t, db)
			expectedConfig := beforeConfig
			expectedConfig.EnableCompression = enabled
			expectedConfig.UpdatedAt = config.UpdatedAt
			if !reflect.DeepEqual(config, expectedConfig) || !reflect.DeepEqual(setting, beforeSetting) {
				t.Fatalf("compression-only save changed unrelated settings or failed to persist: config=%+v expected=%+v setting=%+v expectedSetting=%+v", config, expectedConfig, setting, beforeSetting)
			}
		}
	}
	response := storageSettingsRequest(t, delegated.ID, false, `{"attachmentStorageConfig":{"enableCompression":"true"}}`)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid compression type: %d %s", response.Code, response.Body.String())
	}
	if err := db.Where("user_id = ? AND capability = ?", delegated.ID, authorization.CapabilityStorageView).Delete(&models.AdminCapabilityGrant{}).Error; err != nil {
		t.Fatal(err)
	}
	response = storageSettingsRequest(t, delegated.ID, false, `{"attachmentStorageConfig":{"enableCompression":true}}`)
	if response.Code != http.StatusForbidden {
		t.Fatalf("storage.manage without storage.view still works: %d %s", response.Code, response.Body.String())
	}
	if err := authorization.New(db).ReplaceGrants(primary.ID, delegated.ID, nil); err != nil {
		t.Fatal(err)
	}
	response = storageSettingsRequest(t, delegated.ID, true, `{"attachmentStorageConfig":{"enableCompression":true}}`)
	if response.Code != http.StatusForbidden {
		t.Fatalf("revoked grant still works: %d %s", response.Code, response.Body.String())
	}
}

func TestSettingsOwnerMaySaveCloudAndAttachmentConnection(t *testing.T) {
	db, primary, _ := setupStorageSettingsAuthorizationTest(t)
	for _, token := range []bool{false, true} {
		response := storageSettingsRequest(t, primary.ID, token, `{"storageEnabled":false,"storageConfig":{"bucket":"owner-backup","accessKey":"new-backup-access","clearSecretKey":true},"attachmentStorageConfig":{"bucket":"owner-attachments","accessKey":"new-attachment-access","clearSecretKey":true,"enableCompression":true}}`)
		var body struct {
			Code int `json:"code"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if response.Code != http.StatusOK || body.Code != 1 {
			t.Fatalf("owner save denied: %d %s", response.Code, response.Body.String())
		}
		config, _ := readStorageSettingsState(t, db)
		if config.StorageEnabled || config.StorageBucket != "owner-backup" || config.StorageAccessKey != "new-backup-access" || config.StorageSecretKey != "" || config.AttachmentStorageBucket != "owner-attachments" || config.AttachmentStorageAccessKey != "new-attachment-access" || config.AttachmentStorageSecretKey != "" || !config.EnableCompression {
			t.Fatal("owner connection update did not persist")
		}
	}
}

func TestSettingsReadFiltersConnectionsAndRequiresStorageViewForStatus(t *testing.T) {
	db, primary, delegated := setupStorageSettingsAuthorizationTest(t)
	ordinary := models.User{Username: "storage-member"}
	if err := db.Create(&ordinary).Error; err != nil {
		t.Fatal(err)
	}
	for _, grants := range [][]authorization.Capability{
		nil,
		{authorization.CapabilitySiteSettingsView, authorization.CapabilitySiteSettingsManage},
		{authorization.CapabilityStorageView},
	} {
		if err := authorization.New(db).ReplaceGrants(primary.ID, delegated.ID, grants); err != nil {
			t.Fatal(err)
		}
		for _, viewer := range []uint{0, ordinary.ID, delegated.ID, primary.ID} {
			config, err := services.GetFrontendConfig(viewer)
			if err != nil {
				t.Fatal(err)
			}
			storage := config["storageConfig"].(map[string]interface{})
			attachments := config["attachmentStorageConfig"].(map[string]interface{})
			for _, connection := range []map[string]interface{}{storage, attachments} {
				if connection["accessKey"] != "" || connection["secretKey"] != "" {
					t.Fatal("read config returned credentials")
				}
			}
			if viewer == primary.ID {
				if storage["bucket"] != "backup-bucket" || attachments["bucket"] != "attachment-bucket" || storage["accessKeyConfigured"] != true || attachments["secretKeyConfigured"] != true {
					t.Fatal("owner lost connection metadata")
				}
				continue
			}
			if _, exists := config["storageEnabled"]; exists {
				t.Fatal("nonowner received cloud backup mode")
			}
			for _, connection := range []map[string]interface{}{storage, attachments} {
				for _, field := range []string{"provider", "endpoint", "region", "bucket", "usePathStyle", "publicBaseURL", "syncRole", "autoSyncEnabled", "syncConfirmed", "lastSyncTime"} {
					if _, exists := connection[field]; exists {
						t.Fatalf("nonowner received connection field %s", field)
					}
				}
				if connection["accessKeyConfigured"] != false || connection["secretKeyConfigured"] != false {
					t.Fatal("nonowner received credential state")
				}
			}
			wantStatus := viewer == delegated.ID && len(grants) == 1 && grants[0] == authorization.CapabilityStorageView
			for _, field := range []string{"enableCompression", "ffmpegInstalled"} {
				if _, exists := attachments[field]; exists != wantStatus {
					t.Fatalf("viewer %d field %s present=%v, want=%v", viewer, field, exists, wantStatus)
				}
			}
			if _, exists := config["attachmentStorageEnabled"]; exists != wantStatus {
				t.Fatalf("attachment mode present=%v, want=%v", exists, wantStatus)
			}
		}
	}
}

func TestSettingsSaveDoesNotDownloadLegacyCloudDatabase(t *testing.T) {
	db, primary, delegated := setupStorageSettingsAuthorizationTest(t)
	var requests atomic.Int32
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer remote.Close()
	t.Setenv("DB_TYPE", "sqlite")
	if err := db.Model(&models.SiteConfig{}).Where("id > 0").Updates(map[string]interface{}{
		"storage_enabled": true, "storage_public_base_url": remote.URL,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := authorization.New(db).ReplaceGrants(primary.ID, delegated.ID, []authorization.Capability{
		authorization.CapabilitySiteSettingsView, authorization.CapabilitySiteSettingsManage,
		authorization.CapabilityStorageView, authorization.CapabilityStorageManage,
	}); err != nil {
		t.Fatal(err)
	}
	for _, payload := range []string{
		`{"frontendSettings":{"siteTitle":"allowed site change"}}`,
		`{"attachmentStorageConfig":{"enableCompression":true}}`,
		`{"attachmentStorageConfig":{"enableCompression":false},"frontendSettings":{"siteTitle":"allowed mixed change"}}`,
	} {
		response := storageSettingsRequest(t, delegated.ID, false, payload)
		var body struct {
			Code int `json:"code"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if response.Code != http.StatusOK || body.Code != 1 {
			t.Fatalf("authorized settings save failed: %d %s", response.Code, response.Body.String())
		}
	}
	if requests.Load() != 0 {
		t.Fatalf("settings save performed %d implicit remote database requests", requests.Load())
	}
	config, _ := readStorageSettingsState(t, db)
	if config.SiteTitle != "allowed mixed change" || config.EnableCompression {
		t.Fatal("authorized mixed settings did not persist")
	}
}
