package backup

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestOfflineWALArchiveAndActualRestore(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "source.db")
	writer := openTestDatabase(t, path, "committed-in-WAL")
	sqlWriter, _ := writer.DB()
	defer sqlWriter.Close()
	if err := writer.Exec("PRAGMA wal_autocheckpoint=0").Error; err != nil {
		t.Fatal(err)
	}
	if err := writer.Exec("INSERT INTO messages(id, content, user_id) VALUES(1, 'WAL note', 1)").Error; err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	blobs, media, cfg := filepath.Join(root, "external-blobs"), filepath.Join(root, "images"), filepath.Join(root, "config")
	for dir, value := range map[string]string{blobs: "blob", media: "image", cfg: "secret"} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "file"), []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	layout := Layout{DatabasePath: path, Roots: []Root{{"attachment-blobs", blobs}, {"images", media}}}
	db, err := OpenOffline(path, false)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	archive := filepath.Join(root, "backup.zip")
	if err := CreateOfflineArchive(archive, db, layout, cfg); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil || string(before) != string(after) {
		t.Fatal("offline archive changed source main database")
	}
	if err := db.Exec("CREATE TABLE forbidden(x)").Error; err == nil {
		t.Fatal("source connection was writable")
	}
	reader, err := zip.OpenReader(archive)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range reader.File {
		if f.Name == "protected-config/file" {
			r, _ := f.Open()
			b, e := io.ReadAll(r)
			r.Close()
			if e != nil || string(b) != "secret" {
				t.Fatal("configuration missing")
			}
			found = true
		}
	}
	reader.Close()
	if !found {
		t.Fatal("configuration not archived")
	}
	restored := Layout{DatabasePath: filepath.Join(root, "restored", "db"), Roots: []Root{{"attachment-blobs", filepath.Join(root, "restored-blobs")}, {"images", filepath.Join(root, "restored-images")}}}
	if _, err := StageRestoreWithResult(archive, restored); err != nil {
		t.Fatal(err)
	}
	applied, err := ApplyPendingRestore(restored)
	if err != nil {
		t.Fatal(err)
	}
	if err := applied.Commit(); err != nil {
		t.Fatal(err)
	}
	check, err := OpenOffline(restored.DatabasePath, false)
	if err != nil {
		t.Fatal(err)
	}
	sqlCheck, _ := check.DB()
	defer sqlCheck.Close()
	var content string
	if err := check.Raw("SELECT content FROM messages WHERE id=1").Scan(&content).Error; err != nil || content != "WAL note" {
		t.Fatalf("restored WAL note: %q %v", content, err)
	}
	for _, pair := range [][2]string{{restored.Roots[0].Path, "blob"}, {restored.Roots[1].Path, "image"}} {
		b, err := os.ReadFile(filepath.Join(pair[0], "file"))
		if err != nil || string(b) != pair[1] {
			t.Fatal("restored attachment unreadable")
		}
	}
	if err := CreateOfflineArchive(filepath.Join(blobs, "nested.zip"), db, layout, cfg); err == nil {
		t.Fatal("recursive archive accepted")
	}
	if _, err := OpenOffline(filepath.Join(root, "missing.db"), false); err == nil {
		t.Fatal("missing database created")
	}
}

func TestUpdateReservationBlocksEveryRestoreCaller(t *testing.T) {
	layout := Layout{DatabasePath: filepath.Join(t.TempDir(), "db")}
	t.Cleanup(func() { _ = ReserveUpdate("task", layout, true) })
	if err := ReserveUpdate("task", layout, false); err != nil {
		t.Fatal(err)
	}
	if _, err := StageRestoreWithResult("missing.zip", layout); err == nil || err.Error() != "更新停机已准备，不能同时恢复备份" {
		t.Fatalf("restore not refused: %v", err)
	}
	if err := ReserveUpdate("other", layout, true); err == nil {
		t.Fatal("other task released reservation")
	}
	if err := ReserveUpdate("task", layout, true); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(PendingRestorePath(layout), []byte("pending"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := ReserveUpdate("task", layout, false); err == nil {
		t.Fatal("pending restore accepted")
	}
}

func TestOfflineBackupRejectsRemoteSourcesAndPendingRestore(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "db")
	db := openTestDatabase(t, path, "old")
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	layout := Layout{DatabasePath: path}
	if err := db.Exec("CREATE TABLE attachment_blobs(storage_backend TEXT)").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO attachment_blobs VALUES('s3')").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := PlanOffline(db, layout, root); err == nil || err.Error() != "remote_attachment_backup_unsupported" {
		t.Fatalf("remote source accepted: %v", err)
	}
	if err := db.Exec("DELETE FROM attachment_blobs").Error; err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(PendingRestorePath(layout), []byte("pending"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := PlanOffline(db, layout, root); err == nil || err.Error() != "pending_restore_requires_reconciliation" {
		t.Fatalf("pending restore accepted: %v", err)
	}
}
