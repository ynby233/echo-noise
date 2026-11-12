package syncmanager

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	backup "github.com/rcy1314/echo-noise/internal/backup"
	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
)

func TestManagedS3HTTPDiscoverAndOpenOnCleanDatabase(t *testing.T) {
	const id = "938ba162-7f1d-45ef-8d00-df7c45822b79"
	const secondID = "775b9fc4-dc10-4856-94dd-27d241de6b1b"
	const prefix = "fixedfolder/noise-managed-backups/data/"
	var pages atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("list-type") == "2" {
			pages.Add(1)
			if r.URL.Query().Get("prefix") != prefix {
				t.Errorf("prefix=%q", r.URL.Query().Get("prefix"))
			}
			w.Header().Set("Content-Type", "application/xml")
			if r.URL.Query().Get("continuation-token") == "" {
				fmt.Fprintf(w, `<ListBucketResult><IsTruncated>true</IsTruncated><NextContinuationToken>second</NextContinuationToken><Contents><Key>%s%s.zip</Key><Size>7</Size><LastModified>2026-11-11T12:00:00Z</LastModified></Contents><Contents><Key>fixedfolder/backup.zip</Key><Size>6</Size></Contents></ListBucketResult>`, prefix, id)
			} else {
				if r.URL.Query().Get("continuation-token") != "second" {
					t.Errorf("token=%q", r.URL.Query().Get("continuation-token"))
				}
				fmt.Fprintf(w, `<ListBucketResult><IsTruncated>false</IsTruncated><Contents><Key>%s%s.zip</Key><Size>7</Size><LastModified>2026-11-10T12:00:00Z</LastModified></Contents></ListBucketResult>`, prefix, secondID)
			}
			return
		}
		if r.Method != http.MethodGet || r.URL.Path != "/backups/"+prefix+id+".zip" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_, _ = io.WriteString(w, "archive")
	}))
	defer server.Close()
	mu.Lock()
	previous := configured
	configured = models.SiteConfig{StorageEnabled: true, StorageProvider: "s3", StorageEndpoint: server.URL, StorageBucket: "backups", StorageAccessKey: "dummy", StorageSecretKey: "dummy-secret", StorageRegion: "us-east-1", StorageUsePathStyle: true, StoragePublicBaseURL: "https://cdn.invalid/fixedfolder"}
	mu.Unlock()
	t.Cleanup(func() { mu.Lock(); configured = previous; mu.Unlock() })
	m, _ := testManagedManager(t)
	m.cloud = productionManagedCloud{}
	history, err := m.HistoryContext(context.Background(), "data")
	if err != nil || len(history) != 2 || !history[0].Cloud || history[0].Local || history[0].ID != id {
		t.Fatalf("history=%+v error=%v warnings=%v", history, err, m.DiscoveryWarnings("data"))
	}
	if pages.Load() != 2 {
		t.Fatalf("discovery stopped before all pages: %d", pages.Load())
	}
	reader, name, err := m.OpenSource(context.Background(), id, "cloud")
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(reader)
	_ = reader.Close()
	if err != nil || string(body) != "archive" || name != "data-"+id+".zip" {
		t.Fatalf("name=%q body=%q error=%v", name, body, err)
	}
	if _, err := os.Stat(m.Directory("data")); !os.IsNotExist(err) {
		t.Fatalf("discovery materialized local archives: %v", err)
	}
}

type managedExternalDialector struct{ gorm.Dialector }

func (managedExternalDialector) Name() string { return "postgres" }

func TestManagedDataRejectsExternalDatabaseBeforeArchiveCreation(t *testing.T) {
	for _, mode := range []string{"environment", "dialector"} {
		t.Run(mode, func(t *testing.T) {
			m, _ := testManagedManager(t)
			t.Setenv("DB_TYPE", "sqlite")
			if mode == "environment" {
				t.Setenv("DB_TYPE", "postgres")
			} else {
				db, err := m.store()
				if err != nil {
					t.Fatal(err)
				}
				db.Dialector = managedExternalDialector{Dialector: db.Dialector}
			}
			m.archive = func(context.Context, string, *gorm.DB, backup.Layout, backup.ArchiveKind, string) error {
				t.Fatal("unsupported database reached archive creation")
				return nil
			}
			record, err := m.Create(context.Background(), "data", "local", "", "manual")
			if err == nil || !strings.Contains(err.Error(), "仅支持 SQLite") || record.Status != "failed" || record.Local || record.Cloud {
				t.Fatalf("record=%+v error=%v", record, err)
			}
		})
	}
}
