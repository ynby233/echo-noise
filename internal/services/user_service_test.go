package services

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/dto"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/repository"
	"github.com/rcy1314/echo-noise/internal/vocechat"
	"github.com/rcy1314/echo-noise/pkg"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

func setupUserServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "noise-test.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	if err := models.MigrateDB(db); err != nil {
		t.Fatalf("migrate test db: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get test sql db: %v", err)
	}
	t.Cleanup(func() {
		repository.ClearUserCache()
		database.DB = nil
		models.SetDB(nil)
		_ = sqlDB.Close()
	})

	database.DB = db
	models.SetDB(db)
	return db
}

func mustCreateUser(t *testing.T, user models.User) *models.User {
	t.Helper()

	if err := repository.CreateUser(&user); err != nil {
		t.Fatalf("create user %s: %v", user.Username, err)
	}
	return &user
}

func mustGetUserByUsername(t *testing.T, username string) *models.User {
	t.Helper()

	user, err := repository.GetUserByUsername(username)
	if err != nil {
		t.Fatalf("get user by username %s: %v", username, err)
	}
	return user
}

func enableVoceChatLoginForTest(t *testing.T, fallbackEnabled bool) {
	t.Helper()
	configureVoceChatForTest(t, true, true, fallbackEnabled)
}

func configureVoceChatForTest(t *testing.T, enabled bool, loginVerificationEnabled bool, fallbackEnabled bool) {
	t.Helper()

	if err := database.DB.Create(&models.SiteConfig{
		SiteTitle:                        "Test Site",
		VoceChatEnabled:                  enabled,
		VoceChatBaseURL:                  "https://vc.example.com",
		VoceChatAdminUsername:            "admin@vc.com",
		VoceChatAdminPassword:            "admin-secret",
		VoceChatLoginVerificationEnabled: loginVerificationEnabled,
		VoceChatLocalFallbackEnabled:     fallbackEnabled,
		VoceChatEmailDomain:              "vc.com",
	}).Error; err != nil {
		t.Fatalf("create vc site config: %v", err)
	}
}

func stubVoceChatPasswordLogin(t *testing.T, fn voceChatPasswordLoginFunc) {
	t.Helper()

	original := voceChatPasswordLogin
	voceChatPasswordLogin = fn
	t.Cleanup(func() { voceChatPasswordLogin = original })
}

func stubVoceChatAdminUpdateUser(t *testing.T, fn voceChatAdminUpdateUserFunc) {
	t.Helper()

	original := voceChatAdminUpdateUser
	voceChatAdminUpdateUser = fn
	t.Cleanup(func() { voceChatAdminUpdateUser = original })
}

func TestRegisterCreatesPendingApplicationInsteadOfUser(t *testing.T) {
	setupUserServiceTestDB(t)
	storePath := filepath.Join(t.TempDir(), "plain-passwords.db")
	t.Setenv("NOISE_PLAIN_PASSWORD_STORE", storePath)

	originalProvision := registrationVoceChatProvision
	registrationVoceChatProvision = func(applicationID, username, password string) registrationVoceChatProvisionResult {
		return registrationVoceChatProvisionResult{
			Email:      buildVoceChatApplicationEmail(applicationID, vocechat.DefaultEmailDomain),
			Username:   username,
			SyncStatus: models.VoceChatSyncStatusNone,
		}
	}
	defer func() { registrationVoceChatProvision = originalProvision }()

	if err := Register(dto.RegisterDto{Username: "新用户_01", Password: "secret-pass"}); err != nil {
		t.Fatalf("register: %v", err)
	}

	if _, err := repository.GetUserByUsername("新用户_01"); err == nil {
		t.Fatalf("register should not create local user before review")
	} else if err != gorm.ErrRecordNotFound {
		t.Fatalf("get local user: %v", err)
	}

	application, err := repository.GetPendingRegistrationApplicationByUsername("新用户_01")
	if err != nil {
		t.Fatalf("get pending application: %v", err)
	}
	if application.Status != models.RegistrationApplicationStatusPending {
		t.Fatalf("application status = %q", application.Status)
	}
	if application.VoceChatSyncStatus != models.VoceChatSyncStatusUnbound {
		t.Fatalf("vc sync status = %q", application.VoceChatSyncStatus)
	}
	if application.ApplicationID != "1" {
		t.Fatalf("application id = %q, want 1", application.ApplicationID)
	}
	if strings.Contains(application.ApplicationID, "_") {
		t.Fatalf("application id %q must be usable as VoceChat email prefix", application.ApplicationID)
	}
	if application.VoceChatCandidateEmail != application.ApplicationID+"@vc.com" || application.VoceChatEmail != "" {
		t.Fatalf("candidate/actual vc email = %q/%q", application.VoceChatCandidateEmail, application.VoceChatEmail)
	}
	if !strings.HasPrefix(application.PasswordHash, "$2") {
		t.Fatalf("password hash should be bcrypt, got %q", application.PasswordHash)
	}

	record, ok, err := vocechat.NewPlainPasswordStore(storePath).GetApplicationPassword(application.ApplicationID)
	if err != nil {
		t.Fatalf("read plain password record: %v", err)
	}
	if !ok {
		t.Fatalf("plain password record missing")
	}
	if record.VoceChatPassword != "" || record.LocalFallbackPassword != "secret-pass" || record.Username != "新用户_01" || record.VoceChatEmail != application.VoceChatCandidateEmail || record.LocalFallbackPasswordUpdatedAt == nil {
		t.Fatalf("unexpected plain password record: %#v", record)
	}
}

func TestRegisterDoesNotAutoApproveWhenVoceChatDisabled(t *testing.T) {
	setupUserServiceTestDB(t)
	t.Setenv("NOISE_PLAIN_PASSWORD_STORE", filepath.Join(t.TempDir(), "plain-passwords.db"))
	if err := database.DB.Create(&models.Setting{AllowRegistration: true, AutoApproveRegistration: true}).Error; err != nil {
		t.Fatalf("create setting: %v", err)
	}
	setRegistrationProvisionForTest(t, func(applicationID, username, password string) registrationVoceChatProvisionResult {
		return registrationVoceChatProvisionResult{
			Email:      buildVoceChatApplicationEmail(applicationID, vocechat.DefaultEmailDomain),
			Username:   username,
			SyncStatus: models.VoceChatSyncStatusNone,
		}
	})

	result, err := RegisterWithResult(dto.RegisterDto{Username: "auto_ok", Password: "secret-pass"})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if result.AutoApproved {
		t.Fatalf("local registration must not be auto approved")
	}
	if result.Status != models.RegistrationApplicationStatusPending {
		t.Fatalf("status = %q", result.Status)
	}
	if result.LocalUserID != nil {
		t.Fatalf("local user must not exist before review: %d", *result.LocalUserID)
	}

	if _, err := repository.GetUserByUsername("auto_ok"); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("local user lookup err = %v", err)
	}
	application, err := repository.GetRegistrationApplicationByApplicationID(result.ApplicationID)
	if err != nil {
		t.Fatalf("get application: %v", err)
	}
	if application.Status != models.RegistrationApplicationStatusPending {
		t.Fatalf("application status = %q", application.Status)
	}
	if application.VoceChatSyncStatus != models.VoceChatSyncStatusUnbound || application.VoceChatUserID != "" || application.VoceChatEmail != "" || application.VoceChatSyncError != "" {
		t.Fatalf("unexpected vc fields on application: %#v", application)
	}
	if _, err := repository.GetPendingRegistrationApplicationByUsername("auto_ok"); err != nil {
		t.Fatalf("pending application lookup err = %v", err)
	}
}

func TestRegisterAutoApproveDefersWhenVoceChatUnavailable(t *testing.T) {
	setupUserServiceTestDB(t)
	configureVoceChatForTest(t, true, true, false)
	t.Setenv("NOISE_PLAIN_PASSWORD_STORE", filepath.Join(t.TempDir(), "plain-passwords.db"))
	if err := database.DB.Create(&models.Setting{AllowRegistration: true, AutoApproveRegistration: true}).Error; err != nil {
		t.Fatalf("create setting: %v", err)
	}
	setRegistrationProvisionForTest(t, func(applicationID, username, password string) registrationVoceChatProvisionResult {
		return registrationVoceChatProvisionResult{
			Email:      buildVoceChatApplicationEmail(applicationID, vocechat.DefaultEmailDomain),
			Username:   username,
			SyncStatus: models.VoceChatSyncStatusPending,
			SyncError:  "vc down",
		}
	})

	result, err := RegisterWithResult(dto.RegisterDto{Username: "auto_wait", Password: "secret-pass"})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if result.AutoApproved {
		t.Fatalf("auto approved = true")
	}
	if result.Status != models.RegistrationApplicationStatusPending {
		t.Fatalf("status = %q", result.Status)
	}
	if _, err := repository.GetUserByUsername("auto_wait"); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("local user lookup err = %v", err)
	}
	application, err := repository.GetPendingRegistrationApplicationByUsername("auto_wait")
	if err != nil {
		t.Fatalf("get pending application: %v", err)
	}
	if application.VoceChatSyncStatus != models.VoceChatSyncStatusPending {
		t.Fatalf("vc sync status = %q", application.VoceChatSyncStatus)
	}
	if !strings.Contains(application.VoceChatSyncError, "vc down") {
		t.Fatalf("vc sync error = %q", application.VoceChatSyncError)
	}
}

func TestRegisterUsesIncreasingNumericApplicationIDs(t *testing.T) {
	setupUserServiceTestDB(t)
	t.Setenv("NOISE_PLAIN_PASSWORD_STORE", filepath.Join(t.TempDir(), "plain-passwords.db"))

	originalProvision := registrationVoceChatProvision
	registrationVoceChatProvision = func(applicationID, username, password string) registrationVoceChatProvisionResult {
		return registrationVoceChatProvisionResult{
			Email:      buildVoceChatApplicationEmail(applicationID, vocechat.DefaultEmailDomain),
			Username:   username,
			SyncStatus: models.VoceChatSyncStatusNone,
		}
	}
	defer func() { registrationVoceChatProvision = originalProvision }()

	if err := Register(dto.RegisterDto{Username: "alice_01", Password: "secret-pass"}); err != nil {
		t.Fatalf("register first user: %v", err)
	}
	first, err := repository.GetPendingRegistrationApplicationByUsername("alice_01")
	if err != nil {
		t.Fatalf("get first application: %v", err)
	}
	if first.ApplicationID != "1" {
		t.Fatalf("first application id = %q", first.ApplicationID)
	}
	if err := repository.UpdateRegistrationApplicationFields(first.ID, map[string]interface{}{"status": models.RegistrationApplicationStatusRejected}); err != nil {
		t.Fatalf("mark first application rejected: %v", err)
	}

	if err := Register(dto.RegisterDto{Username: "bob_01", Password: "secret-pass"}); err != nil {
		t.Fatalf("register second user: %v", err)
	}
	second, err := repository.GetPendingRegistrationApplicationByUsername("bob_01")
	if err != nil {
		t.Fatalf("get second application: %v", err)
	}
	if second.ApplicationID != "2" {
		t.Fatalf("second application id = %q", second.ApplicationID)
	}
}

func TestRegisterStoresVoceChatPrecreateResult(t *testing.T) {
	setupUserServiceTestDB(t)
	configureVoceChatForTest(t, true, true, false)
	t.Setenv("NOISE_PLAIN_PASSWORD_STORE", filepath.Join(t.TempDir(), "plain-passwords.db"))

	originalProvision := registrationVoceChatProvision
	registrationVoceChatProvision = func(applicationID, username, password string) registrationVoceChatProvisionResult {
		return registrationVoceChatProvisionResult{
			Email:      applicationID + "@vc.com",
			UserID:     "42",
			Username:   username,
			SyncStatus: models.VoceChatSyncStatusCreated,
		}
	}
	defer func() { registrationVoceChatProvision = originalProvision }()

	if err := Register(dto.RegisterDto{Username: "alice_01", Password: "secret-pass"}); err != nil {
		t.Fatalf("register: %v", err)
	}

	application, err := repository.GetPendingRegistrationApplicationByUsername("alice_01")
	if err != nil {
		t.Fatalf("get pending application: %v", err)
	}
	if application.VoceChatSyncStatus != models.VoceChatSyncStatusCreated {
		t.Fatalf("vc sync status = %q", application.VoceChatSyncStatus)
	}
	if application.VoceChatUserID != "42" {
		t.Fatalf("vc user id = %q", application.VoceChatUserID)
	}
}

