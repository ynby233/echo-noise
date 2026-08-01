package routers

import (
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

func TestFeedRefreshRequiresAdministrator(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("ACCESS_LOG", "false")
	t.Setenv("SESSION_SECRET", "feed-refresh-route-test-secret")
	t.Chdir(t.TempDir())
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.SecurityConfig{}, &models.AdminCapabilityGrant{}, &models.AdminAuditLog{}, &models.AdminAuditConfig{}); err != nil {
		t.Fatalf("migrate route test database: %v", err)
	}
	primary := models.User{ID: models.PrimaryAdminUserID, Username: "primary", IsAdmin: true}
	ordinary := models.User{Username: "member"}
	admin := models.User{Username: "admin", IsAdmin: true}
	if err := db.Create(&primary).Error; err != nil {
		t.Fatalf("create primary admin: %v", err)
	}
	if err := db.Create(&ordinary).Error; err != nil {
		t.Fatalf("create ordinary user: %v", err)
	}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatalf("create admin user: %v", err)
	}
	if err := authorization.New(db).ReplaceGrants(primary.ID, admin.ID, []authorization.Capability{authorization.CapabilityFeedManage}); err != nil {
		t.Fatalf("grant feed management: %v", err)
	}
	database.DB = db
	models.SetDB(db)
	middleware.InvalidateAccessLogConfigCache()
	t.Cleanup(func() {
		database.DB = nil
		models.SetDB(nil)
		middleware.InvalidateAccessLogConfigCache()
	})

	r := SetupRouter()
	r.GET("/__test/session/:role", func(c *gin.Context) {
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
			c.String(http.StatusInternalServerError, err.Error())
			return
		}
		c.Status(http.StatusNoContent)
	})

	cookiesFor := func(role string) []*http.Cookie {
		t.Helper()
		response := httptest.NewRecorder()
		r.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/__test/session/"+role, nil))
		if response.Code != http.StatusNoContent {
			t.Fatalf("seed %s session: %d %s", role, response.Code, response.Body.String())
		}
		return response.Result().Cookies()
	}
	requestRefresh := func(cookies []*http.Cookie) *httptest.ResponseRecorder {
		t.Helper()
		request := httptest.NewRequest(http.MethodPost, "/api/feed/refresh", nil)
		for _, cookie := range cookies {
			request.AddCookie(cookie)
		}
		response := httptest.NewRecorder()
		r.ServeHTTP(response, request)
		return response
	}

	if response := requestRefresh(nil); response.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous feed refresh status = %d, want 401: %s", response.Code, response.Body.String())
	}
	if response := requestRefresh(cookiesFor("member")); response.Code != http.StatusForbidden {
		t.Fatalf("ordinary feed refresh status = %d, want 403: %s", response.Code, response.Body.String())
	}
	if response := requestRefresh(cookiesFor("admin")); response.Code != http.StatusOK {
		t.Fatalf("admin feed refresh status = %d, want 200: %s", response.Code, response.Body.String())
	}
}
