package controllers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/rcy1314/echo-noise/internal/authorization"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestAdminAuditConfigOnlyPrimaryMayDisableRecording(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.AdminAuditLog{}, &models.AdminAuditConfig{}); err != nil {
		t.Fatal(err)
	}
	database.DB = db
	t.Cleanup(func() { database.DB = nil })
	primary := models.User{ID: models.PrimaryAdminUserID, Username: "primary", IsAdmin: true}
	delegated := models.User{Username: "delegated", IsAdmin: true}
	if err := db.Create(&primary).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&delegated).Error; err != nil {
		t.Fatal(err)
	}

	r := gin.New()
	r.GET("/audit-config/primary", func(c *gin.Context) { c.Set("user_id", primary.ID) }, GetAdminAuditConfig)
	r.PUT("/audit-config/primary", func(c *gin.Context) { c.Set("user_id", primary.ID) }, UpdateAdminAuditConfig)
	r.PUT("/audit-config/delegated", func(c *gin.Context) { c.Set("user_id", delegated.ID) }, UpdateAdminAuditConfig)

	initial := httptest.NewRecorder()
	r.ServeHTTP(initial, httptest.NewRequest(http.MethodGet, "/audit-config/primary", nil))
	if initial.Code != http.StatusOK {
		t.Fatalf("initial config status=%d body=%s", initial.Code, initial.Body.String())
	}
	var initialBody struct {
		Code int `json:"code"`
		Data struct {
			Enabled bool `json:"enabled"`
		} `json:"data"`
	}
	if err := json.Unmarshal(initial.Body.Bytes(), &initialBody); err != nil {
		t.Fatal(err)
	}
	if initialBody.Code != 1 || !initialBody.Data.Enabled {
		t.Fatalf("audit recording must default to enabled: %#v", initialBody)
	}

	denied := httptest.NewRecorder()
	r.ServeHTTP(denied, httptest.NewRequest(http.MethodPut, "/audit-config/delegated", bytes.NewBufferString(`{"enabled":false}`)))
	if denied.Code != http.StatusForbidden {
		t.Fatalf("delegated config update status=%d body=%s", denied.Code, denied.Body.String())
	}

	disabled := httptest.NewRecorder()
	r.ServeHTTP(disabled, httptest.NewRequest(http.MethodPut, "/audit-config/primary", bytes.NewBufferString(`{"enabled":false}`)))
	if disabled.Code != http.StatusOK {
		t.Fatalf("primary config update status=%d body=%s", disabled.Code, disabled.Body.String())
	}
	if err := authorization.New(db).WriteAudit(models.AdminAuditLog{ActorUserID: primary.ID, Result: "success", Summary: "must not persist while disabled"}); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := db.Model(&models.AdminAuditLog{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("disabled audit config wrote %d records", count)
	}
}