func TestRegisterRejectsInvalidOrDuplicatePendingUsernames(t *testing.T) {
	setupUserServiceTestDB(t)
	t.Setenv("NOISE_PLAIN_PASSWORD_STORE", filepath.Join(t.TempDir(), "plain-passwords.db"))

	originalProvision := registrationVoceChatProvision
	registrationVoceChatProvision = func(applicationID, username, password string) registrationVoceChatProvisionResult {
		return registrationVoceChatProvisionResult{
			Email:      applicationID + "@vc.com",
			Username:   username,
			SyncStatus: models.VoceChatSyncStatusNone,
		}
	}
	defer func() { registrationVoceChatProvision = originalProvision }()

	if err := Register(dto.RegisterDto{Username: "a", Password: "secret-pass"}); err == nil {
		t.Fatalf("one-character username should be rejected")
	}
	if err := Register(dto.RegisterDto{Username: "bad-name", Password: "secret-pass"}); err == nil {
		t.Fatalf("username with hyphen should be rejected")
	}
	if err := Register(dto.RegisterDto{Username: "Tom", Password: "secret-pass"}); err != nil {
		t.Fatalf("register Tom: %v", err)
	}
	if err := Register(dto.RegisterDto{Username: "tom", Password: "secret-pass"}); err == nil {
		t.Fatalf("case-fold duplicate pending username should be rejected")
	}
	if err := Register(dto.RegisterDto{Username: "Tom", Password: "secret-pass"}); err == nil {
		t.Fatalf("duplicate pending username should be rejected")
	}
}

func TestRegisterRejectsCaseFoldExistingUsername(t *testing.T) {
	setupUserServiceTestDB(t)
	t.Setenv("NOISE_PLAIN_PASSWORD_STORE", filepath.Join(t.TempDir(), "plain-passwords.db"))
	mustCreateUser(t, models.User{Username: "Tom", Password: models.HashPassword("tom"), Token: models.GenerateToken(32)})

	if err := Register(dto.RegisterDto{Username: "tom", Password: "secret-pass"}); err == nil {
		t.Fatalf("case-fold duplicate existing username should be rejected")
	}
}

func TestGetUserByUsernameIsCaseSensitive(t *testing.T) {
	setupUserServiceTestDB(t)

	mustCreateUser(t, models.User{Username: "Tom", Password: models.HashPassword("tom"), Token: models.GenerateToken(32)})
	mustCreateUser(t, models.User{Username: "tom", Password: models.HashPassword("tom"), Token: models.GenerateToken(32)})
	mustCreateUser(t, models.User{Username: "TOM", Password: models.HashPassword("tom"), Token: models.GenerateToken(32)})

	if got := mustGetUserByUsername(t, "Tom"); got.Username != "Tom" {
		t.Fatalf("GetUserByUsername(Tom) returned %q", got.Username)
	}
	if got := mustGetUserByUsername(t, "tom"); got.Username != "tom" {
		t.Fatalf("GetUserByUsername(tom) returned %q", got.Username)
	}
	if got := mustGetUserByUsername(t, "TOM"); got.Username != "TOM" {
		t.Fatalf("GetUserByUsername(TOM) returned %q", got.Username)
	}
}

func TestUserProfileUpdatesDoNotOverwritePasswordsAndPasswordFormatsRemainCompatible(t *testing.T) {
	setupUserServiceTestDB(t)

	originalHash := models.HashPassword("admin")
	profileUser := mustCreateUser(t, models.User{
		Username: "admin",
		Password: originalHash,
		IsAdmin:  true,
		Token:    models.GenerateToken(32),
	})

	cachedUser, err := repository.GetUserByID(profileUser.ID)
	if err != nil {
		t.Fatalf("get user by id: %v", err)
	}

	// 模拟旧实现中 GetUserInfo 直接清空缓存对象的副作用。
	cachedUser.Password = ""

	if err := UpdateUser(cachedUser, dto.UserInfoDto{
		Username:    "admin_renamed",
		AvatarURL:   "https://example.com/avatar.png",
		Description: "updated profile",
	}); err != nil {
		t.Fatalf("update user profile: %v", err)
	}

	updatedProfileUser := mustGetUserByUsername(t, "admin_renamed")
	if updatedProfileUser.Password != originalHash {
		t.Fatalf("password hash changed unexpectedly after profile update, got %q want original hash", updatedProfileUser.Password)
	}
	if updatedProfileUser.AvatarURL != "https://example.com/avatar.png" {
		t.Fatalf("avatar not updated, got %q", updatedProfileUser.AvatarURL)
	}
	if updatedProfileUser.Description != "updated profile" {
		t.Fatalf("description not updated, got %q", updatedProfileUser.Description)
	}

	if _, err := Login(dto.LoginDto{Username: "admin_renamed", Password: "admin"}); err != nil {
		t.Fatalf("login with original password after profile update should succeed: %v", err)
	}

	if err := ChangePasswordWithOld(updatedProfileUser, "admin", "new-password"); err != nil {
		t.Fatalf("change password with old password: %v", err)
	}
	if _, err := Login(dto.LoginDto{Username: "admin_renamed", Password: "new-password"}); err != nil {
		t.Fatalf("login with new password should succeed: %v", err)
	}

	mustCreateUser(t, models.User{
		Username: "plain-user",
		Password: "plainpass",
		Token:    models.GenerateToken(32),
	})
	if _, err := Login(dto.LoginDto{Username: "plain-user", Password: "plainpass"}); err != nil {
		t.Fatalf("plain password login should succeed: %v", err)
	}
	plainUserAfter := mustGetUserByUsername(t, "plain-user")
	if !strings.HasPrefix(plainUserAfter.Password, "$2") {
		t.Fatalf("plain password should be upgraded to bcrypt, got %q", plainUserAfter.Password)
	}

	mustCreateUser(t, models.User{
		Username: "md5-user",
		Password: pkg.MD5Encrypt("md5pass"),
		Token:    models.GenerateToken(32),
	})
	if _, err := Login(dto.LoginDto{Username: "md5-user", Password: "md5pass"}); err != nil {
		t.Fatalf("md5 password login should succeed: %v", err)
	}
	md5UserAfter := mustGetUserByUsername(t, "md5-user")
	if !strings.HasPrefix(md5UserAfter.Password, "$2") {
		t.Fatalf("md5 password should be upgraded to bcrypt, got %q", md5UserAfter.Password)
	}

	if err := ChangePasswordWithOld(md5UserAfter, "md5pass", "md5pass-new"); err != nil {
		t.Fatalf("change password on upgraded md5 user should succeed: %v", err)
	}
	if _, err := Login(dto.LoginDto{Username: "md5-user", Password: "md5pass-new"}); err != nil {
		t.Fatalf("login with changed password should succeed: %v", err)
	}
}

func TestLoginWithVoceChatSuccessSyncsLocalHashAndBinding(t *testing.T) {
	setupUserServiceTestDB(t)
	enableVoceChatLoginForTest(t, false)
	storePath := filepath.Join(t.TempDir(), "plain-passwords.db")
	t.Setenv("NOISE_PLAIN_PASSWORD_STORE", storePath)

	user := mustCreateUser(t, models.User{
		Username:           "alice",
		Password:           models.HashPassword("old-local-password"),
		VoceChatEmail:      "alice@vc.com",
		VoceChatUserID:     "12",
		VoceChatSyncStatus: models.VoceChatSyncStatusLinked,
	})

	stubVoceChatPasswordLogin(t, func(ctx context.Context, config vocechat.Config, email, password string) (*vocechat.LoginResponse, error) {
		if email != "alice@vc.com" || password != "vc-password" {
			t.Fatalf("vc login args email=%q password=%q", email, password)
		}
		if !config.LoginVerificationEnabled || config.LocalFallbackEnabled {
			t.Fatalf("unexpected vc config: %#v", config)
		}
		return &vocechat.LoginResponse{User: vocechat.UserInfo{UID: 99, Email: "alice-updated@vc.com", Name: "Alice VC"}}, nil
	})

	loggedIn, err := Login(dto.LoginDto{Username: "alice", Password: "vc-password"})
	if err != nil {
		t.Fatalf("login through vc: %v", err)
	}
	if loggedIn.Token == "" {
		t.Fatalf("login should generate token")
	}

	updated := mustGetUserByUsername(t, "alice")
	if updated.VoceChatUserID != "99" || updated.VoceChatEmail != "alice-updated@vc.com" || updated.VoceChatUsername != "Alice VC" {
		t.Fatalf("vc binding not synced: %#v", updated)
	}
	if updated.VoceChatSyncStatus != models.VoceChatSyncStatusLinked || updated.VoceChatSyncError != "" || updated.VoceChatLastSyncAt == nil {
		t.Fatalf("vc sync status not refreshed: %#v", updated)
	}
	if bcrypt.CompareHashAndPassword([]byte(updated.Password), []byte("vc-password")) != nil {
		t.Fatalf("local password hash should be aligned to VoceChat login password")
	}

	record, ok, err := vocechat.NewPlainPasswordStore(storePath).GetUserPassword(user.ID)
	if err != nil {
		t.Fatalf("read plain password record: %v", err)
	}
	if !ok || record.VoceChatPassword != "vc-password" || record.LocalFallbackPassword != "" || record.VoceChatPasswordUpdatedAt == nil || record.LocalFallbackPasswordUpdatedAt != nil || record.VoceChatEmail != "alice-updated@vc.com" || record.VoceChatUserID != "99" {
		t.Fatalf("plain password record not synced: ok=%v record=%#v", ok, record)
	}
}

func TestDelegatedAdministratorUsesVoceChatLoginAndKeepsPasswordSynced(t *testing.T) {
	setupUserServiceTestDB(t)
	enableVoceChatLoginForTest(t, false)
	storePath := filepath.Join(t.TempDir(), "delegated-plain-passwords.db")
	t.Setenv("NOISE_PLAIN_PASSWORD_STORE", storePath)

	mustCreateUser(t, models.User{Username: "primary", Password: models.HashPassword("primary-local"), IsAdmin: true})
	delegated := mustCreateUser(t, models.User{
		Username:       "delegated",
		Password:       models.HashPassword("stale-local-password"),
		IsAdmin:        true,
		VoceChatEmail:  "delegated@vc.com",
		VoceChatUserID: "42",
	})

	stubVoceChatPasswordLogin(t, func(ctx context.Context, config vocechat.Config, email, password string) (*vocechat.LoginResponse, error) {
		if email != "delegated@vc.com" || password != "delegated-vc-password" {
			t.Fatalf("delegated vc login args email=%q password=%q", email, password)
		}
		return &vocechat.LoginResponse{User: vocechat.UserInfo{UID: 42, Email: email, Name: "Delegated VC"}}, nil
	})

	if _, err := Login(dto.LoginDto{Username: delegated.Username, Password: "delegated-vc-password"}); err != nil {
		t.Fatalf("delegated administrator must authenticate with VoceChat: %v", err)
	}

	updated := mustGetUserByUsername(t, delegated.Username)
	if bcrypt.CompareHashAndPassword([]byte(updated.Password), []byte("delegated-vc-password")) != nil {
		t.Fatal("delegated local password must be synced from the VoceChat login")
	}
	record, ok, err := vocechat.NewPlainPasswordStore(storePath).GetUserPassword(delegated.ID)
	if err != nil || !ok || record.VoceChatPassword != "delegated-vc-password" || record.LocalFallbackPassword != "" {
		t.Fatalf("delegated password record must retain the VoceChat password: ok=%v record=%#v err=%v", ok, record, err)
	}
}

func TestChangePasswordForDelegatedAdministratorUpdatesVoceChatBeforeLocalState(t *testing.T) {
	setupUserServiceTestDB(t)
	enableVoceChatLoginForTest(t, false)
	storePath := filepath.Join(t.TempDir(), "delegated-change-passwords.db")
	t.Setenv("NOISE_PLAIN_PASSWORD_STORE", storePath)

	mustCreateUser(t, models.User{Username: "primary", Password: models.HashPassword("primary-local"), IsAdmin: true})
	delegated := mustCreateUser(t, models.User{
		Username:       "delegated-change",
		Password:       models.HashPassword("old-delegated-password"),
		IsAdmin:        true,
		VoceChatEmail:  "delegated-change@vc.com",
		VoceChatUserID: "43",
	})
	if err := vocechat.NewPlainPasswordStore(storePath).UpsertUserVoceChatPassword(delegated.ID, delegated.Username, "old-delegated-password", delegated.VoceChatEmail, delegated.VoceChatUserID); err != nil {
		t.Fatalf("seed delegated password record: %v", err)
	}

	updatedRemote := false
	stubVoceChatAdminUpdateUser(t, func(ctx context.Context, config vocechat.Config, uid int64, request vocechat.UpdateUserRequest) (*vocechat.User, error) {
		updatedRemote = true
		if uid != 43 || request.Password == nil || *request.Password != "new-delegated-password" {
			t.Fatalf("delegated remote password update = uid %d request %#v", uid, request)
		}
		return &vocechat.User{UID: uid, Email: delegated.VoceChatEmail, Name: "Delegated Change"}, nil
	})

	if err := ChangePassword(delegated, dto.UserInfoDto{Password: "new-delegated-password"}); err != nil {
		t.Fatalf("change delegated password: %v", err)
	}
	if !updatedRemote {
		t.Fatal("delegated password must update VoceChat before local state")
	}
	updated := mustGetUserByUsername(t, delegated.Username)
	if bcrypt.CompareHashAndPassword([]byte(updated.Password), []byte("new-delegated-password")) != nil {
		t.Fatal("delegated local password must update after VoceChat succeeds")
	}
	record, ok, err := vocechat.NewPlainPasswordStore(storePath).GetUserPassword(delegated.ID)
	if err != nil || !ok || record.VoceChatPassword != "new-delegated-password" {
		t.Fatalf("delegated stored VoceChat password = %#v, ok=%v, err=%v", record, ok, err)
	}
}

