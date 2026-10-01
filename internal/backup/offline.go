package backup

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// OpenOffline never creates a missing database, changes journal mode, migrates,
// seeds data or applies a pending restore. mode=ro includes committed WAL data;
// immutable=1 would incorrectly ignore that WAL on the source database.
func OpenOffline(path string, writable bool) (*gorm.DB, error) {
	mode := "ro"
	if writable {
		mode = "rw"
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	dsn := "file:" + strings.ReplaceAll(url.PathEscape(filepath.ToSlash(absolute)), "%2F", "/") + "?mode=" + mode
	// A clean shutdown removes WAL/SHM, while the main file still records WAL
	// mode. SQLite otherwise tries creating sidecars on the read-only mount.
	// Only a source with no WAL may use immutable; existing WAL is always read.
	if !writable {
		if _, err := os.Stat(absolute + "-wal"); errors.Is(err, os.ErrNotExist) {
			dsn += "&immutable=1"
		} else if err != nil {
			return nil, err
		}
	}
	return gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
}

type OfflinePlan struct {
	Version  int    `json:"version"`
	Database string `json:"database"`
	Roots    []Root `json:"roots"`
	Bytes    int64  `json:"bytes"`
}

// PlanOffline validates every source, including external local blobs. Remote
// attachments have no equivalent offline archive in this first SQLite release.
func PlanOffline(db *gorm.DB, layout Layout, configDir string) (OfflinePlan, error) {
	plan := OfflinePlan{Version: 1}
	if HasPendingRestore(layout) {
		return plan, errors.New("pending_restore_requires_reconciliation")
	}
	if db.Migrator().HasTable("attachment_blobs") {
		var count int64
		if err := db.Table("attachment_blobs").Where("storage_backend <> ?", "local").Count(&count).Error; err != nil {
			return plan, err
		}
		if count != 0 {
			return plan, errors.New("remote_attachment_backup_unsupported")
		}
	}
	if db.Migrator().HasColumn("site_configs", "attachment_storage_enabled") {
		var count int64
		if err := db.Table("site_configs").Where("attachment_storage_enabled = ?", true).Count(&count).Error; err != nil {
			return plan, err
		}
		if count != 0 {
			return plan, errors.New("remote_attachment_backup_unsupported")
		}
	}
	var err error
	plan.Database, err = filepath.Abs(layout.DatabasePath)
	if err != nil {
		return plan, err
	}
	if _, err := os.Stat(plan.Database); err != nil {
		return plan, err
	}
	roots := append(append([]Root{}, layout.Roots...), Root{ArchiveName: "protected-config", Path: configDir})
	paths := []string{plan.Database, plan.Database + "-wal"}
	for _, root := range roots {
		root.Path, err = filepath.Abs(root.Path)
		if err != nil {
			return plan, err
		}
		plan.Roots = append(plan.Roots, root)
		paths = append(paths, root.Path)
	}
	for _, path := range paths {
		err = filepath.Walk(path, func(p string, info os.FileInfo, walkErr error) error {
			if errors.Is(walkErr, os.ErrNotExist) && p == path {
				return nil
			}
			if walkErr != nil {
				return walkErr
			}
			if info.Mode()&os.ModeSymlink != 0 || (!info.IsDir() && !info.Mode().IsRegular()) {
				return fmt.Errorf("unsupported_backup_source: %s", p)
			}
			if !info.IsDir() {
				plan.Bytes += info.Size()
			}
			return nil
		})
		if err != nil {
			return plan, err
		}
	}
	return plan, nil
}

func CreateOfflineArchive(destination string, db *gorm.DB, layout Layout, configDir string) error {
	plan, err := PlanOffline(db, layout, configDir)
	if err != nil {
		return err
	}
	absolute, err := filepath.Abs(destination)
	if err != nil {
		return err
	}
	for _, root := range plan.Roots {
		rel, err := filepath.Rel(root.Path, absolute)
		if err != nil {
			return err
		}
		if rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
			return errors.New("backup_destination_inside_source")
		}
	}
	// Config secrets are contained in the same protected archive, never emitted
	// on stdout. Existing restore applies only the documented media/database roots.
	layout.Roots = plan.Roots
	if err := CreateArchive(destination, db, layout); err != nil {
		return err
	}
	if err := os.Chmod(destination, 0600); err != nil {
		return err
	}
	file, err := os.OpenFile(destination, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer file.Close()
	return file.Sync()
}
