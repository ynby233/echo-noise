package services

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/dto"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/vocechat"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

func TestLoginWithVoceChatMetadataWriteFailureRejectsLogin(t *testing.T) {
	db := setupUserServiceTestDB(t)
	enableVoceChatLoginForTest(t, false)
	t.Setenv("NOISE_PLAIN_PASSWORD_STORE", filepath.Join(t.TempDir(), "plain-passwords.db"))
	mustCreateUser(t, models.User{Username: "primary", Password: models.HashPassword("primary-password"), IsAdmin: true})
	user := mustCreateUser(t, models.User{
		Username:           "metadata-write-failure",
		Password:           models.HashPassword("old-password"),
		VoceChatEmail:      "metadata-write-failure@vc.com",
		VoceChatUserID:     "70",
		VoceChatSyncStatus: models.VoceChatSyncStatusFailed,
	})
	if err := vocechat.DefaultPlainPasswordStore().UpsertUserVoceChatPassword(user.ID, user.Username, "old-password", user.VoceChatEmail, user.VoceChatUserID); err != nil {
		t.Fatalf("seed password record: %v", err)
	}
	stubVoceChatPasswordLogin(t, func(_ context.Context, _ vocechat.Config, _, _ string) (*vocechat.LoginResponse, error) {
		return &vocechat.LoginResponse{User: vocechat.UserInfo{UID: 70, Email: user.VoceChatEmail, Name: user.Username}}, nil
	})
	failed := false
	if err := db.Callback().Update().Before("gorm:update").Register("test:fail-vc-metadata-write", func(tx *gorm.DB) {
		if failed || tx.Statement == nil || tx.Statement.Schema == nil || tx.Statement.Schema.Table != "users" {
			return
		}
		updates, ok := tx.Statement.Dest.(map[string]interface{})
		if _, present := updates["voce_chat_sync_status"]; ok && present {
			failed = true
			tx.AddError(errors.New("forced vocechat metadata write failure"))
		}
	}); err != nil {
		t.Fatalf("register fault: %v", err)
	}

	loggedIn, err := Login(dto.LoginDto{Username: user.Username, Password: "new-password"})
	if !failed {
		t.Fatal("metadata fault was not injected")
	}
	if err == nil || loggedIn != nil {
		t.Fatalf("metadata persistence failure must reject login, result present=%v err=%v", loggedIn != nil, err)
	}
}

func TestVoceChatLoginWaitsForSameUserPasswordMutation(t *testing.T) {
	setupUserServiceTestDB(t)
	enableVoceChatLoginForTest(t, false)
	t.Setenv("NOISE_PLAIN_PASSWORD_STORE", filepath.Join(t.TempDir(), "plain-passwords.db"))
	mustCreateUser(t, models.User{Username: "primary", Password: models.HashPassword("primary-password"), IsAdmin: true})
	user := mustCreateUser(t, models.User{
		Username:       "lock-user",
		Password:       models.HashPassword("old-password"),
		VoceChatEmail:  "lock-user@vc.com",
		VoceChatUserID: "71",
	})
	if err := vocechat.DefaultPlainPasswordStore().UpsertUserVoceChatPassword(user.ID, user.Username, "old-password", user.VoceChatEmail, user.VoceChatUserID); err != nil {
		t.Fatalf("seed password record: %v", err)
	}

	updateStarted := make(chan struct{})
	releaseUpdate := make(chan struct{})
	loginReachedRemote := make(chan struct{})
	stubVoceChatAdminUpdateUser(t, func(_ context.Context, _ vocechat.Config, _ int64, _ vocechat.UpdateUserRequest) (*vocechat.User, error) {
		close(updateStarted)
		<-releaseUpdate
		return &vocechat.User{UID: 71, Email: user.VoceChatEmail, Name: user.Username}, nil
	})
	stubVoceChatPasswordLogin(t, func(_ context.Context, _ vocechat.Config, _, _ string) (*vocechat.LoginResponse, error) {
		close(loginReachedRemote)
		return &vocechat.LoginResponse{User: vocechat.UserInfo{UID: 71, Email: user.VoceChatEmail, Name: user.Username}}, nil
	})

	changeDone := make(chan error, 1)
	loginDone := make(chan error, 1)
	go func() {
		changeDone <- ChangePassword(user, dto.UserInfoDto{Password: "new-password"})
	}()
	<-updateStarted
	go func() {
		_, err := Login(dto.LoginDto{Username: user.Username, Password: "old-password"})
		loginDone <- err
	}()

	early := false
	select {
	case <-loginReachedRemote:
		early = true
	case <-time.After(150 * time.Millisecond):
	}
	close(releaseUpdate)
	<-changeDone
	<-loginDone
	if early {
		t.Fatal("login password synchronization bypassed the per-user password mutation lock")
	}
}

