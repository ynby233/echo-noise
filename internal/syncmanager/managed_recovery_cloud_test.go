package syncmanager

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/rcy1314/echo-noise/internal/models"
)

func TestManagedCloudRecoveryRequiresValidatedArchiveAndInactiveJob(t *testing.T) {
	for _, state := range []string{"running", "interrupted", "failed", "canceled", "active", "invalid"} {
		t.Run(state, func(t *testing.T) {
			m := localDiscoveryManager(t)
			cloud := &fakeManagedCloud{objects: make(map[string]string), modified: make(map[string]time.Time)}
			m.cloud = cloud
			id, path := localDiscoveryArchive(t, m, m.now().Add(-time.Hour))
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if state == "invalid" {
				body = []byte("invalid remote zip")
			}
			key := managedKey("server-config", id)
			cloud.objects[key], cloud.modified[key] = string(body), m.now()
			status := state
			if state == "active" || state == "invalid" {
				status = "running"
			}
			record := models.ManagedBackupRecord{ID: id, Kind: "server-config", Target: "cloud", Trigger: "manual", Scope: "default", CreatedAt: m.now().Add(-2 * time.Hour), Status: status, Error: "original outcome"}
			db, err := m.store()
			if err != nil {
				t.Fatal(err)
			}
			if err := db.Create(&record).Error; err != nil {
				t.Fatal(err)
			}
			if state == "active" {
				_, cancel := context.WithCancel(context.Background())
				defer cancel()
				m.activeMu.Lock()
				m.active[id] = cancel
				m.activeMu.Unlock()
			}
			err = m.Discover(context.Background(), "server-config")
			if (err != nil) != (state == "invalid") {
				t.Fatalf("discovery error=%v state=%s", err, state)
			}
			var got models.ManagedBackupRecord
			if err := db.First(&got, "id = ?", id).Error; err != nil {
				t.Fatal(err)
			}
			if got.Trigger != record.Trigger || !got.CreatedAt.Equal(record.CreatedAt) || got.Target != record.Target || got.Scope != record.Scope || got.Local {
				t.Fatalf("reconciliation changed provenance: %+v", got)
			}
			switch state {
			case "running", "interrupted":
				if got.Status != "success" || !got.Cloud || got.CloudKey != key || got.Size != int64(len(body)) || got.FinishedAt == nil || !got.FinishedAt.Equal(cloud.modified[key]) || got.Error != "" {
					t.Fatalf("validated remote archive was not recovered: %+v", got)
				}
				reader, _, err := m.OpenSource(context.Background(), id, "cloud")
				if err != nil {
					t.Fatal(err)
				}
				reader.Close()
			case "invalid":
				if got.Status != "interrupted" || got.Cloud || got.Error == "" || got.FinishedAt == nil {
					t.Fatalf("invalid remote archive promoted: %+v", got)
				}
			default:
				if got.Status != record.Status || got.Cloud || got.FinishedAt != nil || got.Error != record.Error || got.Size != record.Size {
					t.Fatalf("active/failed upload overwritten: got=%+v want=%+v", got, record)
				}
			}
		})
	}
}
