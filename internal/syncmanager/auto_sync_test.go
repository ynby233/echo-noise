package syncmanager

import (
	"archive/zip"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/storage"
)

func TestUploadArchiveAcceptsOnly2xx(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "backup.zip")
	if err := os.WriteFile(archive, []byte("synthetic backup"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, status := range []int{http.StatusForbidden, http.StatusInternalServerError, http.StatusCreated} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPut {
					t.Errorf("method = %s", r.Method)
				}
				w.WriteHeader(status)
			}))
			defer server.Close()
			err := uploadArchive(server.URL, archive, server.Client())
			if status >= http.StatusOK && status < http.StatusMultipleChoices {
				if err != nil {
					t.Fatalf("upload error = %v, want success", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), "status="+strconv.Itoa(status)) {
				t.Fatalf("upload error = %v, want HTTP status failure", err)
			}
		})
	}
}

func TestCompleteUploadDoesNotAdvanceSyncStateAfterFailure(t *testing.T) {
	for _, test := range []struct {
		name      string
		uploadErr error
		headErr   error
		wantHead  bool
	}{
		{name: "upload", uploadErr: errors.New("upload failed")},
		{name: "head", headErr: errors.New("head failed"), wantHead: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			headCalled := false
			persistCalled := false
			err := completeUpload(
				func() error { return test.uploadErr },
				func() (*storage.ObjectMeta, error) {
					headCalled = true
					return &storage.ObjectMeta{ETag: "new"}, test.headErr
				},
				func(*storage.ObjectMeta, *time.Time) error {
					persistCalled = true
					return nil
				},
			)
			if err == nil {
				t.Fatal("completeUpload error = nil")
			}
			if headCalled != test.wantHead {
				t.Fatalf("head called = %v, want %v", headCalled, test.wantHead)
			}
			if persistCalled {
				t.Fatal("failed upload lifecycle advanced LastSyncTime")
			}
		})
	}
}

func TestCompleteUploadPersistsAfterSuccessfulUploadAndHead(t *testing.T) {
	remoteModified := time.Now().Add(-time.Minute)
	meta := &storage.ObjectMeta{ETag: "new", LastModified: &remoteModified}
	var persistedMeta *storage.ObjectMeta
	var persistedTime *time.Time
	err := completeUpload(
		func() error { return nil },
		func() (*storage.ObjectMeta, error) { return meta, nil },
		func(gotMeta *storage.ObjectMeta, gotTime *time.Time) error {
			persistedMeta = gotMeta
			persistedTime = gotTime
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if persistedMeta != meta || persistedTime == nil {
		t.Fatalf("persisted meta=%#v time=%v", persistedMeta, persistedTime)
	}
}

func TestConfigureDoesNotReenableExplicitlyDisabledAutoSync(t *testing.T) {
	Configure(models.SiteConfig{
		StorageEnabled:         true,
		StorageAutoSyncEnabled: false,
		StorageProvider:        "s3",
		StorageEndpoint:        "https://storage.example.test",
		StorageBucket:          "backup",
		StorageAccessKey:       "key",
		StorageSecretKey:       "secret",
	})
	mu.Lock()
	got := configured.StorageAutoSyncEnabled
	mu.Unlock()
	if got {
		t.Fatal("explicitly disabled automatic sync was re-enabled")
	}
}

func TestLockOperationSerializesBackupAndSyncEntrypoints(t *testing.T) {
	releaseFirst := LockOperation()
	acquiredSecond := make(chan func(), 1)
	go func() {
		acquiredSecond <- LockOperation()
	}()

	select {
	case releaseSecond := <-acquiredSecond:
		releaseSecond()
		releaseFirst()
		t.Fatal("second operation entered before the first operation released the lock")
	case <-time.After(50 * time.Millisecond):
	}
	releaseFirst()
	select {
	case releaseSecond := <-acquiredSecond:
		releaseSecond()
	case <-time.After(time.Second):
		t.Fatal("second operation did not resume after the first operation released the lock")
	}
}

func TestLegacyArchiveKeepsMediaWithoutSQLiteSnapshot(t *testing.T) {
	root := t.TempDir()
	images := filepath.Join(root, "images")
	videos := filepath.Join(root, "video")
	for path, content := range map[string]string{
		filepath.Join(images, "cover.png"): "image",
		filepath.Join(videos, "clip.mp4"):  "video",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	archivePath := filepath.Join(root, "legacy.zip")
	if err := createLegacyArchive(archivePath, images, videos); err != nil {
		t.Fatal(err)
	}
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	entries := map[string]string{}
	for _, file := range reader.File {
		handle, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		content, err := io.ReadAll(handle)
		_ = handle.Close()
		if err != nil {
			t.Fatal(err)
		}
		entries[file.Name] = string(content)
	}
	if entries["images/cover.png"] != "image" || entries["video/clip.mp4"] != "video" {
		t.Fatalf("legacy media entries = %#v", entries)
	}
}
