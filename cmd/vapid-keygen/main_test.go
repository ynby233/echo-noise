package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestWritePrivateKeyFileCreatesSecretWithoutOverwriting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "vapid-private-key")
	writtenPath, err := writePrivateKeyFile(path, "private-value")
	if err != nil {
		t.Fatalf("write private key: %v", err)
	}
	if writtenPath != path {
		t.Fatalf("written path = %q, want %q", writtenPath, path)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read private key: %v", err)
	}
	if got := strings.TrimSpace(string(contents)); got != "private-value" {
		t.Fatalf("private key contents = %q", got)
	}
	if info, err := os.Stat(path); err != nil {
		t.Fatalf("stat private key: %v", err)
	} else if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("private key permissions = %o, want no group/other access", info.Mode().Perm())
	}
	if _, err := writePrivateKeyFile(path, "replacement"); err == nil {
		t.Fatal("existing private key file was overwritten")
	}
	contents, err = os.ReadFile(path)
	if err != nil {
		t.Fatalf("read preserved private key: %v", err)
	}
	if got := strings.TrimSpace(string(contents)); got != "private-value" {
		t.Fatalf("private key was replaced with %q", got)
	}
}
