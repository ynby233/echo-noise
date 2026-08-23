package services

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/dto"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/vocechat"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

func resetPasswordAlertCleanupRetryMemoryForTest(t *testing.T) {
	t.Helper()
	resolvedPasswordAlertCleanupRetry.Lock()
	resolvedPasswordAlertCleanupRetry.users = make(map[uint]struct{})
	resolvedPasswordAlertCleanupRetry.Unlock()
	t.Cleanup(func() {
		resolvedPasswordAlertCleanupRetry.Lock()
		resolvedPasswordAlertCleanupRetry.users = make(map[uint]struct{})
		resolvedPasswordAlertCleanupRetry.Unlock()
	})
}

func countPasswordAlertCleanupTasks(t *testing.T, userID uint) int64 {
	t.Helper()
	var count int64
	if err := database.DB.Model(&models.PasswordAlertCleanupTask{}).Where("user_id = ?", userID).Count(&count).Error; err != nil {
		t.Fatalf("count password alert cleanup tasks: %v", err)
	}
	return count
}

func TestPendingPasswordAlertCleanupSurvivesProcessRestart(t *testing.T) {
	db := setupUserServiceTestDB(t)
	resetPasswordAlertCleanupRetryMemoryForTest(t)
	mustCreateUser(t, models.User{Username: "primary", Password: models.HashPassword("primary-password"), IsAdmin: true})
	user := mustCreateUser(t, models.User{
		Username:           "restart-cleanup-user",
		Password:           models.HashPassword("healthy-password"),
		VoceChatSyncStatus: models.VoceChatSyncStatusLinked,
	})
	if err := database.DB.Create(&models.UserNotification{
		RecipientUserID: user.ID,
		Type:            models.UserNotificationTypeVoceChatPasswordChanged,
	}).Error; err != nil {
		t.Fatalf("seed resolved password alert: %v", err)
	}

	failNextDelete := true
	if err := db.Callback().Delete().Before("gorm:delete").Register("test:fail-password-alert-delete-before-restart", func(tx *gorm.DB) {
		if !failNextDelete || tx.Statement == nil || tx.Statement.Schema == nil || tx.Statement.Schema.Table != "user_notifications" {
			return
		}
		failNextDelete = false
		tx.AddError(errors.New("forced password alert delete failure before restart"))
	}); err != nil {
		t.Fatalf("register delete fault: %v", err)
	}

	if err := ReconcileResolvedPasswordAlerts(user.ID); err == nil {
		t.Fatal("first reconciliation must report the injected delete failure")
	}
	resetPasswordAlertCleanupRetryMemoryForTest(t)

	if err := ReconcilePendingResolvedPasswordAlerts(user.ID); err != nil {
		t.Fatalf("reconcile after simulated process restart: %v", err)
	}
	var count int64
	if err := database.DB.Model(&models.UserNotification{}).Where("recipient_user_id = ?", user.ID).Count(&count).Error; err != nil {
		t.Fatalf("count notifications after restart reconciliation: %v", err)
	}
	if count != 0 {
		t.Fatalf("process restart lost the pending cleanup state; %d resolved notifications remain", count)
	}
}

