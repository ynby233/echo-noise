package services

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/dto"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/repository"
	"github.com/rcy1314/echo-noise/internal/vocechat"
)

func TestConcurrentLocalRegistrationsReceivePermanentNonReusableNumbers(t *testing.T) {
	setupUserServiceTestDB(t)
	createRuntimeModeConfigForTest(t, models.RuntimeModeLocal, "")
	t.Setenv("NOISE_PLAIN_PASSWORD_STORE", filepath.Join(t.TempDir(), "plain-passwords.db"))
	setRegistrationProvisionForTest(t, func(string, string, string) registrationVoceChatProvisionResult {
		t.Fatal("local registration must not call the VoceChat provision adapter")
		return registrationVoceChatProvisionResult{}
	})

	const registrations = 5
	results := make(chan RegisterResult, registrations)
	errorsSeen := make(chan error, registrations)
	var wait sync.WaitGroup
	for index := 0; index < registrations; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			result, err := RegisterWithResult(dto.RegisterDto{Username: fmt.Sprintf("local_%02d", index), Password: "local-password"})
			if err != nil {
				errorsSeen <- err
				return
			}
			results <- result
		}(index)
	}
	wait.Wait()
	close(results)
	close(errorsSeen)
	for err := range errorsSeen {
		t.Fatalf("concurrent local registration: %v", err)
	}

	ids := make([]string, 0, registrations)
	for result := range results {
		if result.AutoApproved || result.Status != models.RegistrationApplicationStatusPending {
			t.Fatalf("local registration result = %#v", result)
		}
		ids = append(ids, result.ApplicationID)
	}
	sort.Strings(ids)
	if got, want := fmt.Sprint(ids), "[1 2 3 4 5]"; got != want {
		t.Fatalf("concurrent application IDs = %s, want %s", got, want)
	}

	highest, err := repository.GetRegistrationApplicationByApplicationID("5")
	if err != nil {
		t.Fatalf("get highest application: %v", err)
	}
	if err := database.DB.Delete(highest).Error; err != nil {
		t.Fatalf("delete highest application: %v", err)
	}
	result, err := RegisterWithResult(dto.RegisterDto{Username: "local_next", Password: "local-password"})
	if err != nil {
		t.Fatalf("register after deleting highest application: %v", err)
	}
	if result.ApplicationID != "6" {
		t.Fatalf("application ID after deletion = %q, want 6", result.ApplicationID)
	}
}

func TestLocalRegistrationPersistsCandidateBeforeReviewAndNeverAutoApproves(t *testing.T) {
	setupUserServiceTestDB(t)
	createRuntimeModeConfigForTest(t, models.RuntimeModeLocal, "")
	t.Setenv("NOISE_PLAIN_PASSWORD_STORE", filepath.Join(t.TempDir(), "plain-passwords.db"))
	if err := database.DB.Create(&models.Setting{AllowRegistration: true, AutoApproveRegistration: true}).Error; err != nil {
		t.Fatalf("create registration setting: %v", err)
	}
	setRegistrationProvisionForTest(t, func(string, string, string) registrationVoceChatProvisionResult {
		t.Fatal("local registration must not call VoceChat")
		return registrationVoceChatProvisionResult{}
	})

	result, err := RegisterWithResult(dto.RegisterDto{Username: "local_candidate", Password: "local-password"})
	if err != nil {
		t.Fatalf("register local candidate: %v", err)
	}
	if result.AutoApproved || result.Status != models.RegistrationApplicationStatusPending {
		t.Fatalf("local registration result = %#v", result)
	}
	application, err := repository.GetRegistrationApplicationByApplicationID(result.ApplicationID)
	if err != nil {
		t.Fatalf("get local application: %v", err)
	}
	if application.VoceChatCandidateEmail != result.ApplicationID+"@vc.com" {
		t.Fatalf("candidate email = %q", application.VoceChatCandidateEmail)
	}
	if application.VoceChatEmail != "" || application.VoceChatUserID != "" || application.VoceChatSyncStatus != models.VoceChatSyncStatusUnbound {
		t.Fatalf("local application masquerades as bound: %#v", application)
	}

	created, err := ApproveRegistrationApplication(application.ID, models.PrimaryAdminUserID, "local approval")
	if err != nil {
		t.Fatalf("approve local application: %v", err)
	}
	if created.VoceChatSyncStatus != models.VoceChatSyncStatusUnbound || created.VoceChatEmail != "" || created.VoceChatUserID != "" {
		t.Fatalf("approved local user = %#v", created)
	}
	approved, err := repository.GetRegistrationApplicationByID(application.ID)
	if err != nil {
		t.Fatalf("reload approved application: %v", err)
	}
	if approved.VoceChatCandidateEmail != application.VoceChatCandidateEmail || approved.LocalUserID == nil || *approved.LocalUserID != created.ID {
		t.Fatalf("approved application lost permanent allocation or relation: %#v", approved)
	}
	record, ok, err := vocechat.DefaultPlainPasswordStore().GetUserPassword(created.ID)
	if err != nil || !ok || record.LocalFallbackPasswordValue() != "local-password" || record.VoceChatPasswordValue() != "" {
		t.Fatalf("approved local password state: ok=%v err=%v record=%#v", ok, err, record)
	}
}