func TestLoginWithVoceChatPasswordWriteFailureRestoresPreLoginSnapshot(t *testing.T) {
	db := setupUserServiceTestDB(t)
	enableVoceChatLoginForTest(t, false)
	t.Setenv("NOISE_PLAIN_PASSWORD_STORE", filepath.Join(t.TempDir(), "plain-passwords.db"))
	mustCreateUser(t, models.User{Username: "primary", Password: models.HashPassword("primary-password"), IsAdmin: true})
	oldSyncAt := time.Now().UTC().Add(-time.Hour)
	user := mustCreateUser(t, models.User{
		Username:           "snapshot-user",
		Password:           models.HashPassword("old-password"),
		VoceChatEmail:      "snapshot-old@vc.com",
		VoceChatUserID:     "72",
		VoceChatUsername:   "Snapshot Old",
		VoceChatSyncStatus: models.VoceChatSyncStatusFailed,
		VoceChatSyncError:  "previous sync failure",
		VoceChatLastSyncAt: &oldSyncAt,
	})
	if err := vocechat.DefaultPlainPasswordStore().UpsertUserVoceChatPassword(user.ID, user.Username, "old-password", user.VoceChatEmail, user.VoceChatUserID); err != nil {
		t.Fatalf("seed password record: %v", err)
	}
	if err := database.DB.Create(&models.UserNotification{RecipientUserID: user.ID, Type: models.UserNotificationTypeVoceChatPasswordChanged}).Error; err != nil {
		t.Fatalf("seed password-change notification: %v", err)
	}
	stubVoceChatPasswordLogin(t, func(_ context.Context, _ vocechat.Config, _, _ string) (*vocechat.LoginResponse, error) {
		return &vocechat.LoginResponse{User: vocechat.UserInfo{UID: 73, Email: "snapshot-new@vc.com", Name: "Snapshot New"}}, nil
	})

	failedPasswordWrite := false
	if err := db.Callback().Update().Before("gorm:update").Register("test:fail-login-password-write", func(tx *gorm.DB) {
		if failedPasswordWrite || tx.Statement == nil || tx.Statement.Schema == nil || tx.Statement.Schema.Table != "users" {
			return
		}
		updates, ok := tx.Statement.Dest.(map[string]interface{})
		if _, present := updates["password"]; ok && present {
			failedPasswordWrite = true
			tx.AddError(errors.New("forced login password write failure"))
		}
	}); err != nil {
		t.Fatalf("register password fault: %v", err)
	}

	loggedIn, err := Login(dto.LoginDto{Username: user.Username, Password: "new-password"})
	if err == nil || loggedIn != nil {
		t.Fatalf("password persistence failure must reject login, result present=%v err=%v", loggedIn != nil, err)
	}
	if !failedPasswordWrite {
		t.Fatal("password write fault was not injected")
	}

	updated := mustGetUserByUsername(t, user.Username)
	if bcrypt.CompareHashAndPassword([]byte(updated.Password), []byte("old-password")) != nil ||
		updated.VoceChatUserID != "72" || updated.VoceChatEmail != "snapshot-old@vc.com" || updated.VoceChatUsername != "Snapshot Old" ||
		updated.VoceChatSyncStatus != models.VoceChatSyncStatusFailed || updated.VoceChatSyncError != "previous sync failure" ||
		updated.VoceChatLastSyncAt == nil || !updated.VoceChatLastSyncAt.Equal(oldSyncAt) || updated.Token != "" || updated.LoginIssuedAt != nil {
		t.Fatalf("failed login did not restore the complete pre-login user snapshot: %#v", updated)
	}
	record, found, err := vocechat.DefaultPlainPasswordStore().GetUserPassword(user.ID)
	if err != nil || !found || record.VoceChatPassword != "old-password" || record.VoceChatEmail != "snapshot-old@vc.com" || record.VoceChatUserID != "72" {
		t.Fatalf("failed login did not preserve the password record: found=%v err=%v record=%#v", found, err, record)
	}
	var changedCount, incompleteCount int64
	if err := database.DB.Model(&models.UserNotification{}).Where("recipient_user_id = ? AND type = ?", user.ID, models.UserNotificationTypeVoceChatPasswordChanged).Count(&changedCount).Error; err != nil {
		t.Fatalf("count password-change notifications: %v", err)
	}
	if err := database.DB.Model(&models.UserNotification{}).Where("recipient_user_id = ? AND type = ?", user.ID, models.UserNotificationTypePasswordUpdateIncomplete).Count(&incompleteCount).Error; err != nil {
		t.Fatalf("count incomplete-password notifications: %v", err)
	}
	if changedCount != 1 || incompleteCount != 0 {
		t.Fatalf("complete rollback changed password alerts: changed=%d incomplete=%d", changedCount, incompleteCount)
	}
}

