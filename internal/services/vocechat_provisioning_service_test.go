package services

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/repository"
	"github.com/rcy1314/echo-noise/internal/runtimepolicy"
	"github.com/rcy1314/echo-noise/internal/vocechat"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func stubVoceChatProvisioningCreate(t *testing.T, fn voceChatProvisioningCreateFunc) {
	t.Helper()
	original := voceChatProvisioningCreate
	voceChatProvisioningCreate = fn
	t.Cleanup(func() { voceChatProvisioningCreate = original })
}

func stubVoceChatProvisioningDelete(t *testing.T, fn voceChatProvisioningDeleteFunc) {
	t.Helper()
	original := voceChatProvisioningDelete
	voceChatProvisioningDelete = fn
	t.Cleanup(func() { voceChatProvisioningDelete = original })
}

func provisioningUserResult(uid int64, email, username string) voceChatProvisioningCreateResult {
	return voceChatProvisioningCreateResult{
		User:       &vocechat.User{UID: uid, Email: email, Name: username},
		CreatedNow: true,
	}
}

func TestVoceChatProvisioningIsManualPrimaryOnlyPersistentAndIdempotent(t *testing.T) {
	db := setupUserServiceTestDB(t)
	createRuntimeModeConfigForTest(t, models.RuntimeModeVoceChat, "ok")
	t.Setenv("NOISE_PLAIN_PASSWORD_STORE", filepath.Join(t.TempDir(), "plain-passwords.db"))
	primary := mustCreateUser(t, models.User{Username: "primary-provision", Password: models.HashPassword("primary"), IsAdmin: true})
	delegated := mustCreateUser(t, models.User{Username: "delegated-provision", Password: models.HashPassword("delegated"), IsAdmin: true, VoceChatSyncStatus: models.VoceChatSyncStatusPending})
	ordinary := mustCreateUser(t, models.User{Username: "ordinary-provision", Password: models.HashPassword("ordinary"), VoceChatSyncStatus: models.VoceChatSyncStatusPending})
	store := vocechat.DefaultPlainPasswordStore()
	if err := store.UpsertUserLocalFallbackPassword(delegated.ID, delegated.Username, "delegated", "", ""); err != nil {
		t.Fatalf("store delegated password: %v", err)
	}
	if err := store.UpsertUserLocalFallbackPassword(ordinary.ID, ordinary.Username, "ordinary", "", ""); err != nil {
		t.Fatalf("store ordinary password: %v", err)
	}
	application := models.RegistrationApplication{
		Username: ordinary.Username, PasswordHash: ordinary.Password, Status: models.RegistrationApplicationStatusApproved,
		LocalUserID: &ordinary.ID, VoceChatSyncStatus: models.VoceChatSyncStatusUnbound,
	}
	if err := repository.CreateRegistrationApplicationWithPermanentNumber(&application, "vc.com"); err != nil {
		t.Fatalf("create related application: %v", err)
	}

	createCalls := 0
	seenCandidates := []string{}
	stubVoceChatProvisioningCreate(t, func(_ context.Context, _ vocechat.Config, email, username, _ string) (voceChatProvisioningCreateResult, error) {
		createCalls++
		seenCandidates = append(seenCandidates, email)
		if username == ordinary.Username {
			return provisioningUserResult(int64(100+createCalls), "ordinary-actual@vc.example", username), nil
		}
		return provisioningUserResult(int64(100+createCalls), email, username), nil
	})

	if _, err := StartVoceChatProvisioning(context.Background(), delegated.ID); !errors.Is(err, ErrVoceChatProvisioningPrimaryRequired) {
		t.Fatalf("delegated start error = %v", err)
	}
	diagnostics, err := StartVoceChatProvisioning(context.Background(), primary.ID)
	if err != nil {
		t.Fatalf("start provisioning: %v", err)
	}
	if createCalls != 0 {
		t.Fatalf("manual start performed external work synchronously: calls=%d", createCalls)
	}
	if diagnostics.ProvisioningRun == nil || diagnostics.ProvisioningRun.Status != models.VoceChatProvisioningRunStatusRunning || len(diagnostics.ProvisioningTasks) != 2 {
		t.Fatalf("queued diagnostics = %#v", diagnostics)
	}
	for _, task := range diagnostics.ProvisioningTasks {
		if task.UserID == primary.ID {
			t.Fatal("primary administrator entered the ordinary provisioning queue")
		}
	}

	if err := RunActiveVoceChatProvisioning(context.Background()); err != nil {
		t.Fatalf("run provisioning: %v", err)
	}
	if createCalls != 2 {
		t.Fatalf("external create calls = %d, want 2", createCalls)
	}
	sort.Strings(seenCandidates)
	if !provisioningContainsString(seenCandidates, application.VoceChatCandidateEmail) {
		t.Fatalf("related application candidate was not reused: candidates=%v application=%q", seenCandidates, application.VoceChatCandidateEmail)
	}
	var tasks []models.VoceChatProvisioningTask
	if err := db.Order("user_id ASC").Find(&tasks).Error; err != nil {
		t.Fatalf("load tasks: %v", err)
	}
	if len(tasks) != 2 || tasks[0].Status != models.VoceChatSyncStatusLinked || tasks[1].Status != models.VoceChatSyncStatusLinked {
		t.Fatalf("completed tasks = %#v", tasks)
	}
	if tasks[0].UserID != delegated.ID || tasks[0].ApplicationID != "2" || tasks[0].CandidateEmail != "2@vc.com" {
		t.Fatalf("old user did not receive the next shared permanent allocation: %#v", tasks[0])
	}
	for _, userID := range []uint{delegated.ID, ordinary.ID} {
		var user models.User
		if err := db.First(&user, userID).Error; err != nil {
			t.Fatalf("reload user %d: %v", userID, err)
		}
		if user.VoceChatSyncStatus != models.VoceChatSyncStatusLinked || user.VoceChatEmail == "" || user.VoceChatUserID == "" {
			t.Fatalf("linked user %d = %#v", userID, user)
		}
	}
	if err := db.First(&application, application.ID).Error; err != nil {
		t.Fatalf("reload related application: %v", err)
	}
	if application.VoceChatCandidateEmail == application.VoceChatEmail || application.VoceChatEmail != "ordinary-actual@vc.example" || application.VoceChatSyncStatus != models.VoceChatSyncStatusLinked {
		t.Fatalf("related application did not preserve candidate separately from actual binding: %#v", application)
	}

	if _, err := StartVoceChatProvisioning(context.Background(), primary.ID); err != nil {
		t.Fatalf("repeat start: %v", err)
	}
	var runCount int64
	if err := db.Model(&models.VoceChatProvisioningRun{}).Count(&runCount).Error; err != nil {
		t.Fatalf("count runs: %v", err)
	}
	if runCount != 1 || createCalls != 2 {
		t.Fatalf("repeat start was not idempotent: runs=%d calls=%d", runCount, createCalls)
	}
	var auditCount int64
	if err := db.Model(&models.AdminAuditLog{}).Where("action = ?", "vocechat_provisioning_start").Count(&auditCount).Error; err != nil {
		t.Fatalf("count audits: %v", err)
	}
	if auditCount != 1 {
		t.Fatalf("start audit count = %d, want 1", auditCount)
	}
}

