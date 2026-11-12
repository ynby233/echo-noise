package syncmanager

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	backup "github.com/rcy1314/echo-noise/internal/backup"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type fakeManagedCloud struct {
	objects   map[string]string
	modified  map[string]time.Time
	uploadErr error
	listErr   error
	deleteErr error
	listed    []string
	deleted   []string
}

func (f *fakeManagedCloud) Enabled() bool { return true }

func (f *fakeManagedCloud) Upload(ctx context.Context, key, path string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if f.uploadErr != nil {
		return f.uploadErr
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	f.objects[key] = string(body)
	f.modified[key] = time.Now()
	return nil
}
func (f *fakeManagedCloud) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	body, ok := f.objects[key]
	if !ok {
		return nil, os.ErrNotExist
	}
	return io.NopCloser(strings.NewReader(body)), nil
}
func (f *fakeManagedCloud) List(ctx context.Context, kind string) ([]managedObject, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f.listed = append(f.listed, kind)
	if f.listErr != nil {
		return nil, f.listErr
	}
	var objects []managedObject
	for key := range f.objects {
		if strings.HasPrefix(key, managedPrefix+kind+"/") {
			objects = append(objects, managedObject{Key: key, Modified: f.modified[key], Size: int64(len(f.objects[key]))})
		}
	}
	return objects, nil
}
func (f *fakeManagedCloud) Delete(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if f.deleteErr != nil {
		return f.deleteErr
	}
	f.deleted = append(f.deleted, key)
	delete(f.objects, key)
	return nil
}

func testManagedManager(t *testing.T) (*Manager, *fakeManagedCloud) {
	t.Helper()
	root := t.TempDir()
	db, err := gorm.Open(sqlite.Open(filepath.Join(root, "managed.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	cloud := &fakeManagedCloud{objects: make(map[string]string), modified: make(map[string]time.Time)}
	m := NewManager()
	m.db = func() *gorm.DB { return db }
	m.root = filepath.Join(root, "data", "backups")
	m.layout = func() backup.Layout { return backup.Layout{DatabasePath: filepath.Join(root, "source.db")} }
	m.cloud = cloud
	m.now = func() time.Time { return time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC) }
	m.archive = func(ctx context.Context, destination string, _ *gorm.DB, _ backup.Layout, kind backup.ArchiveKind, scope string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		return os.WriteFile(destination, []byte(string(kind)+":"+scope), 0600)
	}
	return m, cloud
}

func managedPlan(kind, target string) models.ManagedBackupPlan {
	return models.ManagedBackupPlan{Kind: kind, Enabled: true, Time: "03:00", Timezone: "UTC", Target: target, KeepLocal: 2, KeepCloud: 2}
}

func TestManagedPlansPersistAndLastRunCannotBeResetByClient(t *testing.T) {
	m, _ := testManagedManager(t)
	plans, err := m.Plans()
	if err != nil || len(plans) != 3 {
		t.Fatalf("plans=%v err=%v", plans, err)
	}
	for _, plan := range plans {
		if plan.Enabled || plan.LastRun != nil {
			t.Fatalf("unsafe default: %+v", plan)
		}
	}
	plan, err := m.SavePlan(managedPlan("attachments", "local"))
	if err != nil || plan.Scope != "all" {
		t.Fatalf("plan=%+v err=%v", plan, err)
	}
	if err := m.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	plan.LastRun = nil
	plan.Scope = "local"
	plan, err = m.SavePlan(plan)
	if err != nil || plan.LastRun == nil {
		t.Fatalf("claim cleared: %+v err=%v", plan, err)
	}
	if err := m.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	history, err := m.History("attachments")
	if err != nil || len(history) != 1 || history[0].Scope != "all" {
		t.Fatalf("history=%+v err=%v", history, err)
	}
	if plan.NextRun == nil || plan.NextRun.Day() != 12 {
		t.Fatalf("nextRun=%v", plan.NextRun)
	}
}

func TestManagedScheduledClaimSurvivesManagerRestartAndFailure(t *testing.T) {
	m, _ := testManagedManager(t)
	if _, err := m.SavePlan(managedPlan("data", "local")); err != nil {
		t.Fatal(err)
	}
	calls := 0
	m.archive = func(context.Context, string, *gorm.DB, backup.Layout, backup.ArchiveKind, string) error {
		calls++
		return errors.New("snapshot unavailable")
	}
	if err := m.Scan(context.Background()); err == nil {
		t.Fatal("expected snapshot failure")
	}
	// A new manager instance reads exactly the same persistent database.
	restarted := NewManager()
	restarted.db, restarted.root, restarted.layout = m.db, m.root, m.layout
	restarted.now, restarted.archive, restarted.cloud = m.now, m.archive, m.cloud
	if err := restarted.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("claimed slot ran %d times", calls)
	}
	history, err := restarted.History("data")
	if err != nil || len(history) != 1 || history[0].Status != "failed" || !strings.Contains(history[0].Error, "snapshot unavailable") {
		t.Fatalf("history=%+v err=%v", history, err)
	}
	restarted.now = func() time.Time { return time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC) }
	if err := restarted.Scan(context.Background()); err == nil {
		t.Fatal("next day must attempt the job")
	}
	if calls != 2 {
		t.Fatalf("next day calls=%d", calls)
	}
}