func TestVoceChatPasswordChangedAlertIsPerNonPrimaryAccountAndResolves(t *testing.T) {
	setupUserServiceTestDB(t)
	mustCreateUser(t, models.User{Username: "primary", Password: models.HashPassword("primary"), IsAdmin: true})
	ordinary := mustCreateUser(t, models.User{Username: "ordinary-alert", Password: models.HashPassword("ordinary")})
	delegated := mustCreateUser(t, models.User{Username: "delegated-alert", Password: models.HashPassword("delegated"), IsAdmin: true})

	countAlerts := func(userID uint) int64 {
		t.Helper()
		var count int64
		if err := database.DB.Model(&models.UserNotification{}).Where("recipient_user_id = ? AND type = ?", userID, models.UserNotificationTypeVoceChatPasswordChanged).Count(&count).Error; err != nil {
			t.Fatalf("count password-change alerts: %v", err)
		}
		return count
	}

	for _, recipient := range []*models.User{ordinary, delegated} {
		CreateVoceChatPasswordChangedAlertOnce(recipient.ID)
		CreateVoceChatPasswordChangedAlertOnce(recipient.ID)
		if got := countAlerts(recipient.ID); got != 1 {
			t.Fatalf("recipient %d duplicate alerts = %d, want 1", recipient.ID, got)
		}
		if err := database.DB.Model(&models.UserNotification{}).Where("recipient_user_id = ? AND type = ?", recipient.ID, models.UserNotificationTypeVoceChatPasswordChanged).Update("read_at", time.Now()).Error; err != nil {
			t.Fatalf("mark alert read: %v", err)
		}
		CreateVoceChatPasswordChangedAlertOnce(recipient.ID)
		if got := countAlerts(recipient.ID); got != 1 {
			t.Fatalf("read alert must not be recreated for recipient %d, got %d", recipient.ID, got)
		}
		ResolveVoceChatPasswordChangedAlert(recipient.ID)
		if got := countAlerts(recipient.ID); got != 0 {
			t.Fatalf("resolved alerts for recipient %d = %d, want 0", recipient.ID, got)
		}
		CreateVoceChatPasswordChangedAlertOnce(recipient.ID)
		if got := countAlerts(recipient.ID); got != 1 {
			t.Fatalf("a new invalid-credential episode must alert recipient %d, got %d", recipient.ID, got)
		}
	}

	CreateVoceChatPasswordChangedAlertOnce(models.PrimaryAdminUserID)
	if got := countAlerts(models.PrimaryAdminUserID); got != 0 {
		t.Fatalf("primary administrator password-change alerts = %d, want 0", got)
	}
}

func TestLoginWithVoceChatCredentialRejectionDoesNotUseLocalFallback(t *testing.T) {
	setupUserServiceTestDB(t)
	enableVoceChatLoginForTest(t, true)

	mustCreateUser(t, models.User{
		Username:       "bob",
		Password:       models.HashPassword("local-password"),
		VoceChatEmail:  "bob@vc.com",
		VoceChatUserID: "21",
	})

	stubVoceChatPasswordLogin(t, func(ctx context.Context, config vocechat.Config, email, password string) (*vocechat.LoginResponse, error) {
		return nil, &vocechat.APIError{StatusCode: 401, Method: "POST", Path: "/api/token/login"}
	})

	if _, err := Login(dto.LoginDto{Username: "bob", Password: "local-password"}); err == nil || err.Error() != models.PasswordIncorrectMessage {
		t.Fatalf("vc credential rejection should not fall back locally, err=%v", err)
	}

	after := mustGetUserByUsername(t, "bob")
	if after.Token != "" {
		t.Fatalf("failed login should not generate token")
	}
}

func TestLoginWithVoceChatEnabledUsesLocalPasswordForUnboundUser(t *testing.T) {
	setupUserServiceTestDB(t)
	enableVoceChatLoginForTest(t, false)

	mustCreateUser(t, models.User{
		Username: "legacy",
		Password: models.HashPassword("local-password"),
	})

	stubVoceChatPasswordLogin(t, func(ctx context.Context, config vocechat.Config, email, password string) (*vocechat.LoginResponse, error) {
		t.Fatalf("unbound user should not call VoceChat login")
		return nil, nil
	})

	loggedIn, err := Login(dto.LoginDto{Username: "legacy", Password: "local-password"})
	if err != nil {
		t.Fatalf("unbound user should keep local login: %v", err)
	}
	if loggedIn.Token == "" {
		t.Fatalf("local login should generate token")
	}
}

func TestLoginWithVoceChatUnavailableRequiresFallbackSwitch(t *testing.T) {
	for _, tc := range []struct {
		name            string
		fallbackEnabled bool
		wantLogin       bool
	}{
		{name: "fallback disabled", fallbackEnabled: false, wantLogin: false},
		{name: "fallback enabled", fallbackEnabled: true, wantLogin: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setupUserServiceTestDB(t)
			enableVoceChatLoginForTest(t, tc.fallbackEnabled)
			storePath := filepath.Join(t.TempDir(), "plain-passwords.db")
			t.Setenv("NOISE_PLAIN_PASSWORD_STORE", storePath)

			user := mustCreateUser(t, models.User{
				Username:       "carol",
				Password:       models.HashPassword("local-password"),
				VoceChatEmail:  "carol@vc.com",
				VoceChatUserID: "33",
			})

			stubVoceChatPasswordLogin(t, func(ctx context.Context, config vocechat.Config, email, password string) (*vocechat.LoginResponse, error) {
				return nil, context.DeadlineExceeded
			})

			loggedIn, err := Login(dto.LoginDto{Username: "carol", Password: "local-password"})
			if tc.wantLogin {
				if err != nil {
					t.Fatalf("login should fall back locally: %v", err)
				}
				if loggedIn.Token == "" {
					t.Fatalf("fallback login should generate token")
				}
				updated := mustGetUserByUsername(t, "carol")
				if bcrypt.CompareHashAndPassword([]byte(updated.Password), []byte("local-password")) != nil {
					t.Fatalf("local password hash should stay aligned to fallback login password")
				}
				record, ok, err := vocechat.NewPlainPasswordStore(storePath).GetUserPassword(user.ID)
				if err != nil {
					t.Fatalf("read fallback password record: %v", err)
				}
				if !ok || record.LocalFallbackPassword != "local-password" || record.VoceChatPassword != "" || record.LocalFallbackPasswordUpdatedAt == nil || record.VoceChatEmail != "carol@vc.com" || record.VoceChatUserID != "33" {
					t.Fatalf("fallback password record not synced: ok=%v record=%#v", ok, record)
				}
			} else {
				if err == nil || !strings.Contains(err.Error(), "VoceChat 登录校验暂不可用") {
					t.Fatalf("login should fail while fallback disabled, err=%v", err)
				}
			}

			var siteConfig models.SiteConfig
			if err := database.DB.First(&siteConfig).Error; err != nil {
				t.Fatalf("read site config: %v", err)
			}
			if siteConfig.VoceChatLastHealthStatus != "failed" || !strings.Contains(siteConfig.VoceChatLastHealthError, "context deadline exceeded") || siteConfig.VoceChatLastHealthCheckAt == nil {
				t.Fatalf("vc health failure not recorded: %#v", siteConfig)
			}
		})
	}
}

func TestLoginWithBoundVoceChatUserRequiresExplicitFallbackWhenVerificationDisabled(t *testing.T) {
	for _, tc := range []struct {
		name                     string
		voceEnabled              bool
		loginVerificationEnabled bool
		fallbackEnabled          bool
		wantLogin                bool
	}{
		{name: "plugin disabled fallback disabled", voceEnabled: false, loginVerificationEnabled: true, fallbackEnabled: false, wantLogin: false},
		{name: "login verification disabled fallback disabled", voceEnabled: true, loginVerificationEnabled: false, fallbackEnabled: false, wantLogin: false},
		{name: "plugin disabled fallback enabled", voceEnabled: false, loginVerificationEnabled: true, fallbackEnabled: true, wantLogin: true},
		{name: "login verification disabled fallback enabled", voceEnabled: true, loginVerificationEnabled: false, fallbackEnabled: true, wantLogin: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setupUserServiceTestDB(t)
			configureVoceChatForTest(t, tc.voceEnabled, tc.loginVerificationEnabled, tc.fallbackEnabled)

			mustCreateUser(t, models.User{
				Username:       "bound",
				Password:       models.HashPassword("local-password"),
				VoceChatEmail:  "bound@vc.com",
				VoceChatUserID: "44",
			})

			stubVoceChatPasswordLogin(t, func(ctx context.Context, config vocechat.Config, email, password string) (*vocechat.LoginResponse, error) {
				t.Fatalf("vc login should not be called when verification is not active")
				return nil, nil
			})

			loggedIn, err := Login(dto.LoginDto{Username: "bound", Password: "local-password"})
			if tc.wantLogin {
				if err != nil {
					t.Fatalf("login should use explicit local fallback: %v", err)
				}
				if loggedIn == nil || loggedIn.Token == "" {
					t.Fatalf("fallback login should generate token")
				}
			} else if err == nil || !strings.Contains(err.Error(), "本地备用登录") {
				t.Fatalf("bound vc user should not be silently taken over by local auth, err=%v", err)
			}
		})
	}
}

func TestChangePasswordWithVoceChatSyncsRemoteThenLocalPassword(t *testing.T) {
	setupUserServiceTestDB(t)
	enableVoceChatLoginForTest(t, false)
	storePath := filepath.Join(t.TempDir(), "plain-passwords.db")
	t.Setenv("NOISE_PLAIN_PASSWORD_STORE", storePath)

	user := mustCreateUser(t, models.User{
		Username:           "dora",
		Password:           models.HashPassword("stale-local-password"),
		VoceChatEmail:      "dora@vc.com",
		VoceChatUserID:     "55",
		VoceChatUsername:   "Dora",
		VoceChatSyncStatus: models.VoceChatSyncStatusLinked,
	})

	stubVoceChatPasswordLogin(t, func(ctx context.Context, config vocechat.Config, email, password string) (*vocechat.LoginResponse, error) {
		if email != "dora@vc.com" || password != "vc-current-password" {
			t.Fatalf("vc password login = %s/%s", email, password)
		}
		return &vocechat.LoginResponse{Token: "token", User: vocechat.UserInfo{UID: 55, Email: "dora@vc.com", Name: "Dora"}}, nil
	})

	called := false
	stubVoceChatAdminUpdateUser(t, func(ctx context.Context, config vocechat.Config, uid int64, request vocechat.UpdateUserRequest) (*vocechat.User, error) {
		called = true
		if uid != 55 {
			t.Fatalf("vc uid = %d", uid)
		}
		if request.Password == nil || *request.Password != "new-password" {
			t.Fatalf("vc password request = %#v", request.Password)
		}
		if request.Name != nil {
			t.Fatalf("password change should not send name update: %#v", request.Name)
		}
		if !config.LoginVerificationEnabled || !config.HasAdminCredential() {
			t.Fatalf("unexpected vc config: %#v", config)
		}
		return &vocechat.User{UID: 55, Email: "dora@vc.com", Name: "Dora"}, nil
	})

	if err := ChangePasswordWithOld(user, "vc-current-password", "new-password"); err != nil {
		t.Fatalf("change password: %v", err)
	}
	if !called {
		t.Fatalf("vc admin update was not called")
	}

	updated := mustGetUserByUsername(t, "dora")
	if bcrypt.CompareHashAndPassword([]byte(updated.Password), []byte("new-password")) != nil {
		t.Fatalf("local password was not updated after vc sync")
	}
	if updated.VoceChatSyncStatus != models.VoceChatSyncStatusLinked || updated.VoceChatSyncError != "" || updated.VoceChatLastSyncAt == nil {
		t.Fatalf("vc sync status not marked linked: %#v", updated)
	}
	record, ok, err := vocechat.NewPlainPasswordStore(storePath).GetUserPassword(user.ID)
	if err != nil {
		t.Fatalf("read plain password record: %v", err)
	}
	if !ok || record.VoceChatPassword != "new-password" || record.LocalFallbackPassword != "" || record.VoceChatPasswordUpdatedAt == nil || record.Username != "dora" || record.VoceChatUserID != "55" {
		t.Fatalf("plain password record not updated: ok=%v record=%#v", ok, record)
	}
}

