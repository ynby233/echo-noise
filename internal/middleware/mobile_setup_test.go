package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
)

func setupMobileGateTest(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/mobile-gate.db"), &gorm.Config{})
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
	return db
}

func TestMobileSetupGateBlocksAPIsUntilExplicitPrimaryOwnerExists(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupMobileGateTest(t)
	t.Setenv("NOISE_MOBILE", "1")
	r := gin.New()
	r.Use(MobileSetupGate())
	r.GET("/api/setup/status", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	r.GET("/api/messages", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	assertStatus := func(path string, want int) {
		t.Helper()
		recorder := httptest.NewRecorder()
		r.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != want {
			t.Fatalf("GET %s status=%d body=%s, want %d", path, recorder.Code, recorder.Body.String(), want)
		}
	}
	assertStatus("/api/setup/status", http.StatusNoContent)
	assertStatus("/", http.StatusNoContent)
	assertStatus("/api/messages", http.StatusPreconditionRequired)

	if err := db.Create(&models.User{ID: models.PrimaryAdminUserID, Username: "owner", Password: models.HashPassword("password"), IsAdmin: true}).Error; err != nil {
		t.Fatal(err)
	}
	assertStatus("/api/messages", http.StatusNoContent)
}

func TestMobileSetupGateFailsClosedForInvalidDataAndIsNoopElsewhere(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupMobileGateTest(t)
	if err := db.Create(&models.User{ID: 2, Username: "existing", Password: models.HashPassword("password")}).Error; err != nil {
		t.Fatal(err)
	}

	t.Setenv("NOISE_MOBILE", "1")
	r := gin.New()
	r.Use(MobileSetupGate())
	r.GET("/api/messages", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	recorder := httptest.NewRecorder()
	r.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/messages", nil))
	if recorder.Code != http.StatusLocked {
		t.Fatalf("invalid mobile database status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	t.Setenv("NOISE_MOBILE", "")
	recorder = httptest.NewRecorder()
	r.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/messages", nil))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("non-mobile route status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
