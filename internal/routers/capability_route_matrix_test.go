package routers

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/rcy1314/echo-noise/internal/authorization"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/middleware"
	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestProtectedAdminRouteMatrixRejectsDelegatedAdministratorWithoutRequiredGrant(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("ACCESS_LOG", "false")
	t.Setenv("SESSION_SECRET", "capability-route-matrix-test-secret-32")
	t.Chdir(t.TempDir())
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.AutoMigrate(
		&models.User{}, &models.SecurityConfig{}, &models.AdminCapabilityGrant{}, &models.AdminAuditLog{}, &models.AdminAuditConfig{},
	); err != nil {
		t.Fatalf("migrate route matrix database: %v", err)
	}
	database.DB = db
	models.SetDB(db)
	middleware.InvalidateAccessLogConfigCache()
	t.Cleanup(func() {
		database.DB = nil
		models.SetDB(nil)
		middleware.InvalidateAccessLogConfigCache()
	})

	primary := models.User{ID: models.PrimaryAdminUserID, Username: "primary", IsAdmin: true}
	delegated := models.User{Username: "delegated", IsAdmin: true, Token: "delegated-route-token"}
	if err := db.Create(&primary).Error; err != nil {
		t.Fatalf("create primary administrator: %v", err)
	}
	if err := db.Create(&delegated).Error; err != nil {
		t.Fatalf("create delegated administrator: %v", err)
	}

	r := SetupRouter()
	r.GET("/__test/route-matrix-session", func(c *gin.Context) {
		session := sessions.Default(c)
		session.Set("user_id", delegated.ID)
		session.Set("username", delegated.Username)
		session.Set("is_admin", delegated.IsAdmin)
		session.Set("login_expire_at", time.Now().Add(time.Hour).Unix())
		if err := session.Save(); err != nil {
			c.Status(http.StatusInternalServerError)
			return
		}
		c.Status(http.StatusNoContent)
	})
	seed := httptest.NewRecorder()
	r.ServeHTTP(seed, httptest.NewRequest(http.MethodGet, "/__test/route-matrix-session", nil))
	if seed.Code != http.StatusNoContent {
		t.Fatalf("seed delegated session status=%d body=%s", seed.Code, seed.Body.String())
	}
	cookies := seed.Result().Cookies()

	type routeCase struct {
		name       string
		method     string
		path       string
		capability authorization.Capability
		token      bool
	}
	cases := []routeCase{
		{name: "authorization catalog", method: http.MethodGet, path: "/api/admin/authorization/catalog", capability: authorization.CapabilityAuthorizationManage},
		{name: "audit logs", method: http.MethodGet, path: "/api/admin/audit-logs", capability: authorization.CapabilityAuditView},
		{name: "attachment list", method: http.MethodGet, path: "/api/attachments/images", capability: authorization.CapabilityAttachmentsView},
		{name: "version update", method: http.MethodPost, path: "/api/version/update", capability: authorization.CapabilityVersionUpdate},
		{name: "session settings", method: http.MethodPut, path: "/api/settings", capability: authorization.CapabilitySiteSettingsManage},
		{name: "token settings", method: http.MethodPut, path: "/api/token/settings", capability: authorization.CapabilitySiteSettingsManage, token: true},
		{name: "notification config", method: http.MethodGet, path: "/api/notify/config", capability: authorization.CapabilityNotificationsView},
		{name: "email test", method: http.MethodPost, path: "/api/email/test", capability: authorization.CapabilityEmailManage},
		{name: "announcement list", method: http.MethodGet, path: "/api/admin/announcements", capability: authorization.CapabilityAnnouncementsView},
		{name: "security attacks", method: http.MethodGet, path: "/api/security/attacks", capability: authorization.CapabilitySecurityView},
		{name: "backup download", method: http.MethodGet, path: "/api/backup/download", capability: authorization.CapabilityDatabaseBackup},
		{name: "user password reset", method: http.MethodPost, path: "/api/user/reset_password", capability: authorization.CapabilityUsersResetPassword},
		{name: "registration applications", method: http.MethodGet, path: "/api/registration/applications", capability: authorization.CapabilityRegistrationView},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if _, ok := authorization.DefinitionFor(testCase.capability); !ok {
				t.Fatalf("route matrix references unknown capability %q", testCase.capability)
			}
			request := httptest.NewRequest(testCase.method, testCase.path, bytes.NewBufferString(`{}`))
			request.Header.Set("Content-Type", "application/json")
			if testCase.token {
				request.Header.Set("Authorization", "Bearer "+delegated.Token)
			} else {
				for _, cookie := range cookies {
					request.AddCookie(cookie)
				}
			}
			response := httptest.NewRecorder()
			r.ServeHTTP(response, request)
			if response.Code != http.StatusForbidden {
				t.Fatalf("%s without %s status=%d body=%s", testCase.path, testCase.capability, response.Code, response.Body.String())
			}
		})
	}
}