func TestChangePasswordWithVoceChatFailureDoesNotUpdateLocalPassword(t *testing.T) {
	setupUserServiceTestDB(t)
	enableVoceChatLoginForTest(t, false)
	storePath := filepath.Join(t.TempDir(), "plain-passwords.db")
	t.Setenv("NOISE_PLAIN_PASSWORD_STORE", storePath)

	user := mustCreateUser(t, models.User{
		Username:       "erin",
		Password:       models.HashPassword("old-password"),
		VoceChatEmail:  "erin@vc.com",
		VoceChatUserID: "56",
	})

	stubVoceChatPasswordLogin(t, func(ctx context.Context, config vocechat.Config, email, password string) (*vocechat.LoginResponse, error) {
		if email != "erin@vc.com" || password != "old-password" {
			t.Fatalf("vc password login = %s/%s", email, password)
		}
		return &vocechat.LoginResponse{Token: "token", User: vocechat.UserInfo{UID: 56, Email: "erin@vc.com", Name: "Erin"}}, nil
	})

	stubVoceChatAdminUpdateUser(t, func(ctx context.Context, config vocechat.Config, uid int64, request vocechat.UpdateUserRequest) (*vocechat.User, error) {
		return nil, errors.New("vc update failed")
	})

	err := ChangePasswordWithOld(user, "old-password", "new-password")
	if err == nil || !strings.Contains(err.Error(), "同步 VoceChat 账号失败") {
		t.Fatalf("expected vc sync error, got %v", err)
	}

	updated := mustGetUserByUsername(t, "erin")
	if bcrypt.CompareHashAndPassword([]byte(updated.Password), []byte("old-password")) != nil {
		t.Fatalf("local password changed even though vc sync failed")
	}
	if updated.VoceChatSyncStatus != models.VoceChatSyncStatusFailed || !strings.Contains(updated.VoceChatSyncError, "vc update failed") {
		t.Fatalf("vc failure not recorded: %#v", updated)
	}
	if _, ok, err := vocechat.NewPlainPasswordStore(storePath).GetUserPassword(user.ID); err != nil || ok {
		t.Fatalf("plain password record should not be created on failed change, ok=%v err=%v", ok, err)
	}
}

func TestChangePasswordWithInternalPasswordWriteFailureRollsBackRemoteAndLocalState(t *testing.T) {
	setupUserServiceTestDB(t)
	enableVoceChatLoginForTest(t, false)
	storePath := filepath.Join(t.TempDir(), "plain-passwords.db")
	t.Setenv("NOISE_PLAIN_PASSWORD_STORE", storePath)
	mustCreateUser(t, models.User{Username: "primary", Password: models.HashPassword("primary-password"), IsAdmin: true})

	user := mustCreateUser(t, models.User{
		Username:       "rollback-user",
		Password:       models.HashPassword("old-password"),
		VoceChatEmail:  "rollback-user@vc.com",
		VoceChatUserID: "58",
	})
	store := vocechat.NewPlainPasswordStore(storePath)
	if err := store.UpsertUserVoceChatPassword(user.ID, user.Username, "old-password", user.VoceChatEmail, user.VoceChatUserID); err != nil {
		t.Fatalf("seed password record: %v", err)
	}

	stubVoceChatPasswordLogin(t, func(_ context.Context, _ vocechat.Config, _, _ string) (*vocechat.LoginResponse, error) {
		return &vocechat.LoginResponse{User: vocechat.UserInfo{UID: 58, Email: user.VoceChatEmail, Name: user.Username}}, nil
	})

	remotePassword := "old-password"
	backupPath := filepath.Join(t.TempDir(), "password-store-backup.db")
	storeMadeUnavailable := false
	stubVoceChatAdminUpdateUser(t, func(_ context.Context, _ vocechat.Config, _ int64, request vocechat.UpdateUserRequest) (*vocechat.User, error) {
		if request.Password == nil {
			t.Fatal("password update must include a password")
		}
		remotePassword = *request.Password
		if !storeMadeUnavailable {
			if err := os.Rename(storePath, backupPath); err != nil {
				t.Fatalf("move password store before injected failure: %v", err)
			}
			if err := os.Mkdir(storePath, 0700); err != nil {
				t.Fatalf("make password store unavailable: %v", err)
			}
			storeMadeUnavailable = true
		}
		return &vocechat.User{UID: 58, Email: user.VoceChatEmail, Name: user.Username}, nil
	})

	err := ChangePasswordWithOld(user, "old-password", "new-password")
	if err == nil {
		t.Fatal("password change must report the injected storage failure")
	}
	if err := os.Remove(storePath); err != nil {
		t.Fatalf("restore password store directory: %v", err)
	}
	if err := os.Rename(backupPath, storePath); err != nil {
		t.Fatalf("restore password store file: %v", err)
	}

	updated := mustGetUserByUsername(t, user.Username)
	if bcrypt.CompareHashAndPassword([]byte(updated.Password), []byte("old-password")) != nil || remotePassword != "old-password" {
		t.Fatal("failed operation partially changed the local password or remote password")
	}
	record, ok, err := store.GetUserPassword(user.ID)
	if err != nil || !ok || record.VoceChatPassword != "old-password" {
		t.Fatalf("password record after failed change: found=%v err=%v", ok, err)
	}
}

func TestChangePasswordWithPrimaryDatabaseWriteFailureRollsBackRemoteAndInternalState(t *testing.T) {
	db := setupUserServiceTestDB(t)
	enableVoceChatLoginForTest(t, false)
	storePath := filepath.Join(t.TempDir(), "plain-passwords.db")
	t.Setenv("NOISE_PLAIN_PASSWORD_STORE", storePath)
	mustCreateUser(t, models.User{Username: "primary", Password: models.HashPassword("primary-password"), IsAdmin: true})

	user := mustCreateUser(t, models.User{
		Username:       "database-failure-user",
		Password:       models.HashPassword("old-password"),
		VoceChatEmail:  "database-failure-user@vc.com",
		VoceChatUserID: "61",
	})
	store := vocechat.NewPlainPasswordStore(storePath)
	if err := store.UpsertUserVoceChatPassword(user.ID, user.Username, "old-password", user.VoceChatEmail, user.VoceChatUserID); err != nil {
		t.Fatalf("seed password record: %v", err)
	}

	remotePassword := "old-password"
	stubVoceChatAdminUpdateUser(t, func(_ context.Context, _ vocechat.Config, _ int64, request vocechat.UpdateUserRequest) (*vocechat.User, error) {
		if request.Password == nil {
			t.Fatal("password update must include a password")
		}
		remotePassword = *request.Password
		return &vocechat.User{UID: 61, Email: user.VoceChatEmail, Name: user.Username}, nil
	})

	failedPasswordWrite := false
	if err := db.Callback().Update().Before("gorm:update").Register("test:fail-password-write-once", func(tx *gorm.DB) {
		if failedPasswordWrite || tx.Statement == nil || tx.Statement.Schema == nil || tx.Statement.Schema.Table != "users" {
			return
		}
		updates, ok := tx.Statement.Dest.(map[string]interface{})
		if _, hasPassword := updates["password"]; ok && hasPassword {
			failedPasswordWrite = true
			tx.AddError(errors.New("forced primary password write failure"))
		}
	}); err != nil {
		t.Fatalf("register primary password write failure: %v", err)
	}

	err := ChangePassword(user, dto.UserInfoDto{Password: "new-password"})
	if err == nil {
		t.Fatal("password change must report the injected primary database failure")
	}
	if !failedPasswordWrite {
		t.Fatal("primary password write fault was not injected")
	}

	updated := mustGetUserByUsername(t, user.Username)
	if bcrypt.CompareHashAndPassword([]byte(updated.Password), []byte("old-password")) != nil || remotePassword != "old-password" {
		t.Fatal("primary database failure partially changed the local password or remote password")
	}
	record, ok, err := store.GetUserPassword(user.ID)
	if err != nil || !ok || record.VoceChatPassword != "old-password" {
		t.Fatalf("password record after primary database failure: found=%v err=%v", ok, err)
	}
	var incompleteCount int64
	if err := database.DB.Model(&models.UserNotification{}).Where("recipient_user_id = ? AND type = ?", user.ID, models.UserNotificationTypePasswordUpdateIncomplete).Count(&incompleteCount).Error; err != nil {
		t.Fatalf("count incomplete-password notifications: %v", err)
	}
	if incompleteCount != 0 {
		t.Fatalf("complete rollback must not create an incomplete-password notification, got %d", incompleteCount)
	}
}

func TestChangePasswordSerializesConcurrentWritesForTheSameUser(t *testing.T) {
	setupUserServiceTestDB(t)
	enableVoceChatLoginForTest(t, false)
	storePath := filepath.Join(t.TempDir(), "plain-passwords.db")
	t.Setenv("NOISE_PLAIN_PASSWORD_STORE", storePath)
	mustCreateUser(t, models.User{Username: "primary", Password: models.HashPassword("primary-password"), IsAdmin: true})

	user := mustCreateUser(t, models.User{
		Username:       "serialized-user",
		Password:       models.HashPassword("old-password"),
		VoceChatEmail:  "serialized-user@vc.com",
		VoceChatUserID: "62",
	})
	if err := vocechat.NewPlainPasswordStore(storePath).UpsertUserVoceChatPassword(user.ID, user.Username, "old-password", user.VoceChatEmail, user.VoceChatUserID); err != nil {
		t.Fatalf("seed password record: %v", err)
	}

	firstStarted := make(chan struct{})
	secondStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	var mu sync.Mutex
	updates := 0
	stubVoceChatAdminUpdateUser(t, func(_ context.Context, _ vocechat.Config, _ int64, request vocechat.UpdateUserRequest) (*vocechat.User, error) {
		if request.Password == nil {
			t.Fatal("password update must include a password")
		}
		mu.Lock()
		updates++
		order := updates
		mu.Unlock()
		if order == 1 {
			close(firstStarted)
			<-releaseFirst
		} else {
			close(secondStarted)
		}
		return &vocechat.User{UID: 62, Email: user.VoceChatEmail, Name: user.Username}, nil
	})

	firstDone := make(chan error, 1)
	secondDone := make(chan error, 1)
	go func() { firstDone <- ChangePassword(user, dto.UserInfoDto{Password: "new-password-one"}) }()
	<-firstStarted
	go func() { secondDone <- ChangePassword(user, dto.UserInfoDto{Password: "new-password-two"}) }()
	select {
	case <-secondStarted:
		t.Fatal("a second password write reached the remote update before the first completed")
	case <-time.After(100 * time.Millisecond):
	}
	close(releaseFirst)
	if err := <-firstDone; err != nil {
		t.Fatalf("first password change: %v", err)
	}
	if err := <-secondDone; err != nil {
		t.Fatalf("second password change: %v", err)
	}
	updated := mustGetUserByUsername(t, user.Username)
	if bcrypt.CompareHashAndPassword([]byte(updated.Password), []byte("new-password-two")) != nil {
		t.Fatal("serialized password changes must leave the second complete write as the final local state")
	}
}

func TestPasswordChangePublicFailureMessagesDoNotExposeInternalPasswordStorage(t *testing.T) {
	tests := []struct {
		name  string
		err   error
		reset bool
		want  string
	}{
		{
			name: "completed rollback for self-service change",
			err:  &passwordUpdateFailure{rolledBack: true},
			want: "密码修改失败，请稍后重试。原密码仍可使用。",
		},
		{
			name:  "completed rollback for administrator reset",
			err:   &passwordUpdateFailure{rolledBack: true},
			reset: true,
			want:  "密码重置失败，请稍后重试。用户原密码未改变。",
		},
		{
			name: "incomplete change",
			err:  &passwordUpdateFailure{incomplete: true},
			want: "密码保存未完成，请重新设置密码。若仍无法登录，请联系1号管理员。",
		},
		{
			name:  "incomplete reset",
			err:   &passwordUpdateFailure{incomplete: true},
			reset: true,
			want:  "密码保存未完成，请重新为该用户设置密码。",
		},
		{
			name: "unexpected internal failure",
			err:  errors.New("internal password storage failure"),
			want: "密码修改失败，请稍后重试。",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := PasswordChangePublicFailureMessage(tc.err, tc.reset)
			if got != tc.want {
				t.Fatalf("public failure message = %q, want %q", got, tc.want)
			}
			if strings.Contains(got, "internal") || strings.Contains(got, "storage") || strings.Contains(got, "数据库") || strings.Contains(got, "凭据") {
				t.Fatalf("public failure message leaked implementation detail: %q", got)
			}
		})
	}
}

