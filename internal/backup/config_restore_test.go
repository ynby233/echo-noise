package backup

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigRestoreRejectsRuntimeLoaderAndPathRedirection(t *testing.T) {
	root := t.TempDir()
	layout := typedTestLayout(root)
	stage := filepath.Join(root, "stage")
	if err := os.MkdirAll(filepath.Join(stage, "server-config"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, content := range []string{"PATH=/tmp/attacker\n", "LD_PRELOAD=/tmp/library.so\n", "BASH_ENV=/tmp/commands\n"} {
		file := filepath.Join(stage, "server-config/runtime.env")
		if err := os.WriteFile(file, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		if err := validateConfigFile(file, "runtime.env"); err == nil {
			t.Fatalf("accepted %s", content)
		}
	}
	file := filepath.Join(stage, "server-config/runtime.env")
	if err := os.WriteFile(file, []byte("DB_PATH=/unrelated/database.db\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := validateRestoreConfiguration(stage, layout); err == nil {
		t.Fatal("accepted DB path change")
	}
	if err := os.WriteFile(file, []byte("TZ=UTC\nSESSION_SECRET=restored-test-value\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := validateConfigFile(file, "runtime.env"); err != nil {
		t.Fatal(err)
	}
	if err := validateRestoreConfiguration(stage, layout); err != nil {
		t.Fatal(err)
	}
}