func TestManagedConcurrentScansClaimOnce(t *testing.T) {
	m, _ := testManagedManager(t)
	if _, err := m.SavePlan(managedPlan("data", "local")); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	failures := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() { defer wg.Done(); failures <- m.Scan(context.Background()) }()
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	history, err := m.History("data")
	if err != nil || len(history) != 1 {
		t.Fatalf("history=%+v err=%v", history, err)
	}
}

func TestManagedTimezoneDSTAndDueTime(t *testing.T) {
	plan := managedPlan("data", "local")
	plan.Timezone, plan.Time = "Asia/Shanghai", "03:00"
	before := time.Date(2026, 11, 10, 18, 59, 0, 0, time.UTC)
	slot, ok := dailySlot(plan, before)
	if !ok || !slot.Equal(time.Date(2026, 11, 10, 19, 0, 0, 0, time.UTC)) || !slot.After(before) {
		t.Fatalf("slot=%v ok=%v", slot, ok)
	}
	plan.Timezone, plan.Time = "America/New_York", "02:30"
	if _, ok := dailySlot(plan, time.Date(2026, 3, 8, 12, 0, 0, 0, time.UTC)); ok {
		t.Fatal("nonexistent DST time must be skipped")
	}
	plan.Time = "01:30"
	first := time.Date(2026, 11, 1, 5, 30, 0, 0, time.UTC)
	plan.LastRun = &first
	foldSlot, ok := dailySlot(plan, time.Date(2026, 11, 1, 6, 45, 0, 0, time.UTC))
	if !ok || !claimedToday(plan, foldSlot) {
		t.Fatal("repeated DST clock time was not recognized as claimed")
	}
}

func TestManagedCreateBothAndIndependentRetention(t *testing.T) {
	m, cloud := testManagedManager(t)
	plan := managedPlan("data", "both")
	plan.KeepLocal, plan.KeepCloud = 1, 2
	if _, err := m.SavePlan(plan); err != nil {
		t.Fatal(err)
	}
	var records []models.ManagedBackupRecord
	for day := range 3 {
		now := time.Date(2026, 11, 11+day, 12, 0, 0, 0, time.UTC)
		m.now = func() time.Time { return now }
		record, err := m.Create(context.Background(), "data", "both", "", "manual")
		if err != nil {
			t.Fatal(err)
		}
		cloud.modified[record.CloudKey] = now
		records = append(records, record)
	}
	old, err := m.FetchRecord(context.Background(), records[0].ID)
	if err != nil || old.Local || old.Cloud || old.Status != "success" {
		t.Fatalf("old=%+v err=%v", old, err)
	}
	middle, err := m.FetchRecord(context.Background(), records[1].ID)
	if err != nil || middle.Local || !middle.Cloud {
		t.Fatalf("middle=%+v err=%v", middle, err)
	}
	latest, err := m.FetchRecord(context.Background(), records[2].ID)
	if err != nil || !latest.Local || !latest.Cloud {
		t.Fatalf("latest=%+v err=%v", latest, err)
	}
	if _, err := os.Stat(filepath.Join(m.Directory("data"), old.ID+".zip")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("old local archive survived: %v", err)
	}
	if len(cloud.objects) != 2 {
		t.Fatalf("cloud copies=%d", len(cloud.objects))
	}
}

