package updates

import (
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
)

func setupPreparationTest(t *testing.T) (*gorm.DB, *TaskService, models.UpdateExecutorCredential, string) {
	t.Helper()
	db := openTaskTestDB(t, ":memory:")
	service := NewTaskService(db)
	credential, token, err := service.CreateCredential(models.PrimaryAdminUserID, "prepare host")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := authenticateReadyForTest(service, token); err != nil {
		t.Fatal(err)
	}
	stale := time.Now().UTC().Add(-ExecutorCheckWindow - time.Minute)
	if err := db.Model(&credential).Update("checked_at", stale).Error; err != nil {
		t.Fatal(err)
	}
	return db, service, credential, token
}

func TestExecutorPreparationPersistsAndConcurrentRequestsCoalesce(t *testing.T) {
	db, service, credential, token := setupPreparationTest(t)
	before, err := service.PreparationStatus(strings.Repeat("1", 40))
	if err != nil || before.Installation.Reason != "executor_offline" || before.RequestedAt != nil {
		t.Fatalf("before=%#v err=%v", before, err)
	}
	type result struct {
		preparation DeploymentPreparation
		created     bool
		err         error
	}
	const workers = 12
	results := make(chan result, workers)
	var group sync.WaitGroup
	for range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			p, created, err := NewTaskService(db).RequestDeploymentCheck(models.PrimaryAdminUserID, strings.Repeat("1", 40))
			results <- result{p, created, err}
		}()
	}
	group.Wait()
	close(results)
	var requested *time.Time
	writes := 0
	for result := range results {
		if result.err != nil || result.preparation.CredentialID != credential.ID || result.preparation.RequestedAt == nil {
			t.Fatalf("prepare=%#v err=%v", result.preparation, result.err)
		}
		if requested == nil {
			requested = result.preparation.RequestedAt
		} else if !requested.Equal(*result.preparation.RequestedAt) {
			t.Fatal("concurrent requests did not share the durable timestamp")
		}
		if result.created {
			writes++
		}
	}
	if writes != 1 {
		t.Fatalf("wake-authorizing writes=%d, want 1", writes)
	}
	var tasks int64
	if err := db.Model(&models.UpdateTask{}).Count(&tasks).Error; err != nil || tasks != 0 {
		t.Fatalf("preparation created tasks=%d err=%v", tasks, err)
	}
	var original models.UpdateExecutorCredential
	if err := db.First(&original, credential.ID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := service.Authenticate(token); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		work, err := service.ExecutorWork(credential.ID)
		if err != nil || !work.CheckRequested || work.TaskAvailable {
			t.Fatalf("work=%#v err=%v", work, err)
		}
	}
	var after models.UpdateExecutorCredential
	if err := db.First(&after, credential.ID).Error; err != nil {
		t.Fatal(err)
	}
	if after.CheckedAt == nil || !after.CheckedAt.Equal(*original.CheckedAt) || after.CheckOK != original.CheckOK || !after.CheckRequestedAt.Equal(*requested) {
		t.Fatal("authenticate/work refreshed or consumed the deployment check")
	}
}