func TestLoginWithVoceChatLoginStateWriteFailureRestoresPreLoginSnapshot(t *testing.T) {
	db := setupUserServiceTestDB(t)
	enableVoceChatLoginForTest(t, false)
	t.Setenv("NOISE_PLAIN_PASSWORD_STORE", filepath.Join(t.TempDir(), "plain-passwords.db"))
	mustCreateUser(t, models.User{Username: "primary", Password: models.HashPassword("primary-password"), IsAdmin: true})
	oldSyncAt := time.Now().UTC().Add(-time.Hour)
	user := mustCreateUser(t, models.User{
		Username:           "login-state-user",
		Password:           models.HashPassword("old-password"),
		VoceChatEmail:      "login-state-old@vc.com",
		VoceChatUserID:     "74",
		VoceChatUsername:   "Login State Old",
		VoceChatSyncStatus: models.VoceChatSyncStatusFailed,
		VoceChatSyncError:  "previous sync failure",
		VoceChatLastSyncAt: &oldSyncAt,
	})
	if err := vocechat.DefaultPlainPasswordStore().UpsertUserVoceChatPassword(user.ID, user.Username, "old-password", user.VoceChatEmail, user.VoceChatUserID); err != nil {
		t.Fatalf("seed password record: %v", err)
	}
	if err := database.DB.Create(&models.UserNotification{RecipientUserID: user.ID, Type: models.UserNotificationTypeVoceChatPasswordChanged}).Error; err != nil {
		t.Fatalf("seed password-change notification: %v", err)
	}
	stubVoceChatPasswordLogin(t, func(_ context.Context, _ vocechat.Config, _, _ string) (*vocechat.LoginResponse, error) {
		return &vocechat.LoginResponse{User: vocechat.UserInfo{UID: 75, Email: "login-state-new@vc.com", Name: "Login State New"}}, nil
	})

	failedTokenWrite := false
	if err := db.Callback().Update().Before("gorm:update").Register("test:fail-login-token-write", func(tx *gorm.DB) {
		if failedTokenWrite || tx.Statement == nil || tx.Statement.Schema == nil || tx.Statement.Schema.Table != "users" {
			return
		}
		updates, ok := tx.Statement.Dest.(map[string]interface{})
		if _, present := updates["token"]; ok && present {
			failedTokenWrite = true
			tx.AddError(errors.New("forced login token write failure"))
		}
	}); err != nil {
		t.Fatalf("register login-state fault: %v", err)
	}

	loggedIn, err := Login(dto.LoginDto{Username: user.Username, Password: "new-password"})
	if err == nil || loggedIn != nil {
		t.Fatalf("login-state persistence failure must reject login, result present=%v err=%v", loggedIn != nil, err)
	}
	if !failedTokenWrite {
		t.Fatal("login-state write fault was not injected")
	}

	updated := mustGetUserByUsername(t, user.Username)
	if bcrypt.CompareHashAndPassword([]byte(updated.Password), []byte("old-password")) != nil ||
		updated.VoceChatUserID != "74" || updated.VoceChatEmail != "login-state-old@vc.com" || updated.VoceChatUsername != "Login State Old" ||
		updated.VoceChatSyncStatus != models.VoceChatSyncStatusFailed || updated.VoceChatSyncError != "previous sync failure" ||
		updated.VoceChatLastSyncAt == nil || !updated.VoceChatLastSyncAt.Equal(oldSyncAt) || updated.Token != "" || updated.LoginIssuedAt != nil {
		t.Fatalf("failed login did not restore state written before login issuance: %#v", updated)
	}
	record, found, err := vocechat.DefaultPlainPasswordStore().GetUserPassword(user.ID)
	if err != nil || !found || record.VoceChatPassword != "old-password" || record.VoceChatEmail != "login-state-old@vc.com" || record.VoceChatUserID != "74" {
		t.Fatalf("failed login did not restore the password record: found=%v err=%v record=%#v", found, err, record)
	}
	var changedCount, incompleteCount int64
	if err := database.DB.Model(&models.UserNotification{}).Where("recipient_user_id = ? AND type = ?", user.ID, models.UserNotificationTypeVoceChatPasswordChanged).Count(&changedCount).Error; err != nil {
		t.Fatalf("count password-change notifications: %v", err)
	}
	if err := database.DB.Model(&models.UserNotification{}).Where("recipient_user_id = ? AND type = ?", user.ID, models.UserNotificationTypePasswordUpdateIncomplete).Count(&incompleteCount).Error; err != nil {
		t.Fatalf("count incomplete-password notifications: %v", err)
	}
	if changedCount != 1 || incompleteCount != 0 {
		t.Fatalf("login-state rollback changed password alerts: changed=%d incomplete=%d", changedCount, incompleteCount)
	}
}