func TestVoceChatProvisioningIsolatesMissingPasswordAndRetriesAfterReset(t *testing.T) {
	db := setupUserServiceTestDB(t)
	createRuntimeModeConfigForTest(t, models.RuntimeModeVoceChat, "ok")
	t.Setenv("NOISE_PLAIN_PASSWORD_STORE", filepath.Join(t.TempDir(), "plain-passwords.db"))
	primary := mustCreateUser(t, models.User{Username: "primary-retry", Password: models.HashPassword("primary"), IsAdmin: true})
	missing := mustCreateUser(t, models.User{Username: "missing-material", Password: models.HashPassword("missing-password"), VoceChatSyncStatus: models.VoceChatSyncStatusPending})
	success := mustCreateUser(t, models.User{Username: "success-material", Password: models.HashPassword("success-password"), VoceChatSyncStatus: models.VoceChatSyncStatusPending})
	if err := vocechat.DefaultPlainPasswordStore().UpsertUserLocalFallbackPassword(success.ID, success.Username, "success-password", "", ""); err != nil {
		t.Fatalf("store success password: %v", err)
	}
	createdUsers := []string{}
	stubVoceChatProvisioningCreate(t, func(_ context.Context, _ vocechat.Config, email, username, _ string) (voceChatProvisioningCreateResult, error) {
		createdUsers = append(createdUsers, username)
		return provisioningUserResult(int64(200+len(createdUsers)), email, username), nil
	})

	if _, err := StartVoceChatProvisioning(context.Background(), primary.ID); err != nil {
		t.Fatalf("start provisioning: %v", err)
	}
	if err := RunActiveVoceChatProvisioning(context.Background()); err != nil {
		t.Fatalf("run provisioning: %v", err)
	}
	if fmt.Sprint(createdUsers) != "[success-material]" {
		t.Fatalf("successful users = %v", createdUsers)
	}
	var missingTask models.VoceChatProvisioningTask
	if err := db.Where("user_id = ?", missing.ID).First(&missingTask).Error; err != nil {
		t.Fatalf("load missing task: %v", err)
	}
	if missingTask.Status != models.VoceChatSyncStatusPasswordSyncRequired || missingTask.ErrorCode != "password_material_missing" || strings.Contains(strings.ToLower(missingTask.ErrorSummary), "hash") {
		t.Fatalf("missing material task = %#v", missingTask)
	}
	var successfulUser models.User
	if err := db.First(&successfulUser, success.ID).Error; err != nil {
		t.Fatalf("reload successful user: %v", err)
	}
	if successfulUser.VoceChatSyncStatus != models.VoceChatSyncStatusLinked {
		t.Fatalf("single failure blocked another user: %#v", successfulUser)
	}

	if err := vocechat.DefaultPlainPasswordStore().UpsertUserLocalFallbackPassword(missing.ID, missing.Username, "missing-password", "", ""); err != nil {
		t.Fatalf("store reset password: %v", err)
	}
	if _, err := RetryVoceChatProvisioningFailures(context.Background(), primary.ID); err != nil {
		t.Fatalf("retry failures: %v", err)
	}
	if err := RunActiveVoceChatProvisioning(context.Background()); err != nil {
		t.Fatalf("run retry: %v", err)
	}
	if fmt.Sprint(createdUsers) != "[success-material missing-material]" {
		t.Fatalf("retry users = %v", createdUsers)
	}
	if err := db.Where("user_id = ?", missing.ID).First(&missingTask).Error; err != nil {
		t.Fatalf("reload missing task: %v", err)
	}
	if missingTask.Status != models.VoceChatSyncStatusLinked || missingTask.AttemptCount != 2 {
		t.Fatalf("retried task = %#v", missingTask)
	}
}

