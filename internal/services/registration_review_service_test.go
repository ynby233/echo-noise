package services

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rcy1314/echo-noise/internal/dto"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/repository"
	"github.com/rcy1314/echo-noise/internal/vocechat"
)

func TestListRegistrationApplicationsRedactsSyncErrorForDelegatedViewer(t *testing.T) {
	db := setupUserServiceTestDB(t)
	primary := mustCreateUser(t, models.User{Username: "primary-admin", Password: models.HashPassword("admin"), IsAdmin: true, Token: models.GenerateToken(32)})
	delegated := mustCreateUser(t, models.User{Username: "delegated-admin", Password: models.HashPassword("admin"), IsAdmin: true, Token: models.GenerateToken(32)})
	if err := db.Create(&models.RegistrationApplication{
		ApplicationID:      "redaction-test",
		Username:           "candidate",
		Status:             models.RegistrationApplicationStatusPending,
		VoceChatUserID:     "vc-42",
		VoceChatEmail:      "candidate@vc.example",
		VoceChatSyncStatus: models.VoceChatSyncStatusFailed,
		VoceChatSyncError:  "upstream secret diagnostic",
	}).Error; err != nil {
		t.Fatalf("create registration application: %v", err)
	}

	delegatedResult, err := ListRegistrationApplicationsForViewer(delegated.ID, "", 20, 0)
	if err != nil {
		t.Fatalf("list delegated applications: %v", err)
	}
	if len(delegatedResult.Items) != 1 || delegatedResult.Items[0].VoceChatSyncError != "" {
		t.Fatalf("delegated view = %#v, want redacted sync error", delegatedResult.Items)
	}

	primaryResult, err := ListRegistrationApplicationsForViewer(primary.ID, "", 20, 0)
	if err != nil {
		t.Fatalf("list primary applications: %v", err)
	}
	if len(primaryResult.Items) != 1 || !strings.Contains(primaryResult.Items[0].VoceChatSyncError, "upstream secret diagnostic") {
		t.Fatalf("primary view = %#v, want diagnostic", primaryResult.Items)
	}
}

func setRegistrationProvisionForTest(t *testing.T, fn registrationVoceChatProvisionFunc) {
	t.Helper()
	original := registrationVoceChatProvision
	registrationVoceChatProvision = fn
	t.Cleanup(func() { registrationVoceChatProvision = original })
}

func setRegistrationDeleteForTest(t *testing.T, fn registrationVoceChatDeleteFunc) {
	t.Helper()
	original := registrationVoceChatDelete
	registrationVoceChatDelete = fn
	t.Cleanup(func() { registrationVoceChatDelete = original })
}

func setRegistrationVerifyForTest(t *testing.T, fn registrationVoceChatVerifyFunc) {
	t.Helper()
	original := registrationVoceChatVerify
	registrationVoceChatVerify = fn
	t.Cleanup(func() { registrationVoceChatVerify = original })
}

func TestApproveRegistrationApplicationCreatesLocalUserAndMovesPlainPassword(t *testing.T) {
	setupUserServiceTestDB(t)
	configureVoceChatForTest(t, true, true, false)
	storePath := filepath.Join(t.TempDir(), "plain-passwords.db")
	t.Setenv("NOISE_PLAIN_PASSWORD_STORE", storePath)
	admin := mustCreateUser(t, models.User{Username: "admin", Password: models.HashPassword("admin"), IsAdmin: true, Token: models.GenerateToken(32)})

	setRegistrationProvisionForTest(t, func(applicationID, username, password string) registrationVoceChatProvisionResult {
		return registrationVoceChatProvisionResult{
			Email:      applicationID + "@vc.com",
			UserID:     "99",
			Username:   username,
			SyncStatus: models.VoceChatSyncStatusCreated,
		}
	})
	setRegistrationVerifyForTest(t, func(application models.RegistrationApplication) (bool, error) {
		return true, nil
	})

	if err := Register(dto.RegisterDto{Username: "alice", Password: "secret-pass"}); err != nil {
		t.Fatalf("register: %v", err)
	}
	application, err := repository.GetPendingRegistrationApplicationByUsername("alice")
	if err != nil {
		t.Fatalf("get pending application: %v", err)
	}

	created, err := ApproveRegistrationApplication(application.ID, admin.ID, "ok")
	if err != nil {
		t.Fatalf("approve application: %v", err)
	}
	if created.Username != "alice" || created.VoceChatUserID != "99" || created.VoceChatSyncStatus != models.VoceChatSyncStatusLinked {
		t.Fatalf("unexpected created user: %#v", created)
	}
	loadedUser := mustGetUserByUsername(t, "alice")
	if loadedUser.ID != created.ID {
		t.Fatalf("local user id = %d, want %d", loadedUser.ID, created.ID)
	}

	approved, err := repository.GetRegistrationApplicationByID(application.ID)
	if err != nil {
		t.Fatalf("get approved application: %v", err)
	}
	if approved.Status != models.RegistrationApplicationStatusApproved || approved.LocalUserID == nil || *approved.LocalUserID != created.ID {
		t.Fatalf("application not approved correctly: %#v", approved)
	}
	store := vocechat.NewPlainPasswordStore(storePath)
	if _, ok, err := store.GetApplicationPassword(application.ApplicationID); err != nil || ok {
		t.Fatalf("application plain password should be removed, ok=%v err=%v", ok, err)
	}
	record, ok, err := store.GetUserPassword(created.ID)
	if err != nil {
		t.Fatalf("read user plain password: %v", err)
	}
	if !ok || record.VoceChatPassword != "secret-pass" || record.LocalFallbackPassword != "" || record.VoceChatPasswordUpdatedAt == nil || record.VoceChatUserID != "99" {
		t.Fatalf("unexpected user plain password record: ok=%v record=%#v", ok, record)
	}
}