func TestManagedUploadFailurePreservesCopiesAndSkipsAllRetention(t *testing.T) {
	m, cloud := testManagedManager(t)
	plan := managedPlan("data", "both")
	plan.KeepLocal, plan.KeepCloud = 1, 1
	if _, err := m.SavePlan(plan); err != nil {
		t.Fatal(err)
	}
	first, err := m.Create(context.Background(), "data", "both", "", "manual")
	if err != nil {
		t.Fatal(err)
	}
	cloud.listed = nil
	cloud.uploadErr = errors.New("remote rejected upload")
	failed, err := m.Create(context.Background(), "data", "both", "", "manual")
	if err == nil || failed.Status != "failed" || !failed.Local || failed.Cloud {
		t.Fatalf("failed=%+v err=%v", failed, err)
	}
	if len(cloud.listed) != 0 || len(cloud.deleted) != 0 {
		t.Fatal("failed upload performed retention")
	}
	if _, err := os.Stat(filepath.Join(m.Directory("data"), first.ID+".zip")); err != nil {
		t.Fatal("failed upload deleted previous local copy", err)
	}
}

func TestManagedRetentionLeavesOtherKindsAndLegacyObjects(t *testing.T) {
	m, cloud := testManagedManager(t)
	cloud.objects["backup.zip"] = "legacy"
	cloud.objects[managedPrefix+"attachments/old.zip"] = "attachment"
	oldKey := managedKey("data", "10a24a2a-7366-4c10-a5f4-e3b287dfae7d")
	cloud.objects[oldKey] = "old data"
	cloud.modified[oldKey] = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	cloud.objects[managedPrefix+"data/unknown.zip"] = "unowned"
	plan := managedPlan("data", "cloud")
	plan.KeepCloud = 1
	if _, err := m.SavePlan(plan); err != nil {
		t.Fatal(err)
	}
	record, err := m.Create(context.Background(), "data", "cloud", "", "manual")
	if err != nil {
		t.Fatal(err)
	}
	if record.Local || !record.Cloud {
		t.Fatalf("record=%+v", record)
	}
	if _, err := os.Stat(filepath.Join(m.Directory("data"), record.ID+".zip")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("cloud-only left local archive")
	}
	if cloud.objects["backup.zip"] != "legacy" || cloud.objects[managedPrefix+"attachments/old.zip"] != "attachment" || cloud.objects[managedPrefix+"data/unknown.zip"] != "unowned" {
		t.Fatal("retention touched unrelated objects")
	}
	if len(cloud.deleted) != 1 || cloud.deleted[0] != oldKey {
		t.Fatalf("deleted=%v", cloud.deleted)
	}
}

func TestManagedExplicitSourceDoesNotFallBack(t *testing.T) {
	m, cloud := testManagedManager(t)
	record, err := m.Create(context.Background(), "data", "both", "", "manual")
	if err != nil {
		t.Fatal(err)
	}
	cloud.objects[record.CloudKey] = "cloud bytes"
	reader, _, err := m.OpenSource(context.Background(), record.ID, "cloud")
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(reader)
	_ = reader.Close()
	if err != nil || string(body) != "cloud bytes" {
		t.Fatalf("body=%q err=%v", body, err)
	}
	delete(cloud.objects, record.CloudKey)
	if reader, _, err := m.OpenSource(context.Background(), record.ID, "cloud"); err == nil {
		_ = reader.Close()
		t.Fatal("explicit cloud source fell back to local")
	}
	if _, _, err := m.OpenSource(context.Background(), "../../outside", "local"); err == nil {
		t.Fatal("path accepted as record ID")
	}
}