func TestVoceChatProvisioningClassifiesIdentityConflictAndContinues(t *testing.T) {
	db := setupUserServiceTestDB(t)
	createRuntimeModeConfigForTest(t, models.RuntimeModeVoceChat, "ok")
	t.Setenv("NOISE_PLAIN_PASSWORD_STORE", filepath.Join(t.TempDir(), "plain-passwords.db"))
	primary := mustCreateUser(t, models.User{Username: "primary-conflict", Password: models.HashPassword("primary"), IsAdmin: true})
	conflicted := mustCreateUser(t, models.User{Username: "conflicted-user", Password: models.HashPassword("conflicted"), VoceChatSyncStatus: models.VoceChatSyncStatusPending})
	continued := mustCreateUser(t, models.User{Username: "continued-user", Password: models.HashPassword("continued"), VoceChatSyncStatus: models.VoceChatSyncStatusPending})
	for _, state := range []struct {
		user     *models.User
		password string
	}{{conflicted, "conflicted"}, {continued, "continued"}} {
		if err := vocechat.DefaultPlainPasswordStore().UpsertUserLocalFallbackPassword(state.user.ID, state.user.Username, state.password, "", ""); err != nil {
			t.Fatalf("store %s password: %v", state.user.Username, err)
		}
	}
	stubVoceChatProvisioningCreate(t, func(_ context.Context, _ vocechat.Config, email, username, _ string) (voceChatProvisioningCreateResult, error) {
		if username == conflicted.Username {
			return voceChatProvisioningCreateResult{}, errVoceChatProvisioningConflict
		}
		return provisioningUserResult(250, email, username), nil
	})

	if _, err := StartVoceChatProvisioning(context.Background(), primary.ID); err != nil {
		t.Fatalf("start provisioning: %v", err)
	}
	if err := RunActiveVoceChatProvisioning(context.Background()); err != nil {
		t.Fatalf("run provisioning: %v", err)
	}
	var conflictedTask models.VoceChatProvisioningTask
	if err := db.Where("user_id = ?", conflicted.ID).First(&conflictedTask).Error; err != nil {
		t.Fatalf("load conflicted task: %v", err)
	}
	if conflictedTask.Status != models.VoceChatSyncStatusConflicted || conflictedTask.ErrorCode != "identity_conflict" || strings.Contains(strings.ToLower(conflictedTask.ErrorSummary), "api") {
		t.Fatalf("conflicted task = %#v", conflictedTask)
	}
	var continuedUser models.User
	if err := db.First(&continuedUser, continued.ID).Error; err != nil {
		t.Fatalf("reload continued user: %v", err)
	}
	if continuedUser.VoceChatSyncStatus != models.VoceChatSyncStatusLinked {
		t.Fatalf("identity conflict blocked later user: %#v", continuedUser)
	}
}

