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
	"sync"
	"time"

	"github.com/rcy1314/echo-noise/config"
	attachmentregistry "github.com/rcy1314/echo-noise/internal/attachments"
	"github.com/rcy1314/echo-noise/internal/database"
	"gorm.io/gorm"
)

const (
	pendingRestoreName         = ".echo-noise-restore-pending.zip"
	previousPendingRestoreName = ".echo-noise-restore-previous.zip"
	incompleteMigrationWarning = "该备份不含附件，不能用于完整迁移"
)

var (
	archiveMu sync.Mutex
	restoreMu sync.Mutex
)

// ErrRestartRequired tells callers that a verified restore is safely staged
// but cannot be applied until the process is restarted.
var ErrRestartRequired = errors.New("备份已暂存，需重启服务后应用恢复")

// RestartRequiredError carries non-fatal compatibility information to the UI
// while preserving errors.Is(err, ErrRestartRequired).
type RestartRequiredError struct{ Warning string }

func (e *RestartRequiredError) Error() string { return ErrRestartRequired.Error() }
func (e *RestartRequiredError) Unwrap() error { return ErrRestartRequired }

func RestartWarning(err error) string {
	var restartErr *RestartRequiredError
	if errors.As(err, &restartErr) {
		return restartErr.Warning
	}
	return ""
}

// StageResult describes whether a staged archive can perform a complete data
// migration. Old database-only archives remain usable but require a warning.
type StageResult struct{ Warning string }

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
	archiveMu.Lock()
	defer archiveMu.Unlock()
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
			// An absent configured root means the installation currently has no
			// files there. Keep an empty root entry so a newly-created archive is
			// distinguishable from an older database-only archive.
			if err := addDirectoryHeader(archive, root.ArchiveName); err != nil {
				return err
			}
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
	if err := addDirectoryHeader(archive, archiveRoot); err != nil {
		return err
	}
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

func addDirectoryHeader(archive *zip.Writer, archiveRoot string) error {
	_, err := archive.Create(strings.TrimRight(filepath.ToSlash(archiveRoot), "/") + "/")
	return err
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

type ArchiveInspection struct {
	HasAttachmentData bool
}

// InspectArchive checks the portable archive contract before it can be staged.
func InspectArchive(path string) error {
	_, err := inspectArchive(path)
	return err
}

func inspectArchive(path string) (ArchiveInspection, error) {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return ArchiveInspection{}, fmt.Errorf("打开备份文件失败: %w", err)
	}
	defer reader.Close()
	inspection := ArchiveInspection{}
	foundDatabase := false
	seen := map[string]struct{}{}
	for _, file := range reader.File {
		name, err := safeArchiveName(file.Name)
		if err != nil {
			return ArchiveInspection{}, err
		}
		if _, duplicate := seen[name]; duplicate && !file.FileInfo().IsDir() {
			return ArchiveInspection{}, fmt.Errorf("备份包含重复文件: %s", name)
		}
		seen[name] = struct{}{}
		if name == "database.db" && !file.FileInfo().IsDir() {
			foundDatabase = true
		}
		if name == "attachment-blobs" || strings.HasPrefix(name, "attachment-blobs/") || name == "attachments" || strings.HasPrefix(name, "attachments/") {
			inspection.HasAttachmentData = true
		}
	}
	if !foundDatabase {
		return ArchiveInspection{}, errors.New("备份不包含 database.db")
	}
	return inspection, nil
}

func safeArchiveName(name string) (string, error) {
	cleaned := filepath.ToSlash(filepath.Clean(name))
	if cleaned == "." || strings.HasPrefix(cleaned, "../") || strings.HasPrefix(cleaned, "/") || strings.Contains(cleaned, ":") {
		return "", fmt.Errorf("备份包含非法路径: %s", name)
	}
	return cleaned, nil
}

// StageRestore validates and publishes a ZIP beside the database atomically.
// The package is only applied before the next database initialization.
func StageRestore(source string, layout Layout) error {
	_, err := StageRestoreWithResult(source, layout)
	return err
}

