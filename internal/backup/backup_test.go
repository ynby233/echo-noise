package backup

import (
	"archive/zip"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func openTestDatabase(t *testing.T, path, value string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+filepath.ToSlash(path)+"?_pragma=journal_mode(WAL)"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE TABLE users (id INTEGER PRIMARY KEY)").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE TABLE messages (id INTEGER PRIMARY KEY)").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE TABLE site_configs (id INTEGER PRIMARY KEY)").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE TABLE records (value TEXT NOT NULL)").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO records(value) VALUES (?)", value).Error; err != nil {
		t.Fatal(err)
	}
	return db
}

func databaseValue(t *testing.T, path string) string {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+filepath.ToSlash(path)+"?mode=ro"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	var value string
	if err := db.Raw("SELECT value FROM records LIMIT 1").Scan(&value).Error; err != nil {
		t.Fatal(err)
	}
	return value
}

func TestCreateArchiveIncludesWALAndConfiguredBlobRoot(t *testing.T) {
	root := t.TempDir()
	databasePath := filepath.Join(root, "数据库 空格", "database.db")
	if err := os.MkdirAll(filepath.Dir(databasePath), 0755); err != nil {
		t.Fatal(err)
	}
	db := openTestDatabase(t, databasePath, "committed-in-wal")
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()

	blobRoot := filepath.Join(root, "外置附件")
	imageRoot := filepath.Join(root, "images")
	for path, content := range map[string]string{
		filepath.Join(blobRoot, "aa", "blob.bin"): "blob-bytes",
		filepath.Join(imageRoot, "cover.png"):     "image-bytes",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	archivePath := filepath.Join(root, "archive.zip")
	layout := Layout{DatabasePath: databasePath, Roots: []Root{
		{ArchiveName: "attachment-blobs", Path: blobRoot},
		{ArchiveName: "images", Path: imageRoot},
	}}
	if err := CreateArchive(archivePath, db, layout); err != nil {
		t.Fatal(err)
	}
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	names := map[string]bool{}
	for _, file := range reader.File {
		names[file.Name] = true
	}
	for _, expected := range []string{"database.db", "attachment-blobs/aa/blob.bin", "images/cover.png"} {
		if !names[expected] {
			t.Fatalf("archive missing %s: %#v", expected, names)
		}
	}
	stage := filepath.Join(root, "inspect")
	if err := extractArchive(archivePath, stage); err != nil {
		t.Fatal(err)
	}
	if got := databaseValue(t, filepath.Join(stage, "database.db")); got != "committed-in-wal" {
		t.Fatalf("snapshot = %q, want WAL value", got)
	}
}

func TestStagedRestoreLeavesLiveDataUntouchedUntilStartupAndCanRollback(t *testing.T) {
	root := t.TempDir()
	sourceDatabase := filepath.Join(root, "source", "database.db")
	if err := os.MkdirAll(filepath.Dir(sourceDatabase), 0755); err != nil {
		t.Fatal(err)
	}
	sourceDB := openTestDatabase(t, sourceDatabase, "restored")
	sourceSQL, err := sourceDB.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer sourceSQL.Close()
	sourceBlob := filepath.Join(root, "source", "blobs")
	if err := os.MkdirAll(sourceBlob, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceBlob, "new.bin"), []byte("new"), 0644); err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(root, "restore.zip")
	if err := CreateArchive(archivePath, sourceDB, Layout{DatabasePath: sourceDatabase, Roots: []Root{{ArchiveName: "attachment-blobs", Path: sourceBlob}}}); err != nil {
		t.Fatal(err)
	}

	targetDatabase := filepath.Join(root, "target", "database.db")
	if err := os.MkdirAll(filepath.Dir(targetDatabase), 0755); err != nil {
		t.Fatal(err)
	}
	targetDB := openTestDatabase(t, targetDatabase, "original")
	targetSQL, err := targetDB.DB()
	if err != nil {
		t.Fatal(err)
	}
	targetSQL.Close()
	targetBlob := filepath.Join(root, "target", "blobs")
	if err := os.MkdirAll(targetBlob, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(targetBlob, "old.bin"), []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	legacyAudio := filepath.Join(root, "target", "audio")
	if err := os.MkdirAll(legacyAudio, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyAudio, "keep.mp3"), []byte("old audio"), 0644); err != nil {
		t.Fatal(err)
	}
	layout := Layout{DatabasePath: targetDatabase, Roots: []Root{
		{ArchiveName: "attachment-blobs", Path: targetBlob},
		{ArchiveName: "audio", Path: legacyAudio},
	}}
	if err := StageRestore(archivePath, layout); err != nil {
		t.Fatal(err)
	}
	if got := databaseValue(t, targetDatabase); got != "original" {
		t.Fatalf("staging modified live database: %q", got)
	}
	for _, sidecar := range []string{targetDatabase + "-wal", targetDatabase + "-shm"} {
		if err := os.WriteFile(sidecar, []byte("old-sidecar"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	applied, err := ApplyPendingRestore(layout)
	if err != nil {
		t.Fatal(err)
	}
	for _, sidecar := range []string{targetDatabase + "-wal", targetDatabase + "-shm"} {
		if _, err := os.Stat(sidecar); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("stale sidecar remained after restore: %s (%v)", sidecar, err)
		}
	}
	if got := databaseValue(t, targetDatabase); got != "restored" {
		t.Fatalf("applied database = %q", got)
	}
	if _, err := os.Stat(filepath.Join(targetBlob, "new.bin")); err != nil {
		t.Fatalf("restored blob missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(legacyAudio, "keep.mp3")); err != nil {
		t.Fatalf("backup without audio unexpectedly removed existing audio: %v", err)
	}
	if err := applied.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err := applied.DiscardPending(); err != nil {
		t.Fatal(err)
	}
	if HasPendingRestore(layout) {
		t.Fatal("failed restore package remained pending after rollback")
	}
	if _, err := os.Stat(filepath.Join(targetBlob, "old.bin")); err != nil {
		t.Fatalf("rollback blob missing: %v", err)
	}
	for _, sidecar := range []string{targetDatabase + "-wal", targetDatabase + "-shm"} {
		if got, err := os.ReadFile(sidecar); err != nil || string(got) != "old-sidecar" {
			t.Fatalf("rollback sidecar %s = %q, %v", sidecar, got, err)
		}
	}
}

func TestStageRestoreRejectsEmptySQLiteDatabase(t *testing.T) {
	root := t.TempDir()
	emptyPath := filepath.Join(root, "empty.db")
	empty, err := gorm.Open(sqlite.Open("file:"+filepath.ToSlash(emptyPath)), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	emptySQL, err := empty.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := emptySQL.Close(); err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(root, "empty.zip")
	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	if err := addFile(writer, emptyPath, "database.db"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	layout := Layout{DatabasePath: filepath.Join(root, "target", "database.db")}
	err = StageRestore(archivePath, layout)
	if err == nil || !strings.Contains(err.Error(), "缺少必要业务表") {
		t.Fatalf("StageRestore empty database error = %v", err)
	}
}
