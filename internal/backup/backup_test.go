package backup

import (
	"archive/zip"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
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
	if err := db.Exec("CREATE TABLE users (id INTEGER PRIMARY KEY, username TEXT, password TEXT, is_admin INTEGER)").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE TABLE messages (id INTEGER PRIMARY KEY, content TEXT, user_id INTEGER)").Error; err != nil {
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

func TestApplyPendingRestoreAcrossVolumes(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("the CI environment needs two mounted filesystems to exercise a real cross-volume rename")
	}
	root := t.TempDir()
	externalBase := filepath.Join(os.Getenv("LOCALAPPDATA"), "Temp")
	if externalBase == "Temp" || !strings.EqualFold(filepath.VolumeName(externalBase), "C:") || strings.EqualFold(filepath.VolumeName(root), filepath.VolumeName(externalBase)) {
		t.Skip("no writable second volume is available")
	}
	external, err := os.MkdirTemp(externalBase, "echo-noise-cross-volume-*")
	if err != nil {
		t.Skipf("cannot create cross-volume fixture: %v", err)
	}
	defer os.RemoveAll(external)

	sourceDatabase := filepath.Join(root, "source.db")
	sourceDB := openTestDatabase(t, sourceDatabase, "restored")
	sourceSQL, err := sourceDB.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer sourceSQL.Close()
	sourceBlobs := filepath.Join(root, "source-blobs")
	if err := os.MkdirAll(sourceBlobs, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceBlobs, "new.bin"), []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(root, "restore.zip")
	if err := CreateArchive(archive, sourceDB, Layout{DatabasePath: sourceDatabase, Roots: []Root{{ArchiveName: "attachment-blobs", Path: sourceBlobs}}}); err != nil {
		t.Fatal(err)
	}

	targetDatabase := filepath.Join(root, "target.db")
	targetDB := openTestDatabase(t, targetDatabase, "original")
	targetSQL, err := targetDB.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := targetSQL.Close(); err != nil {
		t.Fatal(err)
	}
	targetBlobs := filepath.Join(external, "blobs")
	if err := os.MkdirAll(targetBlobs, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(targetBlobs, "old.bin"), []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	layout := Layout{DatabasePath: targetDatabase, Roots: []Root{{ArchiveName: "attachment-blobs", Path: targetBlobs}}}
	if err := StageRestore(archive, layout); err != nil {
		t.Fatal(err)
	}
	applied, err := ApplyPendingRestore(layout)
	if err != nil {
		t.Fatalf("cross-volume restore failed: %v", err)
	}
	if err := applied.Commit(); err != nil {
		t.Fatal(err)
	}
	if got := databaseValue(t, targetDatabase); got != "restored" {
		t.Fatalf("database value = %q", got)
	}
	if got, err := os.ReadFile(filepath.Join(targetBlobs, "new.bin")); err != nil || string(got) != "new" {
		t.Fatalf("restored external blob = %q, %v", got, err)
	}
}

func TestApplyPendingRestoreReplacesExistingDirectoryInPlace(t *testing.T) {
	root := t.TempDir()
	sourceDatabase := filepath.Join(root, "source.db")
	sourceDB := openTestDatabase(t, sourceDatabase, "restored")
	sourceSQL, err := sourceDB.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer sourceSQL.Close()
	sourceBlobs := filepath.Join(root, "source-blobs")
	if err := os.MkdirAll(filepath.Join(sourceBlobs, "nested"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceBlobs, "nested", "new.bin"), []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(root, "restore.zip")
	if err := CreateArchive(archive, sourceDB, Layout{
		DatabasePath: sourceDatabase,
		Roots:        []Root{{ArchiveName: "attachment-blobs", Path: sourceBlobs}},
	}); err != nil {
		t.Fatal(err)
	}

	targetDatabase := filepath.Join(root, "target.db")
	targetDB := openTestDatabase(t, targetDatabase, "original")
	targetSQL, err := targetDB.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := targetSQL.Close(); err != nil {
		t.Fatal(err)
	}
	targetBlobs := filepath.Join(root, "mounted-blobs")
	if err := os.MkdirAll(targetBlobs, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(targetBlobs, "old.bin"), []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	targetInfoBefore, err := os.Stat(targetBlobs)
	if err != nil {
		t.Fatal(err)
	}
	layout := Layout{DatabasePath: targetDatabase, Roots: []Root{{ArchiveName: "attachment-blobs", Path: targetBlobs}}}
	if err := StageRestore(archive, layout); err != nil {
		t.Fatal(err)
	}
	applied, err := ApplyPendingRestore(layout)
	if err != nil {
		t.Fatal(err)
	}
	targetInfoAfter, err := os.Stat(targetBlobs)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(targetInfoBefore, targetInfoAfter) {
		t.Fatal("existing attachment root was renamed instead of being preserved in place")
	}
	if _, err := os.Stat(filepath.Join(targetBlobs, "old.bin")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("old blob remained after apply: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(targetBlobs, "nested", "new.bin")); err != nil || string(got) != "new" {
		t.Fatalf("new blob = %q, %v", got, err)
	}
	if err := applied.Rollback(); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(filepath.Join(targetBlobs, "old.bin")); err != nil || string(got) != "old" {
		t.Fatalf("rolled-back blob = %q, %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(targetBlobs, "nested", "new.bin")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("restored blob remained after rollback: %v", err)
	}
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

func createDatabaseOnlyArchive(t *testing.T, root, name, value string) string {
	t.Helper()
	databasePath := filepath.Join(root, name+".db")
	db := openTestDatabase(t, databasePath, value)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(root, name+".zip")
	if err := CreateArchive(archivePath, db, Layout{DatabasePath: databasePath}); err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}
	return archivePath
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

func TestCreateArchiveMarksMissingConfiguredAttachmentRootAsEmpty(t *testing.T) {
	root := t.TempDir()
	databasePath := filepath.Join(root, "source.db")
	db := openTestDatabase(t, databasePath, "empty-attachments")
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()

	archivePath := filepath.Join(root, "archive.zip")
	missingRoot := filepath.Join(root, "missing-attachment-blobs")
	if err := CreateArchive(archivePath, db, Layout{
		DatabasePath: databasePath,
		Roots:        []Root{{ArchiveName: "attachment-blobs", Path: missingRoot}},
	}); err != nil {
		t.Fatal(err)
	}
	inspection, err := inspectArchive(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	if !inspection.HasAttachmentData {
		t.Fatal("new archive with an empty configured attachment root was classified as an old database-only archive")
	}
	result, err := StageRestoreWithResult(archivePath, Layout{DatabasePath: filepath.Join(root, "target.db")})
	if err != nil {
		t.Fatal(err)
	}
	if result.Warning != "" {
		t.Fatalf("new archive with an empty attachment root warning = %q", result.Warning)
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

func TestStageRestoreRejectsLookalikeSQLiteDatabase(t *testing.T) {
	root := t.TempDir()
	databasePath := filepath.Join(root, "lookalike.db")
	db, err := gorm.Open(sqlite.Open("file:"+filepath.ToSlash(databasePath)), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		"CREATE TABLE users (id INTEGER PRIMARY KEY)",
		"CREATE TABLE messages (id INTEGER PRIMARY KEY)",
		"CREATE TABLE site_configs (id INTEGER PRIMARY KEY)",
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(root, "lookalike.zip")
	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	if err := addFile(writer, databasePath, "database.db"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	err = StageRestore(archivePath, Layout{DatabasePath: filepath.Join(root, "target.db")})
	if err == nil || !strings.Contains(err.Error(), "缺少必要字段") {
		t.Fatalf("lookalike database error = %v", err)
	}
}

func TestStageRestoreWarnsWhenArchiveHasNoAttachmentRoots(t *testing.T) {
	root := t.TempDir()
	sourcePath := filepath.Join(root, "source.db")
	sourceDB := openTestDatabase(t, sourcePath, "database-only")
	sourceSQL, err := sourceDB.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer sourceSQL.Close()
	archivePath := filepath.Join(root, "old-backup.zip")
	if err := CreateArchive(archivePath, sourceDB, Layout{DatabasePath: sourcePath}); err != nil {
		t.Fatal(err)
	}
	result, err := StageRestoreWithResult(archivePath, Layout{DatabasePath: filepath.Join(root, "target.db")})
	if err != nil {
		t.Fatal(err)
	}
	if result.Warning != "该备份不含附件，不能用于完整迁移" {
		t.Fatalf("warning = %q", result.Warning)
	}
}

func TestConcurrentStageRestorePublishesOneCompleteArchive(t *testing.T) {
	root := t.TempDir()
	archives := make([]string, 2)
	for index, value := range []string{"first", "second"} {
		databasePath := filepath.Join(root, value+".db")
		db := openTestDatabase(t, databasePath, value)
		sqlDB, err := db.DB()
		if err != nil {
			t.Fatal(err)
		}
		archives[index] = filepath.Join(root, value+".zip")
		if err := CreateArchive(archives[index], db, Layout{DatabasePath: databasePath}); err != nil {
			t.Fatal(err)
		}
		if err := sqlDB.Close(); err != nil {
			t.Fatal(err)
		}
	}
	layout := Layout{DatabasePath: filepath.Join(root, "target.db")}
	var wait sync.WaitGroup
	errorsByRequest := make([]error, len(archives))
	for index := range archives {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			_, errorsByRequest[index] = StageRestoreWithResult(archives[index], layout)
		}(index)
	}
	wait.Wait()
	for _, err := range errorsByRequest {
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := InspectArchive(PendingRestorePath(layout)); err != nil {
		t.Fatalf("published pending archive is incomplete: %v", err)
	}
	applied, err := ApplyPendingRestore(layout)
	if err != nil {
		t.Fatal(err)
	}
	if err := applied.Commit(); err != nil {
		t.Fatal(err)
	}
	if got := databaseValue(t, layout.DatabasePath); got != "first" && got != "second" {
		t.Fatalf("restored value = %q", got)
	}
}

func TestPendingPublicationFailurePreservesPreviouslyStagedArchive(t *testing.T) {
	root := t.TempDir()
	firstArchive := createDatabaseOnlyArchive(t, root, "first", "first")
	secondArchive := createDatabaseOnlyArchive(t, root, "second", "second")
	layout := Layout{DatabasePath: filepath.Join(root, "target.db")}
	if err := StageRestore(firstArchive, layout); err != nil {
		t.Fatal(err)
	}

	// A non-empty directory at the rollback-journal path simulates a
	// filesystem failure before the old pending package can be rotated.
	previous := previousPendingRestorePath(layout)
	if err := os.Mkdir(previous, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(previous, "blocked"), []byte("blocked"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := StageRestoreWithResult(secondArchive, layout); err == nil {
		t.Fatal("second staging unexpectedly succeeded")
	}
	if err := InspectArchive(PendingRestorePath(layout)); err != nil {
		t.Fatalf("previous pending archive was damaged: %v", err)
	}
	if err := os.RemoveAll(previous); err != nil {
		t.Fatal(err)
	}
	applied, err := ApplyPendingRestore(layout)
	if err != nil {
		t.Fatal(err)
	}
	if err := applied.Commit(); err != nil {
		t.Fatal(err)
	}
	if got := databaseValue(t, layout.DatabasePath); got != "first" {
		t.Fatalf("restored value = %q, want the previously staged archive", got)
	}
}

func TestPendingWriteFailurePreservesPreviouslyStagedArchive(t *testing.T) {
	root := t.TempDir()
	firstArchive := createDatabaseOnlyArchive(t, root, "first", "first")
	secondArchive := createDatabaseOnlyArchive(t, root, "second", "second")
	layout := Layout{DatabasePath: filepath.Join(root, "target.db")}
	if err := StageRestore(firstArchive, layout); err != nil {
		t.Fatal(err)
	}

	injected := errors.New("injected disk write failure")
	err := publishPendingRestoreWithCopy(secondArchive, layout, func(destination io.Writer, source io.Reader) (int64, error) {
		buffer := make([]byte, 32)
		read, readErr := source.Read(buffer)
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return 0, readErr
		}
		written, writeErr := destination.Write(buffer[:read])
		if writeErr != nil {
			return int64(written), writeErr
		}
		return int64(written), injected
	})
	if !errors.Is(err, injected) {
		t.Fatalf("publish error = %v", err)
	}
	if err := InspectArchive(PendingRestorePath(layout)); err != nil {
		t.Fatalf("previous pending archive was damaged: %v", err)
	}
	applied, err := ApplyPendingRestore(layout)
	if err != nil {
		t.Fatal(err)
	}
	if err := applied.Commit(); err != nil {
		t.Fatal(err)
	}
	if got := databaseValue(t, layout.DatabasePath); got != "first" {
		t.Fatalf("restored value = %q, want the previously staged archive", got)
	}
	if matches, _ := filepath.Glob(filepath.Join(root, ".echo-noise-restore-publish-*.tmp")); len(matches) != 0 {
		t.Fatalf("temporary publication files remain: %#v", matches)
	}
}

func TestApplyPendingRestoreRecoversInterruptedPublicationWithoutReplayingOldArchive(t *testing.T) {
	root := t.TempDir()
	firstArchive := createDatabaseOnlyArchive(t, root, "first", "first")
	secondArchive := createDatabaseOnlyArchive(t, root, "second", "second")
	layout := Layout{DatabasePath: filepath.Join(root, "target.db")}
	pending := PendingRestorePath(layout)
	previous := previousPendingRestorePath(layout)

	if err := StageRestore(firstArchive, layout); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(pending, previous); err != nil {
		t.Fatal(err)
	}
	if !HasPendingRestore(layout) {
		t.Fatal("interrupted publication journal was not recognized as a pending restore")
	}
	applied, err := ApplyPendingRestore(layout)
	if err != nil {
		t.Fatal(err)
	}
	if err := applied.Commit(); err != nil {
		t.Fatal(err)
	}
	if got := databaseValue(t, layout.DatabasePath); got != "first" {
		t.Fatalf("recovered publication value = %q", got)
	}

	if err := StageRestore(secondArchive, layout); err != nil {
		t.Fatal(err)
	}
	firstBytes, err := os.ReadFile(firstArchive)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(previous, firstBytes, 0600); err != nil {
		t.Fatal(err)
	}
	applied, err = ApplyPendingRestore(layout)
	if err != nil {
		t.Fatal(err)
	}
	if err := applied.Commit(); err != nil {
		t.Fatal(err)
	}
	if got := databaseValue(t, layout.DatabasePath); got != "second" {
		t.Fatalf("new pending value = %q", got)
	}
	if _, err := os.Stat(previous); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale previous pending archive remains: %v", err)
	}
	if applied, err := ApplyPendingRestore(layout); err != nil || applied != nil {
		t.Fatalf("stale archive replayed after commit: applied=%v err=%v", applied, err)
	}
}

func TestInterruptedPublicationFallsBackWhenNewPendingArchiveIsCorrupt(t *testing.T) {
	root := t.TempDir()
	firstArchive := createDatabaseOnlyArchive(t, root, "first", "first")
	layout := Layout{DatabasePath: filepath.Join(root, "target.db")}
	pending := PendingRestorePath(layout)
	previous := previousPendingRestorePath(layout)
	if err := StageRestore(firstArchive, layout); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(pending, previous); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pending, []byte("corrupted replacement"), 0600); err != nil {
		t.Fatal(err)
	}

	if _, err := ApplyPendingRestore(layout); err == nil || !RestoreFailureRecovered(err) {
		t.Fatalf("corrupt replacement error = %v", err)
	}
	if _, err := os.Stat(previous); err != nil {
		t.Fatalf("previous pending archive was lost: %v", err)
	}
	applied, err := ApplyPendingRestore(layout)
	if err != nil {
		t.Fatal(err)
	}
	if err := applied.Commit(); err != nil {
		t.Fatal(err)
	}
	if got := databaseValue(t, layout.DatabasePath); got != "first" {
		t.Fatalf("fallback restore value = %q", got)
	}
}

func TestApplyFailureQuarantinesPendingAndAllowsOldDataToBoot(t *testing.T) {
	root := t.TempDir()
	sourcePath := filepath.Join(root, "source.db")
	sourceDB := openTestDatabase(t, sourcePath, "restored")
	sourceSQL, err := sourceDB.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer sourceSQL.Close()
	sourceBlobs := filepath.Join(root, "source-blobs")
	if err := os.MkdirAll(sourceBlobs, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceBlobs, "new.bin"), []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(root, "restore.zip")
	if err := CreateArchive(archivePath, sourceDB, Layout{DatabasePath: sourcePath, Roots: []Root{{ArchiveName: "attachment-blobs", Path: sourceBlobs}}}); err != nil {
		t.Fatal(err)
	}
	targetPath := filepath.Join(root, "target.db")
	targetDB := openTestDatabase(t, targetPath, "original")
	targetSQL, err := targetDB.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := targetSQL.Close(); err != nil {
		t.Fatal(err)
	}
	blockedParent := filepath.Join(root, "not-a-directory")
	if err := os.WriteFile(blockedParent, []byte("blocked"), 0600); err != nil {
		t.Fatal(err)
	}
	layout := Layout{DatabasePath: targetPath, Roots: []Root{{ArchiveName: "attachment-blobs", Path: filepath.Join(blockedParent, "blobs")}}}
	if err := StageRestore(archivePath, layout); err != nil {
		t.Fatal(err)
	}
	_, err = ApplyPendingRestore(layout)
	if err == nil || !RestoreFailureRecovered(err) {
		t.Fatalf("apply error = %v, want safely recovered failure", err)
	}
	if HasPendingRestore(layout) {
		t.Fatal("failed pending archive was not quarantined")
	}
	if matches, _ := filepath.Glob(filepath.Join(root, ".echo-noise-restore-failed-*.zip")); len(matches) != 1 {
		t.Fatalf("quarantined archives = %#v", matches)
	}
	if got := databaseValue(t, targetPath); got != "original" {
		t.Fatalf("old database changed after preparation failure: %q", got)
	}
}

func TestDirectoryRollbackFailureIsPropagatedAsUnsafe(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "mounted-root")
	candidate := filepath.Join(target, ".echo-noise-restore-candidate-test")
	if err := os.MkdirAll(candidate, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "old.bin"), []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(candidate, "new.bin"), []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	switchFailure := errors.New("injected switch failure")
	rollbackFailure := errors.New("injected rollback failure")
	renameCalls := 0
	_, err := replaceDirectoryContentsWithRename(candidate, target, func(from, to string) error {
		renameCalls++
		switch renameCalls {
		case 1:
			return os.Rename(from, to)
		case 2:
			return switchFailure
		default:
			return rollbackFailure
		}
	})
	if !errors.Is(err, switchFailure) || !errors.Is(err, rollbackFailure) {
		t.Fatalf("directory replacement error = %v", err)
	}
	pending := filepath.Join(root, pendingRestoreName)
	if err := os.WriteFile(pending, []byte("pending"), 0600); err != nil {
		t.Fatal(err)
	}
	applyErr := rollbackAndQuarantine(&AppliedRestore{pending: pending}, err)
	if RestoreFailureRecovered(applyErr) {
		t.Fatalf("rollback failure incorrectly marked recovered: %v", applyErr)
	}
	if _, statErr := os.Stat(pending); statErr != nil {
		t.Fatalf("unsafe failure package was discarded: %v", statErr)
	}
	if matches, _ := filepath.Glob(filepath.Join(target, ".echo-noise-restore-backup-*")); len(matches) != 1 {
		t.Fatalf("rollback evidence directories = %#v", matches)
	}
}
