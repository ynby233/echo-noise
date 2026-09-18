package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/updates"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestExecutorCredentialIsScopedAndRevocationIsImmediate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.SiteConfig{}, &models.UpdatePreference{}, &models.UpdateExecutorCredential{}); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	primary := models.User{ID: models.PrimaryAdminUserID, Username: "primary-executor-auth", Token: "primary-user-token", IsAdmin: true, LoginIssuedAt: &now}
	if err := db.Create(&primary).Error; err != nil {
		t.Fatal(err)
	}
	database.DB = db
	models.SetDB(db)
	t.Cleanup(func() { database.DB = nil; models.SetDB(nil) })
	service := updates.NewTaskService(db)
	_, executorToken, err := service.CreateCredential(primary.ID, "host")
	if err != nil {
		t.Fatal(err)
	}

	router := gin.New()
	router.Use(sessions.Sessions("test", cookie.NewStore([]byte("executor-auth-test"))))
	router.GET("/executor", UpdateExecutorAuthMiddleware(), func(c *gin.Context) { c.Status(http.StatusNoContent) })
	router.GET("/business", SessionAuthMiddleware(), func(c *gin.Context) { c.Status(http.StatusNoContent) })
	call := func(path, token string) int {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Header.Set("Authorization", "Bearer "+token)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response.Code
	}
	if status := call("/executor", executorToken); status != http.StatusNoContent {
		t.Fatalf("executor token status=%d", status)
	}
	if status := call("/executor", primary.Token); status != http.StatusUnauthorized {
		t.Fatalf("administrator token reached executor API: %d", status)
	}
	if status := call("/business", executorToken); status != http.StatusUnauthorized {
		t.Fatalf("executor token reached business API: %d", status)
	}
	if err := service.RevokeCredential(primary.ID); err != nil {
		t.Fatal(err)
	}
	if status := call("/executor", executorToken); status != http.StatusUnauthorized {
		t.Fatalf("revoked executor token status=%d", status)
	}
}