func TestVoceChatProvisioningAttemptsAllUsersBeforeMarkingUpstreamDegraded(t *testing.T) {
	db := setupUserServiceTestDB(t)
	createRuntimeModeConfigForTest(t, models.RuntimeModeVoceChat, "ok")
	t.Setenv("NOISE_PLAIN_PASSWORD_STORE", filepath.Join(t.TempDir(), "plain-passwords.db"))
	primary := mustCreateUser(t, models.User{Username: "primary-upstream", Password: models.HashPassword("primary"), IsAdmin: true})
	first := mustCreateUser(t, models.User{Username: "first-upstream", Password: models.HashPassword("first"), VoceChatSyncStatus: models.VoceChatSyncStatusPending})
	second := mustCreateUser(t, models.User{Username: "second-upstream", Password: models.HashPassword("second"), VoceChatSyncStatus: models.VoceChatSyncStatusPending})
	for _, state := range []struct {
		user     *models.User
		password string
	}{{first, "first"}, {second, "second"}} {
		if err := vocechat.DefaultPlainPasswordStore().UpsertUserLocalFallbackPassword(state.user.ID, state.user.Username, state.password, "", ""); err != nil {
			t.Fatalf("store %s password: %v", state.user.Username, err)
		}
	}
	calls := 0
	stubVoceChatProvisioningCreate(t, func(context.Context, vocechat.Config, string, string, string) (voceChatProvisioningCreateResult, error) {
		calls++
		return voceChatProvisioningCreateResult{}, &vocechat.APIError{StatusCode: http.StatusServiceUnavailable, Method: http.MethodPost, Path: "/api/admin/user", Body: "secret upstream detail"}
	})
	if _, err := StartVoceChatProvisioning(context.Background(), primary.ID); err != nil {
		t.Fatalf("start provisioning: %v", err)
	}
	if err := RunActiveVoceChatProvisioning(context.Background()); err != nil {
		t.Fatalf("run provisioning: %v", err)
	}
	if calls != 2 {
		t.Fatalf("first upstream failure blocked later users: calls=%d", calls)
	}
	policy, err := ResolveRuntimePolicy()
	if err != nil {
		t.Fatalf("resolve degraded policy: %v", err)
	}
	if policy.RuntimeState != runtimepolicy.StateVoceChatDegraded {
		t.Fatalf("all-upstream-failure policy = %#v", policy)
	}
	var config models.SiteConfig
	if err := db.First(&config).Error; err != nil {
		t.Fatalf("reload degraded config: %v", err)
	}
	if strings.Contains(config.VoceChatLastHealthError, "secret upstream detail") || !strings.Contains(config.VoceChatLastHealthError, "provisioning requests unavailable") {
		t.Fatalf("stored health error was not redacted: %q", config.VoceChatLastHealthError)
	}
}