func TestExecutorPreparationFailedCheckCompletesRequestAndUsesCooldown(t *testing.T) {
	db, service, credential, _ := setupPreparationTest(t)
	p, created, err := service.RequestDeploymentCheck(1, strings.Repeat("1", 40))
	if err != nil || !created || p.RequestedAt == nil {
		t.Fatalf("prepare=%#v created=%v err=%v", p, created, err)
	}
	instance, _ := service.InstanceID()
	check := DeploymentCheck{InstanceID: instance, Version: ExecutorVersion, Platform: "linux/amd64", Revision: strings.Repeat("1", 40), OK: false}
	if err := service.RecordDeploymentCheck(credential.ID, check); err != nil {
		t.Fatal(err)
	}
	work, err := service.ExecutorWork(credential.ID)
	if err != nil || work.CheckRequested || work.TaskAvailable {
		t.Fatalf("failed check left pending work=%#v err=%v", work, err)
	}
	cooled, created, err := service.RequestDeploymentCheck(1, check.Revision)
	if !errors.Is(err, ErrExecutorPrepareCooldown) || created || cooled.Installation.Reason != "deployment_check_failed" || !cooled.RequestedAt.Equal(*p.RequestedAt) {
		t.Fatalf("cooldown=%#v created=%v err=%v", cooled, created, err)
	}
	past := time.Now().UTC().Add(-ExecutorPrepareCooldown - time.Second)
	if err := db.Model(&credential).Update("check_requested_at", past).Error; err != nil {
		t.Fatal(err)
	}
	next, created, err := service.RequestDeploymentCheck(1, check.Revision)
	if err != nil || !created || !next.RequestedAt.After(past) {
		t.Fatalf("retry=%#v created=%v err=%v", next, created, err)
	}
	check.OK = true
	if err := service.RecordDeploymentCheck(credential.ID, check); err != nil {
		t.Fatal(err)
	}
	ready, created, err := service.RequestDeploymentCheck(1, check.Revision)
	if err != nil || created || !ready.Installation.Available || !ready.RequestedAt.Equal(*next.RequestedAt) {
		t.Fatalf("ready=%#v created=%v err=%v", ready, created, err)
	}
	work, err = service.ExecutorWork(credential.ID)
	if err != nil || work.CheckRequested {
		t.Fatalf("successful old check did not consume request: %#v %v", work, err)
	}
}

func TestExecutorPreparationHardConditionsAndActiveTaskDoNotRequestCheck(t *testing.T) {
	for _, condition := range []string{"missing", "expired", "revoked", "platform", "version", "instance", "active", "actor", "ready"} {
		t.Run(condition, func(t *testing.T) {
			db, service, credential, _ := setupPreparationTest(t)
			actorID := uint(1)
			want := ErrExecutorNotConfigured
			switch condition {
			case "missing":
				if err := db.Delete(&credential).Error; err != nil {
					t.Fatal(err)
				}
			case "expired":
				if err := db.Model(&credential).Update("expires_at", time.Now().Add(-time.Minute)).Error; err != nil {
					t.Fatal(err)
				}
			case "revoked":
				if err := service.RevokeCredential(1); err != nil {
					t.Fatal(err)
				}
			case "platform", "version", "instance", "ready":
				values := map[string]any{"checked_at": time.Now().UTC()}
				switch condition {
				case "platform":
					values["platform"] = "linux/arm64"
				case "version":
					values["executor_version"] = "u5-1"
				case "instance":
					values["instance_id"] = "wrong-instance"
				case "ready":
					want = nil
				}
				if err := db.Model(&credential).Updates(values).Error; err != nil {
					t.Fatal(err)
				}
			case "active":
				slot := uint(1)
				if err := db.Create(&models.UpdateTask{PublicID: "occupied", Status: TaskNeedsAttention, ActiveSlot: &slot}).Error; err != nil {
					t.Fatal(err)
				}
				want = ErrUpdateTaskActive
			case "actor":
				actorID, want = 2, ErrCredentialInvalid
			}
			_, created, err := service.RequestDeploymentCheck(actorID, strings.Repeat("1", 40))
			if !errors.Is(err, want) || created {
				t.Fatalf("created=%v err=%v want=%v", created, err, want)
			}
			var count int64
			if err := db.Model(&models.UpdateExecutorCredential{}).Where("check_requested_at IS NOT NULL").Count(&count).Error; err != nil || count != 0 {
				t.Fatalf("hard condition wrote %d requests: %v", count, err)
			}
		})
	}
}