func TestLoginWithVoceChatIdentityMetadataWriteFailureRejectsWithoutSideEffects(t *testing.T) {
	db := setupUserServiceTestDB(t)
	enableVoceChatLoginForTest(t, false)
	t.Setenv("NOISE_PLAIN_PASSWORD_STORE", filepath.Join(t.TempDir(), "plain-passwords.db"))
	mustCreateUser(t, models.User{Username: "primary", Password: models.HashPassword("primary-password"), IsAdmin: true})
	user := mustCreateUser(t, models.User{
		Username:           "identity-metadata-user",
		Password:           models.HashPassword("old-password"),
		VoceChatEmail:      "identity-old@vc.com",
		VoceChatUserID:     "77",
		VoceChatUsername:   "Identity Old",
		VoceChatSyncStatus: models.VoceChatSyncStatusFailed,
		VoceChatSyncError:  "previous sync failure",
	})
	if err := vocechat.DefaultPlainPasswordStore().UpsertUserVoceChatPassword(user.ID, user.Username, "old-password", user.VoceChatEmail, user.VoceChatUserID); err != nil {
		t.Fatalf("seed password record: %v", err)
	}
	stubVoceChatPasswordLogin(t, func(_ context.Context, _ vocechat.Config, _, _ string) (*vocechat.LoginResponse, error) {
		return &vocechat.LoginResponse{User: vocechat.UserInfo{UID: 78, Email: "identity-new@vc.com", Name: "Identity New"}}, nil
	})
	failedIdentityWrite := false
	if err := db.Callback().Update().Before("gorm:update").Register("test:fail-login-identity-write", func(tx *gorm.DB) {
		if failedIdentityWrite || tx.Statement == nil || tx.Statement.Schema == nil || tx.Statement.Schema.Table != "users" {
			return
		}
		updates, ok := tx.Statement.Dest.(map[string]interface{})
		if _, present := updates["voce_chat_email"]; ok && present {
			failedIdentityWrite = true
			tx.AddError(errors.New("forced VoceChat identity metadata write failure"))
		}
	}); err != nil {
		t.Fatalf("register identity metadata fault: %v", err)
	}

	loggedIn, err := Login(dto.LoginDto{Username: user.Username, Password: "new-password"})
	if err == nil || loggedIn != nil {
		t.Fatalf("identity metadata failure must reject login, result present=%v err=%v", loggedIn != nil, err)
	}
	if !failedIdentityWrite {
		t.Fatal("identity metadata fault was not injected")
	}
	updated := mustGetUserByUsername(t, user.Username)
	if updated.Password != user.Password || updated.VoceChatEmail != "identity-old@vc.com" || updated.VoceChatUserID != "77" ||
		updated.VoceChatUsername != "Identity Old" || updated.VoceChatSyncStatus != models.VoceChatSyncStatusFailed ||
		updated.VoceChatSyncError != "previous sync failure" || updated.Token != "" || updated.LoginIssuedAt != nil {
		t.Fatalf("identity metadata failure changed login state: %#v", updated)
	}
}

