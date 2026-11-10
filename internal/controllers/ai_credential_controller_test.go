package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/repository"
	"github.com/rcy1314/echo-noise/internal/services"
	"gorm.io/gorm"
)

func setupAICredentialControllerTest(t *testing.T) (*gorm.DB, *gin.Engine, models.User, models.User) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	oldDB, oldModelDB := database.DB, models.GetDB()
	t.Cleanup(func() {
		repository.ClearUserCache()
		database.DB = oldDB
		models.SetDB(oldModelDB)
		_ = sqlDB.Close()
	})
	if err := db.AutoMigrate(&models.User{}, &models.AICredential{}, &models.SiteConfig{}, &models.SecurityLoginAudit{}, &models.LoginAuditConfig{}); err != nil {
		t.Fatal(err)
	}
	database.DB = db
	models.SetDB(db)
	repository.ClearUserCache()
	config := models.SiteConfig{RuntimeMode: models.RuntimeModeLocal, RuntimeModeMigrationVersion: models.RuntimeModeMigrationVersionCurrent}
	if err := db.Create(&config).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&config).Updates(map[string]any{"login_expire_days": 0, "login_expire_hours": 1}).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now().Add(-time.Minute).UTC()
	owner := models.User{ID: 41, Username: "controller-owner", Password: models.HashPassword("owner-password"), Token: "controller-owner-token", LoginIssuedAt: &now}
	other := models.User{ID: 42, Username: "controller-other", Password: models.HashPassword("other-password"), Token: "controller-other-token", LoginIssuedAt: &now}
	for _, user := range []*models.User{&owner, &other} {
		if err := db.Create(user).Error; err != nil {
			t.Fatal(err)
		}
	}
	router := gin.New()
	// Test the controller's own guard independently from route middleware.
	// Identity/login also receive this conflicting context to prove they ignore it.
	router.Use(func(c *gin.Context) {
		c.Set("user_id", owner.ID)
		c.Set("auth_via", c.GetHeader("X-Test-Auth-Via"))
	})
	router.GET("/api/user/ai-credentials", ListAICredentials)
	router.POST("/api/user/ai-credentials", CreateAICredential)
	router.DELETE("/api/user/ai-credentials/:id", RevokeAICredential)
	router.GET("/api/auth/identity", GetAuthIdentity)
	router.POST("/api/ai/login", AILogin)
	return db, router, owner, other
}