func TestExecutorPreparationPersistsAcrossRestartAndRotationDoesNotInherit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prepare.db")
	db := openTaskTestDB(t, path)
	service := NewTaskService(db)
	credential, _, err := service.CreateCredential(1, "host")
	if err != nil {
		t.Fatal(err)
	}
	request, created, err := service.RequestDeploymentCheck(1, strings.Repeat("1", 40))
	if err != nil || !created {
		t.Fatalf("created=%v err=%v", created, err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := openTaskTestDB(t, path)
	reopenedSQL, err := reopened.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopenedSQL.Close() })
	service = NewTaskService(reopened)
	p, err := service.PreparationStatus(strings.Repeat("1", 40))
	if err != nil || p.CredentialID != credential.ID || p.RequestedAt == nil || !p.RequestedAt.Equal(*request.RequestedAt) {
		t.Fatalf("restart lost request: %#v %v", p, err)
	}
	rotated, _, err := service.CreateCredential(1, "new host")
	if err != nil {
		t.Fatal(err)
	}
	p, err = service.PreparationStatus(strings.Repeat("1", 40))
	if err != nil || p.CredentialID != rotated.ID || p.RequestedAt != nil {
		t.Fatalf("rotation inherited request: %#v %v", p, err)
	}
	work, err := service.ExecutorWork(rotated.ID)
	if err != nil || work.CheckRequested || work.TaskAvailable {
		t.Fatalf("rotation inherited work: %#v %v", work, err)
	}
	if _, err := service.ExecutorWork(credential.ID); err != nil {
		t.Fatal(err)
	}
	if err := service.RevokeCredential(1); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ExecutorWork(rotated.ID); !errors.Is(err, ErrCredentialInvalid) {
		t.Fatalf("revoked work err=%v", err)
	}
	if _, created, err := service.RequestDeploymentCheck(1, ""); !errors.Is(err, ErrExecutorNotConfigured) || created {
		t.Fatalf("revoked prepare created=%v err=%v", created, err)
	}
}

func TestExecutorWorkPendingStaleCheckAndAssignedOwnership(t *testing.T) {
	db, service, credential, token := setupPreparationTest(t)
	if _, err := authenticateReadyForTest(service, token); err != nil {
		t.Fatal(err)
	}
	task, _, err := service.Create(1, Target{Channel: "edge", Image: officialUpdateImage, Digest: "sha256:" + strings.Repeat("a", 64), Revision: strings.Repeat("2", 40)})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&credential).Update("checked_at", time.Now().Add(-time.Hour)).Error; err != nil {
		t.Fatal(err)
	}
	work, err := service.ExecutorWork(credential.ID)
	if err != nil || !work.TaskAvailable || work.CheckRequested {
		t.Fatalf("pending stale work=%#v err=%v", work, err)
	}
	// The old check/report/claim sequence is still sufficient; no preparation
	// request is necessary for an already pending task.
	if _, err := authenticateReadyForTest(service, token); err != nil {
		t.Fatal(err)
	}
	claimed, err := service.Claim(credential.ID)
	if err != nil || claimed.PublicID != task.PublicID {
		t.Fatalf("claimed=%#v err=%v", claimed, err)
	}
	other, _, err := service.CreateCredential(1, "rotated")
	if err != nil {
		t.Fatal(err)
	}
	work, err = service.ExecutorWork(other.ID)
	if err != nil || work.TaskAvailable || work.CheckRequested {
		t.Fatalf("other credential was offered assigned work=%#v err=%v", work, err)
	}
	work, err = service.ExecutorWork(credential.ID)
	if err != nil || !work.TaskAvailable || work.CheckRequested {
		t.Fatalf("owner lost recovery work=%#v err=%v", work, err)
	}
	var events int64
	if err := db.Model(&models.UpdateTaskEvent{}).Count(&events).Error; err != nil || events != 1 {
		t.Fatalf("work created events=%d err=%v", events, err)
	}
}

func TestExecutorPreparationConditionalWriteRejectsCredentialInvalidatedBeforeUpdate(t *testing.T) {
	db, service, credential, _ := setupPreparationTest(t)
	name := "test:invalidate_prepare_credential"
	if err := db.Callback().Update().Before("gorm:update").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table == "update_executor_credentials" {
			// Invalidate precisely after the service has read its snapshot but
			// before its guarded write. Exec does not re-enter update callbacks.
			if err := tx.Exec("UPDATE update_executor_credentials SET revoked_at = ? WHERE id = ?", time.Now().UTC(), credential.ID).Error; err != nil {
				tx.AddError(err)
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Callback().Update().Remove(name) })
	_, created, err := service.RequestDeploymentCheck(1, strings.Repeat("1", 40))
	if !errors.Is(err, ErrExecutorNotConfigured) || created {
		t.Fatalf("invalidated credential wrote request: created=%v err=%v", created, err)
	}
	var saved models.UpdateExecutorCredential
	if err := db.First(&saved, credential.ID).Error; err != nil {
		t.Fatal(err)
	}
	if saved.CheckRequestedAt != nil {
		t.Fatal("conditional credential guard was bypassed")
	}
}

