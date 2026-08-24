package services

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/dto"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/vocechat"
	"golang.org/x/crypto/bcrypt"
)

func TestUnboundPasswordChangeUsesAtomicLocalPolicyInEveryRuntimeState(t *testing.T) {
	tests := []struct {
		name         string
		mode         string
		health       string
		initialState string
	}{
		{name: "local pending", mode: models.RuntimeModeLocal, health: "ok", initialState: models.VoceChatSyncStatusPending},
		{name: "VoceChat normal unbound", mode: models.RuntimeModeVoceChat, health: "ok", initialState: models.VoceChatSyncStatusUnbound},
		{name: "VoceChat degraded pending", mode: models.RuntimeModeVoceChat, health: "failed", initialState: models.VoceChatSyncStatusPending},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			setupUserServiceTestDB(t)
			createRuntimeModeConfigForTest(t, tc.mode, tc.health)
			storePath := filepath.Join(t.TempDir(), "plain-passwords.db")
			t.Setenv("NOISE_PLAIN_PASSWORD_STORE", storePath)
			mustCreateUser(t, models.User{Username: "primary-local-policy", Password: models.HashPassword("primary"), IsAdmin: true})
			user := mustCreateUser(t, models.User{
				Username:           "unbound-local-policy",
				Password:           models.HashPassword("old-password"),
				VoceChatSyncStatus: tc.initialState,
			})
			stubVoceChatAdminUpdateUser(t, func(context.Context, vocechat.Config, int64, vocechat.UpdateUserRequest) (*vocechat.User, error) {
				t.Fatal("unbound password change must not call VoceChat")
				return nil, nil
			})

			if err := ChangePassword(user, dto.UserInfoDto{Password: "new-password"}); err != nil {
				t.Fatalf("change unbound password: %v", err)
			}
			var updated models.User
			if err := database.DB.First(&updated, user.ID).Error; err != nil {
				t.Fatalf("reload user: %v", err)
			}
			if bcrypt.CompareHashAndPassword([]byte(updated.Password), []byte("new-password")) != nil {
				t.Fatal("new local password was not committed")
			}
			if updated.VoceChatSyncStatus != tc.initialState {
				t.Fatalf("unbound sync status=%q, want %q", updated.VoceChatSyncStatus, tc.initialState)
			}
			record, found, err := vocechat.NewPlainPasswordStore(storePath).GetUserPassword(user.ID)
			if err != nil || !found || record.LocalFallbackPasswordValue() != "new-password" || record.VoceChatPasswordValue() != "" {
				t.Fatalf("local password material found=%v err=%v record=%#v", found, err, record)
			}
		})
	}
}

func TestUnboundPasswordChangeRollsBackPrimaryStateWhenLocalMaterialWriteFails(t *testing.T) {
	setupUserServiceTestDB(t)
	createRuntimeModeConfigForTest(t, models.RuntimeModeLocal, "ok")
	storePath := filepath.Join(t.TempDir(), "plain-passwords.db")
	t.Setenv("NOISE_PLAIN_PASSWORD_STORE", storePath)
	mustCreateUser(t, models.User{Username: "primary-local-rollback", Password: models.HashPassword("primary"), IsAdmin: true})
	user := mustCreateUser(t, models.User{
		Username:           "unbound-local-rollback",
		Password:           models.HashPassword("old-password"),
		VoceChatSyncStatus: models.VoceChatSyncStatusPending,
	})
	if err := os.Mkdir(storePath, 0700); err != nil {
		t.Fatalf("create unavailable password-store path: %v", err)
	}

	err := ChangePassword(user, dto.UserInfoDto{Password: "new-password"})
	if err == nil {
		t.Fatal("local password material failure must be reported")
	}
	var updated models.User
	if err := database.DB.First(&updated, user.ID).Error; err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if bcrypt.CompareHashAndPassword([]byte(updated.Password), []byte("old-password")) != nil {
		t.Fatal("failed unbound password change modified the primary password hash")
	}
	if updated.VoceChatSyncStatus != models.VoceChatSyncStatusPending || updated.VoceChatSyncError != "" {
		t.Fatalf("failed unbound password change modified sync state: %#v", updated)
	}
	var incompleteAlerts int64
	if err := database.DB.Model(&models.UserNotification{}).
		Where("recipient_user_id = ? AND type = ?", user.ID, models.UserNotificationTypePasswordUpdateIncomplete).
		Count(&incompleteAlerts).Error; err != nil {
		t.Fatalf("count incomplete alerts: %v", err)
	}
	if incompleteAlerts != 0 {
		t.Fatalf("preflight failure created %d incomplete alerts", incompleteAlerts)
	}
}
