package routers

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/rcy1314/echo-noise/internal/authorization"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/middleware"
	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestAttachmentManagementListsRequireAdminSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Chdir(t.TempDir())

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.Message{}, &models.SiteConfig{}, &models.AdminCapabilityGrant{}, &models.AdminAuditLog{}, &models.AdminAuditConfig{}); err != nil {
		t.Fatalf("migrate test database: %v", err)
	}
	database.DB = db
	models.SetDB(db)
	t.Cleanup(func() {
		database.DB = nil
		models.SetDB(nil)
	})

	primary := models.User{ID: models.PrimaryAdminUserID, Username: "primary", Token: "primary-token", IsAdmin: true}
	ordinary := models.User{Username: "member", Token: "member-token"}
	admin := models.User{Username: "admin", Token: "admin-token", IsAdmin: true}
	if err := db.Create(&primary).Error; err != nil {
		t.Fatalf("create primary admin: %v", err)
	}
	if err := db.Create(&ordinary).Error; err != nil {
		t.Fatalf("create ordinary user: %v", err)
	}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatalf("create admin user: %v", err)
	}
	if err := authorization.New(db).ReplaceGrants(primary.ID, admin.ID, []authorization.Capability{authorization.CapabilityAttachmentsView}); err != nil {
		t.Fatalf("grant attachment view: %v", err)
	}

	r := gin.New()
	r.Use(sessions.Sessions("test", cookie.NewStore([]byte("attachment-route-test-secret"))))
	r.GET("/seed/:role", func(c *gin.Context) {
		selected := ordinary
		if c.Param("role") == "admin" {
			selected = admin
		}
		session := sessions.Default(c)
		session.Set("user_id", selected.ID)
		session.Set("username", selected.Username)
		session.Set("is_admin", selected.IsAdmin)
		session.Set("login_expire_at", time.Now().Add(time.Hour).Unix())
		if err := session.Save(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.Status(http.StatusNoContent)
	})
	api := r.Group("/api")
	authRoutes := api.Group("")
	authRoutes.Use(middleware.SessionAuthMiddleware())
	registerAttachmentManagementRoutes(authRoutes)

	cookiesFor := func(role string) []*http.Cookie {
		t.Helper()
		response := httptest.NewRecorder()
		r.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/seed/"+role, nil))
		if response.Code != http.StatusNoContent {
			t.Fatalf("seed %s session: %d %s", role, response.Code, response.Body.String())
		}
		return response.Result().Cookies()
	}
	request := func(cookies []*http.Cookie) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/api/attachments/images", nil)
		for _, value := range cookies {
			req.AddCookie(value)
		}
		response := httptest.NewRecorder()
		r.ServeHTTP(response, req)
		return response
	}

	if response := request(cookiesFor("member")); response.Code != http.StatusForbidden {
		t.Fatalf("ordinary attachment list status = %d, want 403: %s", response.Code, response.Body.String())
	}
	if response := request(cookiesFor("admin")); response.Code != http.StatusOK {
		t.Fatalf("admin attachment list status = %d, want 200: %s", response.Code, response.Body.String())
	}
}
