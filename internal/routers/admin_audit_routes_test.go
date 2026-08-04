package routers

import (
	"bytes"
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

func TestAdminAuditExportRouteRequiresAuditViewCapability(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.AdminCapabilityGrant{}, &models.AdminAuditLog{}, &models.AdminAuditConfig{}); err != nil {
		t.Fatalf("migrate database: %v", err)
	}
	primary := models.User{ID: models.PrimaryAdminUserID, Username: "primary", IsAdmin: true}
	delegated := models.User{Username: "delegated", IsAdmin: true}
	if err := db.Create(&primary).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&delegated).Error; err != nil {
		t.Fatal(err)
	}
	database.DB = db
	t.Cleanup(func() { database.DB = nil })

	r := gin.New()
	authRoutes := r.Group("/api")
	authRoutes.Use(func(c *gin.Context) {
		c.Set("user_id", delegated.ID)
		c.Set("auth_via", "session")
		c.Next()
	})
	registerAdminAuthorizationRoutes(authRoutes)

	denied := httptest.NewRecorder()
	r.ServeHTTP(denied, httptest.NewRequest(http.MethodGet, "/api/admin/audit-logs/export", nil))
	if denied.Code != http.StatusForbidden {
		t.Fatalf("export without audit.view status=%d body=%s", denied.Code, denied.Body.String())
	}

	if err := db.Create(&models.AdminCapabilityGrant{UserID: delegated.ID, Capability: "audit.view", GrantedByUserID: primary.ID}).Error; err != nil {
		t.Fatal(err)
	}
	allowed := httptest.NewRecorder()
	r.ServeHTTP(allowed, httptest.NewRequest(http.MethodGet, "/api/admin/audit-logs/export", nil))
	if allowed.Code != http.StatusOK || !bytes.HasPrefix(allowed.Body.Bytes(), []byte{0xef, 0xbb, 0xbf}) {
		t.Fatalf("export with audit.view status=%d body=%s", allowed.Code, allowed.Body.String())
	}
}
