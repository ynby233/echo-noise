package controllers

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/rcy1314/echo-noise/internal/middleware"
	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupSecurityControllerTest(t *testing.T) (*gorm.DB, *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open security test db: %v", err)
	}
	if err := db.AutoMigrate(&models.SecurityIPBan{}, &models.SecurityAttackLog{}, &models.SecurityConfig{}); err != nil {
		t.Fatalf("migrate security test db: %v", err)
	}
	models.SetDB(db)
	t.Cleanup(func() { models.SetDB(nil) })

	r := gin.New()
	r.Use(middleware.SecurityMiddleware())
	r.GET("/ok", func(c *gin.Context) { c.Status(http.StatusOK) })
	r.POST("/security/bans", AddIPBan)
	r.DELETE("/security/bans", RemoveIPBan)
	r.PUT("/security/config", UpdateSecurityConfig)
	return db, r
}

func performSecurityRequest(r http.Handler, method, path, remoteAddr, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.RemoteAddr = remoteAddr
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func securityResultCode(t *testing.T, w *httptest.ResponseRecorder) int {
	t.Helper()
	var response struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode security response %q: %v", w.Body.String(), err)
	}
	return response.Code
}

func TestManualBanInvalidatesNegativeCacheImmediately(t *testing.T) {
	_, r := setupSecurityControllerTest(t)
	target := "198.51.100.21"
	if got := performSecurityRequest(r, http.MethodGet, "/ok", target+":1000", "").Code; got != http.StatusOK {
		t.Fatalf("initial request got %d", got)
	}

	ban := performSecurityRequest(r, http.MethodPost, "/security/bans", "127.0.0.1:1000", `{"ip":"`+target+`","minutes":60}`)
	if securityResultCode(t, ban) != 1 {
		t.Fatalf("manual ban failed: %s", ban.Body.String())
	}
	if got := performSecurityRequest(r, http.MethodGet, "/ok", target+":1000", "").Code; got != http.StatusForbidden {
		t.Fatalf("manual ban must apply immediately, got %d", got)
	}
}

func TestManualUnbanInvalidatesPositiveCacheImmediately(t *testing.T) {
	db, r := setupSecurityControllerTest(t)
	target := "198.51.100.22"
	if err := db.Create(&models.SecurityIPBan{IP: target, Reason: "test"}).Error; err != nil {
		t.Fatalf("seed ban: %v", err)
	}
	if got := performSecurityRequest(r, http.MethodGet, "/ok", target+":1000", "").Code; got != http.StatusForbidden {
		t.Fatalf("seeded ban got %d", got)
	}

	unban := performSecurityRequest(r, http.MethodDelete, "/security/bans?ip="+target, "127.0.0.1:1000", "")
	if securityResultCode(t, unban) != 1 {
		t.Fatalf("manual unban failed: %s", unban.Body.String())
	}
	if got := performSecurityRequest(r, http.MethodGet, "/ok", target+":1000", "").Code; got != http.StatusOK {
		t.Fatalf("manual unban must apply immediately, got %d", got)
	}
}

func TestAddIPBanRejectsInvalidAndExemptAddresses(t *testing.T) {
	_, r := setupSecurityControllerTest(t)
	for _, ip := range []string{"not-an-ip", "192.168.1.10"} {
		t.Run(ip, func(t *testing.T) {
			w := performSecurityRequest(r, http.MethodPost, "/security/bans", "127.0.0.1:1000", `{"ip":"`+ip+`","minutes":60}`)
			if securityResultCode(t, w) != 0 {
				t.Fatalf("address %q must be rejected: %s", ip, w.Body.String())
			}
		})
	}
}

func TestRemoveIPBanCanCleanUpLegacyPrivateAddress(t *testing.T) {
	db, r := setupSecurityControllerTest(t)
	target := "192.168.1.10"
	if err := db.Create(&models.SecurityIPBan{IP: target, Reason: "legacy"}).Error; err != nil {
		t.Fatalf("seed legacy ban: %v", err)
	}

	w := performSecurityRequest(r, http.MethodDelete, "/security/bans?ip="+target, "127.0.0.1:1000", "")
	if securityResultCode(t, w) != 1 {
		t.Fatalf("legacy private ban must remain removable: %s", w.Body.String())
	}
	var count int64
	if err := db.Model(&models.SecurityIPBan{}).Where("ip = ?", target).Count(&count).Error; err != nil {
		t.Fatalf("count legacy ban: %v", err)
	}
	if count != 0 {
		t.Fatalf("legacy private ban was not removed")
	}
}

func TestRemoveIPBanCanCleanUpLegacyInvalidAddress(t *testing.T) {
	db, r := setupSecurityControllerTest(t)
	target := "legacy-invalid-ip"
	if err := db.Create(&models.SecurityIPBan{IP: target, Reason: "legacy"}).Error; err != nil {
		t.Fatalf("seed legacy ban: %v", err)
	}

	w := performSecurityRequest(r, http.MethodDelete, "/security/bans?ip="+target, "127.0.0.1:1000", "")
	if securityResultCode(t, w) != 1 {
		t.Fatalf("legacy invalid ban must remain removable: %s", w.Body.String())
	}
	var count int64
	if err := db.Model(&models.SecurityIPBan{}).Where("ip = ?", target).Count(&count).Error; err != nil {
		t.Fatalf("count legacy ban: %v", err)
	}
	if count != 0 {
		t.Fatalf("legacy invalid ban was not removed")
	}
}

func TestAddIPBanReportsUpdateFailure(t *testing.T) {
	db, r := setupSecurityControllerTest(t)
	target := "198.51.100.23"
	if err := db.Create(&models.SecurityIPBan{IP: target, Reason: "old"}).Error; err != nil {
		t.Fatalf("seed ban: %v", err)
	}
	if err := db.Callback().Update().Before("gorm:update").Register("test:fail-security-ban-update", func(tx *gorm.DB) {
		tx.AddError(errors.New("forced security update failure"))
	}); err != nil {
		t.Fatalf("register failing callback: %v", err)
	}

	w := performSecurityRequest(r, http.MethodPost, "/security/bans", "127.0.0.1:1000", `{"ip":"`+target+`","minutes":60}`)
	if securityResultCode(t, w) != 0 {
		t.Fatalf("write failure must be reported: %s", w.Body.String())
	}
}