func TestApproveRegistrationApplicationRecreatesMissingPrecreatedVoceChatUser(t *testing.T) {
	setupUserServiceTestDB(t)
	configureVoceChatForTest(t, true, true, false)
	t.Setenv("NOISE_PLAIN_PASSWORD_STORE", filepath.Join(t.TempDir(), "plain-passwords.db"))
	admin := mustCreateUser(t, models.User{Username: "admin", Password: models.HashPassword("admin"), IsAdmin: true, Token: models.GenerateToken(32)})
	calls := 0
	setRegistrationProvisionForTest(t, func(applicationID, username, password string) registrationVoceChatProvisionResult {
		calls++
		userID := "55"
		if calls > 1 {
			userID = "77"
		}
		return registrationVoceChatProvisionResult{
			Email:      applicationID + "@vc.com",
			UserID:     userID,
			Username:   username,
			SyncStatus: models.VoceChatSyncStatusCreated,
		}
	})
	setRegistrationVerifyForTest(t, func(application models.RegistrationApplication) (bool, error) {
		return false, nil
	})

	if err := Register(dto.RegisterDto{Username: "missing_vc", Password: "secret-pass"}); err != nil {
		t.Fatalf("register: %v", err)
	}
	application, err := repository.GetPendingRegistrationApplicationByUsername("missing_vc")
	if err != nil {
		t.Fatalf("get pending application: %v", err)
	}
	if application.VoceChatUserID != "55" {
		t.Fatalf("initial vc user id = %q", application.VoceChatUserID)
	}

	created, err := ApproveRegistrationApplication(application.ID, admin.ID, "ok")
	if err != nil {
		t.Fatalf("approve application after recreating vc user: %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected provision to run twice, got %d", calls)
	}
	if created.VoceChatUserID != "77" {
		t.Fatalf("recreated vc user id = %q", created.VoceChatUserID)
	}
}

func TestApproveRegistrationApplicationRetriesVoceChatCreation(t *testing.T) {
	setupUserServiceTestDB(t)
	configureVoceChatForTest(t, true, true, false)
	t.Setenv("NOISE_PLAIN_PASSWORD_STORE", filepath.Join(t.TempDir(), "plain-passwords.db"))
	admin := mustCreateUser(t, models.User{Username: "admin", Password: models.HashPassword("admin"), IsAdmin: true, Token: models.GenerateToken(32)})
	calls := 0
	setRegistrationProvisionForTest(t, func(applicationID, username, password string) registrationVoceChatProvisionResult {
		calls++
		if calls == 1 {
			return registrationVoceChatProvisionResult{
				Email:      applicationID + "@vc.com",
				Username:   username,
				SyncStatus: models.VoceChatSyncStatusPending,
				SyncError:  "temporary failure",
			}
		}
		return registrationVoceChatProvisionResult{
			Email:      applicationID + "@vc.com",
			UserID:     "100",
			Username:   username,
			SyncStatus: models.VoceChatSyncStatusCreated,
		}
	})

	if err := Register(dto.RegisterDto{Username: "bob", Password: "secret-pass"}); err != nil {
		t.Fatalf("register: %v", err)
	}
	application, err := repository.GetPendingRegistrationApplicationByUsername("bob")
	if err != nil {
		t.Fatalf("get pending application: %v", err)
	}
	if application.VoceChatSyncStatus != models.VoceChatSyncStatusPending {
		t.Fatalf("initial vc status = %q", application.VoceChatSyncStatus)
	}

	created, err := ApproveRegistrationApplication(application.ID, admin.ID, "ok")
	if err != nil {
		t.Fatalf("approve application after retry: %v", err)
	}
	if created.VoceChatUserID != "100" {
		t.Fatalf("vc user id = %q", created.VoceChatUserID)
	}
}