func TestVoceChatProvisioningResumesExpiredLeaseAndPausesBetweenUsersInLocalMode(t *testing.T) {
	db := setupUserServiceTestDB(t)
	createRuntimeModeConfigForTest(t, models.RuntimeModeVoceChat, "ok")
	t.Setenv("NOISE_PLAIN_PASSWORD_STORE", filepath.Join(t.TempDir(), "plain-passwords.db"))
	primary := mustCreateUser(t, models.User{Username: "primary-resume", Password: models.HashPassword("primary"), IsAdmin: true})
	first := mustCreateUser(t, models.User{Username: "first-resume", Password: models.HashPassword("first"), VoceChatSyncStatus: models.VoceChatSyncStatusPending})
	second := mustCreateUser(t, models.User{Username: "second-resume", Password: models.HashPassword("second"), VoceChatSyncStatus: models.VoceChatSyncStatusPending})
	if err := vocechat.DefaultPlainPasswordStore().UpsertUserLocalFallbackPassword(first.ID, first.Username, "first", "", ""); err != nil {
		t.Fatalf("store first password: %v", err)
	}
	if err := vocechat.DefaultPlainPasswordStore().UpsertUserLocalFallbackPassword(second.ID, second.Username, "second", "", ""); err != nil {
		t.Fatalf("store second password: %v", err)
	}

	if _, err := StartVoceChatProvisioning(context.Background(), primary.ID); err != nil {
		t.Fatalf("start provisioning: %v", err)
	}
	var firstTask models.VoceChatProvisioningTask
	if err := db.Where("user_id = ?", first.ID).First(&firstTask).Error; err != nil {
		t.Fatalf("load first task: %v", err)
	}
	expired := time.Now().UTC().Add(-time.Minute)
	if err := db.Model(&models.VoceChatProvisioningTask{}).Where("id = ?", firstTask.ID).Updates(map[string]interface{}{
		"status":      models.VoceChatSyncStatusProvisioning,
		"lease_until": expired,
	}).Error; err != nil {
		t.Fatalf("seed interrupted lease: %v", err)
	}

	calls := 0
	stubVoceChatProvisioningCreate(t, func(ctx context.Context, _ vocechat.Config, email, username, _ string) (voceChatProvisioningCreateResult, error) {
		calls++
		if calls == 1 {
			if _, err := SwitchConfiguredMode(ctx, primary.ID, runtimepolicy.ModeLocal); err != nil {
				t.Fatalf("switch local during first task: %v", err)
			}
		}
		return provisioningUserResult(int64(300+calls), email, username), nil
	})
	if err := RunActiveVoceChatProvisioning(context.Background()); err != nil {
		t.Fatalf("resume provisioning: %v", err)
	}
	if calls != 1 {
		t.Fatalf("local switch did not stop claiming new users: calls=%d", calls)
	}
	var run models.VoceChatProvisioningRun
	if err := db.Order("id DESC").First(&run).Error; err != nil {
		t.Fatalf("load paused run: %v", err)
	}
	if run.Status != models.VoceChatProvisioningRunStatusPaused {
		t.Fatalf("run status after local switch = %q", run.Status)
	}
	if err := db.Where("user_id = ?", first.ID).First(&firstTask).Error; err != nil {
		t.Fatalf("reload first task: %v", err)
	}
	if firstTask.Status != models.VoceChatSyncStatusLinked || firstTask.AttemptCount != 1 {
		t.Fatalf("recovered first task = %#v", firstTask)
	}
	var secondTask models.VoceChatProvisioningTask
	if err := db.Where("user_id = ?", second.ID).First(&secondTask).Error; err != nil {
		t.Fatalf("load second task: %v", err)
	}
	if secondTask.Status != models.VoceChatSyncStatusPending || secondTask.AttemptCount != 0 {
		t.Fatalf("unclaimed second task = %#v", secondTask)
	}

	originalHealth := runtimeModeHealthCheck
	runtimeModeHealthCheck = func(context.Context) error { return nil }
	t.Cleanup(func() { runtimeModeHealthCheck = originalHealth })
	if _, err := SwitchConfiguredMode(context.Background(), primary.ID, runtimepolicy.ModeVoceChat); err != nil {
		t.Fatalf("switch back to VoceChat: %v", err)
	}
	if err := RunActiveVoceChatProvisioning(context.Background()); err != nil {
		t.Fatalf("inspect paused run before manual resume: %v", err)
	}
	if calls != 1 {
		t.Fatalf("paused run resumed automatically after mode switch: calls=%d", calls)
	}
	if _, err := StartVoceChatProvisioning(context.Background(), primary.ID); err != nil {
		t.Fatalf("resume active run: %v", err)
	}
	if err := RunActiveVoceChatProvisioning(context.Background()); err != nil {
		t.Fatalf("finish resumed run: %v", err)
	}
	if calls != 2 {
		t.Fatalf("resumed run calls = %d, want 2 total", calls)
	}
	if err := db.Order("id DESC").First(&run).Error; err != nil {
		t.Fatalf("reload completed run: %v", err)
	}
	if run.Status != models.VoceChatProvisioningRunStatusCompleted {
		t.Fatalf("resumed run status = %q", run.Status)
	}
}