func TestExecutorPreparationDatabaseFailureRollsBackAndDoesNotAuthorizeWake(t *testing.T) {
	db, service, credential, _ := setupPreparationTest(t)
	failure := errors.New("injected transaction failure")
	name := "test:fail_after_prepare_write"
	if err := db.Callback().Update().After("gorm:update").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table == "update_executor_credentials" {
			tx.AddError(failure)
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Callback().Update().Remove(name) })
	_, created, err := service.RequestDeploymentCheck(1, strings.Repeat("1", 40))
	if !errors.Is(err, failure) || created {
		t.Fatalf("failed transaction authorized wake: created=%v err=%v", created, err)
	}
	var saved models.UpdateExecutorCredential
	if err := db.First(&saved, credential.ID).Error; err != nil {
		t.Fatal(err)
	}
	if saved.CheckRequestedAt != nil {
		t.Fatal("failed transaction retained the request write")
	}
}

func TestExecutorPreparationEmptyStateDoesNotCreateIdentityOrCredential(t *testing.T) {
	db := openTaskTestDB(t, ":memory:")
	preparation, err := NewTaskService(db).PreparationStatus("")
	if err != nil || preparation.CredentialID != 0 || preparation.RequestedAt != nil || preparation.Installation.Reason != "executor_unconfigured" {
		t.Fatalf("empty preparation=%#v err=%v", preparation, err)
	}
	for _, model := range []any{&models.UpdatePreference{}, &models.UpdateExecutorCredential{}, &models.UpdateTask{}} {
		var count int64
		if err := db.Model(model).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("status created %T rows=%d err=%v", model, count, err)
		}
	}
}

func TestExecutorWorkExpiredCredentialCannotAdvertisePendingRequest(t *testing.T) {
	db, service, credential, _ := setupPreparationTest(t)
	if _, _, err := service.RequestDeploymentCheck(1, ""); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&credential).Update("expires_at", time.Now().UTC().Add(-time.Minute)).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := service.ExecutorWork(credential.ID); !errors.Is(err, ErrCredentialInvalid) {
		t.Fatalf("expired credential work err=%v", err)
	}
	preparation, created, err := service.RequestDeploymentCheck(1, "")
	if !errors.Is(err, ErrExecutorNotConfigured) || created || preparation.Installation.Reason != "credential_expired" {
		t.Fatalf("expired preparation=%#v created=%v err=%v", preparation, created, err)
	}
}

func TestExecutorPreparationConcurrentRotationNeverWritesSupersededCredential(t *testing.T) {
	db, service, old, _ := setupPreparationTest(t)
	var group sync.WaitGroup
	errorsFound := make(chan error, 9)
	start := make(chan struct{})
	for range 8 {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			_, _, err := NewTaskService(db).RequestDeploymentCheck(1, "")
			errorsFound <- err
		}()
	}
	group.Add(1)
	go func() {
		defer group.Done()
		<-start
		_, _, err := service.CreateCredential(1, "concurrent rotation")
		errorsFound <- err
	}()
	close(start)
	group.Wait()
	close(errorsFound)
	for err := range errorsFound {
		if err != nil {
			t.Fatal(err)
		}
	}
	var superseded models.UpdateExecutorCredential
	if err := db.First(&superseded, old.ID).Error; err != nil {
		t.Fatal(err)
	}
	if superseded.SupersededAt == nil || (superseded.CheckRequestedAt != nil && superseded.CheckRequestedAt.After(*superseded.SupersededAt)) {
		t.Fatalf("request written after rotation: %#v", superseded)
	}
	preparation, _, err := service.RequestDeploymentCheck(1, "")
	if err != nil || preparation.CredentialID == old.ID {
		t.Fatalf("prepare returned superseded snapshot: %#v %v", preparation, err)
	}
}
