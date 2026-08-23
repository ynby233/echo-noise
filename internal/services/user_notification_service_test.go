package services

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/dto"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/vocechat"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

func TestReconcileResolvedPasswordAlertsRetriesCleanupAfterDeleteFailure(t *testing.T) {
	db := setupUserServiceTestDB(t)
	mustCreateUser(t, models.User{Username: "primary", Password: models.HashPassword("primary-password"), IsAdmin: true})
	user := mustCreateUser(t, models.User{
		Username:           "resolved-password-alerts",
		Password:           models.HashPassword("healthy-password"),
		VoceChatSyncStatus: models.VoceChatSyncStatusLinked,
	})
	for _, notificationType := range []string{
		models.UserNotificationTypeVoceChatPasswordChanged,
		models.UserNotificationTypePasswordUpdateIncomplete,
	} {
		if err := database.DB.Create(&models.UserNotification{RecipientUserID: user.ID, Type: notificationType}).Error; err != nil {
			t.Fatalf("seed %s notification: %v", notificationType, err)
		}
	}

	failNextDelete := true
	deleteFailed := false
	if err := db.Callback().Delete().Before("gorm:delete").Register("test:fail-password-alert-delete", func(tx *gorm.DB) {
		if !failNextDelete || tx.Statement == nil || tx.Statement.Schema == nil || tx.Statement.Schema.Table != "user_notifications" {
			return
		}
		failNextDelete = false
		deleteFailed = true
		tx.AddError(errors.New("forced password alert delete failure"))
	}); err != nil {
		t.Fatalf("register delete fault: %v", err)
	}

	if err := ReconcileResolvedPasswordAlerts(user.ID); err == nil {
		t.Fatal("first reconciliation must report the injected delete failure")
	}
	if !deleteFailed {
		t.Fatal("password alert delete fault was not injected")
	}
	var count int64
	if err := database.DB.Model(&models.UserNotification{}).Where("recipient_user_id = ?", user.ID).Count(&count).Error; err != nil {
		t.Fatalf("count notifications after failed reconciliation: %v", err)
	}
	if count != 2 {
		t.Fatalf("failed reconciliation removed notifications: got %d, want 2", count)
	}

	if err := ReconcilePendingResolvedPasswordAlerts(user.ID); err != nil {
		t.Fatalf("retry reconciliation: %v", err)
	}
	if err := database.DB.Model(&models.UserNotification{}).Where("recipient_user_id = ?", user.ID).Count(&count).Error; err != nil {
		t.Fatalf("count notifications after retry: %v", err)
	}
	if count != 0 {
		t.Fatalf("retry reconciliation left %d resolved notifications", count)
	}
}