func TestVoceChatProvisioningSurvivesDatabaseRestart(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "provisioning-restart.db")
	openDatabase := func() (*gorm.DB, error) {
		db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
		if err != nil {
			return nil, err
		}
		if err := models.MigrateDB(db); err != nil {
			return nil, err
		}
		return db, nil
	}
	db, err := openDatabase()
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	database.DB = db
	models.SetDB(db)
	repository.ClearUserCache()
	t.Cleanup(func() {
		repository.ClearUserCache()
		if database.DB != nil {
			if sqlDB, err := database.DB.DB(); err == nil {
				_ = sqlDB.Close()
			}
		}
		database.DB = nil
		models.SetDB(nil)
	})
	if err := db.Create(&models.SiteConfig{
		RuntimeMode:                 models.RuntimeModeVoceChat,
		RuntimeModeMigrationVersion: models.RuntimeModeMigrationVersionCurrent,
		VoceChatEnabled:             true,
		VoceChatBaseURL:             "https://vc.example.test",
		VoceChatAdminToken:          "configured-token",
		VoceChatLastHealthStatus:    "ok",
	}).Error; err != nil {
		t.Fatalf("create config: %v", err)
	}
	t.Setenv("NOISE_PLAIN_PASSWORD_STORE", filepath.Join(t.TempDir(), "plain-passwords.db"))
	primary := mustCreateUser(t, models.User{Username: "primary-restart", Password: models.HashPassword("primary"), IsAdmin: true})
	user := mustCreateUser(t, models.User{Username: "user-restart", Password: models.HashPassword("user-password"), VoceChatSyncStatus: models.VoceChatSyncStatusPending})
	if err := vocechat.DefaultPlainPasswordStore().UpsertUserLocalFallbackPassword(user.ID, user.Username, "user-password", "", ""); err != nil {
		t.Fatalf("store user password: %v", err)
	}
	if _, err := StartVoceChatProvisioning(context.Background(), primary.ID); err != nil {
		t.Fatalf("start provisioning: %v", err)
	}
	var task models.VoceChatProvisioningTask
	if err := db.Where("user_id = ?", user.ID).First(&task).Error; err != nil {
		t.Fatalf("load task: %v", err)
	}
	if err := db.Model(&models.VoceChatProvisioningTask{}).Where("id = ?", task.ID).Updates(map[string]interface{}{
		"status":      models.VoceChatSyncStatusProvisioning,
		"lease_until": time.Now().UTC().Add(-time.Minute),
	}).Error; err != nil {
		t.Fatalf("persist interrupted task: %v", err)
	}
	if sqlDB, err := db.DB(); err != nil {
		t.Fatalf("get sql database: %v", err)
	} else if err := sqlDB.Close(); err != nil {
		t.Fatalf("close database: %v", err)
	}
	database.DB = nil
	models.SetDB(nil)
	repository.ClearUserCache()

	db, err = openDatabase()
	if err != nil {
		t.Fatalf("reopen database: %v", err)
	}
	database.DB = db
	models.SetDB(db)
	stubVoceChatProvisioningCreate(t, func(_ context.Context, _ vocechat.Config, email, username, _ string) (voceChatProvisioningCreateResult, error) {
		return provisioningUserResult(351, email, username), nil
	})
	if err := RunActiveVoceChatProvisioning(context.Background()); err != nil {
		t.Fatalf("resume after database restart: %v", err)
	}
	if err := db.First(&task, task.ID).Error; err != nil {
		t.Fatalf("reload resumed task: %v", err)
	}
	if task.Status != models.VoceChatSyncStatusLinked || task.AttemptCount != 1 {
		t.Fatalf("resumed task = %#v", task)
	}
}

