package services

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/dto"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/vocechat"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type injectedLocalPasswordStore struct {
	inner       *vocechat.PlainPasswordStore
	failUpsert  int
	failRestore int
}

func (s *injectedLocalPasswordStore) GetUserPassword(userID uint) (vocechat.PlainPasswordRecord, bool, error) {
	return s.inner.GetUserPassword(userID)
}

func (s *injectedLocalPasswordStore) UpsertUserLocalFallbackPassword(userID uint, username, password, email, voceChatUserID string) error {
	if s.failUpsert > 0 {
		s.failUpsert--
		return errors.New("injected local password material failure")
	}
	return s.inner.UpsertUserLocalFallbackPassword(userID, username, password, email, voceChatUserID)
}

func (s *injectedLocalPasswordStore) RestoreUserPasswordSnapshot(record vocechat.PlainPasswordRecord, existed bool) error {
	if s.failRestore > 0 {
		s.failRestore--
		return errors.New("injected local password rollback failure")
	}
	return s.inner.RestoreUserPasswordSnapshot(record, existed)
}

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

func TestAtomicLocalPasswordChangeRollsBackInjectedMaterialAndVerificationFailures(t *testing.T) {
	for _, tc := range []struct {
		name       string
		failUpsert int
		failVerify bool
	}{
		{name: "password material write", failUpsert: 1},
		{name: "final verification", failVerify: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setupUserServiceTestDB(t)
			createRuntimeModeConfigForTest(t, models.RuntimeModeLocal, "ok")
			storePath := filepath.Join(t.TempDir(), "plain-passwords.db")
			t.Setenv("NOISE_PLAIN_PASSWORD_STORE", storePath)
			mustCreateUser(t, models.User{Username: "primary-local-injected", Password: models.HashPassword("primary"), IsAdmin: true})
			user := mustCreateUser(t, models.User{Username: "local-injected-user", Password: models.HashPassword("old-password"), VoceChatSyncStatus: models.VoceChatSyncStatusPending})
			inner := vocechat.NewPlainPasswordStore(storePath)
			if err := inner.UpsertUserLocalFallbackPassword(user.ID, user.Username, "old-password", "", ""); err != nil {
				t.Fatalf("seed old local password: %v", err)
			}
			store := &injectedLocalPasswordStore{inner: inner, failUpsert: tc.failUpsert}
			verify := localPasswordStateMatches
			if tc.failVerify {
				verify = func(uint, managedPasswordSnapshot, models.User, string, localPasswordMaterialStore) error {
					return errors.New("injected final verification failure")
				}
			}

			_, err := withUserPasswordMutation(user.ID, func(current *models.User) error {
				return changeLocalPasswordAtomicallyWithDependencies(current, "new-password", localPasswordChangeDependencies{store: store, verify: verify})
			})
			var updateFailure *passwordUpdateFailure
			if err == nil || !errors.As(err, &updateFailure) || !updateFailure.rolledBack || updateFailure.incomplete {
				t.Fatalf("injected failure result=%v updateFailure=%#v", err, updateFailure)
			}
			var updated models.User
			if err := database.DB.First(&updated, user.ID).Error; err != nil {
				t.Fatalf("reload rolled-back user: %v", err)
			}
			record, found, err := inner.GetUserPassword(user.ID)
			if err != nil || !found || bcrypt.CompareHashAndPassword([]byte(updated.Password), []byte("old-password")) != nil || updated.VoceChatSyncStatus != models.VoceChatSyncStatusPending || record.LocalFallbackPasswordValue() != "old-password" {
				t.Fatalf("injected failure left partial state: user=%#v record=%#v found=%v err=%v", updated, record, found, err)
			}
		})
	}
}