func StageRestoreWithResult(source string, layout Layout) (StageResult, error) {
	restoreMu.Lock()
	defer restoreMu.Unlock()
	inspection, err := inspectArchive(source)
	if err != nil {
		return StageResult{}, err
	}
	if err := os.MkdirAll(filepath.Dir(layout.DatabasePath), 0755); err != nil {
		return StageResult{}, fmt.Errorf("创建恢复目录失败: %w", err)
	}
	stage, err := os.MkdirTemp(filepath.Dir(layout.DatabasePath), ".echo-noise-restore-check-*")
	if err != nil {
		return StageResult{}, err
	}
	defer os.RemoveAll(stage)
	if err := extractArchive(source, stage); err != nil {
		return StageResult{}, err
	}
	if err := validateSQLite(filepath.Join(stage, "database.db")); err != nil {
		return StageResult{}, fmt.Errorf("备份数据库无效: %w", err)
	}
	if err := publishPendingRestore(source, layout); err != nil {
		return StageResult{}, err
	}
	result := StageResult{}
	if !inspection.HasAttachmentData {
		result.Warning = incompleteMigrationWarning
	}
	return result, nil
}

func PendingRestorePath(layout Layout) string {
	return filepath.Join(filepath.Dir(layout.DatabasePath), pendingRestoreName)
}

func HasPendingRestore(layout Layout) bool {
	for _, path := range []string{PendingRestorePath(layout), previousPendingRestorePath(layout)} {
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}
	return false
}

func previousPendingRestorePath(layout Layout) string {
	return filepath.Join(filepath.Dir(layout.DatabasePath), previousPendingRestoreName)
}

func publishPendingRestore(source string, layout Layout) (err error) {
	return publishPendingRestoreWithCopy(source, layout, io.Copy)
}

