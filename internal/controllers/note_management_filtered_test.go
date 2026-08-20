package controllers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/repository"
	"github.com/rcy1314/echo-noise/internal/services"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestBatchFilteredTrashReturnsSuccessForMoreThanOneThousandMatches(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "filtered-controller.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
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
		repository.ClearUserCache()
		database.DB = nil
		models.SetDB(nil)
		_ = sqlDB.Close()
	})
	primary := models.User{ID: models.PrimaryAdminUserID, Username: "filtered-controller-primary", IsAdmin: true}
	if err := db.Create(&primary).Error; err != nil {
		t.Fatal(err)
	}
	messages := make([]models.Message, 1001)
	for i := range messages {
		messages[i] = models.Message{Content: "filtered-controller-target", Username: primary.Username, UserID: primary.ID, Visibility: services.MessageVisibilityPublic}
	}
	if err := db.CreateInBatches(&messages, 200).Error; err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/admin/notes/batch-trash-filtered", bytes.NewBufferString(`{"filter":{"keyword":"filtered-controller-target"},"reason":"controller test"}`))
	context.Request.Header.Set("Content-Type", "application/json")
	context.Set("user_id", primary.ID)
	BatchFilteredTrash(context)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Code int `json:"code"`
		Data struct {
			Total     int `json:"total"`
			Processed int `json:"processed"`
			Succeeded int `json:"succeeded"`
			Failed    int `json:"failed"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Code != 1 || response.Data.Total != 1001 || response.Data.Processed != 1001 || response.Data.Succeeded != 1001 || response.Data.Failed != 0 {
		t.Fatalf("response=%s", recorder.Body.String())
	}
}
