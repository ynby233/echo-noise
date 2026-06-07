package services

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

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
	if application.VoceChatSyncStatus != models.VoceChatSyncStatusNone {
		t.Fatalf("vc sync status = %q", application.VoceChatSyncStatus)
	}
	if application.ApplicationID != "1" {
		t.Fatalf("application id = %q, want 1", application.ApplicationID)
	}
	if strings.Contains(application.ApplicationID, "_") {
		t.Fatalf("application id %q must be usable as VoceChat email prefix", application.ApplicationID)
	}
	if application.VoceChatEmail != application.ApplicationID+"@vc.com" {
		t.Fatalf("vc email = %q", application.VoceChatEmail)
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
	if record.VoceChatPassword != "secret-pass" || record.LocalFallbackPassword != "" || record.Username != "新用户_01" || record.VoceChatEmail != application.VoceChatEmail || record.VoceChatPasswordUpdatedAt == nil {
		t.Fatalf("unexpected plain password record: %#v", record)
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
				return nil, errors.New("dial tcp failed")
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
			if siteConfig.VoceChatLastHealthStatus != "failed" || !strings.Contains(siteConfig.VoceChatLastHealthError, "dial tcp failed") || siteConfig.VoceChatLastHealthCheckAt == nil {
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

func TestChangePasswordForBoundVoceChatUserRejectsWhenIntegrationDisabled(t *testing.T) {
	for _, fallbackEnabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("fallback %v", fallbackEnabled), func(t *testing.T) {
			setupUserServiceTestDB(t)
			configureVoceChatForTest(t, false, true, fallbackEnabled)

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
	mustCreateUser(t, models.User{
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
	if voceChatEmails["alice"] != "alice@vc.com" {
		t.Fatalf("alice vc email missing from status, got %q", voceChatEmails["alice"])
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

	assertLifeCountdownFrontendSettings(t, lifeCountdownFrontendSettings(t, 0), true, "1980-03-04", 88)
	assertLifeCountdownFrontendSettings(t, lifeCountdownFrontendSettings(t, alice.ID), false, "", 0)

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
