package syncmanager

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
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
	statuses := make(map[string]string)
	for _, status := range []string{"success", "failed", "running", "interrupted", "canceled"} {
		id, path := localDiscoveryArchive(t, m, m.now().Add(-time.Hour))
		statuses[id] = status
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		record := models.ManagedBackupRecord{ID: id, Kind: "server-config", Status: status, Target: "cloud", Trigger: "manual", Size: info.Size(), CreatedAt: m.now()}
		if err := db.Create(&record).Error; err != nil {
			t.Fatal(err)
		}
	}
	var running int64
	if err := db.Model(&models.ManagedBackupRecord{}).Where("status IN ?", []string{"running", "interrupted"}).Count(&running).Error; err != nil || running != 2 {
		t.Fatalf("recovery fixtures: count=%d err=%v", running, err)
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
	if err != nil || len(history) != 6 {
		t.Fatalf("history=%+v err=%v", history, err)
	}
	for _, record := range history {
		wantLocal := record.ID != mismatch.ID && (statuses[record.ID] == "success" || statuses[record.ID] == "running" || statuses[record.ID] == "interrupted")
		original := statuses[record.ID]
		if (original == "running" || original == "interrupted") && (record.Status != "success" || record.FinishedAt == nil || record.Error != "") {
			t.Fatalf("recovered record missing completion metadata: %+v", record)
		}
		if record.Local != wantLocal || record.Trigger == "discovered" {
			t.Fatalf("existing record overwritten: %+v", record)
		}
		if original != "running" && original != "interrupted" && record.ID != mismatch.ID && record.Status != original {
			t.Fatalf("terminal status overwritten: %+v", record)
		}
	}
}