func TestChangePasswordWithFailedCompensationMarksOneIncompletePasswordUpdate(t *testing.T) {
	setupUserServiceTestDB(t)
	enableVoceChatLoginForTest(t, false)
	storePath := filepath.Join(t.TempDir(), "plain-passwords.db")
	t.Setenv("NOISE_PLAIN_PASSWORD_STORE", storePath)
	mustCreateUser(t, models.User{Username: "primary", Password: models.HashPassword("primary-password"), IsAdmin: true})

	user := mustCreateUser(t, models.User{
		Username:           "incomplete-user",
		Password:           models.HashPassword("old-password"),
		VoceChatEmail:      "incomplete-user@vc.com",
		VoceChatUserID:     "59",
		VoceChatSyncStatus: models.VoceChatSyncStatusLinked,
	})
	store := vocechat.NewPlainPasswordStore(storePath)
	if err := store.UpsertUserVoceChatPassword(user.ID, user.Username, "old-password", user.VoceChatEmail, user.VoceChatUserID); err != nil {
		t.Fatalf("seed password record: %v", err)
	}

	stubVoceChatPasswordLogin(t, func(_ context.Context, _ vocechat.Config, _, _ string) (*vocechat.LoginResponse, error) {
		return &vocechat.LoginResponse{User: vocechat.UserInfo{UID: 59, Email: user.VoceChatEmail, Name: user.Username}}, nil
	})

	backupPath := filepath.Join(t.TempDir(), "password-store-backup.db")
	updates := 0
	allowRecovery := false
	stubVoceChatAdminUpdateUser(t, func(_ context.Context, _ vocechat.Config, _ int64, request vocechat.UpdateUserRequest) (*vocechat.User, error) {
		updates++
		if request.Password == nil {
			t.Fatal("password update must include a password")
		}
		if updates == 1 {
			if err := os.Rename(storePath, backupPath); err != nil {
				t.Fatalf("move password store before injected failure: %v", err)
			}
			if err := os.Mkdir(storePath, 0700); err != nil {
				t.Fatalf("make password store unavailable: %v", err)
			}
			return &vocechat.User{UID: 59, Email: user.VoceChatEmail, Name: user.Username}, nil
		}
		if !allowRecovery {
			return nil, errors.New("forced remote rollback failure")
		}
		return &vocechat.User{UID: 59, Email: user.VoceChatEmail, Name: user.Username}, nil
	})

	err := ChangePasswordWithOld(user, "old-password", "new-password")
	if err == nil {
		t.Fatal("password change must report an incomplete result")
	}
	failure, ok := err.(*passwordUpdateFailure)
	if !ok || !failure.incomplete {
		t.Fatalf("password change error must be marked incomplete, got %T %v", err, err)
	}
	if err := os.Remove(storePath); err != nil {
		t.Fatalf("restore password store directory: %v", err)
	}
	if err := os.Rename(backupPath, storePath); err != nil {
		t.Fatalf("restore password store file: %v", err)
	}

	updated := mustGetUserByUsername(t, user.Username)
	if updated.VoceChatSyncStatus != models.VoceChatSyncStatusConflicted || updated.VoceChatSyncError != "password_update_incomplete" {
		t.Fatalf("incomplete password update state = %q/%q", updated.VoceChatSyncStatus, updated.VoceChatSyncError)
	}
	var count int64
	if err := database.DB.Model(&models.UserNotification{}).Where("recipient_user_id = ? AND type = ?", user.ID, "password_update_incomplete").Count(&count).Error; err != nil {
		t.Fatalf("count incomplete-password notifications: %v", err)
	}
	if count != 1 {
		t.Fatalf("incomplete-password notifications = %d, want 1", count)
	}

	allowRecovery = true
	if err := ChangePasswordWithOld(user, "old-password", "retry-password"); err != nil {
		t.Fatalf("retry password change after recovery: %v", err)
	}
	updated = mustGetUserByUsername(t, user.Username)
	if updated.VoceChatSyncStatus != models.VoceChatSyncStatusLinked || updated.VoceChatSyncError != "" || bcrypt.CompareHashAndPassword([]byte(updated.Password), []byte("retry-password")) != nil {
		t.Fatal("successful retry must restore a complete password state")
	}
	if err := database.DB.Model(&models.UserNotification{}).Where("recipient_user_id = ? AND type = ?", user.ID, models.UserNotificationTypePasswordUpdateIncomplete).Count(&count).Error; err != nil {
		t.Fatalf("count incomplete-password notifications after retry: %v", err)
	}
	if count != 0 {
		t.Fatalf("successful retry must resolve incomplete-password notifications, got %d", count)
	}
}

func TestLoginWithVoceChatSaveFailureDoesNotIssueLoginOrResolvePasswordAlerts(t *testing.T) {
	setupUserServiceTestDB(t)
	enableVoceChatLoginForTest(t, false)
	storePath := filepath.Join(t.TempDir(), "plain-passwords.db")
	t.Setenv("NOISE_PLAIN_PASSWORD_STORE", storePath)
	mustCreateUser(t, models.User{Username: "primary", Password: models.HashPassword("primary-password"), IsAdmin: true})

	user := mustCreateUser(t, models.User{
		Username:       "login-save-failure",
		Password:       models.HashPassword("old-password"),
		VoceChatEmail:  "login-save-failure@vc.com",
		VoceChatUserID: "60",
	})
	store := vocechat.NewPlainPasswordStore(storePath)
	if err := store.UpsertUserVoceChatPassword(user.ID, user.Username, "old-password", user.VoceChatEmail, user.VoceChatUserID); err != nil {
		t.Fatalf("seed password record: %v", err)
	}
	if err := database.DB.Create(&models.UserNotification{RecipientUserID: user.ID, Type: models.UserNotificationTypeVoceChatPasswordChanged}).Error; err != nil {
		t.Fatalf("seed password-change notification: %v", err)
	}

	backupPath := filepath.Join(t.TempDir(), "password-store-backup.db")
	stubVoceChatPasswordLogin(t, func(_ context.Context, _ vocechat.Config, _, _ string) (*vocechat.LoginResponse, error) {
		if err := os.Rename(storePath, backupPath); err != nil {
			t.Fatalf("move password store before injected failure: %v", err)
		}
		if err := os.Mkdir(storePath, 0700); err != nil {
			t.Fatalf("make password store unavailable: %v", err)
		}
		return &vocechat.LoginResponse{Token: "test-token", User: vocechat.UserInfo{UID: 60, Email: user.VoceChatEmail, Name: user.Username}}, nil
	})

	loggedIn, err := Login(dto.LoginDto{Username: user.Username, Password: "new-password"})
	if err == nil || err.Error() != "账号信息保存失败，请稍后重新登录。若问题持续，请联系1号管理员。" {
		t.Fatalf("login save failure must return the public retry message, result present=%v", loggedIn != nil)
	}
	if loggedIn != nil {
		t.Fatal("login save failure must not establish a login result")
	}
	if err := os.Remove(storePath); err != nil {
		t.Fatalf("restore password store directory: %v", err)
	}
	if err := os.Rename(backupPath, storePath); err != nil {
		t.Fatalf("restore password store file: %v", err)
	}

	updated := mustGetUserByUsername(t, user.Username)
	if bcrypt.CompareHashAndPassword([]byte(updated.Password), []byte("old-password")) != nil || updated.Token != "" || updated.LoginIssuedAt != nil {
		t.Fatal("failed login synchronization must not change local password or establish login state")
	}
	var changedCount, incompleteCount int64
	if err := database.DB.Model(&models.UserNotification{}).Where("recipient_user_id = ? AND type = ?", user.ID, models.UserNotificationTypeVoceChatPasswordChanged).Count(&changedCount).Error; err != nil {
		t.Fatalf("count password-change notifications: %v", err)
	}
	if err := database.DB.Model(&models.UserNotification{}).Where("recipient_user_id = ? AND type = ?", user.ID, models.UserNotificationTypePasswordUpdateIncomplete).Count(&incompleteCount).Error; err != nil {
		t.Fatalf("count incomplete-password notifications: %v", err)
	}
	if changedCount != 1 || incompleteCount != 1 {
		t.Fatalf("failed login must retain existing and add incomplete alerts, changed=%d incomplete=%d", changedCount, incompleteCount)
	}
}

func TestChangePasswordForBoundVoceChatUserRejectsWhenIntegrationDisabled(t *testing.T) {
	for _, fallbackEnabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("fallback %v", fallbackEnabled), func(t *testing.T) {
			setupUserServiceTestDB(t)
			configureVoceChatForTest(t, false, true, fallbackEnabled)
			mustCreateUser(t, models.User{Username: "primary", Password: models.HashPassword("primary"), IsAdmin: true})

			user := mustCreateUser(t, models.User{
				Username:       "frank",
				Password:       models.HashPassword("old-password"),
				VoceChatEmail:  "frank@vc.com",
				VoceChatUserID: "57",
			})

			stubVoceChatPasswordLogin(t, func(ctx context.Context, config vocechat.Config, email, password string) (*vocechat.LoginResponse, error) {
				t.Fatalf("vc password login should not be attempted when integration is disabled")
				return nil, nil
			})
			stubVoceChatAdminUpdateUser(t, func(ctx context.Context, config vocechat.Config, uid int64, request vocechat.UpdateUserRequest) (*vocechat.User, error) {
				t.Fatalf("vc password update should not be attempted when integration is disabled")
				return nil, nil
			})

			err := ChangePasswordWithOld(user, "old-password", "new-password")
			updated := mustGetUserByUsername(t, "frank")

			if err == nil || !strings.Contains(err.Error(), "VoceChat 登录校验暂不可用") {
				t.Fatalf("bound VoceChat user password change should require active VoceChat validation, err=%v", err)
			}
			if bcrypt.CompareHashAndPassword([]byte(updated.Password), []byte("old-password")) != nil {
				t.Fatalf("local password changed while VoceChat validation was unavailable")
			}
		})
	}
}

func TestUpdateUserVoceChatNameSyncFailureDoesNotBlockLocalProfile(t *testing.T) {
	setupUserServiceTestDB(t)
	enableVoceChatLoginForTest(t, false)
	storePath := filepath.Join(t.TempDir(), "plain-passwords.db")
	t.Setenv("NOISE_PLAIN_PASSWORD_STORE", storePath)
	mustCreateUser(t, models.User{Username: "primary", Password: models.HashPassword("primary"), IsAdmin: true})

	user := mustCreateUser(t, models.User{
		Username:           "faye",
		Password:           models.HashPassword("faye-password"),
		VoceChatEmail:      "faye@vc.com",
		VoceChatUserID:     "57",
		VoceChatSyncStatus: models.VoceChatSyncStatusLinked,
	})
	if err := vocechat.NewPlainPasswordStore(storePath).UpsertUserVoceChatPassword(user.ID, user.Username, "faye-password", user.VoceChatEmail, user.VoceChatUserID); err != nil {
		t.Fatalf("seed plain password record: %v", err)
	}

	stubVoceChatAdminUpdateUser(t, func(ctx context.Context, config vocechat.Config, uid int64, request vocechat.UpdateUserRequest) (*vocechat.User, error) {
		if uid != 57 {
			t.Fatalf("vc uid = %d", uid)
		}
		if request.Name == nil || *request.Name != "faye_new" {
			t.Fatalf("vc name request = %#v", request.Name)
		}
		if request.Password != nil {
			t.Fatalf("profile rename should not send password update")
		}
		return nil, errors.New("vc rename failed")
	})

	if err := UpdateUser(user, dto.UserInfoDto{Username: "faye_new"}); err != nil {
		t.Fatalf("profile update should not fail when vc name sync fails: %v", err)
	}

	updated := mustGetUserByUsername(t, "faye_new")
	if updated.VoceChatSyncStatus != models.VoceChatSyncStatusFailed || !strings.Contains(updated.VoceChatSyncError, "vc rename failed") {
		t.Fatalf("vc rename failure not recorded: %#v", updated)
	}
	record, ok, err := vocechat.NewPlainPasswordStore(storePath).GetUserPassword(user.ID)
	if err != nil {
		t.Fatalf("read plain password record: %v", err)
	}
	if !ok || record.Username != "faye_new" || record.VoceChatPassword != "faye-password" || record.LocalFallbackPassword != "" || record.VoceChatUserID != "57" {
		t.Fatalf("plain password metadata not updated: ok=%v record=%#v", ok, record)
	}
}

func TestGetStatusIncludesUserAvatarURLs(t *testing.T) {
	setupUserServiceTestDB(t)

	mustCreateUser(t, models.User{
		Username:  "admin",
		Password:  models.HashPassword("admin"),
		IsAdmin:   true,
		Token:     models.GenerateToken(32),
		AvatarURL: "https://example.com/admin-avatar.png",
	})
	alice := mustCreateUser(t, models.User{
		Username:      "alice",
		Password:      models.HashPassword("alice"),
		Token:         models.GenerateToken(32),
		AvatarURL:     "/api/images/alice.png",
		VoceChatEmail: "alice@vc.com",
	})

	status, err := GetStatus(0)
	if err != nil {
		t.Fatalf("get status: %v", err)
	}

	avatars := map[string]string{}
	voceChatEmails := map[string]string{}
	for _, user := range status.Users {
		avatars[user.Username] = user.AvatarURL
		voceChatEmails[user.Username] = user.VoceChatEmail
	}
	if avatars["admin"] != "https://example.com/admin-avatar.png" {
		t.Fatalf("admin avatar missing from status, got %q", avatars["admin"])
	}
	if avatars["alice"] != "/api/images/alice.png" {
		t.Fatalf("alice avatar missing from status, got %q", avatars["alice"])
	}
	if voceChatEmails["alice"] != "" {
		t.Fatalf("anonymous status leaked alice vc email %q", voceChatEmails["alice"])
	}

	aliceStatus, err := GetStatus(alice.ID)
	if err != nil {
		t.Fatalf("get alice status: %v", err)
	}
	for _, user := range aliceStatus.Users {
		if user.Username == "alice" && user.VoceChatEmail != "alice@vc.com" {
			t.Fatalf("alice should see own vc email, got %q", user.VoceChatEmail)
		}
		if user.Username == "admin" && user.VoceChatEmail != "" {
			t.Fatalf("alice should not see admin vc email, got %q", user.VoceChatEmail)
		}
	}
}