func TestVoceChatLoginWaitsForSameUserSelfServicePasswordMutation(t *testing.T) {
	setupUserServiceTestDB(t)
	enableVoceChatLoginForTest(t, false)
	t.Setenv("NOISE_PLAIN_PASSWORD_STORE", filepath.Join(t.TempDir(), "plain-passwords.db"))
	mustCreateUser(t, models.User{Username: "primary", Password: models.HashPassword("primary-password"), IsAdmin: true})
	user := mustCreateUser(t, models.User{Username: "self-service-lock", Password: models.HashPassword("old-password"), VoceChatEmail: "self-service-lock@vc.com", VoceChatUserID: "79"})
	if err := vocechat.DefaultPlainPasswordStore().UpsertUserVoceChatPassword(user.ID, user.Username, "old-password", user.VoceChatEmail, user.VoceChatUserID); err != nil {
		t.Fatalf("seed password record: %v", err)
	}

	updateStarted := make(chan struct{})
	releaseUpdate := make(chan struct{})
	loginReachedRemote := make(chan struct{})
	var loginMu sync.Mutex
	loginCalls := 0
	stubVoceChatPasswordLogin(t, func(_ context.Context, _ vocechat.Config, _, _ string) (*vocechat.LoginResponse, error) {
		loginMu.Lock()
		loginCalls++
		call := loginCalls
		loginMu.Unlock()
		if call == 2 {
			close(loginReachedRemote)
		}
		return &vocechat.LoginResponse{User: vocechat.UserInfo{UID: 79, Email: user.VoceChatEmail, Name: user.Username}}, nil
	})
	stubVoceChatAdminUpdateUser(t, func(_ context.Context, _ vocechat.Config, _ int64, _ vocechat.UpdateUserRequest) (*vocechat.User, error) {
		close(updateStarted)
		<-releaseUpdate
		return &vocechat.User{UID: 79, Email: user.VoceChatEmail, Name: user.Username}, nil
	})

	changeDone := make(chan error, 1)
	loginDone := make(chan error, 1)
	go func() { changeDone <- ChangePasswordWithOld(user, "old-password", "new-password") }()
	<-updateStarted
	go func() {
		_, err := Login(dto.LoginDto{Username: user.Username, Password: "new-password"})
		loginDone <- err
	}()
	select {
	case <-loginReachedRemote:
		t.Fatal("login reached VoceChat before the self-service password mutation released the user lock")
	case <-time.After(150 * time.Millisecond):
	}
	close(releaseUpdate)
	if err := <-changeDone; err != nil {
		t.Fatalf("self-service password change: %v", err)
	}
	if err := <-loginDone; err != nil {
		t.Fatalf("login after self-service password change: %v", err)
	}
}

