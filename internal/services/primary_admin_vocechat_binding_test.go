package services

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/repository"
	"github.com/rcy1314/echo-noise/internal/vocechat"
)

func setupPrimaryAdminVoceChatBindingTest(t *testing.T, users []map[string]interface{}) (*models.User, string) {
	t.Helper()

	db := setupUserServiceTestDB(t)
	primary := mustCreateUser(t, models.User{
		Username: "primary-binding",
		Password: models.HashPassword("local-password"),
		IsAdmin:  true,
		Token:    models.GenerateToken(32),
	})
	if primary.ID != models.PrimaryAdminUserID {
		t.Fatalf("primary id = %d, want %d", primary.ID, models.PrimaryAdminUserID)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/token/login" {
			var request struct {
				Credential map[string]string `json:"credential"`
			}
			_ = json.NewDecoder(r.Body).Decode(&request)
			for _, user := range users {
				if strings.EqualFold(strings.TrimSpace(fmt.Sprint(user["email"])), request.Credential["email"]) && request.Credential["password"] == "vc-password" {
					_ = json.NewEncoder(w).Encode(map[string]interface{}{"token": "personal-token", "user": user})
					return
				}
			}
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.Method != http.MethodGet || r.URL.Path != "/api/admin/user" {
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("X-API-Key"); got != "management-api-token" {
			t.Fatalf("management token = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(users)
	}))
	t.Cleanup(server.Close)

	if err := db.Create(&models.SiteConfig{
		VoceChatEnabled:       true,
		VoceChatBaseURL:       server.URL,
		VoceChatAdminUsername: "management-account@vc.test",
		VoceChatAdminToken:    "management-api-token",
	}).Error; err != nil {
		t.Fatalf("create vocechat config: %v", err)
	}
	return primary, server.URL
}

func TestBindPrimaryAdminVoceChatEmailBindsExistingRemoteAccountIndependentlyFromManagementEmail(t *testing.T) {
	primary, _ := setupPrimaryAdminVoceChatBindingTest(t, []map[string]interface{}{
		{"uid": 81, "email": "owner@vc.test", "name": "Owner VC", "is_admin": false},
	})
	CreatePrimaryAdminVoceChatCredentialAlertOnce()

	bound, err := BindPrimaryAdminVoceChatEmail(context.Background(), primary.ID, " OWNER@VC.TEST ", "vc-password")
	if err != nil {
		t.Fatalf("bind primary admin vocechat email: %v", err)
	}
	if bound.ID != primary.ID || bound.VoceChatEmail != "owner@vc.test" || bound.VoceChatUserID != "81" || bound.VoceChatUsername != "Owner VC" || bound.VoceChatSyncStatus != models.VoceChatSyncStatusLinked {
		t.Fatalf("unexpected bound primary: %#v", bound)
	}

	stored, err := repository.GetUserByID(primary.ID)
	if err != nil {
		t.Fatalf("reload primary admin: %v", err)
	}
	if stored.VoceChatEmail != "owner@vc.test" || stored.VoceChatUserID != "81" || stored.VoceChatSyncStatus != models.VoceChatSyncStatusLinked {
		t.Fatalf("stored binding = email %q uid %q status %q", stored.VoceChatEmail, stored.VoceChatUserID, stored.VoceChatSyncStatus)
	}
	var alerts int64
	if err := database.DB.Model(&models.UserNotification{}).Where("recipient_user_id = ? AND type = ?", primary.ID, models.UserNotificationTypeVoceChatCredentials).Count(&alerts).Error; err != nil || alerts != 0 {
		t.Fatalf("successful binding should resolve credential alert: count=%d err=%v", alerts, err)
	}
	CreatePrimaryAdminVoceChatCredentialAlertOnce()
	if err := database.DB.Model(&models.UserNotification{}).Where("recipient_user_id = ? AND type = ?", primary.ID, models.UserNotificationTypeVoceChatCredentials).Count(&alerts).Error; err != nil || alerts != 1 {
		t.Fatalf("a later invalid episode should alert again: count=%d err=%v", alerts, err)
	}
}

func TestBindPrimaryAdminVoceChatEmailRejectsAccountAlreadyBoundToAnotherLocalUser(t *testing.T) {
	primary, _ := setupPrimaryAdminVoceChatBindingTest(t, []map[string]interface{}{
		{"uid": 82, "email": "occupied@vc.test", "name": "Occupied VC", "is_admin": false},
	})
	mustCreateUser(t, models.User{
		Username:       "existing-owner",
		Password:       models.HashPassword("password"),
		VoceChatEmail:  "another-address@vc.test",
		VoceChatUserID: "82",
	})

	if _, err := BindPrimaryAdminVoceChatEmail(context.Background(), primary.ID, "occupied@vc.test", "vc-password"); err == nil || !strings.Contains(err.Error(), "已绑定") {
		t.Fatalf("expected already-bound rejection, got %v", err)
	}
	stored, err := repository.GetUserByID(primary.ID)
	if err != nil {
		t.Fatalf("reload primary admin: %v", err)
	}
	if stored.VoceChatEmail != "" || stored.VoceChatUserID != "" {
		t.Fatalf("rejected binding mutated primary: email %q uid %q", stored.VoceChatEmail, stored.VoceChatUserID)
	}
}

func TestBindPrimaryAdminVoceChatEmailRejectsAccountReservedByRegistration(t *testing.T) {
	primary, _ := setupPrimaryAdminVoceChatBindingTest(t, []map[string]interface{}{
		{"uid": 83, "email": "reserved@vc.test", "name": "Reserved VC", "is_admin": false},
	})
	if err := database.DB.Create(&models.RegistrationApplication{
		ApplicationID:      "reserved-application",
		Username:           "future-user",
		PasswordHash:       models.HashPassword("password"),
		Status:             models.RegistrationApplicationStatusPending,
		VoceChatEmail:      "reserved@vc.test",
		VoceChatUserID:     "83",
		VoceChatSyncStatus: models.VoceChatSyncStatusCreated,
	}).Error; err != nil {
		t.Fatalf("create registration reservation: %v", err)
	}

	if _, err := BindPrimaryAdminVoceChatEmail(context.Background(), primary.ID, "reserved@vc.test", "vc-password"); err == nil || !strings.Contains(err.Error(), "注册申请") {
		t.Fatalf("expected registration-reservation rejection, got %v", err)
	}
}

func TestBindPrimaryAdminVoceChatEmailRejectsMissingRemoteAccountAndNonPrimaryActor(t *testing.T) {
	primary, _ := setupPrimaryAdminVoceChatBindingTest(t, []map[string]interface{}{})
	delegated := mustCreateUser(t, models.User{Username: "delegated-binding", Password: models.HashPassword("password"), IsAdmin: true})

	if _, err := BindPrimaryAdminVoceChatEmail(context.Background(), primary.ID, "missing@vc.test", "vc-password"); err == nil || !strings.Contains(err.Error(), "错误") {
		t.Fatalf("expected missing-account rejection, got %v", err)
	}
	if _, err := BindPrimaryAdminVoceChatEmail(context.Background(), delegated.ID, "missing@vc.test", "vc-password"); err == nil || !strings.Contains(err.Error(), "1号管理员") {
		t.Fatalf("expected non-primary rejection, got %v", err)
	}
}

func TestBoundPrimaryAdminPasswordRemainsLocalAndIndependentFromVoceChat(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "primary-passwords.db")
	t.Setenv("NOISE_PLAIN_PASSWORD_STORE", storePath)
	primary, _ := setupPrimaryAdminVoceChatBindingTest(t, []map[string]interface{}{
		{"uid": 84, "email": "independent@vc.test", "name": "Independent VC", "is_admin": false},
	})
	bound, err := BindPrimaryAdminVoceChatEmail(context.Background(), primary.ID, "independent@vc.test", "vc-password")
	if err != nil {
		t.Fatalf("bind primary admin: %v", err)
	}
	store := vocechat.NewPlainPasswordStore(storePath)
	if err := store.UpsertUserVoceChatPassword(bound.ID, bound.Username, "independent-vc-password", bound.VoceChatEmail, bound.VoceChatUserID); err != nil {
		t.Fatalf("seed independent vocechat password: %v", err)
	}

	if err := ChangePasswordWithOld(bound, "local-password", "new-local-password"); err != nil {
		t.Fatalf("change primary local password: %v", err)
	}
	stored, err := repository.GetUserByID(primary.ID)
	if err != nil {
		t.Fatalf("reload primary admin: %v", err)
	}
	if !passwordMatchesStored(stored.Password, "new-local-password") {
		t.Fatal("primary local password was not updated")
	}
	if stored.VoceChatEmail != "independent@vc.test" || stored.VoceChatUserID != "84" {
		t.Fatalf("password change altered vocechat binding: email %q uid %q", stored.VoceChatEmail, stored.VoceChatUserID)
	}
	record, ok, err := store.GetUserPassword(primary.ID)
	if err != nil || !ok {
		t.Fatalf("read independent passwords: found %v err %v", ok, err)
	}
	if record.VoceChatPassword != "independent-vc-password" || record.LocalFallbackPassword != "new-local-password" {
		t.Fatalf("password independence lost: vc=%q local=%q", record.VoceChatPassword, record.LocalFallbackPassword)
	}
}

func TestBindPrimaryAdminVoceChatEmailRejectsWhenVoceChatIsUnavailable(t *testing.T) {
	primary, _ := setupPrimaryAdminVoceChatBindingTest(t, []map[string]interface{}{
		{"uid": 85, "email": "unavailable@vc.test", "name": "Unavailable VC", "is_admin": false},
	})
	if err := database.DB.Model(&models.SiteConfig{}).Where("1 = 1").Update("voce_chat_enabled", false).Error; err != nil {
		t.Fatalf("disable vocechat: %v", err)
	}

	if _, err := BindPrimaryAdminVoceChatEmail(context.Background(), primary.ID, "unavailable@vc.test", "vc-password"); err == nil || !strings.Contains(err.Error(), "未启用") {
		t.Fatalf("expected unavailable rejection, got %v", err)
	}
}