func TestGetStatusScopesVoceChatFieldsByViewerRole(t *testing.T) {
	setupUserServiceTestDB(t)

	primary := mustCreateUser(t, models.User{
		Username: "primary", Password: models.HashPassword("primary"), IsAdmin: true,
		VoceChatEmail: "primary@vc.example", VoceChatNotificationEnabled: true,
	})
	delegatedWithUsersView := mustCreateUser(t, models.User{
		Username: "delegated-view", Password: models.HashPassword("delegated-view"), IsAdmin: true,
		VoceChatEmail: "delegated-view@vc.example", VoceChatNotificationEnabled: false,
	})
	delegatedWithoutUsersView := mustCreateUser(t, models.User{
		Username: "delegated-no-view", Password: models.HashPassword("delegated-no-view"), IsAdmin: true,
		VoceChatEmail: "delegated-no-view@vc.example", VoceChatNotificationEnabled: true,
	})
	ordinaryA := mustCreateUser(t, models.User{
		Username: "ordinary-a", Password: models.HashPassword("ordinary-a"),
		VoceChatEmail: "ordinary-a@vc.example", VoceChatNotificationEnabled: false,
	})
	ordinaryB := mustCreateUser(t, models.User{
		Username: "ordinary-b", Password: models.HashPassword("ordinary-b"),
		VoceChatEmail: "ordinary-b@vc.example", VoceChatNotificationEnabled: true,
	})
	if primary.ID != models.PrimaryAdminUserID {
		t.Fatalf("primary user id = %d, want %d", primary.ID, models.PrimaryAdminUserID)
	}
	if err := database.DB.Create(&models.AdminCapabilityGrant{
		UserID: delegatedWithUsersView.ID, Capability: "users.view", GrantedByUserID: primary.ID,
	}).Error; err != nil {
		t.Fatalf("grant users.view: %v", err)
	}

	allUsers := []*models.User{primary, delegatedWithUsersView, delegatedWithoutUsersView, ordinaryA, ordinaryB}
	assertStatus := func(t *testing.T, viewerID uint, visibleEmails map[uint]bool, visibleNotifications map[uint]bool) {
		t.Helper()
		status, err := GetStatus(viewerID)
		if err != nil {
			t.Fatalf("GetStatus(%d): %v", viewerID, err)
		}
		byID := make(map[uint]models.UserStatus, len(status.Users))
		for _, user := range status.Users {
			byID[user.ID] = user
		}
		for _, expected := range allUsers {
			actual, ok := byID[expected.ID]
			if !ok {
				t.Fatalf("viewer %d missing user %d", viewerID, expected.ID)
			}
			if got, want := actual.VoceChatEmail != "", visibleEmails[expected.ID]; got != want {
				t.Errorf("viewer %d email visibility for user %d = %v, want %v", viewerID, expected.ID, got, want)
			}
			if got, want := actual.VoceChatNotificationEnabled != nil, visibleNotifications[expected.ID]; got != want {
				t.Errorf("viewer %d notification visibility for user %d = %v, want %v", viewerID, expected.ID, got, want)
			} else if got && *actual.VoceChatNotificationEnabled != expected.VoceChatNotificationEnabled {
				t.Errorf("viewer %d notification value for user %d = %v, want %v", viewerID, expected.ID, *actual.VoceChatNotificationEnabled, expected.VoceChatNotificationEnabled)
			}
		}
	}

	selfFields := func(user *models.User) map[uint]bool { return map[uint]bool{user.ID: true} }
	allNonPrimaryEmails := make(map[uint]bool, len(allUsers)-1)
	allFields := make(map[uint]bool, len(allUsers))
	for _, user := range allUsers {
		allFields[user.ID] = true
		if user.ID != models.PrimaryAdminUserID {
			allNonPrimaryEmails[user.ID] = true
		}
	}

	assertStatus(t, 0, map[uint]bool{}, map[uint]bool{})
	assertStatus(t, ordinaryA.ID, selfFields(ordinaryA), selfFields(ordinaryA))
	assertStatus(t, delegatedWithoutUsersView.ID, selfFields(delegatedWithoutUsersView), selfFields(delegatedWithoutUsersView))
	assertStatus(t, delegatedWithUsersView.ID, allNonPrimaryEmails, selfFields(delegatedWithUsersView))
	assertStatus(t, primary.ID, allNonPrimaryEmails, allFields)

	if err := database.DB.Where("user_id = ? AND capability = ?", delegatedWithUsersView.ID, "users.view").Delete(&models.AdminCapabilityGrant{}).Error; err != nil {
		t.Fatalf("revoke users.view: %v", err)
	}
	assertStatus(t, delegatedWithUsersView.ID, selfFields(delegatedWithUsersView), selfFields(delegatedWithUsersView))
}

func TestGetStatusCountsOnlyCommentsVisibleToViewer(t *testing.T) {
	setupUserServiceTestDB(t)

	admin := mustCreateUser(t, models.User{Username: "admin", Password: models.HashPassword("admin"), IsAdmin: true, Token: models.GenerateToken(32)})
	alice := mustCreateUser(t, models.User{Username: "alice", Password: models.HashPassword("alice"), Token: models.GenerateToken(32)})
	bob := mustCreateUser(t, models.User{Username: "bob", Password: models.HashPassword("bob"), Token: models.GenerateToken(32)})
	publicMessage := models.Message{Content: "public", UserID: alice.ID, Visibility: MessageVisibilityPublic}
	privateMessage := models.Message{Content: "private", UserID: bob.ID, Visibility: MessageVisibilityPrivate, Private: true}
	for _, message := range []*models.Message{&publicMessage, &privateMessage} {
		if err := database.DB.Create(message).Error; err != nil {
			t.Fatalf("create message: %v", err)
		}
	}
	publicRoot := models.Comment{MessageID: publicMessage.ID, UserID: &bob.ID, Content: "public root", Visibility: "public"}
	privateRoot := models.Comment{MessageID: publicMessage.ID, UserID: &bob.ID, Content: "private root", Visibility: "private"}
	privateMessageRoot := models.Comment{MessageID: privateMessage.ID, UserID: &bob.ID, Content: "private message root", Visibility: "private"}
	for _, comment := range []*models.Comment{&publicRoot, &privateRoot, &privateMessageRoot} {
		if err := database.DB.Create(comment).Error; err != nil {
			t.Fatalf("create comment: %v", err)
		}
	}
	parentID := publicRoot.ID
	publicReply := models.Comment{MessageID: publicMessage.ID, UserID: &alice.ID, ParentID: &parentID, Content: "public reply", Visibility: "public"}
	if err := database.DB.Create(&publicReply).Error; err != nil {
		t.Fatalf("create reply: %v", err)
	}

	anonymous, err := GetStatus(0)
	if err != nil {
		t.Fatalf("get anonymous status: %v", err)
	}
	if anonymous.TotalComments != 1 || anonymous.TotalReplies != 1 {
		t.Fatalf("anonymous counts = comments %d replies %d, want 1/1", anonymous.TotalComments, anonymous.TotalReplies)
	}
	aliceStatus, err := GetStatus(alice.ID)
	if err != nil {
		t.Fatalf("get alice status: %v", err)
	}
	if aliceStatus.TotalComments != 2 || aliceStatus.TotalReplies != 1 {
		t.Fatalf("alice counts = comments %d replies %d, want 2/1", aliceStatus.TotalComments, aliceStatus.TotalReplies)
	}
	adminStatus, err := GetStatus(admin.ID)
	if err != nil {
		t.Fatalf("get admin status: %v", err)
	}
	if adminStatus.TotalComments != 3 || adminStatus.TotalReplies != 1 {
		t.Fatalf("primary admin counts = comments %d replies %d, want 3/1", adminStatus.TotalComments, adminStatus.TotalReplies)
	}
}

func TestGetStatusUsesViewerScopedDashboardCounts(t *testing.T) {
	setupUserServiceTestDB(t)

	admin := mustCreateUser(t, models.User{Username: "admin", Password: models.HashPassword("admin"), IsAdmin: true, Token: models.GenerateToken(32)})
	alice := mustCreateUser(t, models.User{Username: "alice", Password: models.HashPassword("alice"), Token: models.GenerateToken(32)})
	bob := mustCreateUser(t, models.User{Username: "bob", Password: models.HashPassword("bob"), Token: models.GenerateToken(32)})

	adminMessage := models.Message{Content: "admin message", UserID: admin.ID, Username: admin.Username}
	aliceMessage := models.Message{Content: "alice message", UserID: alice.ID, Username: alice.Username}
	bobMessage := models.Message{Content: "bob message", UserID: bob.ID, Username: bob.Username}
	if err := database.DB.Create(&adminMessage).Error; err != nil {
		t.Fatalf("create admin message: %v", err)
	}
	if err := database.DB.Create(&aliceMessage).Error; err != nil {
		t.Fatalf("create alice message: %v", err)
	}
	if err := database.DB.Create(&bobMessage).Error; err != nil {
		t.Fatalf("create bob message: %v", err)
	}

	bobCommentOnAlice := models.Comment{MessageID: aliceMessage.ID, UserID: &bob.ID, Content: "bob comment", Visibility: "public"}
	aliceCommentOnAlice := models.Comment{MessageID: aliceMessage.ID, UserID: &alice.ID, Content: "self comment", Visibility: "public"}
	aliceCommentOnBob := models.Comment{MessageID: bobMessage.ID, UserID: &alice.ID, Content: "alice comment", Visibility: "public"}
	bobCommentOnAdmin := models.Comment{MessageID: adminMessage.ID, UserID: &bob.ID, Content: "bob admin comment", Visibility: "public"}
	for _, comment := range []*models.Comment{&bobCommentOnAlice, &aliceCommentOnAlice, &aliceCommentOnBob, &bobCommentOnAdmin} {
		if err := database.DB.Create(comment).Error; err != nil {
			t.Fatalf("create comment: %v", err)
		}
	}
	parentID := aliceCommentOnBob.ID
	bobReplyToAlice := models.Comment{MessageID: bobMessage.ID, UserID: &bob.ID, ParentID: &parentID, Content: "bob reply", Visibility: "public"}
	aliceReplyToSelf := models.Comment{MessageID: bobMessage.ID, UserID: &alice.ID, ParentID: &parentID, Content: "self reply", Visibility: "public"}
	for _, reply := range []*models.Comment{&bobReplyToAlice, &aliceReplyToSelf} {
		if err := database.DB.Create(reply).Error; err != nil {
			t.Fatalf("create reply: %v", err)
		}
	}

	aliceStatus, err := GetStatus(alice.ID)
	if err != nil {
		t.Fatalf("get alice status: %v", err)
	}
	if aliceStatus.TotalMessages != 1 {
		t.Fatalf("alice total messages = %d, want 1", aliceStatus.TotalMessages)
	}
	if aliceStatus.ReceivedComments != 1 {
		t.Fatalf("alice received comments = %d, want 1", aliceStatus.ReceivedComments)
	}
	if aliceStatus.ReceivedReplies != 1 {
		t.Fatalf("alice received replies = %d, want 1", aliceStatus.ReceivedReplies)
	}

	adminStatus, err := GetStatus(admin.ID)
	if err != nil {
		t.Fatalf("get admin status: %v", err)
	}
	if adminStatus.TotalMessages != 3 {
		t.Fatalf("admin total messages = %d, want 3", adminStatus.TotalMessages)
	}
	if adminStatus.ReceivedComments != 1 {
		t.Fatalf("admin received comments = %d, want 1", adminStatus.ReceivedComments)
	}
	if adminStatus.TotalUsers != 3 {
		t.Fatalf("total users = %d, want 3", adminStatus.TotalUsers)
	}
	if adminStatus.TotalComments != 4 {
		t.Fatalf("total comments = %d, want 4", adminStatus.TotalComments)
	}
	if adminStatus.TotalReplies != 2 {
		t.Fatalf("total replies = %d, want 2", adminStatus.TotalReplies)
	}
}

