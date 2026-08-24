package controllers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/repository"
)

func TestLoginResponseRedactionDoesNotPoisonCachedPassword(t *testing.T) {
	db, router, user, _ := setupCommentAccountTest(t)
	password := "repeat-login-password"
	if err := db.Model(&models.User{}).Where("id = ?", user.ID).Updates(map[string]interface{}{
		"password": models.HashPassword(password),
		"token":    models.GenerateToken(32),
	}).Error; err != nil {
		t.Fatalf("set login credentials: %v", err)
	}
	if err := db.Create(&models.SiteConfig{
		RuntimeMode:                 models.RuntimeModeLocal,
		RuntimeModeMigrationVersion: models.RuntimeModeMigrationVersionCurrent,
	}).Error; err != nil {
		t.Fatalf("create runtime config: %v", err)
	}
	repository.ClearUserCache()
	router.POST("/login", Login)

	for attempt := 1; attempt <= 2; attempt++ {
		payload, err := json.Marshal(map[string]string{"username": user.Username, "password": password})
		if err != nil {
			t.Fatalf("marshal login payload: %v", err)
		}
		request := httptest.NewRequest(http.MethodPost, "/login", bytes.NewReader(payload))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)

		var body struct {
			Code int `json:"code"`
			Data struct {
				Password string `json:"password"`
			} `json:"data"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode login response %d: %v", attempt, err)
		}
		if response.Code != http.StatusOK || body.Code != 1 {
			t.Fatalf("login attempt %d failed: status=%d body=%s", attempt, response.Code, response.Body.String())
		}
		if body.Data.Password != "" {
			t.Fatalf("login attempt %d exposed password data", attempt)
		}
	}
}
