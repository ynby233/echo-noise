// Package backup owns the on-disk SQLite backup format used by both HTTP
// downloads and cloud synchronization. It deliberately stages restores for a
// process restart: the running server has background workers holding database
// handles, so replacing live files would create a half-old, half-new runtime.
package backup

import (
	"archive/zip"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/rcy1314/echo-noise/config"
	attachmentregistry "github.com/rcy1314/echo-noise/internal/attachments"
	"github.com/rcy1314/echo-noise/internal/database"
	"gorm.io/gorm"
)

const pendingRestoreName = ".echo-noise-restore-pending.zip"

// ErrRestartRequired tells callers that a verified restore is safely staged
// but cannot be applied until the process is restarted.
var ErrRestartRequired = errors.New("备份已暂存，需重启服务后应用恢复")

// Root is one media tree in an archive. A missing root is valid for an empty
// installation; an existing root that cannot be read makes the backup fail.
type Root struct {
	ArchiveName string
	Path        string
}

// Layout makes backup locations explicit and is injectable by tests.
type Layout struct {
	DatabasePath string
	Roots        []Root
}

// DefaultLayout preserves the established ZIP root names while including the
// local attachment registry's configurable blob root.
func DefaultLayout() Layout {
	imagePath := strings.TrimSpace(config.Config.Upload.SavePath)
	if imagePath == "" {
		imagePath = filepath.Join("data", "images")
	}
	return Layout{
		DatabasePath: database.SQLitePath(),
		Roots: []Root{
			{ArchiveName: "attachment-blobs", Path: attachmentregistry.DefaultLocalRoot()},
			{ArchiveName: "images", Path: imagePath},
			{ArchiveName: "video", Path: filepath.Join("data", "video")},
			{ArchiveName: "audio", Path: filepath.Join("data", "audio")},
			{ArchiveName: "attachments", Path: filepath.Join("data", "attachments")},
		},
	}
}

// CreateArchive creates a consistent SQLite snapshot before adding media. It
// never copies a live WAL main database directly.
func CreateArchive(destination string, db *gorm.DB, layout Layout) error {
	if db == nil {
		return errors.New("数据库未初始化")
	}
	if strings.TrimSpace(layout.DatabasePath) == "" {
		return errors.New("数据库路径为空")
	}
	stage, err := os.MkdirTemp(filepath.Dir(destination), ".echo-noise-backup-*")
	if err != nil {
		return fmt.Errorf("创建备份临时目录失败: %w", err)
	}
	defer os.RemoveAll(stage)

	snapshot := filepath.Join(stage, "database.db")
	if err := snapshotSQLite(db, snapshot); err != nil {
		return err
	}
	if err := writeArchive(destination, snapshot, layout); err != nil {
		return err
	}
	return InspectArchive(destination)
}

func snapshotSQLite(db *gorm.DB, destination string) error {
	if err := os.Remove(destination); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("清理临时快照失败: %w", err)
	}
	if err := db.Exec("VACUUM INTO ?", destination).Error; err != nil {
		return fmt.Errorf("创建 SQLite 一致性快照失败: %w", err)
	}
	return validateSQLite(destination)
}

func writeArchive(destination, snapshot string, layout Layout) (err error) {
	file, err := os.Create(destination)
	if err != nil {
		return fmt.Errorf("创建备份文件失败: %w", err)
	}
	defer func() {
		if closeErr := file.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
	}()
	archive := zip.NewWriter(file)
	defer func() {
		if closeErr := archive.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
	}()
	if err := addFile(archive, snapshot, "database.db"); err != nil {
		return err
	}
	seen := map[string]struct{}{}
	for _, root := range layout.Roots {
		if strings.TrimSpace(root.Path) == "" || strings.TrimSpace(root.ArchiveName) == "" {
			continue
		}
		absolute, absoluteErr := filepath.Abs(root.Path)
		if absoluteErr != nil {
			return absoluteErr
		}
		if _, duplicate := seen[absolute]; duplicate {
			continue
		}
		seen[absolute] = struct{}{}
		info, statErr := os.Stat(absolute)
		if errors.Is(statErr, os.ErrNotExist) {
			continue
		}
		if statErr != nil {
			return fmt.Errorf("读取附件目录失败 %s: %w", absolute, statErr)
		}
		if !info.IsDir() {
			return fmt.Errorf("附件路径不是目录: %s", absolute)
		}
		if err := addDirectory(archive, absolute, filepath.ToSlash(root.ArchiveName)); err != nil {
			return err
		}
	}
	return nil
}