func TestGetStatusSeparatesDashboardMetricsAndSharesSecuritySummary(t *testing.T) {
	setupUserServiceTestDB(t)

	admin := mustCreateUser(t, models.User{Username: "admin", Password: models.HashPassword("admin"), IsAdmin: true, Token: models.GenerateToken(32)})
	alice := mustCreateUser(t, models.User{Username: "alice", Password: models.HashPassword("alice"), Token: models.GenerateToken(32)})
	bob := mustCreateUser(t, models.User{Username: "bob", Password: models.HashPassword("bob"), Token: models.GenerateToken(32)})

	adminNote := models.Message{Content: "admin note", UserID: admin.ID, Username: admin.Username, Visibility: MessageVisibilityPublic}
	aliceNote := models.Message{Content: "alice note", UserID: alice.ID, Username: alice.Username, Visibility: MessageVisibilityPublic}
	bobNote := models.Message{Content: "bob note", UserID: bob.ID, Username: bob.Username, Visibility: MessageVisibilityPublic}
	guestbook := models.Message{Content: models.CanonicalGuestbookContent, UserID: admin.ID, Username: admin.Username, Visibility: MessageVisibilityPublic, IsGuestbook: true}
	for _, message := range []*models.Message{&adminNote, &aliceNote, &bobNote, &guestbook} {
		if err := database.DB.Create(message).Error; err != nil {
			t.Fatalf("create message: %v", err)
		}
	}

	for _, like := range []*models.MessageLike{
		{MessageID: aliceNote.ID, UserID: &bob.ID},
		{MessageID: aliceNote.ID, UserID: &alice.ID},
		{MessageID: adminNote.ID, UserID: &bob.ID},
		{MessageID: guestbook.ID, UserID: &bob.ID},
	} {
		if err := database.DB.Create(like).Error; err != nil {
			t.Fatalf("create like: %v", err)
		}
	}

	bobComment := models.Comment{MessageID: aliceNote.ID, UserID: &bob.ID, Content: "bob comment", Visibility: "public"}
	aliceSelfComment := models.Comment{MessageID: aliceNote.ID, UserID: &alice.ID, Content: "self comment", Visibility: "public"}
	aliceParent := models.Comment{MessageID: bobNote.ID, UserID: &alice.ID, Content: "alice parent", Visibility: "public"}
	for _, comment := range []*models.Comment{&bobComment, &aliceSelfComment, &aliceParent} {
		if err := database.DB.Create(comment).Error; err != nil {
			t.Fatalf("create comment: %v", err)
		}
	}
	parentID := aliceParent.ID
	bobReply := models.Comment{MessageID: bobNote.ID, UserID: &bob.ID, ParentID: &parentID, Content: "bob reply", Visibility: "public"}
	if err := database.DB.Create(&bobReply).Error; err != nil {
		t.Fatalf("create reply: %v", err)
	}
	for _, entry := range []*models.Comment{
		{MessageID: guestbook.ID, UserID: &bob.ID, Content: "bob guestbook", Visibility: "public"},
		{MessageID: guestbook.ID, UserID: &alice.ID, Content: "alice guestbook", Visibility: "public"},
		{MessageID: guestbook.ID, UserID: &admin.ID, Content: "admin guestbook", Visibility: "public"},
	} {
		if err := database.DB.Create(entry).Error; err != nil {
			t.Fatalf("create guestbook entry: %v", err)
		}
	}
	if err := database.DB.Create(&models.SecurityConfig{AutoBanEnabled: true, AutoBanThreshold: 7}).Error; err != nil {
		t.Fatalf("create security config: %v", err)
	}

	aliceStatus, err := GetStatus(alice.ID)
	if err != nil {
		t.Fatalf("get alice status: %v", err)
	}
	if aliceStatus.TotalMessages != 1 || aliceStatus.PersonalMessages != 1 {
		t.Fatalf("alice note counts = total %d personal %d, want 1/1", aliceStatus.TotalMessages, aliceStatus.PersonalMessages)
	}
	if aliceStatus.ReceivedLikes != 1 || aliceStatus.ReceivedComments != 1 || aliceStatus.ReceivedReplies != 1 || aliceStatus.ReceivedGuestbook != 0 {
		t.Fatalf("alice interactions = likes %d comments %d replies %d guestbook %d, want 1/1/1/0", aliceStatus.ReceivedLikes, aliceStatus.ReceivedComments, aliceStatus.ReceivedReplies, aliceStatus.ReceivedGuestbook)
	}
	if aliceStatus.AutoBanEnabled == nil || !*aliceStatus.AutoBanEnabled {
		t.Fatalf("ordinary user should receive enabled auto-ban summary, got %#v", aliceStatus.AutoBanEnabled)
	}

	adminStatus, err := GetStatus(admin.ID)
	if err != nil {
		t.Fatalf("get admin status: %v", err)
	}
	if adminStatus.TotalMessages != 3 || adminStatus.PersonalMessages != 1 {
		t.Fatalf("admin note counts = total %d personal %d, want 3/1", adminStatus.TotalMessages, adminStatus.PersonalMessages)
	}
	if adminStatus.TotalComments != 3 || adminStatus.TotalReplies != 1 || adminStatus.TotalGuestbook != 3 {
		t.Fatalf("admin feedback = comments %d replies %d guestbook %d, want 3/1/3", adminStatus.TotalComments, adminStatus.TotalReplies, adminStatus.TotalGuestbook)
	}
	if adminStatus.ReceivedLikes != 1 || adminStatus.ReceivedGuestbook != 2 {
		t.Fatalf("admin interactions = likes %d guestbook %d, want 1/2", adminStatus.ReceivedLikes, adminStatus.ReceivedGuestbook)
	}
	if adminStatus.AutoBanEnabled == nil || !*adminStatus.AutoBanEnabled {
		t.Fatalf("admin should receive enabled auto-ban summary, got %#v", adminStatus.AutoBanEnabled)
	}
}

func TestCreateUserNotificationsFollowRecipientRules(t *testing.T) {
	setupUserServiceTestDB(t)

	admin := mustCreateUser(t, models.User{Username: "admin", Password: models.HashPassword("admin"), IsAdmin: true, Token: models.GenerateToken(32)})
	adminTwo := mustCreateUser(t, models.User{Username: "admin2", Password: models.HashPassword("admin2"), IsAdmin: true, Token: models.GenerateToken(32)})
	alice := mustCreateUser(t, models.User{Username: "alice", Password: models.HashPassword("alice"), Token: models.GenerateToken(32)})
	bob := mustCreateUser(t, models.User{Username: "bob", Password: models.HashPassword("bob"), Token: models.GenerateToken(32)})

	aliceMessage := models.Message{Content: "alice message", UserID: alice.ID, Username: alice.Username}
	bobMessage := models.Message{Content: "bob message", UserID: bob.ID, Username: bob.Username}
	guestbookMessage := models.Message{Content: models.CanonicalGuestbookContent, UserID: alice.ID, Username: alice.Username}
	for _, message := range []*models.Message{&aliceMessage, &bobMessage, &guestbookMessage} {
		if err := database.DB.Create(message).Error; err != nil {
			t.Fatalf("create message: %v", err)
		}
	}

	bobComment := models.Comment{MessageID: aliceMessage.ID, UserID: &bob.ID, Content: "bob comment", Visibility: "public"}
	if err := database.DB.Create(&bobComment).Error; err != nil {
		t.Fatalf("create bob comment: %v", err)
	}
	if err := CreateNotificationsForComment(aliceMessage, bobComment, nil); err != nil {
		t.Fatalf("create comment notification: %v", err)
	}

	aliceSelfComment := models.Comment{MessageID: aliceMessage.ID, UserID: &alice.ID, Content: "self comment", Visibility: "public"}
	if err := database.DB.Create(&aliceSelfComment).Error; err != nil {
		t.Fatalf("create self comment: %v", err)
	}
	if err := CreateNotificationsForComment(aliceMessage, aliceSelfComment, nil); err != nil {
		t.Fatalf("create self comment notification: %v", err)
	}

	aliceParentComment := models.Comment{MessageID: bobMessage.ID, UserID: &alice.ID, Content: "alice comment", Visibility: "public"}
	if err := database.DB.Create(&aliceParentComment).Error; err != nil {
		t.Fatalf("create parent comment: %v", err)
	}
	parentID := aliceParentComment.ID
	bobReply := models.Comment{MessageID: bobMessage.ID, UserID: &bob.ID, ParentID: &parentID, Content: "bob reply", Visibility: "public"}
	if err := database.DB.Create(&bobReply).Error; err != nil {
		t.Fatalf("create reply: %v", err)
	}
	if err := CreateNotificationsForComment(bobMessage, bobReply, &aliceParentComment); err != nil {
		t.Fatalf("create reply notification: %v", err)
	}

	bobGuestbookComment := models.Comment{MessageID: guestbookMessage.ID, UserID: &bob.ID, Content: "guestbook", Visibility: "public"}
	adminGuestbookComment := models.Comment{MessageID: guestbookMessage.ID, UserID: &admin.ID, Content: "admin guestbook", Visibility: "public"}
	for _, comment := range []*models.Comment{&bobGuestbookComment, &adminGuestbookComment} {
		if err := database.DB.Create(comment).Error; err != nil {
			t.Fatalf("create guestbook comment: %v", err)
		}
		if err := CreateNotificationsForComment(guestbookMessage, *comment, nil); err != nil {
			t.Fatalf("create guestbook notification: %v", err)
		}
	}
	if err := CreateNotificationsForComment(guestbookMessage, bobGuestbookComment, nil); err != nil {
		t.Fatalf("dedupe guestbook notification: %v", err)
	}

	if err := CreateNotificationForLike(aliceMessage.ID, bob.ID); err != nil {
		t.Fatalf("create like notification: %v", err)
	}
	if err := CreateNotificationForLike(aliceMessage.ID, bob.ID); err != nil {
		t.Fatalf("dedupe like notification: %v", err)
	}

	assertNotificationCount := func(recipient uint, notificationType string, want int64) {
		t.Helper()
		var got int64
		if err := database.DB.Model(&models.UserNotification{}).Where("recipient_user_id = ? AND type = ?", recipient, notificationType).Count(&got).Error; err != nil {
			t.Fatalf("count notifications: %v", err)
		}
		if got != want {
			t.Fatalf("recipient %d type %s count = %d, want %d", recipient, notificationType, got, want)
		}
	}

	assertNotificationCount(alice.ID, models.UserNotificationTypeComment, 1)
	assertNotificationCount(alice.ID, models.UserNotificationTypeReply, 1)
	assertNotificationCount(alice.ID, models.UserNotificationTypeLike, 1)
	assertNotificationCount(bob.ID, models.UserNotificationTypeReply, 0)
	assertNotificationCount(admin.ID, models.UserNotificationTypeGuestbook, 1)
	assertNotificationCount(adminTwo.ID, models.UserNotificationTypeGuestbook, 0)
}

func TestCommentThreadVisibilityFollowsAncestorRestrictions(t *testing.T) {
	setupUserServiceTestDB(t)

	alice := mustCreateUser(t, models.User{Username: "thread-alice", Password: models.HashPassword("alice"), Token: models.GenerateToken(32)})
	bob := mustCreateUser(t, models.User{Username: "thread-bob", Password: models.HashPassword("bob"), Token: models.GenerateToken(32)})
	charlie := mustCreateUser(t, models.User{Username: "thread-charlie", Password: models.HashPassword("charlie"), Token: models.GenerateToken(32)})
	message := models.Message{Content: "public note", UserID: alice.ID, Username: alice.Username, Visibility: MessageVisibilityPublic}
	if err := database.DB.Create(&message).Error; err != nil {
		t.Fatalf("create message: %v", err)
	}

	bobRoot := models.Comment{MessageID: message.ID, UserID: &bob.ID, Content: "bob root", Visibility: "public"}
	if err := database.DB.Create(&bobRoot).Error; err != nil {
		t.Fatalf("create root comment: %v", err)
	}
	rootID := bobRoot.ID
	charlieReply := models.Comment{MessageID: message.ID, UserID: &charlie.ID, ParentID: &rootID, Content: "charlie reply", Visibility: "users"}
	if err := database.DB.Create(&charlieReply).Error; err != nil {
		t.Fatalf("create charlie reply: %v", err)
	}
	bobRoot.Visibility = "private"
	if err := database.DB.Save(&bobRoot).Error; err != nil {
		t.Fatalf("make root comment private: %v", err)
	}

	commentMap, err := LoadCommentMapForMessage(message.ID)
	if err != nil {
		t.Fatalf("load comment map: %v", err)
	}
	if CanViewCommentInThread(message, charlieReply, commentMap, charlie.ID, true, false) {
		t.Fatalf("charlie should not see a reply hidden by bob's private root comment")
	}
	if !CanViewCommentInThread(message, charlieReply, commentMap, bob.ID, true, false) {
		t.Fatalf("bob should see replies inside his private root comment")
	}
	if !CanViewCommentInThread(message, charlieReply, commentMap, alice.ID, true, false) {
		t.Fatalf("message author should see replies inside a private root comment on their note")
	}

	parentID := charlieReply.ID
	bobFollowup := models.Comment{MessageID: message.ID, UserID: &bob.ID, ParentID: &parentID, Content: "bob followup", Visibility: "users"}
	if err := database.DB.Create(&bobFollowup).Error; err != nil {
		t.Fatalf("create bob followup: %v", err)
	}
	if err := CreateNotificationsForComment(message, bobFollowup, &charlieReply); err != nil {
		t.Fatalf("create followup notification: %v", err)
	}
	var charlieReplyNotifications int64
	if err := database.DB.Model(&models.UserNotification{}).Where("recipient_user_id = ? AND type = ?", charlie.ID, models.UserNotificationTypeReply).Count(&charlieReplyNotifications).Error; err != nil {
		t.Fatalf("count charlie notifications: %v", err)
	}
	if charlieReplyNotifications != 0 {
		t.Fatalf("charlie reply notifications = %d, want 0", charlieReplyNotifications)
	}
}