func TestVoceChatProvisioningVerifiesBoundUsersAndSynchronizesCurrentLocalPassword(t *testing.T) {
	setupUserServiceTestDB(t)
	createRuntimeModeConfigForTest(t, models.RuntimeModeVoceChat, "ok")
	t.Setenv("NOISE_PLAIN_PASSWORD_STORE", filepath.Join(t.TempDir(), "plain-passwords.db"))
	primary := mustCreateUser(t, models.User{Username: "primary-existing", Password: models.HashPassword("primary"), IsAdmin: true})
	verifyUser := mustCreateUser(t, models.User{
		Username: "verify-existing", Password: models.HashPassword("verify-password"), VoceChatEmail: "verify@vc.com", VoceChatUserID: "401", VoceChatSyncStatus: models.VoceChatSyncStatusLinked,
	})
	syncUser := mustCreateUser(t, models.User{
		Username: "sync-existing", Password: models.HashPassword("new-password"), VoceChatEmail: "sync@vc.com", VoceChatUserID: "402", VoceChatSyncStatus: models.VoceChatSyncStatusPasswordSyncRequired,
	})
	store := vocechat.DefaultPlainPasswordStore()
	if err := store.UpsertUserVoceChatPassword(verifyUser.ID, verifyUser.Username, "verify-password", verifyUser.VoceChatEmail, verifyUser.VoceChatUserID); err != nil {
		t.Fatalf("store verify password: %v", err)
	}
	if err := store.UpsertUserVoceChatPassword(syncUser.ID, syncUser.Username, "old-password", syncUser.VoceChatEmail, syncUser.VoceChatUserID); err != nil {
		t.Fatalf("store old sync password: %v", err)
	}
	if err := store.UpsertUserLocalFallbackPassword(syncUser.ID, syncUser.Username, "new-password", syncUser.VoceChatEmail, syncUser.VoceChatUserID); err != nil {
		t.Fatalf("store current local password: %v", err)
	}
	loginCalls := 0
	stubVoceChatPasswordLogin(t, func(_ context.Context, _ vocechat.Config, email, password string) (*vocechat.LoginResponse, error) {
		loginCalls++
		if email != verifyUser.VoceChatEmail || password != "verify-password" {
			t.Fatalf("verification login = %q/%q", email, password)
		}
		return &vocechat.LoginResponse{User: vocechat.UserInfo{UID: 401, Email: email, Name: verifyUser.Username}}, nil
	})
	updatedPassword := ""
	stubVoceChatAdminUpdateUser(t, func(_ context.Context, _ vocechat.Config, uid int64, request vocechat.UpdateUserRequest) (*vocechat.User, error) {
		if uid != 402 || request.Password == nil {
			t.Fatalf("password sync request uid=%d request=%#v", uid, request)
		}
		updatedPassword = *request.Password
		return &vocechat.User{UID: uid, Email: syncUser.VoceChatEmail, Name: syncUser.Username}, nil
	})
	stubVoceChatProvisioningCreate(t, func(context.Context, vocechat.Config, string, string, string) (voceChatProvisioningCreateResult, error) {
		t.Fatal("bound users must not be recreated")
		return voceChatProvisioningCreateResult{}, nil
	})

	if _, err := StartVoceChatProvisioning(context.Background(), primary.ID); err != nil {
		t.Fatalf("start existing account checks: %v", err)
	}
	if err := RunActiveVoceChatProvisioning(context.Background()); err != nil {
		t.Fatalf("run existing account checks: %v", err)
	}
	if loginCalls != 1 || updatedPassword != "new-password" {
		t.Fatalf("existing account actions loginCalls=%d updatedPassword=%q", loginCalls, updatedPassword)
	}
	record, ok, err := store.GetUserPassword(syncUser.ID)
	if err != nil || !ok || record.VoceChatPasswordValue() != "new-password" {
		t.Fatalf("synchronized password state ok=%v err=%v record=%#v", ok, err, record)
	}
}