func TestSuccessfulLoginReconcilesPersistedPasswordAlertCleanupAfterRestart(t *testing.T) {
	db := setupUserServiceTestDB(t)
	resetPasswordAlertCleanupRetryMemoryForTest(t)
	enableVoceChatLoginForTest(t, false)
	t.Setenv("NOISE_PLAIN_PASSWORD_STORE", filepath.Join(t.TempDir(), "plain-passwords.db"))
	mustCreateUser(t, models.User{Username: "primary", Password: models.HashPassword("primary-password"), IsAdmin: true})
	user := mustCreateUser(t, models.User{
		Username:           "restart-login-cleanup",
		Password:           models.HashPassword("current-password"),
		VoceChatEmail:      "restart-login-cleanup@vc.com",
		VoceChatUserID:     "91",
		VoceChatSyncStatus: models.VoceChatSyncStatusLinked,
	})
	if err := vocechat.DefaultPlainPasswordStore().UpsertUserVoceChatPassword(user.ID, user.Username, "current-password", user.VoceChatEmail, user.VoceChatUserID); err != nil {
		t.Fatalf("seed password record: %v", err)
	}
	if err := database.DB.Create(&models.UserNotification{RecipientUserID: user.ID, Type: models.UserNotificationTypeVoceChatPasswordChanged}).Error; err != nil {
		t.Fatalf("seed resolved password alert: %v", err)
	}
	failNextDelete := true
	if err := db.Callback().Delete().Before("gorm:delete").Register("test:fail-password-alert-delete-before-login-restart", func(tx *gorm.DB) {
		if !failNextDelete || tx.Statement == nil || tx.Statement.Schema == nil || tx.Statement.Schema.Table != "user_notifications" {
			return
		}
		failNextDelete = false
		tx.AddError(errors.New("forced password alert delete failure before login restart"))
	}); err != nil {
		t.Fatalf("register delete fault: %v", err)
	}
	if err := ReconcileResolvedPasswordAlerts(user.ID); err == nil {
		t.Fatal("first reconciliation must report the injected delete failure")
	}
	resetPasswordAlertCleanupRetryMemoryForTest(t)
	stubVoceChatPasswordLogin(t, func(_ context.Context, _ vocechat.Config, _, _ string) (*vocechat.LoginResponse, error) {
		return &vocechat.LoginResponse{User: vocechat.UserInfo{UID: 91, Email: user.VoceChatEmail, Name: user.Username}}, nil
	})

	loggedIn, err := Login(dto.LoginDto{Username: user.Username, Password: "current-password"})
	if err != nil || loggedIn == nil {
		t.Fatalf("login after restart: result present=%v err=%v", loggedIn != nil, err)
	}
	var notificationCount int64
	if err := database.DB.Model(&models.UserNotification{}).Where("recipient_user_id = ?", user.ID).Count(&notificationCount).Error; err != nil {
		t.Fatalf("count notifications after login: %v", err)
	}
	if notificationCount != 0 {
		t.Fatalf("successful login left %d resolved password alerts", notificationCount)
	}
	if taskCount := countPasswordAlertCleanupTasks(t, user.ID); taskCount != 0 {
		t.Fatalf("successful login left %d cleanup tasks", taskCount)
	}
}

func TestSuccessfulPasswordChangeReconcilesPersistedPasswordAlertCleanupAfterRestart(t *testing.T) {
	db := setupUserServiceTestDB(t)
	resetPasswordAlertCleanupRetryMemoryForTest(t)
	enableVoceChatLoginForTest(t, false)
	t.Setenv("NOISE_PLAIN_PASSWORD_STORE", filepath.Join(t.TempDir(), "plain-passwords.db"))
	mustCreateUser(t, models.User{Username: "primary", Password: models.HashPassword("primary-password"), IsAdmin: true})
	user := mustCreateUser(t, models.User{
		Username:           "restart-change-cleanup",
		Password:           models.HashPassword("old-password"),
		VoceChatEmail:      "restart-change-cleanup@vc.com",
		VoceChatUserID:     "92",
		VoceChatSyncStatus: models.VoceChatSyncStatusLinked,
	})
	if err := vocechat.DefaultPlainPasswordStore().UpsertUserVoceChatPassword(user.ID, user.Username, "old-password", user.VoceChatEmail, user.VoceChatUserID); err != nil {
		t.Fatalf("seed password record: %v", err)
	}
	if err := database.DB.Create(&models.UserNotification{RecipientUserID: user.ID, Type: models.UserNotificationTypeVoceChatPasswordChanged}).Error; err != nil {
		t.Fatalf("seed resolved password alert: %v", err)
	}
	failNextDelete := true
	if err := db.Callback().Delete().Before("gorm:delete").Register("test:fail-password-alert-delete-before-change-restart", func(tx *gorm.DB) {
		if !failNextDelete || tx.Statement == nil || tx.Statement.Schema == nil || tx.Statement.Schema.Table != "user_notifications" {
			return
		}
		failNextDelete = false
		tx.AddError(errors.New("forced password alert delete failure before change restart"))
	}); err != nil {
		t.Fatalf("register delete fault: %v", err)
	}
	if err := ReconcileResolvedPasswordAlerts(user.ID); err == nil {
		t.Fatal("first reconciliation must report the injected delete failure")
	}
	resetPasswordAlertCleanupRetryMemoryForTest(t)
	remotePassword := "old-password"
	stubVoceChatAdminUpdateUser(t, func(_ context.Context, _ vocechat.Config, _ int64, request vocechat.UpdateUserRequest) (*vocechat.User, error) {
		if request.Password == nil {
			t.Fatal("password update must include a password")
		}
		remotePassword = *request.Password
		return &vocechat.User{UID: 92, Email: user.VoceChatEmail, Name: user.Username}, nil
	})

	if err := ChangePassword(user, dto.UserInfoDto{Password: "new-password"}); err != nil {
		t.Fatalf("password change after restart: %v", err)
	}
	updated := mustGetUserByUsername(t, user.Username)
	if bcrypt.CompareHashAndPassword([]byte(updated.Password), []byte("new-password")) != nil || remotePassword != "new-password" {
		t.Fatal("password change after restart did not preserve the successful password state")
	}
	var notificationCount int64
	if err := database.DB.Model(&models.UserNotification{}).Where("recipient_user_id = ?", user.ID).Count(&notificationCount).Error; err != nil {
		t.Fatalf("count notifications after password change: %v", err)
	}
	if notificationCount != 0 {
		t.Fatalf("successful password change left %d resolved password alerts", notificationCount)
	}
	if taskCount := countPasswordAlertCleanupTasks(t, user.ID); taskCount != 0 {
		t.Fatalf("successful password change left %d cleanup tasks", taskCount)
	}
}

