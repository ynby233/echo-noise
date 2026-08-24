package services

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/dto"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/vocechat"
	"golang.org/x/crypto/bcrypt"
)

func createRuntimeModeConfigForTest(t *testing.T, mode string, healthStatus string) {
	t.Helper()
	if err := database.DB.Create(&models.SiteConfig{
		RuntimeMode:                      mode,
		RuntimeModeMigrationVersion:      models.RuntimeModeMigrationVersionCurrent,
		VoceChatEnabled:                  mode == models.RuntimeModeVoceChat,
		VoceChatBaseURL:                  "https://vc.example.test",
		VoceChatAdminToken:               "configured-token",
		VoceChatLoginVerificationEnabled: mode == models.RuntimeModeVoceChat,
		VoceChatContactsEnabled:          mode == models.RuntimeModeVoceChat,
		VoceChatNotificationEnabled:      mode == models.RuntimeModeVoceChat,
		VoceChatLastHealthStatus:         healthStatus,
	}).Error; err != nil {
		t.Fatalf("create runtime mode config: %v", err)
	}
}

func TestLocalModeBoundUserLoginAndPasswordChangeNeverCallVoceChat(t *testing.T) {
	setupUserServiceTestDB(t)
	createRuntimeModeConfigForTest(t, models.RuntimeModeLocal, "ok")
	storePath := filepath.Join(t.TempDir(), "plain-passwords.db")
	t.Setenv("NOISE_PLAIN_PASSWORD_STORE", storePath)
	mustCreateUser(t, models.User{Username: "primary-local-mode", Password: models.HashPassword("primary"), IsAdmin: true})
	user := mustCreateUser(t, models.User{
		Username:           "bound-local-mode",
		Password:           models.HashPassword("old-local-password"),
		VoceChatEmail:      "bound-local@vc.example",
		VoceChatUserID:     "42",
		VoceChatSyncStatus: models.VoceChatSyncStatusLinked,
	})
	if err := vocechat.DefaultPlainPasswordStore().UpsertUserVoceChatPassword(user.ID, user.Username, "remote-password", user.VoceChatEmail, user.VoceChatUserID); err != nil {
		t.Fatalf("seed remote password state: %v", err)
	}
	CreateVoceChatPasswordChangedAlertOnce(user.ID)
	stubVoceChatPasswordLogin(t, func(context.Context, vocechat.Config, string, string) (*vocechat.LoginResponse, error) {
		t.Fatal("local mode must not call VoceChat login")
		return nil, nil
	})
	stubVoceChatAdminUpdateUser(t, func(context.Context, vocechat.Config, int64, vocechat.UpdateUserRequest) (*vocechat.User, error) {
		t.Fatal("local mode must not call VoceChat password update")
		return nil, nil
	})

	loggedIn, err := Login(dto.LoginDto{Username: user.Username, Password: "old-local-password"})
	if err != nil || loggedIn == nil {
		t.Fatalf("local mode login = user %#v error %v", loggedIn, err)
	}
	if err := ChangePassword(user, dto.UserInfoDto{Password: "new-local-password"}); err != nil {
		t.Fatalf("local mode password change: %v", err)
	}
	updated := mustGetUserByUsername(t, user.Username)
	if bcrypt.CompareHashAndPassword([]byte(updated.Password), []byte("new-local-password")) != nil {
		t.Fatal("local mode did not commit the new local password")
	}
	if updated.VoceChatSyncStatus != models.VoceChatSyncStatusPasswordSyncRequired {
		t.Fatalf("local mode sync status = %q, want password_sync_required", updated.VoceChatSyncStatus)
	}
	record, ok, err := vocechat.NewPlainPasswordStore(storePath).GetUserPassword(user.ID)
	if err != nil || !ok {
		t.Fatalf("read password state: ok=%v err=%v", ok, err)
	}
	if record.VoceChatPasswordValue() != "remote-password" || record.LocalFallbackPasswordValue() != "new-local-password" {
		t.Fatalf("local mode password state = remote %q local %q", record.VoceChatPasswordValue(), record.LocalFallbackPasswordValue())
	}
	var unresolvedAlertCount int64
	if err := database.DB.Model(&models.UserNotification{}).
		Where("recipient_user_id = ? AND type = ?", user.ID, models.UserNotificationTypeVoceChatPasswordChanged).
		Count(&unresolvedAlertCount).Error; err != nil {
		t.Fatalf("count unresolved alerts: %v", err)
	}
	if unresolvedAlertCount != 1 {
		t.Fatalf("local-only password change cleared %d unresolved VoceChat alerts", 1-unresolvedAlertCount)
	}
}

