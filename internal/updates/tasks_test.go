package updates

import (
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func authenticateReadyForTest(service *TaskService, token string) (models.UpdateExecutorCredential, error) {
	credential, err := service.Authenticate(token)
	if err != nil {
		return credential, err
	}
	instance, err := service.InstanceID()
	if err != nil {
		return credential, err
	}
	err = service.RecordDeploymentCheck(credential.ID, DeploymentCheck{InstanceID: instance, Version: ExecutorVersion, Platform: "linux/amd64", Revision: strings.Repeat("1", 40), OK: true})
	return credential, err
}

func openTaskTestDB(t *testing.T, path string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if sqlDB, err := db.DB(); err != nil {
		t.Fatal(err)
	} else {
		sqlDB.SetMaxOpenConns(1) // Production SQLite serializes writers this way.
	}
	if err := db.AutoMigrate(&models.UpdatePreference{}, &models.UpdateTask{}, &models.UpdateExecutorCredential{}, &models.UpdateTaskEvent{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestOfflineSettlementAfterRevocationPreservesEvidence(t *testing.T) {
	db := openTaskTestDB(t, ":memory:")
	s := NewTaskService(db)
	credential, token, err := s.CreateCredential(1, "host")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := authenticateReadyForTest(s, token); err != nil {
		t.Fatal(err)
	}
	target := Target{Channel: "edge", Image: officialUpdateImage, Digest: "sha256:" + strings.Repeat("a", 64), Revision: strings.Repeat("1", 40)}
	task, _, err := s.Create(1, target)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Claim(credential.ID); err != nil {
		t.Fatal(err)
	}
	for _, state := range []string{TaskDownloading, TaskStopping, TaskNeedsAttention} {
		if err := s.RecordEvent(credential.ID, task.PublicID, state, ""); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.RevokeCredential(1); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AuthenticateTaskReport(token, task.PublicID); err == nil {
		t.Fatal("revocation bypassed")
	}
	instance, _ := s.InstanceID()
	if err := s.SettleOffline("other", task.PublicID, "operator-confirmed-stop"); err == nil {
		t.Fatal("wrong instance accepted")
	}
	if err := s.SettleOffline(instance, task.PublicID, "succeeded"); err == nil {
		t.Fatal("manufactured success")
	}
	for i := 0; i < 2; i++ {
		if err := s.SettleOffline(instance, task.PublicID, "operator-confirmed-stop"); err != nil {
			t.Fatal(err)
		}
	}
	settled, _ := s.Get(task.PublicID)
	if settled.Status != TaskFailed || settled.ActiveSlot != nil || settled.FinishedAt == nil {
		t.Fatal("active slot not released")
	}
	var events []models.UpdateTaskEvent
	if err := db.Where("task_id = ?", task.ID).Order("id").Find(&events).Error; err != nil {
		t.Fatal(err)
	}
	if len(events) != 5 || events[4].Summary != "宿主管理员人工结案: operator-confirmed-stop" {
		t.Fatalf("audit evidence: %+v", events)
	}
	for _, state := range []string{TaskClaimed, TaskDownloading, TaskStopping, TaskBackingUp, TaskReplacing, TaskVerifying} {
		slot := uint(1)
		interrupted := models.UpdateTask{PublicID: "interrupted-" + state, Status: state, ActiveSlot: &slot, ExecutorCredentialID: &credential.ID}
		if err := db.Create(&interrupted).Error; err != nil {
			t.Fatal(err)
		}
		if err := s.SettleOffline(instance, interrupted.PublicID, "operator-confirmed-stop"); err != nil {
			t.Fatalf("lost offline attention at %s: %v", state, err)
		}
		got, _ := s.Get(interrupted.PublicID)
		if got.ActiveSlot != nil || got.Status != TaskFailed {
			t.Fatalf("interrupted %s not closed", state)
		}
	}
}

func TestConcurrentUpdateTaskCreateReturnsOneActiveTask(t *testing.T) {
	db := openTaskTestDB(t, ":memory:")
	service := NewTaskService(db)
	_, token, err := service.CreateCredential(models.PrimaryAdminUserID, "host")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := authenticateReadyForTest(service, token); err != nil {
		t.Fatal(err)
	}
	target := Target{Channel: "edge", Image: officialUpdateImage, Digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Revision: "1111111111111111111111111111111111111111"}
	const callers = 8
	results := make(chan struct {
		id  string
		err error
	}, callers)
	var pending sync.WaitGroup
	for i := 0; i < callers; i++ {
		pending.Add(1)
		go func() {
			defer pending.Done()
			task, _, err := service.Create(models.PrimaryAdminUserID, target)
			results <- struct {
				id  string
				err error
			}{task.PublicID, err}
		}()
	}
	pending.Wait()
	close(results)
	id := ""
	for result := range results {
		if result.err != nil || result.id == "" {
			t.Fatalf("concurrent create: id=%q err=%v", result.id, result.err)
		}
		if id != "" && result.id != id {
			t.Fatalf("parallel requests created different tasks: %q and %q", id, result.id)
		}
		id = result.id
	}
}

func TestUpdateTaskCreateIsIdempotentAndSurvivesRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "updates.db")
	db := openTaskTestDB(t, path)
	service := NewTaskService(db)
	if _, token, err := service.CreateCredential(1, "host executor"); err != nil {
		t.Fatal(err)
	} else if _, err := authenticateReadyForTest(service, token); err != nil {
		t.Fatal(err)
	}
	instanceID, err := service.InstanceID()
	if err != nil || len(instanceID) != 32 {
		t.Fatalf("paired instance id=%q err=%v", instanceID, err)
	}
	target := Target{Channel: "edge", Image: "ghcr.io/ynby233/echo-noise", Digest: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Revision: "2222222222222222222222222222222222222222"}

	first, created, err := service.Create(1, target)
	if err != nil || !created {
		t.Fatalf("first create: task=%#v created=%t err=%v", first, created, err)
	}
	second, created, err := service.Create(1, target)
	if err != nil || created || second.PublicID != first.PublicID {
		t.Fatalf("repeat create: task=%#v created=%t err=%v", second, created, err)
	}
	if sqlDB, err := db.DB(); err != nil {
		t.Fatal(err)
	} else if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}

	reopened := openTaskTestDB(t, path)
	t.Cleanup(func() {
		if sqlDB, err := reopened.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	resumed, err := NewTaskService(reopened).Get(first.PublicID)
	if err != nil || resumed.TargetDigest != target.Digest || resumed.Status != TaskPending {
		t.Fatalf("reopened task=%#v err=%v", resumed, err)
	}
	if id, err := NewTaskService(reopened).InstanceID(); err != nil || id != instanceID {
		t.Fatalf("instance identity changed after restart: %q err=%v", id, err)
	}
}

func TestUpdateTaskClaimIsRecoverableAndEventsAreScopedAndMonotonic(t *testing.T) {
	db := openTaskTestDB(t, ":memory:")
	service := NewTaskService(db)
	credential, _, err := service.CreateCredential(1, "host executor")
	if err != nil {
		t.Fatal(err)
	}
	other, _, err := service.CreateCredential(1, "replacement executor")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&other).Update("last_seen_at", time.Now().UTC()).Error; err != nil {
		t.Fatal(err)
	}
	instance, _ := service.InstanceID()
	if err := service.RecordDeploymentCheck(other.ID, DeploymentCheck{InstanceID: instance, Version: ExecutorVersion, Platform: "linux/amd64", Revision: strings.Repeat("1", 40), OK: true}); err != nil {
		t.Fatal(err)
	}
	task, _, err := service.Create(1, Target{Channel: "stable", Image: "ghcr.io/ynby233/echo-noise", Digest: "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", Revision: "3333333333333333333333333333333333333333"})
	if err != nil {
		t.Fatal(err)
	}

	claimed, err := service.Claim(other.ID)
	if err != nil || claimed == nil || claimed.PublicID != task.PublicID || claimed.Status != TaskClaimed {
		t.Fatalf("claim=%#v err=%v", claimed, err)
	}
	recovered, err := service.Claim(other.ID)
	if err != nil || recovered == nil || recovered.PublicID != task.PublicID {
		t.Fatalf("recover lost claim response=%#v err=%v", recovered, err)
	}
	if err := service.RecordEvent(credential.ID, task.PublicID, TaskDownloading, ""); !errors.Is(err, ErrTaskOwnership) {
		t.Fatalf("cross-executor event err=%v", err)
	}
	if err := service.RecordEvent(other.ID, task.PublicID, TaskDownloading, "pulling image"); err != nil {
		t.Fatal(err)
	}
	if err := service.RecordEvent(other.ID, task.PublicID, TaskClaimed, "late event"); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("reverse event err=%v", err)
	}
	if err := service.RecordEvent(other.ID, task.PublicID, TaskDownloading, "duplicate"); err != nil {
		t.Fatalf("duplicate event should be idempotent: %v", err)
	}
	for _, status := range []string{TaskStopping, TaskBackingUp, TaskReplacing, TaskVerifying, TaskSucceeded} {
		if err := service.RecordEvent(other.ID, task.PublicID, status, ""); err != nil {
			t.Fatalf("advance to %s: %v", status, err)
		}
	}
	next, created, err := service.Create(1, Target{Channel: "edge", Image: officialUpdateImage, Digest: "sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", Revision: "5555555555555555555555555555555555555555"})
	if err != nil || !created || next.PublicID == task.PublicID {
		t.Fatalf("terminal task did not release active slot: task=%#v created=%t err=%v", next, created, err)
	}
}

func TestFollowChannelDoesNotCreateTask(t *testing.T) {
	db := openTaskTestDB(t, ":memory:")
	if err := db.AutoMigrate(&models.UpdatePreference{}); err != nil {
		t.Fatal(err)
	}
	service := NewTaskService(db)
	if _, _, err := service.CreateCredential(models.PrimaryAdminUserID, "host"); err != nil {
		t.Fatal(err)
	}
	before, err := service.InstanceID()
	if err != nil {
		t.Fatal(err)
	}
	if err := service.SetChannel(models.PrimaryAdminUserID, "edge"); err != nil {
		t.Fatal(err)
	}
	if after, err := service.InstanceID(); err != nil || after != before {
		t.Fatalf("channel switch changed instance identity: %q -> %q, err=%v", before, after, err)
	}
	channel, err := service.GetChannel()
	if err != nil || channel != "edge" {
		t.Fatalf("channel=%q err=%v", channel, err)
	}
	var count int64
	if err := db.Model(&models.UpdateTask{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("channel preference created %d tasks: %v", count, err)
	}
}

func TestExecutorCredentialStoresOnlyHashAndRejectsExpiry(t *testing.T) {
	db := openTaskTestDB(t, ":memory:")
	service := NewTaskService(db)
	credential, raw, err := service.CreateCredential(models.PrimaryAdminUserID, "host")
	if err != nil {
		t.Fatal(err)
	}
	if credential.TokenHash == raw || credential.TokenHash != HashExecutorToken(raw) {
		t.Fatalf("credential verifier was not one-way: %#v", credential)
	}
	past := time.Now().UTC().Add(-time.Minute)
	if err := db.Model(&credential).Update("expires_at", past).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := authenticateReadyForTest(service, raw); !errors.Is(err, ErrCredentialInvalid) {
		t.Fatalf("expired token err=%v", err)
	}
}

func TestCredentialRotationLetsAssignedExecutorFinishButNotClaimAgain(t *testing.T) {
	db := openTaskTestDB(t, ":memory:")
	service := NewTaskService(db)
	oldCredential, oldToken, err := service.CreateCredential(models.PrimaryAdminUserID, "old host")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := authenticateReadyForTest(service, oldToken); err != nil {
		t.Fatal(err)
	}
	task, _, err := service.Create(models.PrimaryAdminUserID, Target{Channel: "edge", Image: officialUpdateImage, Digest: "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", Revision: "6666666666666666666666666666666666666666"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Claim(oldCredential.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.CreateCredential(models.PrimaryAdminUserID, "new host"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Authenticate(oldToken); err != nil {
		t.Fatalf("rotated credential lost its assigned task: %v", err)
	}
	if recovered, err := service.Claim(oldCredential.ID); err != nil || recovered == nil || recovered.PublicID != task.PublicID {
		t.Fatalf("recover after rotation: task=%#v err=%v", recovered, err)
	}
	for _, status := range []string{TaskDownloading, TaskStopping, TaskBackingUp, TaskReplacing, TaskVerifying, TaskSucceeded} {
		if err := service.RecordEvent(oldCredential.ID, task.PublicID, status, ""); err != nil {
			t.Fatalf("advance to %s: %v", status, err)
		}
	}
	if _, err := authenticateReadyForTest(service, oldToken); !errors.Is(err, ErrCredentialInvalid) {
		t.Fatalf("completed superseded credential remained valid: %v", err)
	}
}

func TestExecutorEventSummaryRedactsCredentials(t *testing.T) {
	db := openTaskTestDB(t, ":memory:")
	service := NewTaskService(db)
	credential, raw, err := service.CreateCredential(models.PrimaryAdminUserID, "host")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := authenticateReadyForTest(service, raw); err != nil {
		t.Fatal(err)
	}
	task, _, err := service.Create(models.PrimaryAdminUserID, Target{Channel: "edge", Image: officialUpdateImage, Digest: "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd", Revision: "4444444444444444444444444444444444444444"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Claim(credential.ID); err != nil {
		t.Fatal(err)
	}
	if err := service.RecordEvent(credential.ID, task.PublicID, TaskFailed, "request failed with Bearer "+raw); err != nil {
		t.Fatal(err)
	}
	var event models.UpdateTaskEvent
	if err := db.Where("task_id = ? AND status = ?", task.ID, TaskFailed).First(&event).Error; err != nil {
		t.Fatal(err)
	}
	if strings.Contains(event.Summary, raw) || event.Summary != "" {
		t.Fatalf("unsafe event summary=%q", event.Summary)
	}
}

func TestUpdateTaskNeedsAttentionBlocksReplacementUntilResolved(t *testing.T) {
	service := NewTaskService(openTaskTestDB(t, ":memory:"))
	credential, token, err := service.CreateCredential(1, "host")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := authenticateReadyForTest(service, token); err != nil {
		t.Fatal(err)
	}
	target := Target{Channel: "edge", Image: officialUpdateImage, Digest: "sha256:" + strings.Repeat("a", 64), Revision: strings.Repeat("1", 40)}
	task, _, err := service.Create(1, target)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Claim(credential.ID); err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{TaskDownloading, TaskStopping, TaskBackingUp, TaskReplacing, TaskNeedsAttention} {
		if err := service.RecordEvent(credential.ID, task.PublicID, status, ""); err != nil {
			t.Fatal(err)
		}
	}
	repeated, created, err := service.Create(1, target)
	if err != nil || created || repeated.PublicID != task.PublicID {
		t.Fatalf("unresolved task allowed a replacement: task=%#v created=%t err=%v", repeated, created, err)
	}
	recovered, err := service.Claim(credential.ID)
	if err != nil || recovered.PublicID != task.PublicID || recovered.Status != TaskNeedsAttention {
		t.Fatalf("cannot recover unresolved task: task=%#v err=%v", recovered, err)
	}
	if err := service.RecordEvent(credential.ID, task.PublicID, TaskReplacing, ""); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("unresolved task restarted installation: %v", err)
	}
	// The host must reconcile its journal and runtime before resuming verification.
	for _, status := range []string{TaskVerifying, TaskSucceeded} {
		if err := service.RecordEvent(credential.ID, task.PublicID, status, ""); err != nil {
			t.Fatal(err)
		}
	}
	if next, created, err := service.Create(1, target); err != nil || !created || next.PublicID == task.PublicID {
		t.Fatalf("resolved task did not release its slot: task=%#v created=%t err=%v", next, created, err)
	}
}

func TestUpdateTaskRejectsInvalidEventStatuses(t *testing.T) {
	for _, final := range []string{TaskClaimed, TaskSucceeded, TaskFailed} {
		t.Run(final, func(t *testing.T) {
			service := NewTaskService(openTaskTestDB(t, ":memory:"))
			credential, token, err := service.CreateCredential(1, "host")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := authenticateReadyForTest(service, token); err != nil {
				t.Fatal(err)
			}
			task, _, err := service.Create(1, Target{Channel: "edge", Image: officialUpdateImage, Digest: "sha256:" + strings.Repeat("a", 64), Revision: strings.Repeat("1", 40)})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := service.Claim(credential.ID); err != nil {
				t.Fatal(err)
			}
			if final == TaskSucceeded {
				for _, status := range []string{TaskDownloading, TaskStopping, TaskBackingUp, TaskReplacing, TaskVerifying, TaskSucceeded} {
					if err := service.RecordEvent(credential.ID, task.PublicID, status, ""); err != nil {
						t.Fatal(err)
					}
				}
			} else if final == TaskFailed {
				if err := service.RecordEvent(credential.ID, task.PublicID, TaskFailed, ""); err != nil {
					t.Fatal(err)
				}
			}
			for _, invalid := range []string{"", " \t", "unknown"} {
				if err := service.RecordEvent(credential.ID, task.PublicID, invalid, ""); !errors.Is(err, ErrInvalidTransition) {
					t.Errorf("status %q was accepted from %s: %v", invalid, final, err)
				}
			}
			if saved, err := service.Get(task.PublicID); err != nil || saved.Status != final {
				t.Fatalf("invalid event changed task: task=%#v err=%v", saved, err)
			}
		})
	}
}