func TestPendingPasswordAlertCleanupIsIdempotentAcrossConcurrentReads(t *testing.T) {
	setupUserServiceTestDB(t)
	resetPasswordAlertCleanupRetryMemoryForTest(t)
	mustCreateUser(t, models.User{Username: "primary", Password: models.HashPassword("primary-password"), IsAdmin: true})
	user := mustCreateUser(t, models.User{
		Username:           "concurrent-cleanup-user",
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
	if err := setResolvedPasswordAlertCleanupPending(user.ID, true); err != nil {
		t.Fatalf("persist cleanup task: %v", err)
	}
	resetPasswordAlertCleanupRetryMemoryForTest(t)

	const readers = 8
	start := make(chan struct{})
	errorsByReader := make(chan error, readers)
	var wg sync.WaitGroup
	for i := 0; i < readers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errorsByReader <- ReconcilePendingResolvedPasswordAlerts(user.ID)
		}()
	}
	close(start)
	wg.Wait()
	close(errorsByReader)
	for err := range errorsByReader {
		if err != nil {
			t.Fatalf("concurrent cleanup: %v", err)
		}
	}
	if err := ReconcilePendingResolvedPasswordAlerts(user.ID); err != nil {
		t.Fatalf("repeat cleanup after completion: %v", err)
	}
	var notificationCount int64
	if err := database.DB.Model(&models.UserNotification{}).Where("recipient_user_id = ?", user.ID).Count(&notificationCount).Error; err != nil {
		t.Fatalf("count notifications after concurrent cleanup: %v", err)
	}
	if notificationCount != 0 {
		t.Fatalf("concurrent cleanup left %d resolved password alerts", notificationCount)
	}
	if taskCount := countPasswordAlertCleanupTasks(t, user.ID); taskCount != 0 {
		t.Fatalf("concurrent cleanup left %d cleanup tasks", taskCount)
	}
}

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
	if taskCount := countPasswordAlertCleanupTasks(t, user.ID); taskCount != 1 {
		t.Fatalf("failed cleanup tasks = %d, want 1", taskCount)
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
	if taskCount := countPasswordAlertCleanupTasks(t, user.ID); taskCount != 0 {
		t.Fatalf("successful retry left %d cleanup tasks", taskCount)
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
	if taskCount := countPasswordAlertCleanupTasks(t, user.ID); taskCount != 0 {
		t.Fatalf("incomplete state retained %d stale cleanup tasks", taskCount)
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
	if taskCount := countPasswordAlertCleanupTasks(t, user.ID); taskCount != 0 {
		t.Fatalf("healthy reconciliation left %d cleanup tasks", taskCount)
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
	if taskCount := countPasswordAlertCleanupTasks(t, user.ID); taskCount != 1 {
		t.Fatalf("failed cleanup tasks = %d, want 1", taskCount)
	}

	CreateVoceChatPasswordChangedAlertOnce(user.ID)
	if taskCount := countPasswordAlertCleanupTasks(t, user.ID); taskCount != 0 {
		t.Fatalf("new password failure episode retained %d stale cleanup tasks", taskCount)
	}
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

func TestNewIncompletePasswordAlertCancelsPersistedCleanupTask(t *testing.T) {
	setupUserServiceTestDB(t)
	resetPasswordAlertCleanupRetryMemoryForTest(t)
	mustCreateUser(t, models.User{Username: "primary", Password: models.HashPassword("primary-password"), IsAdmin: true})
	user := mustCreateUser(t, models.User{
		Username:           "new-incomplete-password-alert",
		Password:           models.HashPassword("current-password"),
		VoceChatSyncStatus: models.VoceChatSyncStatusLinked,
	})
	if err := setResolvedPasswordAlertCleanupPending(user.ID, true); err != nil {
		t.Fatalf("persist cleanup task: %v", err)
	}
	resetPasswordAlertCleanupRetryMemoryForTest(t)

	if err := CreatePasswordUpdateIncompleteAlertOnce(user.ID); err != nil {
		t.Fatalf("create incomplete-password alert: %v", err)
	}
	if taskCount := countPasswordAlertCleanupTasks(t, user.ID); taskCount != 0 {
		t.Fatalf("new incomplete-password alert retained %d stale cleanup tasks", taskCount)
	}
	if err := ReconcilePendingResolvedPasswordAlerts(user.ID); err != nil {
		t.Fatalf("notification read after incomplete-password alert: %v", err)
	}
	var count int64
	if err := database.DB.Model(&models.UserNotification{}).Where("recipient_user_id = ? AND type = ?", user.ID, models.UserNotificationTypePasswordUpdateIncomplete).Count(&count).Error; err != nil {
		t.Fatalf("count incomplete-password alerts: %v", err)
	}
	if count != 1 {
		t.Fatalf("pending cleanup removed the new incomplete-password alert: got %d, want 1", count)
	}
}

func TestPrimaryAdministratorDoesNotUseManagedPasswordAlertCleanupTasks(t *testing.T) {
	setupUserServiceTestDB(t)
	resetPasswordAlertCleanupRetryMemoryForTest(t)
	primary := mustCreateUser(t, models.User{Username: "primary", Password: models.HashPassword("primary-password"), IsAdmin: true})
	if primary.ID != models.PrimaryAdminUserID {
		t.Fatalf("primary administrator ID = %d, want %d", primary.ID, models.PrimaryAdminUserID)
	}

	CreateVoceChatPasswordChangedAlertOnce(primary.ID)
	if err := CreatePasswordUpdateIncompleteAlertOnce(primary.ID); err != nil {
		t.Fatalf("create primary incomplete-password alert: %v", err)
	}
	if err := ReconcileResolvedPasswordAlerts(primary.ID); err != nil {
		t.Fatalf("reconcile primary password alerts: %v", err)
	}
	if err := ReconcilePendingResolvedPasswordAlerts(primary.ID); err != nil {
		t.Fatalf("reconcile pending primary password alerts: %v", err)
	}
	if taskCount := countPasswordAlertCleanupTasks(t, primary.ID); taskCount != 0 {
		t.Fatalf("primary administrator has %d managed-password cleanup tasks", taskCount)
	}
	var notificationCount int64
	if err := database.DB.Model(&models.UserNotification{}).Where("recipient_user_id = ? AND type IN ?", primary.ID, []string{
		models.UserNotificationTypeVoceChatPasswordChanged,
		models.UserNotificationTypePasswordUpdateIncomplete,
	}).Count(&notificationCount).Error; err != nil {
		t.Fatalf("count primary managed-password alerts: %v", err)
	}
	if notificationCount != 0 {
		t.Fatalf("primary administrator has %d managed-password alerts", notificationCount)
	}
}
