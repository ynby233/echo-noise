package syncmanager

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rcy1314/echo-noise/internal/models"
)

func TestManagedDiscoverCleanDatabaseAllKindsAndOpen(t *testing.T) {
	m, cloud := testManagedManager(t)
	ids := map[string]string{"data": "938ba162-7f1d-45ef-8d00-df7c45822b79", "attachments": "775b9fc4-dc10-4856-94dd-27d241de6b1b", "server-config": "b319c78e-9e2f-4983-aa87-f0cb395d0a4f"}
	for kind, id := range ids {
		key := managedKey(kind, id)
		cloud.objects[key], cloud.modified[key] = kind+" bytes", m.now()
	}
	cloud.objects["backup.zip"] = "legacy"
	cloud.objects[managedPrefix+"data/unknown.zip"] = "unowned"
	if err := m.Discover(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	for kind, id := range ids {
		record, err := m.FetchRecord(context.Background(), id)
		if err != nil || record.Kind != kind || !record.Cloud || record.Local || record.Trigger != "discovered" || record.Scope != "unknown" {
			t.Fatalf("record=%+v error=%v", record, err)
		}
		reader, name, err := m.OpenSource(context.Background(), id, "cloud")
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(reader)
		_ = reader.Close()
		if err != nil || string(body) != kind+" bytes" || name != kind+"-"+id+".zip" {
			t.Fatalf("name=%q body=%q error=%v", name, body, err)
		}
		if _, err := os.Stat(m.Directory(kind)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("discovery created local archive directory: %v", err)
		}
	}
	if err := m.Discover(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := m.db().Model(&models.ManagedBackupRecord{}).Count(&count).Error; err != nil || count != 3 {
		t.Fatalf("count=%d error=%v", count, err)
	}
	delete(cloud.objects, managedKey("data", ids["data"]))
	if err := m.Discover(context.Background(), "data"); err != nil {
		t.Fatal(err)
	}
	record, err := m.FetchRecord(context.Background(), ids["data"])
	if err != nil || record.Cloud || record.Status != "success" {
		t.Fatalf("audit was not preserved: %+v error=%v", record, err)
	}
}

func TestManagedHistoryCloudFailurePreservesLocalAndReportsWarning(t *testing.T) {
	m, cloud := testManagedManager(t)
	record, err := m.Create(context.Background(), "data", "both", "", "manual")
	if err != nil {
		t.Fatal(err)
	}
	cloud.listErr = errors.New("cloud permission denied")
	history, err := m.HistoryContext(context.Background(), "data")
	if err != nil || len(history) != 1 || !history[0].Local || history[0].Cloud {
		t.Fatalf("history=%+v error=%v", history, err)
	}
	if len(m.DiscoveryWarnings("data")) != 1 || len(m.DiscoveryWarnings("attachments")) != 0 {
		t.Fatal("discovery warning was not isolated by kind")
	}
	persisted, err := m.FetchRecord(context.Background(), record.ID)
	if err != nil || !persisted.Cloud {
		t.Fatal("failed discovery altered durable availability", err)
	}
	cloud.listErr = nil
	history, err = m.HistoryContext(context.Background(), "data")
	if err != nil || !history[0].Cloud || len(m.DiscoveryWarnings("data")) != 0 {
		t.Fatal("successful discovery did not clear warning", err)
	}
}

func TestManagedFailedListingStopsBothRetentionDestinations(t *testing.T) {
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
	cloud.listErr = errors.New("page two unavailable")
	second, err := m.Create(context.Background(), "data", "both", "", "manual")
	if err == nil || second.Status != "success" || second.Error == "" {
		t.Fatalf("second=%+v error=%v", second, err)
	}
	if len(cloud.deleted) != 0 {
		t.Fatal("failed listing deleted cloud copies")
	}
	for _, record := range []models.ManagedBackupRecord{first, second} {
		if _, err := os.Stat(filepath.Join(m.Directory("data"), record.ID+".zip")); err != nil {
			t.Fatal("cloud listing failure pruned local copy", err)
		}
	}
}

func TestManagedRetentionOnlyPrunesReplacedDestination(t *testing.T) {
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
	if _, err := m.Create(context.Background(), "data", "cloud", "", "manual"); err != nil {
		t.Fatal(err)
	}
	persisted, err := m.FetchRecord(context.Background(), first.ID)
	if err != nil || !persisted.Local {
		t.Fatal("cloud-only job pruned local destination", err)
	}
	cloud.listed = nil
	if _, err := m.Create(context.Background(), "data", "local", "", "manual"); err != nil {
		t.Fatal(err)
	}
	if len(cloud.listed) != 0 {
		t.Fatal("local-only backup contacted cloud retention")
	}
}

type managedListingOverride struct {
	*fakeManagedCloud
	objects []managedObject
}

func (f *managedListingOverride) List(context.Context, string) ([]managedObject, error) {
	return f.objects, nil
}

func TestManagedRetentionRequiresVisibleReplacementAndPreservesFailedCopies(t *testing.T) {
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
	m.cloud = &managedListingOverride{fakeManagedCloud: cloud, objects: []managedObject{{Key: first.CloudKey, Modified: m.now(), Size: first.Size}}}
	second, err := m.Create(context.Background(), "data", "both", "", "manual")
	if err == nil || second.Status != "success" {
		t.Fatalf("second=%+v error=%v", second, err)
	}
	if len(cloud.deleted) != 0 {
		t.Fatal("listing without replacement deleted remote copy")
	}
	if _, err := os.Stat(filepath.Join(m.Directory("data"), first.ID+".zip")); err != nil {
		t.Fatal("missing remote replacement pruned local copy", err)
	}
	// Failed copies are audit evidence; a later success does not count or remove them.
	m.cloud = cloud
	cloud.uploadErr = errors.New("upload interrupted")
	failed, err := m.Create(context.Background(), "data", "both", "", "manual")
	if err == nil || !failed.Local {
		t.Fatalf("failed=%+v error=%v", failed, err)
	}
	cloud.uploadErr = nil
	if _, err := m.Create(context.Background(), "data", "both", "", "manual"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(m.Directory("data"), failed.ID+".zip")); err != nil {
		t.Fatal("failed backup evidence removed", err)
	}
}

func TestManagedDirectoryMappingAndStableLocalFilename(t *testing.T) {
	m, _ := testManagedManager(t)
	for kind, folder := range map[string]string{"data": "database", "attachments": "attachments", "server-config": "config"} {
		want := filepath.Join(m.root, folder)
		if !filepath.IsAbs(m.Directory(kind)) || m.Directory(kind) != want {
			t.Fatalf("directory(%s)=%q want=%q", kind, m.Directory(kind), want)
		}
	}
	if m.Directory("../outside") != "" {
		t.Fatal("invalid kind exposed directory")
	}
	record, err := m.Create(context.Background(), "data", "local", "", "manual")
	if err != nil {
		t.Fatal(err)
	}
	path, err := m.recordPath(record)
	if err != nil || filepath.Base(path) != record.ID+".zip" {
		t.Fatalf("path=%q error=%v", path, err)
	}
	m.now = func() time.Time { return time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC) }
	if again, err := m.recordPath(record); err != nil || again != path {
		t.Fatalf("path changed: %q error=%v", again, err)
	}
}