func TestVoceChatRegistrationPersistsApplicationBeforeExternalCreation(t *testing.T) {
	setupUserServiceTestDB(t)
	createRuntimeModeConfigForTest(t, models.RuntimeModeVoceChat, "ok")
	t.Setenv("NOISE_PLAIN_PASSWORD_STORE", filepath.Join(t.TempDir(), "plain-passwords.db"))
	setRegistrationProvisionForTest(t, func(applicationID, username, password string) registrationVoceChatProvisionResult {
		application, err := repository.GetRegistrationApplicationByApplicationID(applicationID)
		if err != nil {
			t.Fatalf("application was not traceable before external creation: %v", err)
		}
		if application.VoceChatCandidateEmail != applicationID+"@vc.com" || application.VoceChatEmail != "" {
			t.Fatalf("pre-provision application = %#v", application)
		}
		return registrationVoceChatProvisionResult{Email: "remote-actual@vc.example", UserID: "77", Username: username, SyncStatus: models.VoceChatSyncStatusCreated}
	})

	result, err := RegisterWithResult(dto.RegisterDto{Username: "vc_traceable", Password: "vc-password"})
	if err != nil {
		t.Fatalf("register VoceChat user: %v", err)
	}
	application, err := repository.GetRegistrationApplicationByApplicationID(result.ApplicationID)
	if err != nil {
		t.Fatalf("get provisioned application: %v", err)
	}
	if application.VoceChatCandidateEmail != result.ApplicationID+"@vc.com" || application.VoceChatEmail != "remote-actual@vc.example" || application.VoceChatUserID != "77" {
		t.Fatalf("provisioned application = %#v", application)
	}
}

func TestNormalVoceChatRegistrationPreservesConfiguredAutoApproval(t *testing.T) {
	setupUserServiceTestDB(t)
	createRuntimeModeConfigForTest(t, models.RuntimeModeVoceChat, "ok")
	t.Setenv("NOISE_PLAIN_PASSWORD_STORE", filepath.Join(t.TempDir(), "plain-passwords.db"))
	if err := database.DB.Create(&models.Setting{AllowRegistration: true, AutoApproveRegistration: true}).Error; err != nil {
		t.Fatalf("create registration setting: %v", err)
	}
	setRegistrationProvisionForTest(t, func(applicationID, username, password string) registrationVoceChatProvisionResult {
		return registrationVoceChatProvisionResult{
			Email:      applicationID + "@vc.example",
			UserID:     "88",
			Username:   username,
			SyncStatus: models.VoceChatSyncStatusCreated,
		}
	})
	setRegistrationVerifyForTest(t, func(models.RegistrationApplication) (bool, error) {
		return true, nil
	})

	result, err := RegisterWithResult(dto.RegisterDto{Username: "vc_auto", Password: "vc-password"})
	if err != nil {
		t.Fatalf("register with auto approval: %v", err)
	}
	if !result.AutoApproved || result.Status != models.RegistrationApplicationStatusApproved || result.LocalUserID == nil {
		t.Fatalf("auto approval result = %#v", result)
	}
	created, err := repository.GetUserByUsername("vc_auto")
	if err != nil {
		t.Fatalf("get auto-approved user: %v", err)
	}
	if created.VoceChatEmail != result.ApplicationID+"@vc.example" || created.VoceChatUserID != "88" || created.VoceChatSyncStatus != models.VoceChatSyncStatusLinked {
		t.Fatalf("auto-approved user = %#v", created)
	}
}

func TestDegradedVoceChatRegistrationStaysPendingWithoutExternalCreation(t *testing.T) {
	setupUserServiceTestDB(t)
	createRuntimeModeConfigForTest(t, models.RuntimeModeVoceChat, "failed")
	t.Setenv("NOISE_PLAIN_PASSWORD_STORE", filepath.Join(t.TempDir(), "plain-passwords.db"))
	if err := database.DB.Create(&models.Setting{AllowRegistration: true, AutoApproveRegistration: true}).Error; err != nil {
		t.Fatalf("create registration setting: %v", err)
	}
	setRegistrationProvisionForTest(t, func(string, string, string) registrationVoceChatProvisionResult {
		t.Fatal("degraded VoceChat registration must not call the provision adapter")
		return registrationVoceChatProvisionResult{}
	})

	result, err := RegisterWithResult(dto.RegisterDto{Username: "vc_degraded", Password: "vc-password"})
	if err != nil {
		t.Fatalf("register while degraded: %v", err)
	}
	if result.AutoApproved || result.Status != models.RegistrationApplicationStatusPending || result.LocalUserID != nil {
		t.Fatalf("degraded registration result = %#v", result)
	}
	application, err := repository.GetRegistrationApplicationByApplicationID(result.ApplicationID)
	if err != nil {
		t.Fatalf("get degraded application: %v", err)
	}
	if application.VoceChatCandidateEmail != result.ApplicationID+"@vc.com" || application.VoceChatEmail != "" || application.VoceChatUserID != "" {
		t.Fatalf("degraded application identity = %#v", application)
	}
	if application.VoceChatSyncStatus != models.VoceChatSyncStatusPending || !strings.Contains(application.VoceChatSyncError, "VoceChat") {
		t.Fatalf("degraded application status = %#v", application)
	}
	if _, err := repository.GetUserByUsername("vc_degraded"); err == nil {
		t.Fatal("degraded registration created a local user before review")
	}
}