func TestVoceChatProvisioningCompensatesNewRemoteAccountWhenLocalFinalizationFails(t *testing.T) {
	db := setupUserServiceTestDB(t)
	createRuntimeModeConfigForTest(t, models.RuntimeModeVoceChat, "ok")
	t.Setenv("NOISE_PLAIN_PASSWORD_STORE", filepath.Join(t.TempDir(), "plain-passwords.db"))
	primary := mustCreateUser(t, models.User{Username: "primary-compensation", Password: models.HashPassword("primary"), IsAdmin: true})
	user := mustCreateUser(t, models.User{Username: "compensation-user", Password: models.HashPassword("user-password"), VoceChatSyncStatus: models.VoceChatSyncStatusPending})
	if err := vocechat.DefaultPlainPasswordStore().UpsertUserLocalFallbackPassword(user.ID, user.Username, "user-password", "", ""); err != nil {
		t.Fatalf("store user password: %v", err)
	}
	application := models.RegistrationApplication{
		Username: user.Username, PasswordHash: user.Password, Status: models.RegistrationApplicationStatusApproved,
		LocalUserID: &user.ID, VoceChatSyncStatus: models.VoceChatSyncStatusUnbound,
	}
	if err := repository.CreateRegistrationApplicationWithPermanentNumber(&application, "vc.com"); err != nil {
		t.Fatalf("create related application: %v", err)
	}
	if _, err := StartVoceChatProvisioning(context.Background(), primary.ID); err != nil {
		t.Fatalf("start provisioning: %v", err)
	}
	if err := db.Exec(`CREATE TRIGGER reject_linked_application BEFORE UPDATE ON registration_applications WHEN NEW.id = ` + fmt.Sprint(application.ID) + ` AND NEW.voce_chat_sync_status = 'linked' BEGIN SELECT RAISE(ABORT, 'forced application persistence failure'); END`).Error; err != nil {
		t.Fatalf("create failure trigger: %v", err)
	}
	stubVoceChatProvisioningCreate(t, func(_ context.Context, _ vocechat.Config, email, username, _ string) (voceChatProvisioningCreateResult, error) {
		return provisioningUserResult(501, email, username), nil
	})
	deletedUID := ""
	stubVoceChatProvisioningDelete(t, func(_ context.Context, _ vocechat.Config, uid string) error {
		deletedUID = uid
		return nil
	})

	if err := RunActiveVoceChatProvisioning(context.Background()); err != nil {
		t.Fatalf("run compensated provisioning: %v", err)
	}
	if deletedUID != "501" {
		t.Fatalf("compensation deleted uid = %q", deletedUID)
	}
	var task models.VoceChatProvisioningTask
	if err := db.Where("user_id = ?", user.ID).First(&task).Error; err != nil {
		t.Fatalf("load compensated task: %v", err)
	}
	if task.Status != models.VoceChatSyncStatusFailed || task.ErrorCode != "state_persistence_failed" {
		t.Fatalf("compensated task = %#v", task)
	}
	var restored models.User
	if err := db.First(&restored, user.ID).Error; err != nil {
		t.Fatalf("reload compensated user: %v", err)
	}
	if restored.VoceChatEmail != "" || restored.VoceChatUserID != "" {
		t.Fatalf("compensation left a false binding: %#v", restored)
	}
	if err := db.First(&application, application.ID).Error; err != nil {
		t.Fatalf("reload compensated application: %v", err)
	}
	if application.VoceChatEmail != "" || application.VoceChatUserID != "" || application.VoceChatSyncStatus != models.VoceChatSyncStatusUnbound {
		t.Fatalf("compensation left a false application binding: %#v", application)
	}
}

func TestDefaultVoceChatProvisioningCreateReusesVerifiedExistingCandidate(t *testing.T) {
	createCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/api/admin/user":
			_, _ = response.Write([]byte(`[{"uid":601,"email":"42@vc.example","name":"existing-candidate"}]`))
		case request.Method == http.MethodPost && request.URL.Path == "/api/token/login":
			_, _ = response.Write([]byte(`{"token":"token","refresh_token":"refresh","user":{"uid":601,"email":"42@vc.example","name":"existing-candidate"}}`))
		case request.Method == http.MethodPost && request.URL.Path == "/api/admin/user":
			createCalls++
			response.WriteHeader(http.StatusConflict)
			_, _ = response.Write([]byte(`{"error":"duplicate"}`))
		default:
			response.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	result, err := defaultVoceChatProvisioningCreate(context.Background(), vocechat.Config{
		Enabled: true, BaseURL: server.URL, AdminToken: "configured-token",
	}, "42@vc.example", "existing-candidate", "candidate-password")
	if err != nil {
		t.Fatalf("reuse existing candidate: %v", err)
	}
	if result.CreatedNow || result.User == nil || result.User.UID != 601 || result.User.Email != "42@vc.example" {
		t.Fatalf("reused result = %#v", result)
	}
	if createCalls != 0 {
		t.Fatalf("verified existing candidate was recreated: calls=%d", createCalls)
	}
}

func provisioningContainsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