func addDirectory(archive *zip.Writer, root, archiveRoot string) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		return addFile(archive, path, filepath.ToSlash(filepath.Join(archiveRoot, relative)))
	})
}

func addFile(archive *zip.Writer, source, name string) (err error) {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := in.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
	}()
	out, err := archive.Create(name)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	return err
}

// InspectArchive checks the portable archive contract before it can be staged.
func InspectArchive(path string) error {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return fmt.Errorf("打开备份文件失败: %w", err)
	}
	defer reader.Close()
	foundDatabase := false
	seen := map[string]struct{}{}
	for _, file := range reader.File {
		name, err := safeArchiveName(file.Name)
		if err != nil {
			return err
		}
		if _, duplicate := seen[name]; duplicate && !file.FileInfo().IsDir() {
			return fmt.Errorf("备份包含重复文件: %s", name)
		}
		seen[name] = struct{}{}
		if name == "database.db" && !file.FileInfo().IsDir() {
			foundDatabase = true
		}
	}
	if !foundDatabase {
		return errors.New("备份不包含 database.db")
	}
	return nil
}

func safeArchiveName(name string) (string, error) {
	cleaned := filepath.ToSlash(filepath.Clean(name))
	if cleaned == "." || strings.HasPrefix(cleaned, "../") || strings.HasPrefix(cleaned, "/") || strings.Contains(cleaned, ":") {
		return "", fmt.Errorf("备份包含非法路径: %s", name)
	}
	return cleaned, nil
}

// StageRestore validates a ZIP and stores it beside the database. The package
// is only applied before the next database initialization, never live.
func StageRestore(source string, layout Layout) error {
	if err := InspectArchive(source); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(layout.DatabasePath), 0755); err != nil {
		return fmt.Errorf("创建恢复目录失败: %w", err)
	}
	stage, err := os.MkdirTemp(filepath.Dir(layout.DatabasePath), ".echo-noise-restore-check-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	if err := extractArchive(source, stage); err != nil {
		return err
	}
	if err := validateSQLite(filepath.Join(stage, "database.db")); err != nil {
		return fmt.Errorf("备份数据库无效: %w", err)
	}
	return copyPath(source, PendingRestorePath(layout))
}

func PendingRestorePath(layout Layout) string {
	return filepath.Join(filepath.Dir(layout.DatabasePath), pendingRestoreName)
}

func HasPendingRestore(layout Layout) bool {
	_, err := os.Stat(PendingRestorePath(layout))
	return err == nil
}

// AppliedRestore retains a rollback path until database initialization has
// completed. Call Commit only after the new database is successfully ready.
type AppliedRestore struct {
	backups []renamePair
	targets []string
	pending string
}

type renamePair struct{ from, to string }