func publishPendingRestoreWithCopy(
	source string,
	layout Layout,
	copyArchive func(io.Writer, io.Reader) (int64, error),
) (err error) {
	pending := PendingRestorePath(layout)
	previous := previousPendingRestorePath(layout)
	temp, err := os.CreateTemp(filepath.Dir(pending), ".echo-noise-restore-publish-*.tmp")
	if err != nil {
		return fmt.Errorf("创建待恢复临时文件失败: %w", err)
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	in, err := os.Open(source)
	if err != nil {
		temp.Close()
		return err
	}
	_, copyErr := copyArchive(temp, in)
	inCloseErr := in.Close()
	syncErr := temp.Sync()
	closeErr := temp.Close()
	for _, candidate := range []error{copyErr, inCloseErr, syncErr, closeErr} {
		if candidate != nil {
			return fmt.Errorf("写入待恢复临时文件失败: %w", candidate)
		}
	}
	if _, err := inspectArchive(tempPath); err != nil {
		return err
	}
	if err := os.Remove(previous); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	hadPending := false
	if _, err := os.Stat(pending); err == nil {
		if err := os.Rename(pending, previous); err != nil {
			return fmt.Errorf("保存原待恢复包失败: %w", err)
		}
		hadPending = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Rename(tempPath, pending); err != nil {
		if hadPending {
			if rollbackErr := os.Rename(previous, pending); rollbackErr != nil {
				return fmt.Errorf("发布待恢复包失败: %w", errors.Join(err, fmt.Errorf("恢复原待恢复包失败: %w", rollbackErr)))
			}
		}
		return fmt.Errorf("发布待恢复包失败: %w", err)
	}
	// Keep the previous package until Apply has extracted and validated the new
	// pending package. This is also the recovery state if publication stops
	// immediately after the atomic rename.
	return nil
}

// AppliedRestore retains a rollback path until database initialization has
// completed. Call Commit only after the new database is successfully ready.
type AppliedRestore struct {
	backups     []renamePair
	targets     []string
	directories []*directorySwap
	pending     string
}

type renamePair struct{ from, to string }

// directorySwap keeps a mounted directory itself in place while replacing its
// children. Renaming a mount point is not portable, but renaming children and
// the rollback directory inside that mount stays on the target filesystem.
type directorySwap struct {
	backup     string
	oldEntries []renamePair
	newEntries []string
	rename     func(string, string) error
}

type preparedReplacement struct {
	from    string
	to      string
	inPlace bool
	cleanup string
}

type directoryRollbackError struct{ cause error }

func (e *directoryRollbackError) Error() string { return e.cause.Error() }
func (e *directoryRollbackError) Unwrap() error { return e.cause }

// RestoreApplyError reports whether a failed pending restore was completely
// rolled back and quarantined, allowing the old instance to continue booting.
type RestoreApplyError struct {
	Cause       error
	Recovered   bool
	Quarantined string
}

func (e *RestoreApplyError) Error() string {
	if e.Quarantined != "" {
		return fmt.Sprintf("应用待恢复备份失败，已隔离到 %s: %v", e.Quarantined, e.Cause)
	}
	return fmt.Sprintf("应用待恢复备份失败: %v", e.Cause)
}

func (e *RestoreApplyError) Unwrap() error { return e.Cause }

func RestoreFailureRecovered(err error) bool {
	var applyErr *RestoreApplyError
	return errors.As(err, &applyErr) && applyErr.Recovered
}

// ApplyPendingRestore must run while the database is closed, before InitDB.
func ApplyPendingRestore(layout Layout) (*AppliedRestore, error) {
	restoreMu.Lock()
	defer restoreMu.Unlock()
	return applyPendingRestore(layout)
}

func applyPendingRestore(layout Layout) (*AppliedRestore, error) {
	pending := PendingRestorePath(layout)
	previous := previousPendingRestorePath(layout)
	if _, pendingErr := os.Stat(pending); errors.Is(pendingErr, os.ErrNotExist) {
		if _, previousErr := os.Stat(previous); previousErr == nil {
			if err := os.Rename(previous, pending); err != nil {
				return nil, &RestoreApplyError{Cause: fmt.Errorf("恢复中断的待恢复包失败: %w", err)}
			}
		} else if previousErr != nil && !errors.Is(previousErr, os.ErrNotExist) {
			return nil, &RestoreApplyError{Cause: previousErr}
		}
	} else if pendingErr != nil {
		return nil, &RestoreApplyError{Cause: pendingErr}
	}
	if _, err := os.Stat(pending); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, &RestoreApplyError{Cause: err}
	}
	stage, err := os.MkdirTemp(filepath.Dir(layout.DatabasePath), ".echo-noise-restore-apply-*")
	if err != nil {
		return nil, quarantineFailedRestore(pending, err, true)
	}
	defer os.RemoveAll(stage)
	if err := extractArchive(pending, stage); err != nil {
		return nil, quarantineFailedRestore(pending, err, true)
	}
	if err := validateSQLite(filepath.Join(stage, "database.db")); err != nil {
		return nil, quarantineFailedRestore(pending, fmt.Errorf("待恢复数据库无效: %w", err), true)
	}
	if err := os.Remove(previous); err != nil && !errors.Is(err, os.ErrNotExist) {
		// Both files mean publication stopped after the new package became
		// pending. Retain the predecessor until the new package is known-good so
		// a corrupted new file can still fall back to the last valid pending one.
		return nil, &RestoreApplyError{Cause: fmt.Errorf("清理已替换的待恢复包失败: %w", err)}
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
			return nil, quarantineFailedRestore(pending, statErr, true)
		}
	}
	prepared := make([]preparedReplacement, 0, len(pairs))
	for _, pair := range pairs {
		candidate, prepareErr := prepareReplacement(pair.from, pair.to)
		if prepareErr != nil {
			for _, item := range prepared {
				_ = os.RemoveAll(item.cleanup)
			}
			return nil, quarantineFailedRestore(pending, prepareErr, true)
		}
		prepared = append(prepared, candidate)
	}
	defer func() {
		for _, item := range prepared {
			_ = os.RemoveAll(item.cleanup)
		}
	}()

	applied := &AppliedRestore{pending: pending}
	// The archive contains a self-contained SQLite snapshot, not the live
	// database's WAL sidecars. Keeping an old WAL beside the restored main file
	// would let SQLite replay pre-restore changes after the next startup.
	for _, sidecar := range []string{layout.DatabasePath + "-wal", layout.DatabasePath + "-shm"} {
		// InitDB may create fresh sidecars before a later failure. Rollback must
		// remove those newly generated files before the original sidecars return.
		applied.targets = append(applied.targets, sidecar)
		if err := moveExistingToRestoreBackup(sidecar, applied); err != nil {
			return nil, rollbackAndQuarantine(applied, err)
		}
	}
	for _, pair := range prepared {
		if pair.inPlace {
			swap, err := replaceDirectoryContents(pair.from, pair.to)
			if err != nil {
				return nil, rollbackAndQuarantine(applied, err)
			}
			applied.directories = append(applied.directories, swap)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(pair.to), 0755); err != nil {
			return nil, rollbackAndQuarantine(applied, err)
		}
		if err := moveExistingToRestoreBackup(pair.to, applied); err != nil {
			return nil, rollbackAndQuarantine(applied, err)
		}
		if err := os.Rename(pair.from, pair.to); err != nil {
			return nil, rollbackAndQuarantine(applied, err)
		}
		applied.targets = append(applied.targets, pair.to)
	}
	return applied, nil
}

