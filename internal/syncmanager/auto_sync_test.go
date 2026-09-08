package syncmanager

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/rcy1314/echo-noise/internal/models"
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
