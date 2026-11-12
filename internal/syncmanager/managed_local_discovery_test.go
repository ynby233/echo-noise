package syncmanager

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	backup "github.com/rcy1314/echo-noise/internal/backup"
	"github.com/rcy1314/echo-noise/internal/models"
)

type disabledLocalDiscoveryCloud struct{ *fakeManagedCloud }

func (disabledLocalDiscoveryCloud) Enabled() bool { return false }

func localDiscoveryManager(t *testing.T) *Manager {
	t.Helper()
	m, cloud := testManagedManager(t)
	m.cloud = disabledLocalDiscoveryCloud{cloud}
	root := t.TempDir()
	layout := backup.Layout{DatabasePath: filepath.Join(root, "noise.db"), ConfigDir: filepath.Join(root, "config")}
	if err := os.MkdirAll(layout.ConfigDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(layout.ConfigDir, "config.yaml"), []byte("server:\n  port: '9123'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	m.layout = func() backup.Layout { return layout }
	m.archive = backup.CreateTypedArchive
	if _, err := m.SavePlan(managedPlan("server-config", "local")); err != nil {
		t.Fatal(err)
	}
	return m
}

func localDiscoveryArchive(t *testing.T, m *Manager, modified time.Time) (string, string) {
	t.Helper()
	if err := m.safeDirectory("server-config", true); err != nil {
		t.Fatal(err)
	}
	id := uuid.NewString()
	path := filepath.Join(m.Directory("server-config"), id+".zip")
	if err := backup.CreateTypedArchive(context.Background(), path, nil, m.layout(), backup.KindServerConfig, ""); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, modified, modified); err != nil {
		t.Fatal(err)
	}
	return id, path
}

func TestManagedLocalDiscoveryRestoresLostHistoryWithoutCloud(t *testing.T) {
	m := localDiscoveryManager(t)
	created, err := m.Create(context.Background(), "server-config", "local", "", "manual")
	if err != nil {
		t.Fatal(err)
	}
	db, err := m.store()
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Delete(&models.ManagedBackupRecord{}, "id = ?", created.ID).Error; err != nil {
		t.Fatal(err)
	}
	path, err := m.recordPath(created)
	if err != nil {
		t.Fatal(err)
	}
	modified := m.now().Add(-time.Hour)
	if err := os.Chtimes(path, modified, modified); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	history, err := m.History("server-config")
	if err != nil || len(history) != 1 {
		t.Fatalf("history=%+v err=%v", history, err)
	}
	record := history[0]
	if record.ID != created.ID || record.Trigger != "discovered" || record.Status != "success" || !record.Local || record.Cloud || record.Target != "local" || record.Size != info.Size() || !record.CreatedAt.Equal(info.ModTime()) || record.FinishedAt == nil || !record.FinishedAt.Equal(info.ModTime()) {
		t.Fatalf("discovered record=%+v file=%+v", record, info)
	}
	if _, err := m.History(""); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := db.Model(&models.ManagedBackupRecord{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("duplicate discovery: count=%d err=%v", count, err)
	}
}

func TestManagedLocalDiscoveryCreateRetainsCombinedManualAndScheduled(t *testing.T) {
	m := localDiscoveryManager(t)
	_, oldest := localDiscoveryArchive(t, m, m.now().Add(-3*time.Hour))
	_, newer := localDiscoveryArchive(t, m, m.now().Add(-2*time.Hour))
	// Create must discover lost records even if History has never been called.
	manual, err := m.Create(context.Background(), "server-config", "local", "", "manual")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(oldest); !os.IsNotExist(err) {
		t.Fatalf("oldest archive retained: %v", err)
	}
	if _, err := os.Stat(newer); err != nil {
		t.Fatal(err)
	}
	// Exercise the actual scheduler path, which already holds operationMu.
	if err := m.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(newer); !os.IsNotExist(err) {
		t.Fatalf("scheduled retention did not prune older discovered archive: %v", err)
	}
	history, err := m.History("server-config")
	if err != nil {
		t.Fatal(err)
	}
	available := 0
	manualPresent, scheduledPresent := false, false
	for _, record := range history {
		if record.Local {
			available++
			manualPresent = manualPresent || record.ID == manual.ID
			scheduledPresent = scheduledPresent || record.Trigger == "scheduled"
		}
	}
	if available != 2 || !manualPresent || !scheduledPresent {
		t.Fatalf("combined retention: %+v", history)
	}
}

func TestManagedLocalDiscoveryIgnoresUnsafeAndInvalidArchives(t *testing.T) {
	m := localDiscoveryManager(t)
	_, validPath := localDiscoveryArchive(t, m, m.now().Add(-time.Hour))
	body, err := os.ReadFile(validPath)
	if err != nil {
		t.Fatal(err)
	}
	dir := m.Directory("server-config")
	invalid := filepath.Join(dir, uuid.NewString()+".zip")
	partial := filepath.Join(dir, uuid.NewString()+".zip.partial")
	noncanonical := filepath.Join(dir, strings.ToUpper(uuid.NewString())+".zip")
	unknown := filepath.Join(dir, "backup.zip")
	for _, path := range []string{partial, noncanonical, unknown} {
		if err := os.WriteFile(path, body, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(invalid, []byte("invalid zip"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := m.safeDirectory("data", true); err != nil {
		t.Fatal(err)
	}
	wrongKind := filepath.Join(m.Directory("data"), uuid.NewString()+".zip")
	if err := os.WriteFile(wrongKind, body, 0600); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(m.root, uuid.NewString()+".zip")
	if err := os.WriteFile(outside, body, 0600); err != nil {
		t.Fatal(err)
	}
	preserved := []string{invalid, partial, noncanonical, unknown, wrongKind, outside}
	link := filepath.Join(dir, uuid.NewString()+".zip")
	if err := os.Symlink(validPath, link); err == nil {
		preserved = append(preserved, link)
	}
	history, err := m.History("")
	if err != nil || len(history) != 1 {
		t.Fatalf("invalid archives discovered: %+v err=%v", history, err)
	}
	plan := managedPlan("server-config", "local")
	plan.KeepLocal = 1
	if _, err := m.SavePlan(plan); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Create(context.Background(), "server-config", "local", "", "manual"); err != nil {
		t.Fatal(err)
	}
	for _, path := range preserved {
		if _, err := os.Lstat(path); err != nil {
			t.Fatalf("ignored archive removed: %s: %v", path, err)
		}
	}
}

func TestManagedLocalDiscoveryOnlyUpdatesMatchingSuccessfulRecords(t *testing.T) {
	m := localDiscoveryManager(t)
	db, err := m.store()
	if err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{"success", "failed", "running", "canceled"} {
		id, path := localDiscoveryArchive(t, m, m.now().Add(-time.Hour))
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		record := models.ManagedBackupRecord{ID: id, Kind: "server-config", Status: status, Target: "cloud", Trigger: "manual", Size: info.Size(), CreatedAt: m.now()}
		if err := db.Create(&record).Error; err != nil {
			t.Fatal(err)
		}
	}
	id, path := localDiscoveryArchive(t, m, m.now())
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	mismatch := models.ManagedBackupRecord{ID: id, Kind: "server-config", Status: "success", Size: info.Size() + 1, CreatedAt: m.now()}
	if err := db.Create(&mismatch).Error; err != nil {
		t.Fatal(err)
	}
	history, err := m.History("server-config")
	if err != nil || len(history) != 5 {
		t.Fatalf("history=%+v err=%v", history, err)
	}
	for _, record := range history {
		wantLocal := record.Status == "success" && record.ID != mismatch.ID
		if record.Local != wantLocal || record.Trigger == "discovered" {
			t.Fatalf("existing record overwritten: %+v", record)
		}
	}
}
