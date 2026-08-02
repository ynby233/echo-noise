package middleware

import (
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

func TestRequireCapabilityRejectsDelegatedAdministratorImmediatelyAfterRevocation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.AdminCapabilityGrant{}, &models.AdminAuditLog{}, &models.AdminAuditConfig{}); err != nil {
		t.Fatalf("migrate database: %v", err)
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
	authorizer := authorization.New(db)
	if err := authorizer.ReplaceGrants(primary.ID, delegated.ID, []authorization.Capability{authorization.CapabilityAuditView}); err != nil {
		t.Fatal(err)
	}

	r := gin.New()
	r.GET("/audit", func(c *gin.Context) { c.Set("user_id", delegated.ID); c.Set("auth_via", "token") }, RequireCapability(authorization.CapabilityAuditView), func(c *gin.Context) { c.Status(http.StatusNoContent) })
	request := func() *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/audit", nil))
		return w
	}
	if got := request().Code; got != http.StatusNoContent {
		t.Fatalf("granted request status=%d", got)
	}
	if err := authorizer.ReplaceGrants(primary.ID, delegated.ID, nil); err != nil {
		t.Fatal(err)
	}
	if got := request().Code; got != http.StatusForbidden {
		t.Fatalf("revoked request status=%d, want 403", got)
	}

	var denied int64
	if err := db.Model(&models.AdminAuditLog{}).Where("result = ?", "denied").Count(&denied).Error; err != nil || denied != 1 {
		t.Fatalf("denied audit count=%d err=%v", denied, err)
	}
}

func TestRequireCapabilityWritesSuccessfulAdministrativeMutationAudit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.AdminCapabilityGrant{}, &models.AdminAuditLog{}, &models.AdminAuditConfig{}); err != nil {
		t.Fatalf("migrate database: %v", err)
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
	if err := authorization.New(db).ReplaceGrants(primary.ID, delegated.ID, []authorization.Capability{authorization.CapabilityCommentsEdit}); err != nil {
		t.Fatal(err)
	}

	r := gin.New()
	r.POST("/comments/42", func(c *gin.Context) { c.Set("user_id", delegated.ID); c.Set("auth_via", "session") }, RequireCapability(authorization.CapabilityCommentsEdit), func(c *gin.Context) { c.Status(http.StatusNoContent) })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/comments/42", nil))
	if w.Code != http.StatusNoContent {
		t.Fatalf("write request status=%d", w.Code)
	}
	var record models.AdminAuditLog
	if err := db.Where("capability = ? AND result = ?", authorization.CapabilityCommentsEdit, "success").First(&record).Error; err != nil {
		t.Fatalf("successful audit record: %v", err)
	}
	if record.AuthVia != "session" || record.TargetID != "/comments/42" {
		t.Fatalf("unexpected audit record: %#v", record)
	}
}
