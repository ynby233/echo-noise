package updates

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestExecutorWakeUsesOnlyFixedAuthenticatedRequest(t *testing.T) {
	file := filepath.Join(t.TempDir(), "wake-token")
	if err := os.WriteFile(file, []byte("dedicated-token\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("UPDATE_EXECUTOR_WAKE_TOKEN_FILE", file)
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "POST" || r.URL.Path != "/fixed-task" || r.Header.Get("Authorization") != "Bearer dedicated-token" || r.ContentLength > 0 {
			t.Errorf("unexpected wake request")
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()
	t.Setenv("UPDATE_EXECUTOR_WAKE_URL", server.URL+"/fixed-task")
	if err := WakeExecutor(); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("calls=%d", calls)
	}
	t.Setenv("UPDATE_EXECUTOR_WAKE_URL", "")
	if err := WakeExecutor(); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatal("unconfigured wake sent a request")
	}
}

func TestExecutorWakeDoesNotFollowRedirectOrExposeRemoteError(t *testing.T) {
	file := filepath.Join(t.TempDir(), "wake-token")
	if err := os.WriteFile(file, []byte("dedicated-token"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("UPDATE_EXECUTOR_WAKE_TOKEN_FILE", file)
	var followed bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/other" {
			followed = true
			return
		}
		http.Redirect(w, r, "/other?private=secret", http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	t.Setenv("UPDATE_EXECUTOR_WAKE_URL", server.URL+"/fixed-task")
	if err := WakeExecutor(); err == nil || err.Error() != "executor_wake_rejected" {
		t.Fatalf("unexpected error: %v", err)
	}
	if followed {
		t.Fatal("forwarded credential to redirected endpoint")
	}
}