func TestLocalFallbackPasswordMaintenanceBlocksSameUserPasswordMutation(t *testing.T) {
	db := setupUserServiceTestDB(t)
	enableVoceChatLoginForTest(t, true)
	t.Setenv("NOISE_PLAIN_PASSWORD_STORE", filepath.Join(t.TempDir(), "plain-passwords.db"))
	mustCreateUser(t, models.User{Username: "primary", Password: models.HashPassword("primary-password"), IsAdmin: true})
	user := mustCreateUser(t, models.User{
		Username:           "fallback-lock-user",
		Password:           "old-password",
		VoceChatEmail:      "fallback-lock@vc.com",
		VoceChatUserID:     "80",
		VoceChatSyncStatus: models.VoceChatSyncStatusLinked,
	})
	if err := vocechat.DefaultPlainPasswordStore().UpsertUserVoceChatPassword(user.ID, user.Username, "old-password", user.VoceChatEmail, user.VoceChatUserID); err != nil {
		t.Fatalf("seed password record: %v", err)
	}
	stubVoceChatPasswordLogin(t, func(_ context.Context, _ vocechat.Config, _, _ string) (*vocechat.LoginResponse, error) {
		return nil, errors.New("forced VoceChat outage")
	})

	maintenanceStarted := make(chan struct{})
	releaseMaintenance := make(chan struct{})
	changeReachedRemote := make(chan struct{})
	blockedMaintenance := false
	if err := db.Callback().Update().Before("gorm:update").Register("test:block-local-fallback-maintenance", func(tx *gorm.DB) {
		if blockedMaintenance || tx.Statement == nil || tx.Statement.Schema == nil || tx.Statement.Schema.Table != "users" {
			return
		}
		updates, ok := tx.Statement.Dest.(map[string]interface{})
		if _, present := updates["password"]; ok && present {
			blockedMaintenance = true
			close(maintenanceStarted)
			<-releaseMaintenance
		}
	}); err != nil {
		t.Fatalf("register fallback maintenance block: %v", err)
	}
	stubVoceChatAdminUpdateUser(t, func(_ context.Context, _ vocechat.Config, _ int64, _ vocechat.UpdateUserRequest) (*vocechat.User, error) {
		close(changeReachedRemote)
		return &vocechat.User{UID: 80, Email: user.VoceChatEmail, Name: user.Username}, nil
	})

	loginDone := make(chan error, 1)
	changeDone := make(chan error, 1)
	go func() {
		_, err := Login(dto.LoginDto{Username: user.Username, Password: "old-password"})
		loginDone <- err
	}()
	<-maintenanceStarted
	go func() { changeDone <- ChangePassword(user, dto.UserInfoDto{Password: "new-password"}) }()
	select {
	case <-changeReachedRemote:
		t.Fatal("password change bypassed local fallback password maintenance for the same user")
	case <-time.After(150 * time.Millisecond):
	}
	close(releaseMaintenance)
	if err := <-loginDone; err != nil {
		t.Fatalf("local fallback login: %v", err)
	}
	if err := <-changeDone; err != nil {
		t.Fatalf("password change after local fallback maintenance: %v", err)
	}
}