func lifeCountdownFrontendSettings(t *testing.T, viewerUserID uint) map[string]interface{} {
	t.Helper()

	config, err := GetFrontendConfig(viewerUserID)
	if err != nil {
		t.Fatalf("get frontend config: %v", err)
	}
	frontendSettings, ok := config["frontendSettings"].(map[string]interface{})
	if !ok {
		t.Fatalf("frontendSettings has type %T", config["frontendSettings"])
	}
	return frontendSettings
}

func assertLifeCountdownFrontendSettings(t *testing.T, frontendSettings map[string]interface{}, enabled bool, birthDate string, lifeExpectancyYears int) {
	t.Helper()

	if got, ok := frontendSettings["lifeCountdownEnabled"].(bool); !ok || got != enabled {
		t.Fatalf("lifeCountdownEnabled = %#v, want %v", frontendSettings["lifeCountdownEnabled"], enabled)
	}
	if got, ok := frontendSettings["lifeCountdownBirthDate"].(string); !ok || got != birthDate {
		t.Fatalf("lifeCountdownBirthDate = %#v, want %q", frontendSettings["lifeCountdownBirthDate"], birthDate)
	}
	if got, ok := frontendSettings["lifeExpectancyYears"].(int); !ok || got != lifeExpectancyYears {
		t.Fatalf("lifeExpectancyYears = %#v, want %d", frontendSettings["lifeExpectancyYears"], lifeExpectancyYears)
	}
}

func TestGetFrontendConfigUsesViewerScopedLifeCountdown(t *testing.T) {
	db := setupUserServiceTestDB(t)
	admin := mustCreateUser(t, models.User{Username: "admin", Password: models.HashPassword("admin"), IsAdmin: true, Token: models.GenerateToken(32)})
	alice := mustCreateUser(t, models.User{Username: "alice", Password: models.HashPassword("alice"), Token: models.GenerateToken(32)})

	if err := db.Create(&models.Setting{AllowRegistration: true}).Error; err != nil {
		t.Fatalf("create setting: %v", err)
	}
	if err := db.Create(&models.SiteConfig{
		SiteTitle:                  "Test Site",
		LifeCountdownEnabled:       true,
		LifeCountdownBirthDate:     "1970-01-02",
		LifeExpectancyYears:        90,
		CalendarEnabled:            true,
		TimeEnabled:                true,
		HitokotoEnabled:            true,
		CommentEmailAdminNotifyAll: true,
	}).Error; err != nil {
		t.Fatalf("create site config: %v", err)
	}

	if err := UpdateUserLifeCountdownConfig(admin.ID, map[string]interface{}{
		"lifeCountdownEnabled":   true,
		"lifeCountdownBirthDate": "1980-03-04",
		"lifeExpectancyYears":    float64(88),
	}); err != nil {
		t.Fatalf("save admin life countdown: %v", err)
	}

	assertLifeCountdownFrontendSettings(t, lifeCountdownFrontendSettings(t, 0), true, "1970-01-02", 90)
	assertLifeCountdownFrontendSettings(t, lifeCountdownFrontendSettings(t, admin.ID), true, "1980-03-04", 88)
	assertLifeCountdownFrontendSettings(t, lifeCountdownFrontendSettings(t, alice.ID), true, "1970-01-02", 90)

	if err := UpdateUserLifeCountdownConfig(alice.ID, map[string]interface{}{
		"lifeCountdownEnabled":   true,
		"lifeCountdownBirthDate": "1995-06-07",
		"lifeExpectancyYears":    "75",
	}); err != nil {
		t.Fatalf("save alice life countdown: %v", err)
	}
	assertLifeCountdownFrontendSettings(t, lifeCountdownFrontendSettings(t, alice.ID), true, "1995-06-07", 75)

	if err := db.Where("user_id = ?", admin.ID).Delete(&models.UserLifeCountdownConfig{}).Error; err != nil {
		t.Fatalf("delete admin life countdown: %v", err)
	}
	assertLifeCountdownFrontendSettings(t, lifeCountdownFrontendSettings(t, admin.ID), true, "1970-01-02", 90)
}

func hitokotoEnabledForViewer(t *testing.T, viewerUserID uint) bool {
	t.Helper()
	settings := lifeCountdownFrontendSettings(t, viewerUserID)
	enabled, ok := settings["hitokotoEnabled"].(bool)
	if !ok {
		t.Fatalf("hitokotoEnabled has type %T", settings["hitokotoEnabled"])
	}
	return enabled
}

func TestGetFrontendConfigUsesViewerScopedHitokotoPreference(t *testing.T) {
	db := setupUserServiceTestDB(t)
	admin := mustCreateUser(t, models.User{Username: "admin-hitokoto", Password: models.HashPassword("admin"), IsAdmin: true, Token: models.GenerateToken(32)})
	alice := mustCreateUser(t, models.User{Username: "alice-hitokoto", Password: models.HashPassword("alice"), Token: models.GenerateToken(32)})

	if err := db.Create(&models.Setting{AllowRegistration: true}).Error; err != nil {
		t.Fatalf("create setting: %v", err)
	}
	if err := db.Create(&models.SiteConfig{SiteTitle: "Test Site", HitokotoEnabled: true, CommentEmailAdminNotifyAll: true}).Error; err != nil {
		t.Fatalf("create site config: %v", err)
	}

	if !hitokotoEnabledForViewer(t, 0) || !hitokotoEnabledForViewer(t, admin.ID) || !hitokotoEnabledForViewer(t, alice.ID) {
		t.Fatal("site daily quote setting should be the default for guests, admins, and users without an override")
	}
	if err := UpdateUserFrontendPreferenceConfig(alice.ID, map[string]interface{}{"hitokotoEnabled": false}); err != nil {
		t.Fatalf("save alice daily quote preference: %v", err)
	}
	if hitokotoEnabledForViewer(t, alice.ID) {
		t.Fatal("alice daily quote preference should disable only alice's home widget")
	}
	if !hitokotoEnabledForViewer(t, 0) || !hitokotoEnabledForViewer(t, admin.ID) {
		t.Fatal("alice preference must not change guest or admin daily quote visibility")
	}

	if err := db.Model(&models.SiteConfig{}).Where("1 = 1").Update("hitokoto_enabled", false).Error; err != nil {
		t.Fatalf("disable site daily quote: %v", err)
	}
	if err := UpdateUserFrontendPreferenceConfig(alice.ID, map[string]interface{}{"hitokotoEnabled": true}); err != nil {
		t.Fatalf("enable alice daily quote preference: %v", err)
	}
	if !hitokotoEnabledForViewer(t, alice.ID) {
		t.Fatal("alice should be able to enable daily quote even when the site default is disabled")
	}
	if hitokotoEnabledForViewer(t, 0) || hitokotoEnabledForViewer(t, admin.ID) {
		t.Fatal("an unset administrator field must follow the current site guest default")
	}
}

func TestGetFrontendConfigSeparatesGuestDefaultsFromEveryAccountWidgetPreference(t *testing.T) {
	db := setupUserServiceTestDB(t)
	primary := mustCreateUser(t, models.User{Username: "primary-widgets", Password: models.HashPassword("admin"), IsAdmin: true})
	delegated := mustCreateUser(t, models.User{Username: "delegated-widgets", Password: models.HashPassword("admin"), IsAdmin: true})
	ordinary := mustCreateUser(t, models.User{Username: "ordinary-widgets", Password: models.HashPassword("admin")})

	if primary.ID != models.PrimaryAdminUserID {
		t.Fatalf("primary user id = %d, want %d", primary.ID, models.PrimaryAdminUserID)
	}
	if err := db.Create(&models.Setting{AllowRegistration: true}).Error; err != nil {
		t.Fatalf("create setting: %v", err)
	}
	if err := db.Create(&models.SiteConfig{
		HitokotoEnabled: true, LifeCountdownEnabled: true,
		CommentEmailAdminNotifyAll: true,
	}).Error; err != nil {
		t.Fatalf("create site config: %v", err)
	}
	if err := db.Model(&models.SiteConfig{}).Where("1 = 1").Updates(map[string]interface{}{
		"calendar_enabled": false, "home_stats_enabled": false, "popular_tags_enabled": false,
		"latest_gallery_enabled": false, "heatmap_enabled": false,
	}).Error; err != nil {
		t.Fatalf("set guest widget defaults: %v", err)
	}

	guest := lifeCountdownFrontendSettings(t, 0)
	for key, want := range map[string]bool{
		"lifeCountdownEnabled": true, "hitokotoEnabled": true, "homeStatsEnabled": false,
		"popularTagsEnabled": false, "calendarEnabled": false, "latestGalleryEnabled": false, "heatmapEnabled": false,
	} {
		if got, ok := guest[key].(bool); !ok || got != want {
			t.Fatalf("guest %s = %#v, want %v", key, guest[key], want)
		}
	}

	for _, viewer := range []*models.User{primary, delegated, ordinary} {
		settings := lifeCountdownFrontendSettings(t, viewer.ID)
		for key, want := range map[string]bool{
			"lifeCountdownEnabled": true, "hitokotoEnabled": true, "homeStatsEnabled": false,
			"popularTagsEnabled": false, "calendarEnabled": false, "latestGalleryEnabled": false, "heatmapEnabled": false,
		} {
			if got, ok := settings[key].(bool); !ok || got != want {
				t.Fatalf("viewer %d unset %s = %#v, want current guest default %v", viewer.ID, key, settings[key], want)
			}
		}
	}
	if err := UpdateUserFrontendPreferenceConfig(ordinary.ID, map[string]interface{}{
		"hitokotoEnabled": false,
		"calendarEnabled": true,
	}); err != nil {
		t.Fatalf("save partial ordinary preferences: %v", err)
	}
	if err := UpdateUserWidgetPreferences(ordinary.ID, map[string]interface{}{
		"lifeCountdownEnabled": false,
	}); err != nil {
		t.Fatalf("save explicit ordinary countdown preference: %v", err)
	}
	ordinarySettings := lifeCountdownFrontendSettings(t, ordinary.ID)
	for key, want := range map[string]bool{
		"lifeCountdownEnabled": false,
		"hitokotoEnabled":      false,
		"calendarEnabled":      true,
		"heatmapEnabled":       false,
	} {
		if got, ok := ordinarySettings[key].(bool); !ok || got != want {
			t.Fatalf("ordinary %s = %#v, want explicit-or-inherited value %v", key, ordinarySettings[key], want)
		}
	}
	if err := db.Model(&models.SiteConfig{}).Where("1 = 1").Update("heatmap_enabled", true).Error; err != nil {
		t.Fatalf("change guest default for unset field: %v", err)
	}
	ordinarySettings = lifeCountdownFrontendSettings(t, ordinary.ID)
	for key, want := range map[string]bool{
		"lifeCountdownEnabled": false,
		"hitokotoEnabled":      false,
		"calendarEnabled":      true,
		"heatmapEnabled":       true,
	} {
		if got, ok := ordinarySettings[key].(bool); !ok || got != want {
			t.Fatalf("ordinary %s after guest default update = %#v, want %v", key, ordinarySettings[key], want)
		}
	}

	if err := UpdateUserWidgetPreferences(delegated.ID, map[string]interface{}{
		"hitokotoEnabled": false, "homeStatsEnabled": false, "popularTagsEnabled": false,
		"calendarEnabled": false, "latestGalleryEnabled": false, "heatmapEnabled": false,
		"lifeCountdownEnabled": true, "lifeCountdownBirthDate": "1990-01-02", "lifeExpectancyYears": 80,
	}); err != nil {
		t.Fatalf("save delegated preferences: %v", err)
	}
	delegatedSettings := lifeCountdownFrontendSettings(t, delegated.ID)
	for _, key := range []string{"hitokotoEnabled", "homeStatsEnabled", "popularTagsEnabled", "calendarEnabled", "latestGalleryEnabled", "heatmapEnabled"} {
		if got, ok := delegatedSettings[key].(bool); !ok || got {
			t.Fatalf("delegated %s = %#v, want false", key, delegatedSettings[key])
		}
	}
	if got := lifeCountdownFrontendSettings(t, ordinary.ID)["homeStatsEnabled"]; got != false {
		t.Fatalf("delegated preference leaked to ordinary viewer: %#v", got)
	}
	if got := lifeCountdownFrontendSettings(t, 0)["homeStatsEnabled"]; got != false {
		t.Fatalf("delegated preference leaked to guest: %#v", got)
	}
}
