package backup

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
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

func TestServerConfigArchiveRestoresSupportedRuntimeSettings(t *testing.T) {
	root := t.TempDir()
	source := typedTestLayout(filepath.Join(root, "source"))
	if err := os.MkdirAll(source.ConfigDir, 0700); err != nil {
		t.Fatal(err)
	}
	content := "UPDATE_EXECUTOR_WAKE_URL=https://executor.invalid/wake\nUPDATE_EXECUTOR_WAKE_TOKEN_FILE=/run/secrets/wake-token\nMCP_ENABLED=true\nMCP_TRANSPORT=stdio\nMCP_HTTP_HOST=127.0.0.1\nMCP_HTTP_PORT=1315\nMCP_ALLOWED_HOSTS=localhost:1315,127.0.0.1:1315\nMCP_ALLOWED_ORIGINS=https://client.invalid\nDB_TIMEZONE=Asia/Shanghai\nDB_CHARSET=utf8mb4\nHTTP_PORT=1314\n"
	if err := os.WriteFile(filepath.Join(source.ConfigDir, "runtime.env"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(root, "config.zip")
	if err := CreateTypedArchive(context.Background(), archive, nil, source, KindServerConfig, ""); err != nil {
		t.Fatal(err)
	}
	target := typedTestLayout(filepath.Join(root, "target"))
	for key := range literalEnvironment([]byte(content)) {
		t.Setenv(key, "before-restore")
	}
	if _, err := StageTypedRestore(context.Background(), archive, target, "original"); err != nil {
		t.Fatal(err)
	}
	applied, err := ApplyPendingRestore(target)
	if err != nil {
		t.Fatal(err)
	}
	if err := applied.RestoreRuntimeEnvironment(target); err != nil {
		t.Fatal(err)
	}
	for key, expected := range literalEnvironment([]byte(content)) {
		if got := os.Getenv(key); got != expected {
			t.Errorf("%s = %q, want %q", key, got, expected)
		}
	}
	if err := applied.Commit(); err != nil {
		t.Fatal(err)
	}
	if HasPendingRestore(target) {
		t.Fatal("configuration restore remained pending")
	}
}

func TestRuntimeTemplateSettingsAreAcceptedByConfigBackup(t *testing.T) {
	assignment := regexp.MustCompile(`(?m)^\s*#?\s*([A-Z][A-Z0-9_]*)=`)
	for _, name := range []string{"../../config/runtime.env.example", "../../.env.prod.example"} {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, match := range assignment.FindAllStringSubmatch(string(data), -1) {
			if !allowedRuntimeKey(match[1]) {
				t.Errorf("documented runtime variable %s in %s is rejected", match[1], name)
			}
		}
	}
	for _, key := range []string{"PATH", "LD_PRELOAD", "BASH_ENV", "GODEBUG"} {
		if allowedRuntimeKey(key) {
			t.Errorf("unsafe process setting %s is accepted", key)
		}
	}
}
