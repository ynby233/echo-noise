package vocechat

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPlainPasswordStoreUpsertDeleteAndPermissions(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "plain-passwords.db")
	store := NewPlainPasswordStore(storePath)

	if err := store.UpsertUserPassword(7, "Tom", "initial-password", "7@vc.com", "vc-7"); err != nil {
		t.Fatalf("upsert user password: %v", err)
	}
	if err := store.UpsertApplicationPassword("app-1", "alice", "pending-password", "app-1@vc.com", "vc-app-1"); err != nil {
		t.Fatalf("upsert application password: %v", err)
	}
	if err := store.UpsertUserPassword(7, "Tom", "updated-password", "Tom@vc.com", "vc-7"); err != nil {
		t.Fatalf("update user password: %v", err)
	}

	info, err := os.Stat(storePath)
	if err != nil {
		t.Fatalf("stat password store: %v", err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Fatalf("password store permissions = %o, want 600", got)
	}

	userRecord, found, err := store.GetUserPassword(7)
	if err != nil {
		t.Fatalf("get user password: %v", err)
	}
	if !found {
		t.Fatal("user password record not found")
	}
	if userRecord.Password != "updated-password" || userRecord.VoceChatEmail != "Tom@vc.com" {
		t.Fatalf("user record = %#v", userRecord)
	}

	applicationRecord, found, err := store.GetApplicationPassword("app-1")
	if err != nil {
		t.Fatalf("get application password: %v", err)
	}
	if !found || applicationRecord.Kind != PlainPasswordKindApplication || applicationRecord.Password != "pending-password" {
		t.Fatalf("application record found=%v record=%#v", found, applicationRecord)
	}

	if err := store.DeleteUserPassword(7); err != nil {
		t.Fatalf("delete user password: %v", err)
	}
	if _, found, err := store.GetUserPassword(7); err != nil || found {
		t.Fatalf("user record after delete found=%v err=%v", found, err)
	}
}

func TestDefaultPlainPasswordStorePathUsesAppDataDatabase(t *testing.T) {
	t.Setenv(plainPasswordStoreEnv, "")

	if got := DefaultPlainPasswordStorePath(); got != "/app/data/plain-passwords.db" {
		t.Fatalf("default store path = %q, want /app/data/plain-passwords.db", got)
	}
}
