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

func TestNoteManagementRoutesEnforceViewPrerequisitesForSessionAndBearer(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("ACCESS_LOG", "false")
	t.Setenv("SESSION_SECRET", "note-management-route-test-secret-32")
	t.Chdir(t.TempDir())
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if sqlDB, err := db.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1)
	}
	if err := db.AutoMigrate(&models.User{}, &models.Message{}, &models.Comment{}, &models.MessageLike{}, &models.UserNotification{}, &models.SecurityConfig{}, &models.AdminCapabilityGrant{}, &models.AdminAuditLog{}, &models.AdminAuditConfig{}); err != nil {
		t.Fatalf("migrate database: %v", err)
	}
	database.DB = db
	models.SetDB(db)
	middleware.InvalidateAccessLogConfigCache()
	t.Cleanup(func() {
		database.DB = nil
		models.SetDB(nil)
		middleware.InvalidateAccessLogConfigCache()
	})

	primary := models.User{ID: models.PrimaryAdminUserID, Username: "route-primary", IsAdmin: true}
	delegated := models.User{Username: "route-delegated", IsAdmin: true, Token: "route-delegated-token"}
	if err := db.Create(&primary).Error; err != nil {
		t.Fatalf("create primary: %v", err)
	}
	if err := db.Create(&delegated).Error; err != nil {
		t.Fatalf("create delegated: %v", err)
	}
	deletedAt := time.Now().UTC()
	note := models.Message{Content: "route recycle note", Username: "route-author", UserID: delegated.ID + 1, Visibility: "public", DeletedAt: &deletedAt}
	if err := db.Create(&note).Error; err != nil {
		t.Fatalf("create recycle note: %v", err)
	}

	r := SetupRouter()
	r.GET("/__test/note-management-session", func(c *gin.Context) {
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
	r.ServeHTTP(seed, httptest.NewRequest(http.MethodGet, "/__test/note-management-session", nil))
	if seed.Code != http.StatusNoContent {
		t.Fatalf("seed session status=%d body=%s", seed.Code, seed.Body.String())
	}
	cookies := seed.Result().Cookies()

	request := func(method, path string, bearer bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, bytes.NewBufferString(`{}`))
		req.Header.Set("Content-Type", "application/json")
		if bearer {
			req.Header.Set("Authorization", "Bearer "+delegated.Token)
		} else {
			for _, cookie := range cookies {
				req.AddCookie(cookie)
			}
		}
		response := httptest.NewRecorder()
		r.ServeHTTP(response, req)
		return response
	}
	assertBoth := func(name, method, sessionPath, bearerPath string, wantStatus int) {
		t.Run(name, func(t *testing.T) {
			sessionResponse := request(method, sessionPath, false)
			bearerResponse := request(method, bearerPath, true)
			if sessionResponse.Code != wantStatus || bearerResponse.Code != wantStatus {
				t.Fatalf("session=%d bearer=%d want=%d; bodies=%s / %s", sessionResponse.Code, bearerResponse.Code, wantStatus, sessionResponse.Body.String(), bearerResponse.Body.String())
			}
		})
	}

	assertBoth("notes view is required", http.MethodGet, "/api/admin/notes", "/api/token/admin/notes", http.StatusForbidden)
	assertBoth("recycle-bin view is required", http.MethodGet, "/api/admin/recycle-bin", "/api/token/admin/recycle-bin", http.StatusForbidden)

	if err := db.Create(&[]models.AdminCapabilityGrant{
		{UserID: delegated.ID, Capability: string(authorization.CapabilityNotesRestore), GrantedByUserID: primary.ID},
		{UserID: delegated.ID, Capability: string(authorization.CapabilityNotesDelete), GrantedByUserID: primary.ID},
	}).Error; err != nil {
		t.Fatalf("create orphan recycle-bin grants: %v", err)
	}
	assertBoth("restore cannot bypass recycle-bin view", http.MethodPost, "/api/admin/recycle-bin/1/restore", "/api/token/admin/recycle-bin/1/restore", http.StatusForbidden)
	assertBoth("permanent delete cannot bypass recycle-bin view", http.MethodDelete, "/api/admin/recycle-bin/1", "/api/token/admin/recycle-bin/1", http.StatusForbidden)

	if err := authorization.New(db).ReplaceGrants(primary.ID, delegated.ID, []authorization.Capability{authorization.CapabilityNotesView, authorization.CapabilityNotesRecycleBinView}); err != nil {
		t.Fatalf("grant view capabilities: %v", err)
	}
	assertBoth("activity list with notes view", http.MethodGet, "/api/admin/notes", "/api/token/admin/notes", http.StatusOK)
	sessionRecycle := request(http.MethodGet, "/api/admin/recycle-bin", false)
	bearerRecycle := request(http.MethodGet, "/api/token/admin/recycle-bin", true)
	if sessionRecycle.Code != http.StatusOK || bearerRecycle.Code != http.StatusOK {
		t.Fatalf("recycle list status session=%d bearer=%d", sessionRecycle.Code, bearerRecycle.Code)
	}
	if sessionRecycle.Body.String() != bearerRecycle.Body.String() {
		t.Fatalf("session and bearer recycle results differ:\nsession=%s\nbearer=%s", sessionRecycle.Body.String(), bearerRecycle.Body.String())
	}
}
