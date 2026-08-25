package controllers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/pkg"
	"gorm.io/gorm"
)

func setupMobileSetupControllerTest(t *testing.T) (*gorm.DB, *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "mobile-controller.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := models.MigrateDB(db); err != nil {
		t.Fatal(err)
	}
	database.DB = db
	models.SetDB(db)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		database.DB = nil
		models.SetDB(nil)
		_ = sqlDB.Close()
	})

	r := gin.New()
	pkg.InitSession(r)
	r.GET("/api/setup/status", GetMobileSetupStatus)
	r.POST("/api/setup/owner", InitializeMobileSiteOwner)
	r.GET("/session", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"user_id": sessions.Default(c).Get("user_id")})
	})
	return db, r
}

func TestMobileSetupControllerInitializesAndLogsInOwner(t *testing.T) {
	db, r := setupMobileSetupControllerTest(t)
	t.Setenv("NOISE_MOBILE", "1")
	t.Setenv("SESSION_SECRET", "mobile-setup-test-session-secret-123456")

	statusRecorder := httptest.NewRecorder()
	r.ServeHTTP(statusRecorder, httptest.NewRequest(http.MethodGet, "/api/setup/status", nil))
	if statusRecorder.Code != http.StatusOK || !bytes.Contains(statusRecorder.Body.Bytes(), []byte(`"setup_state":"required"`)) {
		t.Fatalf("initial status=%d body=%s", statusRecorder.Code, statusRecorder.Body.String())
	}

	body := bytes.NewBufferString(`{"username":"site_owner","password":"strong-password","confirm_password":"strong-password"}`)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/setup/owner", body)
	request.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("initialize status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Code int `json:"code"`
		Data struct {
			ID       uint   `json:"id"`
			Username string `json:"username"`
			Password string `json:"password"`
			Token    string `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Code != 1 || response.Data.ID != models.PrimaryAdminUserID || response.Data.Username != "site_owner" {
		t.Fatalf("response = %#v body=%s", response, recorder.Body.String())
	}
	if response.Data.Password != "" || response.Data.Token != "" {
		t.Fatalf("setup response exposed credentials: %#v", response.Data)
	}

	var owner models.User
	if err := db.First(&owner, models.PrimaryAdminUserID).Error; err != nil || !owner.IsAdmin {
		t.Fatalf("stored owner=%#v err=%v", owner, err)
	}
	if len(recorder.Result().Cookies()) == 0 {
		t.Fatal("setup response did not establish a login session")
	}
}

func TestMobileSetupControllerIsUnavailableOutsideAndroidStandalone(t *testing.T) {
	_, r := setupMobileSetupControllerTest(t)
	t.Setenv("NOISE_MOBILE", "")
	recorder := httptest.NewRecorder()
	r.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/setup/status", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
