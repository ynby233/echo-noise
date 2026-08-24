package controllers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/repository"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestBindPrimaryAdminVoceChatEmailControllerRejectsDelegatedAdministrator(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.SiteConfig{}, &models.RegistrationApplication{}); err != nil {
		t.Fatalf("migrate test db: %v", err)
	}
	database.DB = db
	models.SetDB(db)
	repository.ClearUserCache()
	t.Cleanup(func() {
		repository.ClearUserCache()
		database.DB = nil
		models.SetDB(nil)
	})
	users := []models.User{
		{Username: "primary", Password: models.HashPassword("password"), IsAdmin: true},
		{Username: "delegated", Password: models.HashPassword("password"), IsAdmin: true},
	}
	if err := db.Create(&users).Error; err != nil {
		t.Fatalf("create users: %v", err)
	}

	router := gin.New()
	router.PUT("/api/user/vocechat/bind", func(c *gin.Context) {
		c.Set("user_id", users[1].ID)
		BindPrimaryAdminVoceChatEmail(c)
	})
	body, _ := json.Marshal(map[string]string{"email": "existing@vc.test", "password": "vc-password"})
	request := httptest.NewRequest(http.MethodPut, "/api/user/vocechat/bind", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "站长") || !strings.Contains(response.Body.String(), `"code":0`) {
		t.Fatalf("delegated binding response = %d %s", response.Code, response.Body.String())
	}
}
