package controllers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/updates"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupUpdateControllerTest(t *testing.T) (*gorm.DB, *gin.Engine, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.AdminAuditLog{}, &models.AdminAuditConfig{}, &models.UpdatePreference{}, &models.UpdateExecutorCredential{}, &models.UpdateTask{}, &models.UpdateTaskEvent{}); err != nil {
		t.Fatal(err)
	}
	users := []models.User{{ID: models.PrimaryAdminUserID, Username: "primary", IsAdmin: true}, {Username: "delegated", IsAdmin: true}, {Username: "ordinary"}}
	if err := db.Create(&users).Error; err != nil {
		t.Fatal(err)
	}
	database.DB = db
	models.SetDB(db)
	t.Cleanup(func() { database.DB = nil; models.SetDB(nil) })
	_, rawToken, err := updates.NewTaskService(db).CreateCredential(models.PrimaryAdminUserID, "test executor")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := updates.NewTaskService(db).Authenticate(rawToken); err != nil {
		t.Fatal(err)
	}
	originalDiscovery := discoverUpdates
	discoverUpdates = func(*gin.Context) updates.Report {
		return updates.Report{Channels: []updates.Channel{{
			Name: "edge", Image: "ghcr.io/ynby233/echo-noise", Digest: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			Revision: targetRevisionForController, Status: updates.StatusUpdateAvailable, Installable: true, HasUpdate: true,
		}}}
	}
	t.Cleanup(func() { discoverUpdates = originalDiscovery })

	router := gin.New()
	router.Use(func(c *gin.Context) {
		switch c.GetHeader("X-Test-Role") {
		case "primary":
			c.Set("user_id", users[0].ID)
		case "delegated":
			c.Set("user_id", users[1].ID)
		case "ordinary":
			c.Set("user_id", users[2].ID)
		}
		c.Next()
	})
	router.POST("/updates/tasks", CreateUpdateTask)
	router.GET("/updates/tasks/:id", GetUpdateTask)
	router.GET("/version/update/stream", UpdateVersionStream)
	return db, router, rawToken
}

func TestUpdateTaskHTTPRequiresPrimaryAndDeduplicatesRepeatedPost(t *testing.T) {
	db, router, _ := setupUpdateControllerTest(t)
	request := func(role string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/updates/tasks", bytes.NewBufferString(`{"channel":"edge"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Test-Role", role)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		return response
	}
	for _, role := range []string{"", "ordinary", "delegated", "executor"} {
		if response := request(role); response.Code != http.StatusForbidden {
			t.Fatalf("role=%q status=%d body=%s", role, response.Code, response.Body.String())
		}
	}
	first, second := request("primary"), request("primary")
	if first.Code != http.StatusCreated || second.Code != http.StatusOK {
		t.Fatalf("statuses=%d,%d bodies=%s / %s", first.Code, second.Code, first.Body.String(), second.Body.String())
	}
	var a, b struct {
		Data models.UpdateTask `json:"data"`
	}
	if err := json.Unmarshal(first.Body.Bytes(), &a); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(second.Body.Bytes(), &b); err != nil {
		t.Fatal(err)
	}
	if a.Data.PublicID == "" || a.Data.PublicID != b.Data.PublicID {
		t.Fatalf("task ids differ: %q %q", a.Data.PublicID, b.Data.PublicID)
	}
	var count int64
	if err := db.Model(&models.UpdateTask{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("task count=%d err=%v", count, err)
	}
}

func TestUpdateTaskReadRedactsSensitiveTargetOutsidePrimaryAdministrator(t *testing.T) {
	_, router, _ := setupUpdateControllerTest(t)
	createRequest := httptest.NewRequest(http.MethodPost, "/updates/tasks", bytes.NewBufferString(`{"channel":"edge"}`))
	createRequest.Header.Set("Content-Type", "application/json")
	createRequest.Header.Set("X-Test-Role", "primary")
	created := httptest.NewRecorder()
	router.ServeHTTP(created, createRequest)
	var payload struct {
		Data models.UpdateTask `json:"data"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &payload); err != nil || payload.Data.PublicID == "" {
		t.Fatalf("created body=%s err=%v", created.Body.String(), err)
	}
	read := func(role, id string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodGet, "/updates/tasks/"+id, nil)
		request.Header.Set("X-Test-Role", role)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}
	if response := read("delegated", payload.Data.PublicID); response.Code != http.StatusOK || bytes.Contains(response.Body.Bytes(), []byte("sha256:")) || bytes.Contains(response.Body.Bytes(), []byte(targetRevisionForController)) {
		t.Fatalf("delegated response leaked target: %d %s", response.Code, response.Body.String())
	}
	if response := read("primary", payload.Data.PublicID); response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte("sha256:")) || !bytes.Contains(response.Body.Bytes(), []byte(targetRevisionForController)) {
		t.Fatalf("primary response lost target: %d %s", response.Code, response.Body.String())
	}
	if response := read("primary", "1"); response.Code != http.StatusNotFound {
		t.Fatalf("sequential task id was accepted: %d %s", response.Code, response.Body.String())
	}
}

func TestLegacyUpdateStreamIsReadOnlyAndExplicitlyRetired(t *testing.T) {
	db, router, _ := setupUpdateControllerTest(t)
	req := httptest.NewRequest(http.MethodGet, "/version/update/stream", nil)
	req.Header.Set("X-Test-Role", "primary")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if response.Code != http.StatusGone {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var count int64
	if err := db.Model(&models.UpdateTask{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("legacy GET created %d tasks: %v", count, err)
	}
}