func prepareReplacement(source, target string) (preparedReplacement, error) {
	sourceInfo, err := os.Stat(source)
	if err != nil {
		return preparedReplacement{}, err
	}
	targetInfo, targetErr := os.Stat(target)
	if targetErr == nil {
		if sourceInfo.IsDir() != targetInfo.IsDir() {
			return preparedReplacement{}, fmt.Errorf("恢复源与目标类型不一致: %s", target)
		}
		if sourceInfo.IsDir() {
			candidate, err := os.MkdirTemp(target, ".echo-noise-restore-candidate-*")
			if err != nil {
				return preparedReplacement{}, fmt.Errorf("在目标目录创建恢复临时目录失败 %s: %w", target, err)
			}
			if err := copyDirectoryContents(source, candidate); err != nil {
				_ = os.RemoveAll(candidate)
				return preparedReplacement{}, fmt.Errorf("准备恢复目标失败 %s: %w", target, err)
			}
			return preparedReplacement{from: candidate, to: target, inPlace: true, cleanup: candidate}, nil
		}
	} else if !errors.Is(targetErr, os.ErrNotExist) {
		return preparedReplacement{}, targetErr
	}
	parent := filepath.Dir(target)
	if err := os.MkdirAll(parent, 0755); err != nil {
		return preparedReplacement{}, err
	}
	stage, err := os.MkdirTemp(parent, ".echo-noise-restore-target-*")
	if err != nil {
		return preparedReplacement{}, fmt.Errorf("在目标文件系统创建恢复临时目录失败 %s: %w", parent, err)
	}
	candidate := filepath.Join(stage, filepath.Base(target))
	if err := copyEntry(source, candidate); err != nil {
		_ = os.RemoveAll(stage)
		return preparedReplacement{}, fmt.Errorf("准备恢复目标失败 %s: %w", target, err)
	}
	return preparedReplacement{from: candidate, to: target, cleanup: stage}, nil
}

func replaceDirectoryContents(candidate, target string) (*directorySwap, error) {
	return replaceDirectoryContentsWithRename(candidate, target, os.Rename)
}

func replaceDirectoryContentsWithRename(candidate, target string, rename func(string, string) error) (*directorySwap, error) {
	backup, err := os.MkdirTemp(target, ".echo-noise-restore-backup-*")
	if err != nil {
		return nil, fmt.Errorf("在目标目录创建回退目录失败 %s: %w", target, err)
	}
	swap := &directorySwap{backup: backup, rename: rename}
	rollbackFailure := func(cause error) (*directorySwap, error) {
		if rollbackErr := swap.rollback(); rollbackErr != nil {
			return nil, &directoryRollbackError{cause: errors.Join(cause, fmt.Errorf("目录回退失败: %w", rollbackErr))}
		}
		return nil, cause
	}

	entries, err := os.ReadDir(target)
	if err != nil {
		return rollbackFailure(err)
	}
	candidateName := filepath.Base(candidate)
	backupName := filepath.Base(backup)
	for _, entry := range entries {
		if entry.Name() == candidateName || entry.Name() == backupName {
			continue
		}
		from := filepath.Join(target, entry.Name())
		to := filepath.Join(backup, entry.Name())
		if err := rename(from, to); err != nil {
			return rollbackFailure(err)
		}
		swap.oldEntries = append(swap.oldEntries, renamePair{from: to, to: from})
	}

	entries, err = os.ReadDir(candidate)
	if err != nil {
		return rollbackFailure(err)
	}
	for _, entry := range entries {
		from := filepath.Join(candidate, entry.Name())
		to := filepath.Join(target, entry.Name())
		if err := rename(from, to); err != nil {
			return rollbackFailure(err)
		}
		swap.newEntries = append(swap.newEntries, to)
	}
	if err := os.Remove(candidate); err != nil {
		return rollbackFailure(err)
	}
	return swap, nil
}