func aiCredentialRequest(router http.Handler, method, path, body, bearer, via string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", bearer)
	request.Header.Set("X-Test-Auth-Via", via)
	request.AddCookie(&http.Cookie{Name: "session", Value: "ignored-session-cookie"})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func aiCredentialResponseData(t *testing.T, response *httptest.ResponseRecorder) json.RawMessage {
	t.Helper()
	var body struct {
		Code int             `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || body.Code != 1 {
		t.Fatalf("expected success: status=%d body=%s", response.Code, response.Body.String())
	}
	return body.Data
}

func TestAICredentialControllerCreationIsOnlySecretDisclosure(t *testing.T) {
	_, router, owner, other := setupAICredentialControllerTest(t)
	path := "/api/user/ai-credentials"
	empty := aiCredentialResponseData(t, aiCredentialRequest(router, http.MethodGet, path, "", "", "session"))
	if string(empty) != "[]" {
		t.Fatalf("empty list must be an array: %s", empty)
	}
	response := aiCredentialRequest(router, http.MethodPost, path, `{"name":"  写作助手  "}`, "", "session")
	var created map[string]json.RawMessage
	if err := json.Unmarshal(aiCredentialResponseData(t, response), &created); err != nil {
		t.Fatal(err)
	}
	if len(created) != 4 || string(created["name"]) != `"写作助手"` || len(created["created_at"]) == 0 {
		t.Fatalf("creation fields must be exactly id/name/created_at/token: %v", created)
	}
	var id uint
	var token string
	if err := json.Unmarshal(created["id"], &id); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(created["token"], &token); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(token, services.AICredentialTokenPrefix) {
		t.Fatal("missing creation secret")
	}
	foreign, _, err := services.CreateAICredential(other.ID, "private-other")
	if err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		listedResponse := aiCredentialRequest(router, http.MethodGet, path, "", "", "session")
		var listed []map[string]json.RawMessage
		if err := json.Unmarshal(aiCredentialResponseData(t, listedResponse), &listed); err != nil {
			t.Fatal(err)
		}
		if len(listed) != 1 || len(listed[0]) != 3 || string(listed[0]["id"]) != fmt.Sprint(id) || string(listed[0]["name"]) != `"写作助手"` || len(listed[0]["created_at"]) == 0 {
			t.Fatalf("list must contain only own public metadata: %s", listedResponse.Body.String())
		}
		if strings.Contains(listedResponse.Body.String(), token) || strings.Contains(listedResponse.Body.String(), "token") {
			t.Fatal("subsequent lists must not reveal the secret or hash")
		}
	}
	foreignDelete := aiCredentialRequest(router, http.MethodDelete, fmt.Sprintf("%s/%d", path, foreign.ID), "", "", "session")
	if foreignDelete.Code != http.StatusNotFound {
		t.Fatal("foreign credential deletion must be rejected")
	}
	deleted := aiCredentialRequest(router, http.MethodDelete, fmt.Sprintf("%s/%d", path, id), "", "", "session")
	aiCredentialResponseData(t, deleted)
	var deleteBody map[string]any
	if err := json.Unmarshal(deleted.Body.Bytes(), &deleteBody); err != nil || len(deleteBody) != 1 {
		t.Fatal("delete response must only contain code")
	}
	if string(aiCredentialResponseData(t, aiCredentialRequest(router, http.MethodGet, path, "", "", "session"))) != "[]" {
		t.Fatal("revoked credentials must disappear from the list")
	}
	if _, _, _, err := services.ResolveBearerIdentity("Bearer "+token, true); err == nil {
		t.Fatal("deleted secret must no longer authenticate")
	}
	if remaining, err := services.ListAICredentials(other.ID); err != nil || len(remaining) != 1 {
		t.Fatal("foreign credential must remain intact")
	}
	if remaining, err := services.ListAICredentials(owner.ID); err != nil || len(remaining) != 0 {
		t.Fatal("owner credential must be deleted")
	}
}

func TestAICredentialManagementRequiresSessionContext(t *testing.T) {
	_, router, owner, _ := setupAICredentialControllerTest(t)
	credential, token, err := services.CreateAICredential(owner.ID, "session-only")
	if err != nil {
		t.Fatal(err)
	}
	for _, via := range []string{"", "token", "ai_token", "unknown"} {
		for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodDelete} {
			path := "/api/user/ai-credentials"
			if method == http.MethodDelete {
				path += fmt.Sprintf("/%d", credential.ID)
			}
			response := aiCredentialRequest(router, method, path, `{"name":"denied"}`, "Bearer "+token, via)
			if response.Code != http.StatusUnauthorized {
				t.Fatalf("%s via %q must be denied: %d", method, via, response.Code)
			}
		}
	}
	listed, err := services.ListAICredentials(owner.ID)
	if err != nil || len(listed) != 1 || listed[0].ID != credential.ID {
		t.Fatal("denied requests must not mutate credentials")
	}
	for _, name := range []string{"", " \t ", strings.Repeat("界", 101)} {
		body, err := json.Marshal(map[string]string{"name": name})
		if err != nil {
			t.Fatal(err)
		}
		if response := aiCredentialRequest(router, http.MethodPost, "/api/user/ai-credentials", string(body), "", "session"); response.Code != http.StatusBadRequest {
			t.Fatal("invalid names must be rejected")
		}
	}
}

func TestGetAuthIdentityUsesOnlyBearerAndCurrentDatabaseIdentity(t *testing.T) {
	db, router, _, other := setupAICredentialControllerTest(t)
	credential, token, err := services.CreateAICredential(other.ID, "identity")
	if err != nil {
		t.Fatal(err)
	}
	for _, header := range []string{"", other.Token, "Basic " + other.Token, "Bearer invalid", "Bearer " + token + " extra"} {
		response := aiCredentialRequest(router, http.MethodGet, "/api/auth/identity", "", header, "session")
		if response.Code != http.StatusUnauthorized {
			t.Fatal("identity must not use session context/cookies as fallback")
		}
	}
	if _, err := repository.GetUserByID(other.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&other).Update("username", "current-db-username").Error; err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		token string
		via   string
		id    uint
	}{{other.Token, "token", 0}, {token, "ai_token", credential.ID}} {
		response := aiCredentialRequest(router, http.MethodGet, "/api/auth/identity", "", "Bearer "+test.token, "session")
		var identity map[string]any
		if err := json.Unmarshal(aiCredentialResponseData(t, response), &identity); err != nil {
			t.Fatal(err)
		}
		if len(identity) != 4 || identity["user_id"] != float64(other.ID) || identity["username"] != "current-db-username" || identity["auth_via"] != test.via || identity["credential_id"] != float64(test.id) {
			t.Fatalf("unexpected identity fields: %v", identity)
		}
	}
	if err := db.Model(&other).Update("login_issued_at", time.Now().Add(-2*time.Hour)).Error; err != nil {
		t.Fatal(err)
	}
	for _, bearer := range []string{token, other.Token} {
		if response := aiCredentialRequest(router, http.MethodGet, "/api/auth/identity", "", "Bearer "+bearer, "session"); response.Code != http.StatusUnauthorized {
			t.Fatal("identity must enforce expiry for both credential types")
		}
	}
}

func TestAILoginRenewsOnlyOwnerWithoutCookieOrTokenDisclosure(t *testing.T) {
	db, router, owner, other := setupAICredentialControllerTest(t)
	credential, token, err := services.CreateAICredential(owner.ID, "renew")
	if err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-3 * time.Hour).UTC()
	if err := db.Model(&models.User{}).Where("id IN ?", []uint{owner.ID, other.ID}).Update("login_issued_at", old).Error; err != nil {
		t.Fatal(err)
	}
	for _, attempt := range []struct{ bearer, username, password string }{
		{"Bearer " + token, other.Username, "other-password"},
		{"Bearer " + token, owner.Username, "wrong-password"},
		{"Bearer " + owner.Token, owner.Username, "owner-password"},
		{"", owner.Username, "owner-password"},
	} {
		payload, err := json.Marshal(map[string]string{"username": attempt.username, "password": attempt.password})
		if err != nil {
			t.Fatal(err)
		}
		response := aiCredentialRequest(router, http.MethodPost, "/api/ai/login", string(payload), attempt.bearer, "session")
		if response.Code != http.StatusUnauthorized || len(response.Header().Values("Set-Cookie")) != 0 {
			t.Fatal("invalid AI login must fail without setting cookies")
		}
		for _, id := range []uint{owner.ID, other.ID} {
			var user models.User
			if err := db.First(&user, id).Error; err != nil {
				t.Fatal(err)
			}
			if user.LoginIssuedAt == nil || !user.LoginIssuedAt.Equal(old) {
				t.Fatal("rejected login must not refresh either account")
			}
		}
	}
	var auditsBefore int64
	if err := db.Model(&models.SecurityLoginAudit{}).Count(&auditsBefore).Error; err != nil || auditsBefore != 0 {
		t.Fatal("failed renewal must not record successful login")
	}
	payload, err := json.Marshal(map[string]string{"username": owner.Username, "password": "owner-password"})
	if err != nil {
		t.Fatal(err)
	}
	response := aiCredentialRequest(router, http.MethodPost, "/api/ai/login", string(payload), "Bearer "+token, "session")
	var identity map[string]any
	if err := json.Unmarshal(aiCredentialResponseData(t, response), &identity); err != nil {
		t.Fatal(err)
	}
	if len(identity) != 4 || identity["auth_via"] != "ai_token" || identity["credential_id"] != float64(credential.ID) || identity["user_id"] != float64(owner.ID) {
		t.Fatalf("unexpected AI login response: %v", identity)
	}
	if len(response.Header().Values("Set-Cookie")) != 0 || strings.Contains(response.Body.String(), owner.Token) || strings.Contains(response.Body.String(), token) || strings.Contains(response.Body.String(), `"token":`) || strings.Contains(response.Body.String(), `"password":`) {
		t.Fatal("AI login must never set cookies or disclose tokens/passwords")
	}
	var renewed, untouched models.User
	if err := db.First(&renewed, owner.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&untouched, other.ID).Error; err != nil {
		t.Fatal(err)
	}
	if renewed.LoginIssuedAt == nil || !renewed.LoginIssuedAt.After(old) || renewed.Token != owner.Token || !untouched.LoginIssuedAt.Equal(old) || untouched.Token != other.Token {
		t.Fatal("renewal must update only the owner's expiry and preserve ordinary tokens")
	}
	var audits []models.SecurityLoginAudit
	if err := db.Find(&audits).Error; err != nil || len(audits) != 1 || audits[0].UserID != owner.ID || audits[0].AuthVia != "ai_token" || audits[0].Action != "login" {
		t.Fatal("successful renewal must record exactly one ai_token login audit")
	}
	aiCredentialResponseData(t, aiCredentialRequest(router, http.MethodGet, "/api/auth/identity", "", "Bearer "+token, ""))
	if err := services.RevokeAICredential(owner.ID, credential.ID); err != nil {
		t.Fatal(err)
	}
	if response := aiCredentialRequest(router, http.MethodPost, "/api/ai/login", string(payload), "Bearer "+token, "session"); response.Code != http.StatusUnauthorized {
		t.Fatal("revoked AI credentials must not renew login")
	}
}