func TestSuccessfulPasswordChangeDoesNotRollbackWhenAlertCleanupFails(t *testing.T) {
	db := setupUserServiceTestDB(t)
	enableVoceChatLoginForTest(t, false)
	t.Setenv("NOISE_PLAIN_PASSWORD_STORE", filepath.Join(t.TempDir(), "plain-passwords.db"))
	mustCreateUser(t, models.User{Username: "primary", Password: models.HashPassword("primary-password"), IsAdmin: true})
	user := mustCreateUser(t, models.User{
		Username:           "cleanup-failure-user",
		Password:           models.HashPassword("old-password"),
		VoceChatEmail:      "cleanup-failure@vc.com",
		VoceChatUserID:     "76",
		VoceChatSyncStatus: models.VoceChatSyncStatusLinked,
	})
	if err := vocechat.DefaultPlainPasswordStore().UpsertUserVoceChatPassword(user.ID, user.Username, "old-password", user.VoceChatEmail, user.VoceChatUserID); err != nil {
		t.Fatalf("seed password record: %v", err)
	}
	for _, notificationType := range []string{
		models.UserNotificationTypeVoceChatPasswordChanged,
		models.UserNotificationTypePasswordUpdateIncomplete,
	} {
		if err := database.DB.Create(&models.UserNotification{RecipientUserID: user.ID, Type: notificationType}).Error; err != nil {
			t.Fatalf("seed %s notification: %v", notificationType, err)
		}
	}
	remotePassword := "old-password"
	stubVoceChatAdminUpdateUser(t, func(_ context.Context, _ vocechat.Config, _ int64, request vocechat.UpdateUserRequest) (*vocechat.User, error) {
		if request.Password == nil {
			t.Fatal("password update must include a password")
		}
		remotePassword = *request.Password
		return &vocechat.User{UID: 76, Email: user.VoceChatEmail, Name: user.Username}, nil
	})
	failNextDelete := true
	if err := db.Callback().Delete().Before("gorm:delete").Register("test:fail-success-cleanup-delete", func(tx *gorm.DB) {
		if !failNextDelete || tx.Statement == nil || tx.Statement.Schema == nil || tx.Statement.Schema.Table != "user_notifications" {
			return
		}
		failNextDelete = false
		tx.AddError(errors.New("forced cleanup failure after successful password change"))
	}); err != nil {
		t.Fatalf("register cleanup fault: %v", err)
	}

	if err := ChangePassword(user, dto.UserInfoDto{Password: "new-password"}); err != nil {
		t.Fatalf("notification cleanup failure must not fail password change: %v", err)
	}
	updated := mustGetUserByUsername(t, user.Username)
	if bcrypt.CompareHashAndPassword([]byte(updated.Password), []byte("new-password")) != nil || remotePassword != "new-password" {
		t.Fatal("notification cleanup failure rolled back a successful password change")
	}
	record, found, err := vocechat.DefaultPlainPasswordStore().GetUserPassword(user.ID)
	if err != nil || !found || record.VoceChatPassword != "new-password" {
		t.Fatalf("successful password state was not retained: found=%v err=%v record=%#v", found, err, record)
	}
	var count int64
	if err := database.DB.Model(&models.UserNotification{}).Where("recipient_user_id = ?", user.ID).Count(&count).Error; err != nil {
		t.Fatalf("count notifications after failed cleanup: %v", err)
	}
	if count != 2 {
		t.Fatalf("failed cleanup duplicated or partially removed notifications: got %d, want 2", count)
	}
	if err := ReconcilePendingResolvedPasswordAlerts(user.ID); err != nil {
		t.Fatalf("retry password alert cleanup: %v", err)
	}
	if err := database.DB.Model(&models.UserNotification{}).Where("recipient_user_id = ?", user.ID).Count(&count).Error; err != nil {
		t.Fatalf("count notifications after retry: %v", err)
	}
	if count != 0 {
		t.Fatalf("retry left %d resolved password alerts", count)
	}
}

func TestReconcileResolvedPasswordAlertsKeepsIncompleteAlertUntilStateIsHealthy(t *testing.T) {
	setupUserServiceTestDB(t)
	mustCreateUser(t, models.User{Username: "primary", Password: models.HashPassword("primary-password"), IsAdmin: true})
	user := mustCreateUser(t, models.User{
		Username:           "incomplete-alert-user",
		Password:           models.HashPassword("current-password"),
		VoceChatSyncStatus: models.VoceChatSyncStatusConflicted,
		VoceChatSyncError:  "password_update_incomplete",
	})
	for _, notificationType := range []string{
		models.UserNotificationTypeVoceChatPasswordChanged,
		models.UserNotificationTypePasswordUpdateIncomplete,
	} {
		if err := database.DB.Create(&models.UserNotification{RecipientUserID: user.ID, Type: notificationType}).Error; err != nil {
			t.Fatalf("seed %s notification: %v", notificationType, err)
		}
	}

	if err := ReconcileResolvedPasswordAlerts(user.ID); err != nil {
		t.Fatalf("reconcile incomplete state: %v", err)
	}
	var count int64
	if err := database.DB.Model(&models.UserNotification{}).Where("recipient_user_id = ?", user.ID).Count(&count).Error; err != nil {
		t.Fatalf("count notifications while incomplete: %v", err)
	}
	if count != 2 {
		t.Fatalf("incomplete state lost a real password alert: got %d, want 2", count)
	}
	if err := database.DB.Model(&models.User{}).Where("id = ?", user.ID).Updates(map[string]interface{}{
		"voce_chat_sync_status": models.VoceChatSyncStatusLinked,
		"voce_chat_sync_error":  "",
	}).Error; err != nil {
		t.Fatalf("mark account healthy: %v", err)
	}
	if err := ReconcileResolvedPasswordAlerts(user.ID); err != nil {
		t.Fatalf("reconcile healthy state: %v", err)
	}
	if err := database.DB.Model(&models.UserNotification{}).Where("recipient_user_id = ?", user.ID).Count(&count).Error; err != nil {
		t.Fatalf("count notifications after recovery: %v", err)
	}
	if count != 0 {
		t.Fatalf("healthy state left %d resolved password alerts", count)
	}
}