func TestManagedLocalDiscoveryRecoversRunningSelfFromDataSnapshot(t *testing.T) {
	t.Setenv("DB_TYPE", "sqlite")
	m := localDiscoveryManager(t)
	db, err := m.store()
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.Message{}, &models.SiteConfig{}); err != nil {
		t.Fatal(err)
	}
	created, err := m.Create(context.Background(), "data", "local", "", "manual")
	if err != nil {
		t.Fatal(err)
	}
	path, err := m.recordPath(created)
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	z, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	snapshotPath := filepath.Join(t.TempDir(), "snapshot.db")
	found := false
	for _, entry := range z.File {
		if entry.Name != "database.db" {
			continue
		}
		reader, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		snapshot, readErr := io.ReadAll(reader)
		closeErr := reader.Close()
		if readErr != nil || closeErr != nil {
			t.Fatalf("extract database: read=%v close=%v", readErr, closeErr)
		}
		if err := os.WriteFile(snapshotPath, snapshot, 0600); err != nil {
			t.Fatal(err)
		}
		found = true
	}
	if !found {
		t.Fatal("data archive lacks database.db")
	}
	// Close the ZIP before retention removes it, including on Windows.
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	snapshot, err := backup.OpenOffline(snapshotPath, false)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := snapshot.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	var self models.ManagedBackupRecord
	if err := snapshot.First(&self, "id = ?", created.ID).Error; err != nil {
		t.Fatal(err)
	}
	if self.Status != "running" || self.Local || self.Cloud || self.Size != 0 || self.FinishedAt != nil {
		t.Fatalf("archive must contain its actual unfinished self row: %+v", self)
	}
	// Restore the archived row, rather than synthesizing a running record.
	if err := db.Delete(&models.ManagedBackupRecord{}, "id = ?", created.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&self).Error; err != nil {
		t.Fatal(err)
	}
	restarted := NewManager()
	restarted.db, restarted.root, restarted.layout = m.db, m.root, m.layout
	restarted.cloud, restarted.now = m.cloud, m.now
	history, err := restarted.History("data")
	if err != nil || len(history) != 1 {
		t.Fatalf("restored history=%+v err=%v", history, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	recovered := history[0]
	if recovered.ID != self.ID || recovered.Status != "success" || !recovered.Local || recovered.Cloud || recovered.Size != info.Size() || recovered.Error != "" || recovered.FinishedAt == nil || !recovered.FinishedAt.Equal(info.ModTime()) || !recovered.CreatedAt.Equal(self.CreatedAt) || recovered.Trigger != self.Trigger || recovered.Target != self.Target || recovered.Scope != self.Scope {
		t.Fatalf("recovered self metadata=%+v archived self=%+v", recovered, self)
	}
	reader, name, err := restarted.Open(context.Background(), self.ID)
	if err != nil {
		t.Fatal(err)
	}
	opened, readErr := io.ReadAll(reader)
	closeErr := reader.Close()
	if readErr != nil || closeErr != nil || !bytes.Equal(opened, body) || name != "data-"+self.ID+".zip" {
		t.Fatalf("open recovered archive: name=%q read=%v close=%v", name, readErr, closeErr)
	}
	plan := managedPlan("data", "local")
	plan.KeepLocal = 1
	if _, err := restarted.SavePlan(plan); err != nil {
		t.Fatal(err)
	}
	restarted.now = func() time.Time { return self.CreatedAt.Add(time.Hour) }
	replacement, err := restarted.Create(context.Background(), "data", "local", "", "manual")
	if err != nil {
		t.Fatal(err)
	}
	if replacement.Status != "success" || !replacement.Local {
		t.Fatalf("replacement=%+v", replacement)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("restored archive not pruned: %v", err)
	}
	var pruned models.ManagedBackupRecord
	if err := db.First(&pruned, "id = ?", self.ID).Error; err != nil {
		t.Fatal(err)
	}
	if pruned.Local || pruned.Status != "success" {
		t.Fatalf("pruned audit record=%+v", pruned)
	}
}

func TestManagedLocalDiscoveryRunningWithoutCompleteArchiveIsInterrupted(t *testing.T) {
	for _, state := range []string{"missing", "invalid", "partial"} {
		t.Run(state, func(t *testing.T) {
			m := localDiscoveryManager(t)
			db, err := m.store()
			if err != nil {
				t.Fatal(err)
			}
			id, path := localDiscoveryArchive(t, m, m.now().Add(-time.Hour))
			switch state {
			case "missing":
				err = os.Remove(path)
			case "invalid":
				err = os.WriteFile(path, []byte("invalid zip"), 0600)
			case "partial":
				err = os.Rename(path, path+".partial")
			}
			if err != nil {
				t.Fatal(err)
			}
			record := models.ManagedBackupRecord{ID: id, Kind: "server-config", Status: "running", Target: "local", Trigger: "manual", Scope: "default", CreatedAt: m.now().Add(-2 * time.Hour), Local: true, Cloud: true}
			if err := db.Create(&record).Error; err != nil {
				t.Fatal(err)
			}
			history, err := m.History("server-config")
			if err != nil || len(history) != 1 {
				t.Fatalf("history=%+v err=%v", history, err)
			}
			var got models.ManagedBackupRecord
			if err := db.First(&got, "id = ?", id).Error; err != nil {
				t.Fatal(err)
			}
			if got.Status != "interrupted" || got.Local || got.Cloud || got.Error == "" || got.FinishedAt == nil || !got.FinishedAt.Equal(m.now()) || !got.CreatedAt.Equal(record.CreatedAt) || got.Trigger != record.Trigger || got.Target != record.Target || got.Scope != record.Scope {
				t.Fatalf("inactive %s record=%+v", state, got)
			}
			if state != "missing" {
				preserved := path
				if state == "partial" {
					preserved += ".partial"
				}
				if _, err := os.Stat(preserved); err != nil {
					t.Fatalf("incomplete archive removed: %v", err)
				}
			}
		})
	}
}

func TestManagedLocalDiscoveryLeavesActiveRunningRecordsUntouched(t *testing.T) {
	for _, state := range []string{"valid", "missing"} {
		t.Run(state, func(t *testing.T) {
			m := localDiscoveryManager(t)
			db, err := m.store()
			if err != nil {
				t.Fatal(err)
			}
			id, path := localDiscoveryArchive(t, m, m.now().Add(-time.Hour))
			if state == "missing" {
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			}
			record := models.ManagedBackupRecord{ID: id, Kind: "server-config", Status: "running", Target: "local", Trigger: "manual", Scope: "default", CreatedAt: m.now(), Error: "in progress"}
			if err := db.Create(&record).Error; err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			m.activeMu.Lock()
			m.active[id] = cancel
			m.activeMu.Unlock()
			history, err := m.History("server-config")
			if err != nil || len(history) != 1 {
				t.Fatalf("history=%+v err=%v", history, err)
			}
			var got models.ManagedBackupRecord
			if err := db.First(&got, "id = ?", id).Error; err != nil {
				t.Fatal(err)
			}
			if got.ID != record.ID || got.Kind != record.Kind || got.Status != record.Status || got.Target != record.Target || got.Trigger != record.Trigger || got.Scope != record.Scope || !got.CreatedAt.Equal(record.CreatedAt) || got.Error != record.Error || got.FinishedAt != nil || got.Size != record.Size || got.Local != record.Local || got.Cloud != record.Cloud || got.CloudKey != record.CloudKey || ctx.Err() != nil || !m.activeJob(id) {
				t.Fatalf("active %s record changed: got=%+v want=%+v context=%v", state, got, record, ctx.Err())
			}
		})
	}
}