func TestAtomicLocalBoundPasswordChangeRollsBackSyncStatusWriteFailure(t *testing.T) {
	db := setupUserServiceTestDB(t)
	createRuntimeModeConfigForTest(t, models.RuntimeModeLocal, "ok")
	storePath := filepath.Join(t.TempDir(), "plain-passwords.db")
	t.Setenv("NOISE_PLAIN_PASSWORD_STORE", storePath)
	mustCreateUser(t, models.User{Username: "primary-local-status", Password: models.HashPassword("primary"), IsAdmin: true})
	user := mustCreateUser(t, models.User{
		Username: "bound-local-status", Password: models.HashPassword("old-password"), VoceChatEmail: "bound-status@vc.example", VoceChatUserID: "830", VoceChatSyncStatus: models.VoceChatSyncStatusLinked,
	})
	store := vocechat.NewPlainPasswordStore(storePath)
	if err := store.UpsertUserVoceChatPassword(user.ID, user.Username, "old-password", user.VoceChatEmail, user.VoceChatUserID); err != nil {
		t.Fatalf("seed remote password: %v", err)
	}
	if err := store.UpsertUserLocalFallbackPassword(user.ID, user.Username, "old-password", user.VoceChatEmail, user.VoceChatUserID); err != nil {
		t.Fatalf("seed local password: %v", err)
	}
	faultInjected := false
	if err := db.Callback().Update().Before("gorm:update").Register("test:fail-local-sync-status-once", func(tx *gorm.DB) {
		if faultInjected || tx.Statement == nil || tx.Statement.Schema == nil || tx.Statement.Schema.Table != "users" {
			return
		}
		updates, ok := tx.Statement.Dest.(map[string]interface{})
		if !ok || updates["voce_chat_sync_status"] != models.VoceChatSyncStatusPasswordSyncRequired {
			return
		}
		faultInjected = true
		tx.AddError(errors.New("injected sync status write failure"))
	}); err != nil {
		t.Fatalf("register status failure: %v", err)
	}

	err := ChangePassword(user, dto.UserInfoDto{Password: "new-password"})
	if err == nil || !faultInjected {
		t.Fatalf("status failure result=%v injected=%v", err, faultInjected)
	}
	var updated models.User
	if err := db.First(&updated, user.ID).Error; err != nil {
		t.Fatalf("reload user: %v", err)
	}
	record, found, err := store.GetUserPassword(user.ID)
	if err != nil || !found || bcrypt.CompareHashAndPassword([]byte(updated.Password), []byte("old-password")) != nil || updated.VoceChatSyncStatus != models.VoceChatSyncStatusLinked || record.VoceChatPasswordValue() != "old-password" || record.LocalFallbackPasswordValue() != "old-password" {
		t.Fatalf("status failure left partial state: user=%#v record=%#v found=%v err=%v", updated, record, found, err)
	}
}

func TestAtomicLocalPasswordRollbackFailureMarksIncompleteOnlyOnce(t *testing.T) {
	setupUserServiceTestDB(t)
	createRuntimeModeConfigForTest(t, models.RuntimeModeLocal, "ok")
	storePath := filepath.Join(t.TempDir(), "plain-passwords.db")
	t.Setenv("NOISE_PLAIN_PASSWORD_STORE", storePath)
	mustCreateUser(t, models.User{Username: "primary-local-incomplete", Password: models.HashPassword("primary"), IsAdmin: true})
	user := mustCreateUser(t, models.User{Username: "local-incomplete-user", Password: models.HashPassword("old-password"), VoceChatSyncStatus: models.VoceChatSyncStatusPending})
	inner := vocechat.NewPlainPasswordStore(storePath)
	if err := inner.UpsertUserLocalFallbackPassword(user.ID, user.Username, "old-password", "", ""); err != nil {
		t.Fatalf("seed local password: %v", err)
	}
	store := &injectedLocalPasswordStore{inner: inner, failUpsert: 2, failRestore: 2}

	for attempt := 0; attempt < 2; attempt++ {
		_, err := withUserPasswordMutation(user.ID, func(current *models.User) error {
			return changeLocalPasswordAtomicallyWithDependencies(current, "new-password", localPasswordChangeDependencies{store: store, verify: localPasswordStateMatches})
		})
		var updateFailure *passwordUpdateFailure
		if err == nil || !errors.As(err, &updateFailure) || !updateFailure.incomplete {
			t.Fatalf("attempt %d result=%v updateFailure=%#v", attempt+1, err, updateFailure)
		}
		if got := PasswordChangePublicFailureMessage(err, true); got != "密码保存未完成，请重新为该用户设置密码。" {
			t.Fatalf("attempt %d public message=%q", attempt+1, got)
		}
	}
	var updated models.User
	if err := database.DB.First(&updated, user.ID).Error; err != nil {
		t.Fatalf("reload incomplete user: %v", err)
	}
	if updated.VoceChatSyncStatus != models.VoceChatSyncStatusConflicted || updated.VoceChatSyncError != "password_update_incomplete" {
		t.Fatalf("incomplete status=%#v", updated)
	}
	var notificationCount int64
	if err := database.DB.Model(&models.UserNotification{}).Where("recipient_user_id = ? AND type = ?", user.ID, models.UserNotificationTypePasswordUpdateIncomplete).Count(&notificationCount).Error; err != nil {
		t.Fatalf("count incomplete notifications: %v", err)
	}
	if notificationCount != 1 {
		t.Fatalf("incomplete notification count=%d, want 1", notificationCount)
	}
}