// ApplyPendingRestore must run while the database is closed, before InitDB.
func ApplyPendingRestore(layout Layout) (*AppliedRestore, error) {
	pending := PendingRestorePath(layout)
	if _, err := os.Stat(pending); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	stage, err := os.MkdirTemp(filepath.Dir(layout.DatabasePath), ".echo-noise-restore-apply-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(stage)
	if err := extractArchive(pending, stage); err != nil {
		return nil, err
	}
	if err := validateSQLite(filepath.Join(stage, "database.db")); err != nil {
		return nil, fmt.Errorf("待恢复数据库无效: %w", err)
	}

	pairs := []renamePair{{from: filepath.Join(stage, "database.db"), to: layout.DatabasePath}}
	for _, root := range layout.Roots {
		if strings.TrimSpace(root.ArchiveName) == "" || strings.TrimSpace(root.Path) == "" {
			continue
		}
		candidate := filepath.Join(stage, filepath.FromSlash(root.ArchiveName))
		if info, statErr := os.Stat(candidate); statErr == nil && info.IsDir() {
			pairs = append(pairs, renamePair{from: candidate, to: root.Path})
		} else if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
			return nil, statErr
		}
	}
	applied := &AppliedRestore{pending: pending}
	// The archive contains a self-contained SQLite snapshot, not the live
	// database's WAL sidecars. Keeping an old WAL beside the restored main file
	// would let SQLite replay pre-restore changes after the next startup.
	for _, sidecar := range []string{layout.DatabasePath + "-wal", layout.DatabasePath + "-shm"} {
		// InitDB may create fresh sidecars before a later failure. Rollback must
		// remove those newly generated files before the original sidecars return.
		applied.targets = append(applied.targets, sidecar)
		if err := moveExistingToRestoreBackup(sidecar, applied); err != nil {
			applied.Rollback()
			return nil, err
		}
	}
	for _, pair := range pairs {
		if err := os.MkdirAll(filepath.Dir(pair.to), 0755); err != nil {
			applied.Rollback()
			return nil, err
		}
		if err := moveExistingToRestoreBackup(pair.to, applied); err != nil {
			applied.Rollback()
			return nil, err
		}
		if err := os.Rename(pair.from, pair.to); err != nil {
			applied.Rollback()
			return nil, err
		}
		applied.targets = append(applied.targets, pair.to)
	}
	return applied, nil
}

func moveExistingToRestoreBackup(path string, applied *AppliedRestore) error {
	backup := path + ".restore-backup"
	if err := os.RemoveAll(backup); err != nil {
		return err
	}
	if _, err := os.Stat(path); err == nil {
		if err := os.Rename(path, backup); err != nil {
			return err
		}
		applied.backups = append(applied.backups, renamePair{from: backup, to: path})
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (a *AppliedRestore) Commit() error {
	if a == nil {
		return nil
	}
	if err := os.Remove(a.pending); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	for _, backup := range a.backups {
		if err := os.RemoveAll(backup.from); err != nil {
			return err
		}
	}
	return nil
}

// DiscardPending removes a restore package after a failed startup rollback so
// one incompatible archive cannot trap every later restart in the same loop.
func (a *AppliedRestore) DiscardPending() error {
	if a == nil {
		return nil
	}
	if err := os.Remove(a.pending); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (a *AppliedRestore) Rollback() error {
	if a == nil {
		return nil
	}
	var firstErr error
	for index := len(a.targets) - 1; index >= 0; index-- {
		if err := os.RemoveAll(a.targets[index]); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	for index := len(a.backups) - 1; index >= 0; index-- {
		backup := a.backups[index]
		if err := os.Rename(backup.from, backup.to); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func extractArchive(source, destination string) error {
	reader, err := zip.OpenReader(source)
	if err != nil {
		return err
	}
	defer reader.Close()
	for _, file := range reader.File {
		name, err := safeArchiveName(file.Name)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, filepath.FromSlash(name))
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		in, err := file.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, file.Mode())
		if err == nil {
			_, err = io.Copy(out, in)
			closeErr := out.Close()
			if err == nil {
				err = closeErr
			}
		}
		closeErr := in.Close()
		if err == nil {
			err = closeErr
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func validateSQLite(path string) error {
	dsn := "file:" + filepath.ToSlash(path) + "?mode=ro&immutable=1"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	var result string
	if err := db.QueryRow("PRAGMA quick_check").Scan(&result); err != nil {
		return err
	}
	if result != "ok" {
		return fmt.Errorf("PRAGMA quick_check=%s", result)
	}
	for _, table := range []string{"users", "messages", "site_configs"} {
		var found string
		err := db.QueryRow("SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?", table).Scan(&found)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("备份缺少必要业务表: %s", table)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func copyPath(source, destination string) (err error) {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
		return err
	}
	out, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := out.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
	}()
	_, err = io.Copy(out, in)
	return err
}