func TestRejectRegistrationApplicationDeletesPrecreatedVoceChatUser(t *testing.T) {
	setupUserServiceTestDB(t)
	configureVoceChatForTest(t, true, true, false)
	storePath := filepath.Join(t.TempDir(), "plain-passwords.db")
	t.Setenv("NOISE_PLAIN_PASSWORD_STORE", storePath)
	admin := mustCreateUser(t, models.User{Username: "admin", Password: models.HashPassword("admin"), IsAdmin: true, Token: models.GenerateToken(32)})
	setRegistrationProvisionForTest(t, func(applicationID, username, password string) registrationVoceChatProvisionResult {
		return registrationVoceChatProvisionResult{
			Email:      applicationID + "@vc.com",
			UserID:     "55",
			Username:   username,
			SyncStatus: models.VoceChatSyncStatusCreated,
		}
	})
	deletedUID := ""
	setRegistrationDeleteForTest(t, func(application models.RegistrationApplication) error {
		deletedUID = application.VoceChatUserID
		return nil
	})

	if err := Register(dto.RegisterDto{Username: "charlie", Password: "secret-pass"}); err != nil {
		t.Fatalf("register: %v", err)
	}
	application, err := repository.GetPendingRegistrationApplicationByUsername("charlie")
	if err != nil {
		t.Fatalf("get pending application: %v", err)
	}
	if err := RejectRegistrationApplication(application.ID, admin.ID, "reject"); err != nil {
		t.Fatalf("reject application: %v", err)
	}
	if deletedUID != "55" {
		t.Fatalf("deleted vc uid = %q", deletedUID)
	}
	rejected, err := repository.GetRegistrationApplicationByID(application.ID)
	if err != nil {
		t.Fatalf("get rejected application: %v", err)
	}
	if rejected.Status != models.RegistrationApplicationStatusRejected {
		t.Fatalf("application status = %q", rejected.Status)
	}
	if _, ok, err := vocechat.NewPlainPasswordStore(storePath).GetApplicationPassword(application.ApplicationID); err != nil || ok {
		t.Fatalf("application plain password should be removed, ok=%v err=%v", ok, err)
	}
}

func TestRejectRegistrationApplicationKeepsPendingWhenVoceChatDeleteFails(t *testing.T) {
	setupUserServiceTestDB(t)
	configureVoceChatForTest(t, true, true, false)
	t.Setenv("NOISE_PLAIN_PASSWORD_STORE", filepath.Join(t.TempDir(), "plain-passwords.db"))
	admin := mustCreateUser(t, models.User{Username: "admin", Password: models.HashPassword("admin"), IsAdmin: true, Token: models.GenerateToken(32)})
	setRegistrationProvisionForTest(t, func(applicationID, username, password string) registrationVoceChatProvisionResult {
		return registrationVoceChatProvisionResult{
			Email:      applicationID + "@vc.com",
			UserID:     "66",
			Username:   username,
			SyncStatus: models.VoceChatSyncStatusCreated,
		}
	})
	deleteCalls := 0
	setRegistrationDeleteForTest(t, func(application models.RegistrationApplication) error {
		deleteCalls++
		if deleteCalls == 1 {
			return errors.New("delete failed")
		}
		return nil
	})

	if err := Register(dto.RegisterDto{Username: "david", Password: "secret-pass"}); err != nil {
		t.Fatalf("register: %v", err)
	}
	application, err := repository.GetPendingRegistrationApplicationByUsername("david")
	if err != nil {
		t.Fatalf("get pending application: %v", err)
	}
	if err := RejectRegistrationApplication(application.ID, admin.ID, "reject"); err == nil {
		t.Fatalf("reject should fail when vc delete fails")
	}
	stillPending, err := repository.GetRegistrationApplicationByID(application.ID)
	if err != nil {
		t.Fatalf("get application after failed reject: %v", err)
	}
	if stillPending.Status != models.RegistrationApplicationStatusPending {
		t.Fatalf("application should remain pending, got %q", stillPending.Status)
	}
	if stillPending.VoceChatSyncStatus != models.VoceChatSyncStatusFailed {
		t.Fatalf("vc sync status = %q", stillPending.VoceChatSyncStatus)
	}
	if err := RejectRegistrationApplication(application.ID, admin.ID, "retry reject"); err != nil {
		t.Fatalf("reject retry should delete vc user and succeed: %v", err)
	}
	if deleteCalls != 2 {
		t.Fatalf("delete calls = %d", deleteCalls)
	}
	rejected, err := repository.GetRegistrationApplicationByID(application.ID)
	if err != nil {
		t.Fatalf("get rejected application: %v", err)
	}
	if rejected.Status != models.RegistrationApplicationStatusRejected || rejected.VoceChatSyncStatus != models.VoceChatSyncStatusNone || rejected.VoceChatSyncError != "" {
		t.Fatalf("application not rejected cleanly after retry: %#v", rejected)
	}
}