func TestPasswordMutationsForDifferentUsersRunInParallel(t *testing.T) {
	setupUserServiceTestDB(t)
	enableVoceChatLoginForTest(t, false)
	t.Setenv("NOISE_PLAIN_PASSWORD_STORE", filepath.Join(t.TempDir(), "plain-passwords.db"))
	mustCreateUser(t, models.User{Username: "primary", Password: models.HashPassword("primary-password"), IsAdmin: true})
	first := mustCreateUser(t, models.User{Username: "parallel-first", Password: models.HashPassword("old-first"), VoceChatEmail: "parallel-first@vc.com", VoceChatUserID: "81"})
	second := mustCreateUser(t, models.User{Username: "parallel-second", Password: models.HashPassword("old-second"), VoceChatEmail: "parallel-second@vc.com", VoceChatUserID: "82"})
	for _, item := range []struct {
		user     *models.User
		password string
	}{{first, "old-first"}, {second, "old-second"}} {
		if err := vocechat.DefaultPlainPasswordStore().UpsertUserVoceChatPassword(item.user.ID, item.user.Username, item.password, item.user.VoceChatEmail, item.user.VoceChatUserID); err != nil {
			t.Fatalf("seed password record for %s: %v", item.user.Username, err)
		}
	}
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	secondStarted := make(chan struct{})
	stubVoceChatAdminUpdateUser(t, func(_ context.Context, _ vocechat.Config, uid int64, _ vocechat.UpdateUserRequest) (*vocechat.User, error) {
		switch uid {
		case 81:
			close(firstStarted)
			<-releaseFirst
		case 82:
			close(secondStarted)
		}
		return &vocechat.User{UID: uid}, nil
	})
	firstDone := make(chan error, 1)
	secondDone := make(chan error, 1)
	go func() { firstDone <- ChangePassword(first, dto.UserInfoDto{Password: "new-first"}) }()
	<-firstStarted
	go func() { secondDone <- ChangePassword(second, dto.UserInfoDto{Password: "new-second"}) }()
	select {
	case <-secondStarted:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("different users were serialized behind one global password mutation lock")
	}
	close(releaseFirst)
	if err := <-firstDone; err != nil {
		t.Fatalf("first user password change: %v", err)
	}
	if err := <-secondDone; err != nil {
		t.Fatalf("second user password change: %v", err)
	}
}

func TestLocalFallbackMaintenanceFailureKeepsLoginAvailableWithoutPretendingToPersist(t *testing.T) {
	db := setupUserServiceTestDB(t)
	enableVoceChatLoginForTest(t, true)
	t.Setenv("NOISE_PLAIN_PASSWORD_STORE", filepath.Join(t.TempDir(), "plain-passwords.db"))
	mustCreateUser(t, models.User{Username: "primary", Password: models.HashPassword("primary-password"), IsAdmin: true})
	user := mustCreateUser(t, models.User{
		Username:           "fallback-maintenance-failure",
		Password:           "legacy-password",
		VoceChatEmail:      "fallback-maintenance-failure@vc.com",
		VoceChatUserID:     "83",
		VoceChatSyncStatus: models.VoceChatSyncStatusLinked,
	})
	if err := vocechat.DefaultPlainPasswordStore().UpsertUserVoceChatPassword(user.ID, user.Username, "legacy-password", user.VoceChatEmail, user.VoceChatUserID); err != nil {
		t.Fatalf("seed password record: %v", err)
	}
	if err := database.DB.Create(&models.UserNotification{RecipientUserID: user.ID, Type: models.UserNotificationTypeVoceChatPasswordChanged}).Error; err != nil {
		t.Fatalf("seed password-change notification: %v", err)
	}
	stubVoceChatPasswordLogin(t, func(_ context.Context, _ vocechat.Config, _, _ string) (*vocechat.LoginResponse, error) {
		return nil, errors.New("forced VoceChat outage")
	})
	passwordWritesFailed := 0
	if err := db.Callback().Update().Before("gorm:update").Register("test:fail-local-maintenance-password-write", func(tx *gorm.DB) {
		if tx.Statement == nil || tx.Statement.Schema == nil || tx.Statement.Schema.Table != "users" {
			return
		}
		updates, ok := tx.Statement.Dest.(map[string]interface{})
		if _, present := updates["password"]; ok && present {
			passwordWritesFailed++
			tx.AddError(errors.New("forced local maintenance password write failure"))
		}
	}); err != nil {
		t.Fatalf("register local maintenance fault: %v", err)
	}

	loggedIn, err := Login(dto.LoginDto{Username: user.Username, Password: "legacy-password"})
	if err != nil || loggedIn == nil {
		t.Fatalf("non-critical local maintenance failure blocked fallback login: result present=%v err=%v", loggedIn != nil, err)
	}
	if passwordWritesFailed != 1 {
		t.Fatalf("local fallback maintenance attempted %d primary password writes, want 1", passwordWritesFailed)
	}
	updated := mustGetUserByUsername(t, user.Username)
	if updated.Password != "legacy-password" || loggedIn.Password != updated.Password || updated.Token == "" || updated.LoginIssuedAt == nil {
		t.Fatalf("failed maintenance left inconsistent login state: returned=%#v stored=%#v", loggedIn, updated)
	}
	var changedCount int64
	if err := database.DB.Model(&models.UserNotification{}).Where("recipient_user_id = ? AND type = ?", user.ID, models.UserNotificationTypeVoceChatPasswordChanged).Count(&changedCount).Error; err != nil {
		t.Fatalf("count password-change notifications: %v", err)
	}
	if changedCount != 1 {
		t.Fatalf("fallback maintenance failure cleared a password alert: got %d, want 1", changedCount)
	}
}

