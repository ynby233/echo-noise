package routers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/rcy1314/echo-noise/internal/authorization"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestAnnouncementRetryRouteRequiresPushCapabilityInAdditionToView(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("SESSION_SECRET", "announcement-route-test-secret-32-bytes-long")
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.Announcement{}, &models.AdminCapabilityGrant{}, &models.AdminAuditLog{}, &models.AdminAuditConfig{}); err != nil {
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
	if err := authorization.New(db).ReplaceGrants(primary.ID, delegated.ID, []authorization.Capability{authorization.CapabilityAnnouncementsView}); err != nil {
		t.Fatalf("grant announcement view: %v", err)
	}

	r := SetupRouter()
	r.GET("/test-seed-announcement-session", func(c *gin.Context) {
		session := sessions.Default(c)
		session.Set("user_id", delegated.ID)
		session.Set("username", delegated.Username)
		session.Set("is_admin", delegated.IsAdmin)
		if err := session.Save(); err != nil {
			c.Status(http.StatusInternalServerError)
			return
		}
		c.Status(http.StatusNoContent)
	})

	seedResponse := httptest.NewRecorder()
	r.ServeHTTP(seedResponse, httptest.NewRequest(http.MethodGet, "/test-seed-announcement-session", nil))
	if seedResponse.Code != http.StatusNoContent {
		t.Fatalf("seed session status=%d body=%s", seedResponse.Code, seedResponse.Body.String())
	}

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/announcements/999999/retry-push", nil)
	for _, cookie := range seedResponse.Result().Cookies() {
		request.AddCookie(cookie)
	}
	r.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("announcement retry with only view capability status=%d body=%s", response.Code, response.Body.String())
	}
}