func (s *directorySwap) rollback() error {
	if s == nil {
		return nil
	}
	var rollbackErr error
	rename := s.rename
	if rename == nil {
		rename = os.Rename
	}
	for index := len(s.newEntries) - 1; index >= 0; index-- {
		if err := os.RemoveAll(s.newEntries[index]); err != nil {
			rollbackErr = errors.Join(rollbackErr, err)
		}
	}
	for index := len(s.oldEntries) - 1; index >= 0; index-- {
		entry := s.oldEntries[index]
		if err := rename(entry.from, entry.to); err != nil {
			rollbackErr = errors.Join(rollbackErr, err)
		}
	}
	// Keep the rollback directory intact if any entry could not be restored;
	// deleting it here would turn a recoverable operator intervention into data
	// loss. It is removed only after every rollback step succeeded.
	if rollbackErr == nil {
		if err := os.RemoveAll(s.backup); err != nil {
			rollbackErr = err
		}
	}
	return rollbackErr
}

func rollbackAndQuarantine(applied *AppliedRestore, cause error) error {
	var unsafeDirectoryRollback *directoryRollbackError
	directoryRollbackFailed := errors.As(cause, &unsafeDirectoryRollback)
	if rollbackErr := applied.Rollback(); rollbackErr != nil {
		return &RestoreApplyError{Cause: errors.Join(cause, fmt.Errorf("回退失败: %w", rollbackErr))}
	}
	if directoryRollbackFailed {
		return &RestoreApplyError{Cause: cause}
	}
	return quarantineFailedRestore(applied.pending, cause, true)
}

func quarantineFailedRestore(pending string, cause error, recovered bool) error {
	quarantined := filepath.Join(filepath.Dir(pending), fmt.Sprintf(".echo-noise-restore-failed-%d.zip", time.Now().UnixNano()))
	if err := os.Rename(pending, quarantined); err != nil {
		return &RestoreApplyError{Cause: errors.Join(cause, fmt.Errorf("隔离失败包失败: %w", err))}
	}
	return &RestoreApplyError{Cause: cause, Recovered: recovered, Quarantined: quarantined}
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
	for _, directory := range a.directories {
		if err := os.RemoveAll(directory.backup); err != nil {
			return err
		}
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
	for index := len(a.directories) - 1; index >= 0; index-- {
		if err := a.directories[index].rollback(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
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
	requiredSchema := map[string][]string{
		"users":        {"id", "username", "password", "is_admin"},
		"messages":     {"id", "content", "user_id"},
		"site_configs": {"id"},
	}
	for table, requiredColumns := range requiredSchema {
		var found string
		err := db.QueryRow("SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?", table).Scan(&found)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("备份缺少必要业务表: %s", table)
		}
		if err != nil {
			return err
		}
		rows, err := db.Query("SELECT name FROM pragma_table_info(?)", table)
		if err != nil {
			return err
		}
		columns := map[string]struct{}{}
		for rows.Next() {
			var column string
			if err := rows.Scan(&column); err != nil {
				rows.Close()
				return err
			}
			columns[column] = struct{}{}
		}
		if err := rows.Close(); err != nil {
			return err
		}
		if err := rows.Err(); err != nil {
			return err
		}
		for _, column := range requiredColumns {
			if _, exists := columns[column]; !exists {
				return fmt.Errorf("备份业务表 %s 缺少必要字段: %s", table, column)
			}
		}
	}
	return nil
}

func copyEntry(source, destination string) error {
	info, err := os.Stat(source)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return copyPath(source, destination, info.Mode())
	}
	if err := os.MkdirAll(destination, info.Mode().Perm()); err != nil {
		return err
	}
	return filepath.Walk(source, func(path string, entry os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return nil
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, entry.Mode().Perm())
		}
		return copyPath(path, target, entry.Mode())
	})
}

func copyDirectoryContents(source, destination string) error {
	entries, err := os.ReadDir(source)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := copyEntry(filepath.Join(source, entry.Name()), filepath.Join(destination, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func copyPath(source, destination string, mode os.FileMode) (err error) {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
		return err
	}
	out, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode.Perm())
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := out.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
	}()
	if _, err = io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}
