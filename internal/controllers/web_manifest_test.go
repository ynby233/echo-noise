package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupWebManifestTestDB(t *testing.T, enabled bool) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open manifest test db: %v", err)
	}
	if err := db.AutoMigrate(&models.SiteConfig{}, &models.Setting{}, &models.User{}); err != nil {
		t.Fatalf("migrate manifest test db: %v", err)
	}
	config := models.SiteConfig{SiteTitle: "测试站点", PwaTitle: "测试应用", PwaDescription: "离线与推送测试", PwaEnabled: true}
	if err := db.Create(&config).Error; err != nil {
		t.Fatalf("create site config: %v", err)
	}
	if err := db.Model(&config).Update("pwa_enabled", enabled).Error; err != nil {
		t.Fatalf("set pwa enabled: %v", err)
	}
	database.DB = db
	t.Cleanup(func() { database.DB = nil })
}

func TestGetWebManifestReturnsInstallableAppContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupWebManifestTestDB(t, true)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/manifest.webmanifest", nil)

	GetWebManifest(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("manifest status = %d: %s", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("Content-Type"); got != "application/manifest+json; charset=utf-8" {
		t.Fatalf("manifest content type = %q", got)
	}
	var manifest map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	for key, want := range map[string]string{
		"id": "/", "scope": "/", "start_url": "/", "display": "standalone", "name": "测试应用",
	} {
		if got := manifest[key]; got != want {
			t.Fatalf("manifest %s = %#v, want %q", key, got, want)
		}
	}
	icons, ok := manifest["icons"].([]any)
	if !ok || len(icons) < 2 {
		t.Fatalf("manifest icons = %#v", manifest["icons"])
	}
	wantSizes := map[string]bool{"192x192": false, "512x512": false}
	for _, raw := range icons {
		icon, _ := raw.(map[string]any)
		if size, _ := icon["sizes"].(string); size != "" {
			if _, exists := wantSizes[size]; exists {
				wantSizes[size] = true
			}
		}
	}
	for size, found := range wantSizes {
		if !found {
			t.Fatalf("manifest is missing %s icon: %#v", size, icons)
		}
	}
}

func TestGetWebManifestIsUnavailableWhenPwaIsDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupWebManifestTestDB(t, false)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/manifest.webmanifest", nil)

	GetWebManifest(ctx)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("disabled manifest status = %d, want 404: %s", recorder.Code, recorder.Body.String())
	}
}
