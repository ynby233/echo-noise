package services

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/rcy1314/echo-noise/internal/dto"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/repository"
)

func TestConcurrentRegistrationAdmissionPreservesQuotaAndNames(t *testing.T) {
	for _, tc := range []struct {
		name  string
		names []string
		want  int
	}{
		{"quota", []string{"burst0", "burst1", "burst2", "burst3", "burst4", "burst5", "burst6", "burst7"}, 5},
		{"same_name", []string{"same_name", "same_name", "same_name"}, 1},
		{"case_fold", []string{"Same_Name", "same_name", "SAME_NAME"}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupUserServiceTestDB(t)
			createRuntimeModeConfigForTest(t, models.RuntimeModeLocal, "")
			t.Setenv("NOISE_PLAIN_PASSWORD_STORE", filepath.Join(t.TempDir(), "passwords.db"))
			var success atomic.Int32
			start := make(chan struct{})
			var wg sync.WaitGroup
			for _, name := range tc.names {
				wg.Add(1)
				go func(name string) {
					defer wg.Done()
					<-start
					if _, err := RegisterWithResult(dto.RegisterDto{Username: name, Password: "test-password"}); err == nil {
						success.Add(1)
					}
				}(name)
			}
			close(start)
			wg.Wait()
			var count int64
			if err := db.Model(&models.RegistrationApplication{}).Where("status = ?", models.RegistrationApplicationStatusPending).Count(&count).Error; err != nil {
				t.Fatal(err)
			}
			if int(success.Load()) != tc.want || count != int64(tc.want) {
				t.Fatalf("admitted=%d pending=%d want=%d", success.Load(), count, tc.want)
			}
		})
	}
}

func TestConcurrentRegistrationReviewHasOneWinner(t *testing.T) {
	for _, secondAction := range []string{"approve", "reject"} {
		t.Run(secondAction, func(t *testing.T) {
			setupUserServiceTestDB(t)
			createRuntimeModeConfigForTest(t, models.RuntimeModeVoceChat, "ok")
			t.Setenv("NOISE_PLAIN_PASSWORD_STORE", filepath.Join(t.TempDir(), "passwords.db"))
			admin := mustCreateUser(t, models.User{Username: "admin", Password: models.HashPassword("admin"), IsAdmin: true})
			setRegistrationProvisionForTest(t, func(id, name, password string) registrationVoceChatProvisionResult {
				return registrationVoceChatProvisionResult{UserID: "42", Email: id + "@vc.com", SyncStatus: models.VoceChatSyncStatusCreated}
			})
			if err := Register(dto.RegisterDto{Username: "race_user", Password: "test-password"}); err != nil {
				t.Fatal(err)
			}
			app, err := repository.GetPendingRegistrationApplicationByUsername("race_user")
			if err != nil {
				t.Fatal(err)
			}
			entered, release := make(chan struct{}), make(chan struct{})
			var releaseOnce sync.Once
			t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
			var verifies atomic.Int32
			setRegistrationVerifyForTest(t, func(models.RegistrationApplication) (bool, error) {
				if verifies.Add(1) == 1 {
					close(entered)
					<-release
				}
				return true, nil
			})
			var deletes atomic.Int32
			setRegistrationDeleteForTest(t, func(models.RegistrationApplication) error { deletes.Add(1); return nil })
			first := make(chan error, 1)
			go func() { _, err := ApproveRegistrationApplication(app.ID, admin.ID, "first"); first <- err }()
			<-entered
			second := make(chan error, 1)
			started := make(chan struct{})
			go func() {
				close(started)
				if secondAction == "approve" {
					_, err := ApproveRegistrationApplication(app.ID, admin.ID, "second")
					second <- err
				} else {
					second <- RejectRegistrationApplication(app.ID, admin.ID, "second")
				}
			}()
			<-started
			releaseOnce.Do(func() { close(release) })
			if err := <-first; err != nil {
				t.Fatal(err)
			}
			if err := <-second; err == nil || !strings.Contains(err.Error(), "已处理") {
				t.Fatalf("second review must see final state, got %v", err)
			}
			if deletes.Load() != 0 || verifies.Load() != 1 {
				t.Fatalf("unexpected remote calls verify=%d delete=%d", verifies.Load(), deletes.Load())
			}
			approved, err := repository.GetRegistrationApplicationByID(app.ID)
			if err != nil || approved.Status != models.RegistrationApplicationStatusApproved {
				t.Fatal("winning approval was overwritten")
			}
		})
	}
}

func TestRegistrationReviewFailureReleasesWriter(t *testing.T) {
	setupUserServiceTestDB(t)
	createRuntimeModeConfigForTest(t, models.RuntimeModeVoceChat, "ok")
	t.Setenv("NOISE_PLAIN_PASSWORD_STORE", filepath.Join(t.TempDir(), "passwords.db"))
	setRegistrationProvisionForTest(t, func(id, name, password string) registrationVoceChatProvisionResult {
		return registrationVoceChatProvisionResult{UserID: "42", Email: id + "@vc.com", SyncStatus: models.VoceChatSyncStatusCreated}
	})
	if err := Register(dto.RegisterDto{Username: "retry_review", Password: "test-password"}); err != nil {
		t.Fatal(err)
	}
	app, err := repository.GetPendingRegistrationApplicationByUsername("retry_review")
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	setRegistrationDeleteForTest(t, func(models.RegistrationApplication) error {
		calls++
		if calls == 1 {
			return errors.New("temporary remote failure")
		}
		return nil
	})
	if err := RejectRegistrationApplication(app.ID, 1, ""); err == nil {
		t.Fatal("first rejection should fail")
	}
	if err := RejectRegistrationApplication(app.ID, 1, ""); err != nil {
		t.Fatal(err)
	}
	for i := range 2 {
		if _, err := RegisterWithResult(dto.RegisterDto{Username: fmt.Sprintf("after_retry%d", i), Password: "test-password"}); err != nil {
			t.Fatal(err)
		}
	}
}