func TestChangePasswordWithVoceChatMetadataWriteFailureRollsBackRemoteAndLocalState(t *testing.T) {
	db := setupUserServiceTestDB(t)
	enableVoceChatLoginForTest(t, false)
	t.Setenv("NOISE_PLAIN_PASSWORD_STORE", filepath.Join(t.TempDir(), "plain-passwords.db"))
	mustCreateUser(t, models.User{Username: "primary", Password: models.HashPassword("primary-password"), IsAdmin: true})
	user := mustCreateUser(t, models.User{
		Username:           "change-metadata-failure",
		Password:           models.HashPassword("old-password"),
		VoceChatEmail:      "change-metadata-failure@vc.com",
		VoceChatUserID:     "84",
		VoceChatUsername:   "Change Metadata Old",
		VoceChatSyncStatus: models.VoceChatSyncStatusFailed,
		VoceChatSyncError:  "previous sync failure",
	})
	if err := vocechat.DefaultPlainPasswordStore().UpsertUserVoceChatPassword(user.ID, user.Username, "old-password", user.VoceChatEmail, user.VoceChatUserID); err != nil {
		t.Fatalf("seed password record: %v", err)
	}
	remotePassword := "old-password"
	stubVoceChatAdminUpdateUser(t, func(_ context.Context, _ vocechat.Config, _ int64, request vocechat.UpdateUserRequest) (*vocechat.User, error) {
		if request.Password == nil {
			t.Fatal("password update must include a password")
		}
		remotePassword = *request.Password
		return &vocechat.User{UID: 84, Email: user.VoceChatEmail, Name: "Change Metadata New"}, nil
	})
	failedMetadataWrite := false
	if err := db.Callback().Update().Before("gorm:update").Register("test:fail-change-metadata-write", func(tx *gorm.DB) {
		if failedMetadataWrite || tx.Statement == nil || tx.Statement.Schema == nil || tx.Statement.Schema.Table != "users" {
			return
		}
		updates, ok := tx.Statement.Dest.(map[string]interface{})
		if _, present := updates["voce_chat_sync_status"]; ok && present {
			failedMetadataWrite = true
			tx.AddError(errors.New("forced password-change metadata write failure"))
		}
	}); err != nil {
		t.Fatalf("register password-change metadata fault: %v", err)
	}

	err := ChangePassword(user, dto.UserInfoDto{Password: "new-password"})
	if err == nil {
		t.Fatal("password change must report the metadata persistence failure")
	}
	failure, ok := err.(*passwordUpdateFailure)
	if !ok || !failure.rolledBack {
		t.Fatalf("metadata persistence failure must report a complete rollback, got %T %v", err, err)
	}
	if !failedMetadataWrite {
		t.Fatal("password-change metadata fault was not injected")
	}
	updated := mustGetUserByUsername(t, user.Username)
	if bcrypt.CompareHashAndPassword([]byte(updated.Password), []byte("old-password")) != nil || remotePassword != "old-password" ||
		updated.VoceChatUsername != "Change Metadata Old" || updated.VoceChatSyncStatus != models.VoceChatSyncStatusFailed ||
		updated.VoceChatSyncError != "previous sync failure" {
		t.Fatalf("metadata failure left partial password state: remote=%t user=%#v", remotePassword == "old-password", updated)
	}
	record, found, err := vocechat.DefaultPlainPasswordStore().GetUserPassword(user.ID)
	if err != nil || !found || record.VoceChatPassword != "old-password" {
		t.Fatalf("metadata failure changed the password record: found=%v err=%v record=%#v", found, err, record)
	}
}