func TestManagedQueuedCancellationDoesNotClaimSlot(t *testing.T) {
	m, _ := testManagedManager(t)
	if _, err := m.SavePlan(managedPlan("data", "local")); err != nil {
		t.Fatal(err)
	}
	release := LockOperation()
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { result <- m.Scan(ctx) }()
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error=%v", err)
		}
	case <-time.After(time.Second):
		release()
		t.Fatal("canceled scan did not exit while operation lock was held")
	}
	release()
	plans, err := m.Plans()
	if err != nil {
		t.Fatal(err)
	}
	for _, plan := range plans {
		if plan.LastRun != nil {
			t.Fatal("queued cancellation consumed schedule slot")
		}
	}
	if err := m.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestManagedCancelRunningJobPersistsCanceledHistory(t *testing.T) {
	m, _ := testManagedManager(t)
	entered := make(chan struct{})
	m.archive = func(ctx context.Context, _ string, _ *gorm.DB, _ backup.Layout, _ backup.ArchiveKind, _ string) error {
		close(entered)
		<-ctx.Done()
		return ctx.Err()
	}
	done := make(chan error, 1)
	go func() {
		_, err := m.Create(context.Background(), "attachments", "local", "local", "manual")
		done <- err
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("archive did not start")
	}
	history, err := m.History("attachments")
	if err != nil || len(history) != 1 {
		t.Fatalf("history=%+v err=%v", history, err)
	}
	if !m.Cancel(history[0].ID) {
		t.Fatal("running record could not be canceled")
	}
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error=%v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled job did not exit")
	}
	record, err := m.FetchRecord(context.Background(), history[0].ID)
	if err != nil || record.Status != "canceled" || record.FinishedAt == nil {
		t.Fatalf("record=%+v err=%v", record, err)
	}
	if m.Cancel(record.ID) {
		t.Fatal("completed record reported cancellation")
	}
}

func TestManagedRejectsInvalidPlansBeforeDatabaseAccess(t *testing.T) {
	for _, mutate := range []func(*models.ManagedBackupPlan){
		func(p *models.ManagedBackupPlan) { p.Kind = "../../outside" },
		func(p *models.ManagedBackupPlan) { p.Time = "25:00" },
		func(p *models.ManagedBackupPlan) { p.Timezone = "unknown/zone" },
		func(p *models.ManagedBackupPlan) { p.Timezone = "Local" },
		func(p *models.ManagedBackupPlan) { p.KeepLocal = -1 },
		func(p *models.ManagedBackupPlan) { p.KeepLocal = 0 },
		func(p *models.ManagedBackupPlan) { p.KeepCloud = 0 },
		func(p *models.ManagedBackupPlan) { p.KeepCloud = 1001 },
		func(p *models.ManagedBackupPlan) { p.Target = "/tmp/archive.zip" },
		func(p *models.ManagedBackupPlan) { p.Scope = "all" },
	} {
		plan := managedPlan("data", "local")
		mutate(&plan)
		m := NewManager()
		m.db = func() *gorm.DB { t.Fatal("invalid input accessed database"); return nil }
		if _, err := m.SavePlan(plan); err == nil {
			t.Fatalf("accepted invalid plan: %+v", plan)
		}
	}
}

func TestManagedDefaultUsesExistingDatabaseGlobal(t *testing.T) {
	m, _ := testManagedManager(t)
	previous := database.DB
	database.DB = m.db()
	t.Cleanup(func() { database.DB = previous })
	other := NewManager()
	plans, err := other.Plans()
	if err != nil || len(plans) != 3 {
		t.Fatalf("plans=%v err=%v", plans, err)
	}
	if other.db() != database.DB {
		t.Fatal("manager created a separate database")
	}
}

func TestConfigureDoesNotStartUnownedManagerOrLegacyTimers(t *testing.T) {
	m, _ := testManagedManager(t)
	previous := DefaultManager
	mu.Lock()
	previousConfig := configured
	mu.Unlock()
	DefaultManager = m
	t.Cleanup(func() { m.Stop(); DefaultManager = previous; Configure(previousConfig) })
	Configure(models.SiteConfig{StorageEnabled: true, StorageAutoSyncEnabled: true, StorageSyncMode: "instant", StorageSyncConfirmed: true})
	Trigger()
	m.lifecycleMu.Lock()
	started := m.stop != nil || m.done != nil
	m.lifecycleMu.Unlock()
	mu.Lock()
	timers := scheduledStop != nil || debounceTimer != nil
	mu.Unlock()
	if started || timers {
		t.Fatal("configuration created workers outside main's lifecycle")
	}
}

func TestSyncNowDisabledAndSecondaryAreErrors(t *testing.T) {
	mu.Lock()
	previous := configured
	mu.Unlock()
	t.Cleanup(func() { mu.Lock(); configured = previous; mu.Unlock() })
	for _, cfg := range []models.SiteConfig{{StorageEnabled: false}, {StorageEnabled: true, StorageSyncRole: "secondary"}} {
		mu.Lock()
		configured = cfg
		mu.Unlock()
		if err := SyncNow(); err == nil {
			t.Fatalf("configuration reported false success: %+v", cfg)
		}
	}
}