func TestVoceChatTransientFailureFallsBackLocallyWithoutCredentialAlert(t *testing.T) {
	setupUserServiceTestDB(t)
	createRuntimeModeConfigForTest(t, models.RuntimeModeVoceChat, "ok")
	t.Setenv("NOISE_PLAIN_PASSWORD_STORE", filepath.Join(t.TempDir(), "plain-passwords.db"))
	mustCreateUser(t, models.User{Username: "primary-degraded", Password: models.HashPassword("primary"), IsAdmin: true})
	user := mustCreateUser(t, models.User{
		Username:           "bound-degraded",
		Password:           models.HashPassword("fallback-password"),
		VoceChatEmail:      "bound-degraded@vc.example",
		VoceChatUserID:     "43",
		VoceChatSyncStatus: models.VoceChatSyncStatusLinked,
	})
	stubVoceChatPasswordLogin(t, func(context.Context, vocechat.Config, string, string) (*vocechat.LoginResponse, error) {
		return nil, errors.New("dial tcp: temporary upstream outage")
	})

	loggedIn, err := Login(dto.LoginDto{Username: user.Username, Password: "fallback-password"})
	if err != nil || loggedIn == nil {
		t.Fatalf("degraded fallback login = user %#v error %v", loggedIn, err)
	}
	var config models.SiteConfig
	if err := database.DB.First(&config).Error; err != nil {
		t.Fatalf("reload site config: %v", err)
	}
	if config.VoceChatLastHealthStatus != "failed" {
		t.Fatalf("site health = %q, want failed", config.VoceChatLastHealthStatus)
	}
	var credentialAlerts int64
	if err := database.DB.Model(&models.UserNotification{}).
		Where("recipient_user_id = ? AND type = ?", user.ID, models.UserNotificationTypeVoceChatPasswordChanged).
		Count(&credentialAlerts).Error; err != nil {
		t.Fatalf("count credential alerts: %v", err)
	}
	if credentialAlerts != 0 {
		t.Fatalf("site outage created %d credential alerts", credentialAlerts)
	}
}

func TestVoceChatDegradedModeRejectsBoundPasswordChangeWithoutPartialState(t *testing.T) {
	setupUserServiceTestDB(t)
	createRuntimeModeConfigForTest(t, models.RuntimeModeVoceChat, "failed")
	storePath := filepath.Join(t.TempDir(), "plain-passwords.db")
	t.Setenv("NOISE_PLAIN_PASSWORD_STORE", storePath)
	mustCreateUser(t, models.User{Username: "primary-degraded-change", Password: models.HashPassword("primary"), IsAdmin: true})
	user := mustCreateUser(t, models.User{
		Username:           "bound-degraded-change",
		Password:           models.HashPassword("old-password"),
		VoceChatEmail:      "bound-degraded-change@vc.example",
		VoceChatUserID:     "44",
		VoceChatSyncStatus: models.VoceChatSyncStatusLinked,
	})
	if err := vocechat.DefaultPlainPasswordStore().UpsertUserVoceChatPassword(user.ID, user.Username, "old-password", user.VoceChatEmail, user.VoceChatUserID); err != nil {
		t.Fatalf("seed password state: %v", err)
	}
	stubVoceChatAdminUpdateUser(t, func(context.Context, vocechat.Config, int64, vocechat.UpdateUserRequest) (*vocechat.User, error) {
		t.Fatal("degraded mode must reject before a remote password write")
		return nil, nil
	})

	err := ChangePassword(user, dto.UserInfoDto{Password: "new-password"})
	if err == nil || !strings.Contains(err.Error(), "暂不可用") {
		t.Fatalf("degraded password change error = %v", err)
	}
	updated := mustGetUserByUsername(t, user.Username)
	if bcrypt.CompareHashAndPassword([]byte(updated.Password), []byte("old-password")) != nil || updated.VoceChatSyncStatus != models.VoceChatSyncStatusLinked {
		t.Fatalf("degraded password change altered account state: %#v", updated)
	}
	record, ok, readErr := vocechat.NewPlainPasswordStore(storePath).GetUserPassword(user.ID)
	if readErr != nil || !ok || record.VoceChatPasswordValue() != "old-password" {
		t.Fatalf("degraded password state changed: ok=%v err=%v record=%#v", ok, readErr, record)
	}
}

func TestVoceChatDegradedLoginAutomaticallyRecoversAfterSuccessfulUpstreamVerification(t *testing.T) {
	setupUserServiceTestDB(t)
	createRuntimeModeConfigForTest(t, models.RuntimeModeVoceChat, "failed")
	t.Setenv("NOISE_PLAIN_PASSWORD_STORE", filepath.Join(t.TempDir(), "plain-passwords.db"))
	mustCreateUser(t, models.User{Username: "primary-recovery", Password: models.HashPassword("primary"), IsAdmin: true})
	user := mustCreateUser(t, models.User{
		Username:           "bound-recovery",
		Password:           models.HashPassword("current-password"),
		VoceChatEmail:      "bound-recovery@vc.example",
		VoceChatUserID:     "45",
		VoceChatSyncStatus: models.VoceChatSyncStatusLinked,
	})
	stubVoceChatPasswordLogin(t, func(context.Context, vocechat.Config, string, string) (*vocechat.LoginResponse, error) {
		return &vocechat.LoginResponse{Token: "recovered-token", User: vocechat.UserInfo{UID: 45, Email: user.VoceChatEmail, Name: user.Username}}, nil
	})

	loggedIn, err := Login(dto.LoginDto{Username: user.Username, Password: "current-password"})
	if err != nil || loggedIn == nil {
		t.Fatalf("recovery login = user %#v error %v", loggedIn, err)
	}
	var config models.SiteConfig
	if err := database.DB.First(&config).Error; err != nil {
		t.Fatalf("reload site config: %v", err)
	}
	if config.VoceChatLastHealthStatus != "ok" || config.VoceChatLastHealthError != "" {
		t.Fatalf("recovered site health = %q/%q", config.VoceChatLastHealthStatus, config.VoceChatLastHealthError)
	}
}
