package backup

import (
	"archive/zip"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func typedTestLayout(root string) Layout {
	return Layout{DatabasePath: filepath.Join(root, "data", "noise.db"), PlainPasswordStorePath: filepath.Join(root, "secrets", "passwords.db"), ConfigDir: filepath.Join(root, "config"), Roots: []Root{{ArchiveName: "attachment-blobs", Path: filepath.Join(root, "blobs")}}}
}

func TestTypedDataEmptyPasswordStoreAndConfigQueuedTogether(t *testing.T) {
	root := t.TempDir()
	source := typedTestLayout(filepath.Join(root, "source"))
	if err := os.MkdirAll(filepath.Dir(source.DatabasePath), 0700); err != nil {
		t.Fatal(err)
	}
	db := openTestDatabase(t, source.DatabasePath, "new data")
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	dataArchive := filepath.Join(root, "data.zip")
	if err := CreateTypedArchive(context.Background(), dataArchive, db, source, KindData, ""); err != nil {
		t.Fatal(err)
	}
	inspection, err := InspectTypedArchive(dataArchive)
	if err != nil || len(inspection.Warnings) < 2 {
		t.Fatalf("empty password warning: %+v %v", inspection, err)
	}
	if err := os.MkdirAll(source.ConfigDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source.ConfigDir, "config.yaml"), []byte("server:\n  port: '9123'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	configArchive := filepath.Join(root, "config.zip")
	if err := CreateTypedArchive(context.Background(), configArchive, nil, source, KindServerConfig, ""); err != nil {
		t.Fatal(err)
	}
	target := typedTestLayout(filepath.Join(root, "target"))
	t.Setenv("DB_PATH", target.DatabasePath)
	if _, err := StageTypedRestore(context.Background(), dataArchive, target, "original"); err != nil {
		t.Fatal(err)
	}
	if _, err := StageTypedRestore(context.Background(), configArchive, target, "original"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(target.DatabasePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("staging touched live database")
	}
	applied, err := ApplyPendingRestore(target)
	if err != nil {
		t.Fatal(err)
	}
	if got := databaseValue(t, target.DatabasePath); got != "new data" {
		t.Fatal(got)
	}
	if err := validatePasswordSQLite(target.passwordPath()); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(filepath.Join(target.ConfigDir, "config.yaml")); err != nil || !strings.Contains(string(got), "9123") {
		t.Fatalf("config %q %v", got, err)
	}
	if err := applied.Commit(); err != nil {
		t.Fatal(err)
	}
	if HasPendingRestore(target) {
		t.Fatal("committed packages still pending")
	}
}

func TestGroupedAttachmentFailureRollsBackDataAndPasswords(t *testing.T) {
	root := t.TempDir()
	source := typedTestLayout(filepath.Join(root, "source"))
	if err := os.MkdirAll(filepath.Dir(source.DatabasePath), 0700); err != nil {
		t.Fatal(err)
	}
	db := openTestDatabase(t, source.DatabasePath, "replacement")
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	dataArchive := filepath.Join(root, "data.zip")
	if err := CreateTypedArchive(context.Background(), dataArchive, db, source, KindData, ""); err != nil {
		t.Fatal(err)
	}
	stage := t.TempDir()
	if err := os.WriteFile(filepath.Join(stage, "attachment-inventory.json"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	attachmentArchive := filepath.Join(root, "attachments.zip")
	if err := publishTypedArchive(context.Background(), attachmentArchive, stage, &Manifest{Version: 1, Kind: KindAttachments, Scope: "local"}); err != nil {
		t.Fatal(err)
	}
	target := typedTestLayout(filepath.Join(root, "target"))
	if err := os.MkdirAll(filepath.Dir(target.DatabasePath), 0700); err != nil {
		t.Fatal(err)
	}
	old := openTestDatabase(t, target.DatabasePath, "original")
	oldSQL, _ := old.DB()
	if err := oldSQL.Close(); err != nil {
		t.Fatal(err)
	}
	target.AttachmentHooks = &AttachmentHooks{PrepareRestore: func(context.Context, string, string) error { return nil }, PrepareDatabase: func(context.Context, string, string, string) (string, error) {
		return "", errors.New("injected merge failure")
	}}
	if _, err := StageTypedRestore(context.Background(), dataArchive, target, "original"); err != nil {
		t.Fatal(err)
	}
	if _, err := StageTypedRestore(context.Background(), attachmentArchive, target, "local"); err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyPendingRestore(target); !RestoreFailureRecovered(err) {
		t.Fatalf("failure was not safely recovered: %v", err)
	}
	if got := databaseValue(t, target.DatabasePath); got != "original" {
		t.Fatal(got)
	}
	if _, err := os.Stat(target.passwordPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("password file was not rolled back")
	}
}

func TestCommitFailureKeepsRollbackCopies(t *testing.T) {
	root := t.TempDir()
	original := filepath.Join(root, "old.db")
	target := filepath.Join(root, "new.db")
	if err := os.WriteFile(original, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	pending := filepath.Join(root, "pending.zip")
	if err := os.WriteFile(pending, []byte("archive"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(pending+".applied", 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pending+".applied", "occupied"), []byte("block rename"), 0600); err != nil {
		t.Fatal(err)
	}
	applied := &AppliedRestore{pending: pending, backups: []renamePair{{original, target}}, targets: []string{target}}
	if err := applied.Commit(); err == nil || applied.Committed() {
		t.Fatal("commit unexpectedly succeeded")
	}
	if err := applied.Rollback(); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(target); string(got) != "old" {
		t.Fatalf("rollback %q", got)
	}
}

func TestTypedArchiveRejectsPortablePathAndMetadataHazards(t *testing.T) {
	for _, names := range [][]string{{"database.db", `images\escape`}, {"database.db", "images/CON.txt"}, {"database.db", "images/a.", "images/a"}, {"database.db", "images/A/x", "images/a/y"}, {"database.db", "images/x", "images/x/y"}, {"database.db", "config.yaml"}, {"database.db", "images/a:b"}} {
		t.Run(strings.Join(names, "_"), func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "bad.zip")
			f, err := os.Create(p)
			if err != nil {
				t.Fatal(err)
			}
			z := zip.NewWriter(f)
			for _, name := range names {
				w, err := z.Create(name)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := w.Write([]byte("payload")); err != nil {
					t.Fatal(err)
				}
			}
			if err := z.Close(); err != nil {
				t.Fatal(err)
			}
			if err := f.Close(); err != nil {
				t.Fatal(err)
			}
			if _, err := InspectTypedArchive(p); err == nil {
				t.Fatal("unsafe archive accepted")
			}
		})
	}
}

func TestConfigRestoreRejectsInvalidTypesAndExecutableEnvironment(t *testing.T) {
	root := t.TempDir()
	for _, tc := range []struct{ name, content string }{{"config.yaml", "upload:\n  maxsize: [bad]\n"}, {"runtime.env", "TOKEN=$(id)\n"}, {"runtime.env", "TOKEN='value'; touch /tmp/bad\n"}, {"runtime.env", "1BAD=value\n"}} {
		p := filepath.Join(root, tc.name)
		if err := os.WriteFile(p, []byte(tc.content), 0600); err != nil {
			t.Fatal(err)
		}
		if err := validateConfigFile(p, tc.name); err == nil {
			t.Fatalf("accepted invalid %s", tc.name)
		}
	}
}