func TestPendingPasswordAlertReconciliationDoesNotDeleteRealUnresolvedAlert(t *testing.T) {
	setupUserServiceTestDB(t)
	mustCreateUser(t, models.User{Username: "primary", Password: models.HashPassword("primary-password"), IsAdmin: true})
	user := mustCreateUser(t, models.User{
		Username:           "real-password-alert",
		Password:           models.HashPassword("current-password"),
		VoceChatSyncStatus: models.VoceChatSyncStatusLinked,
	})
	if err := database.DB.Create(&models.UserNotification{RecipientUserID: user.ID, Type: models.UserNotificationTypeVoceChatPasswordChanged}).Error; err != nil {
		t.Fatalf("seed real password-change notification: %v", err)
	}

	if err := ReconcilePendingResolvedPasswordAlerts(user.ID); err != nil {
		t.Fatalf("reconcile without a pending cleanup: %v", err)
	}
	var count int64
	if err := database.DB.Model(&models.UserNotification{}).Where("recipient_user_id = ? AND type = ?", user.ID, models.UserNotificationTypeVoceChatPasswordChanged).Count(&count).Error; err != nil {
		t.Fatalf("count real password-change notification: %v", err)
	}
	if count != 1 {
		t.Fatalf("notification read retry removed a real unresolved alert: got %d, want 1", count)
	}
}

func TestNewPasswordFailureEpisodeCancelsPendingAlertCleanup(t *testing.T) {
	db := setupUserServiceTestDB(t)
	mustCreateUser(t, models.User{Username: "primary", Password: models.HashPassword("primary-password"), IsAdmin: true})
	user := mustCreateUser(t, models.User{
		Username:           "new-password-failure-episode",
		Password:           models.HashPassword("current-password"),
		VoceChatSyncStatus: models.VoceChatSyncStatusLinked,
	})
	if err := database.DB.Create(&models.UserNotification{RecipientUserID: user.ID, Type: models.UserNotificationTypeVoceChatPasswordChanged}).Error; err != nil {
		t.Fatalf("seed resolved password-change notification: %v", err)
	}
	failNextDelete := true
	if err := db.Callback().Delete().Before("gorm:delete").Register("test:fail-cleanup-before-new-episode", func(tx *gorm.DB) {
		if !failNextDelete || tx.Statement == nil || tx.Statement.Schema == nil || tx.Statement.Schema.Table != "user_notifications" {
			return
		}
		failNextDelete = false
		tx.AddError(errors.New("forced cleanup failure before a new password episode"))
	}); err != nil {
		t.Fatalf("register cleanup fault: %v", err)
	}
	if err := ReconcileResolvedPasswordAlerts(user.ID); err == nil {
		t.Fatal("initial cleanup must report the injected failure")
	}

	CreateVoceChatPasswordChangedAlertOnce(user.ID)
	if err := ReconcilePendingResolvedPasswordAlerts(user.ID); err != nil {
		t.Fatalf("notification read after a new failure episode: %v", err)
	}
	var count int64
	if err := database.DB.Model(&models.UserNotification{}).Where("recipient_user_id = ? AND type = ?", user.ID, models.UserNotificationTypeVoceChatPasswordChanged).Count(&count).Error; err != nil {
		t.Fatalf("count password-change notifications: %v", err)
	}
	if count != 1 {
		t.Fatalf("pending cleanup removed the new unresolved episode: got %d, want 1", count)
	}
}
