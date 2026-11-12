package backup

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInterruptedRestoreNeverSilentlyBootsOrReplays(t *testing.T) {
	layout := typedTestLayout(t.TempDir())
	if err := os.MkdirAll(filepath.Dir(layout.DatabasePath), 0700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(filepath.Dir(layout.DatabasePath), ".echo-noise-restore-in-progress")
	if err := os.WriteFile(marker, []byte("interrupted"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyPendingRestore(layout); err == nil || RestoreFailureRecovered(err) {
		t.Fatal("interrupted restore did not fail closed", err)
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != "interrupted" {
		t.Fatal("recovery evidence was modified")
	}
}
