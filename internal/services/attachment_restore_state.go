package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/rcy1314/echo-noise/internal/backup"
	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// This table is deliberately part of data snapshots. It records attachment-only
// mappings, never credentials or site settings. Ordered IDs preserve later choices.
type attachmentRestoreMigration struct {
	ID       uint   `gorm:"primaryKey"`
	Digest   string `gorm:"uniqueIndex;not null"`
	Mode     string `gorm:"not null"`
	Document string `gorm:"not null"`
}

func rememberAttachmentRestore(tx *gorm.DB, doc attachmentArchiveDocument, mode string) error {
	if err := tx.AutoMigrate(&attachmentRestoreMigration{}); err != nil {
		return err
	}
	data, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	hash := sha256.Sum256(append([]byte(mode+"\n"), data...))
	row := attachmentRestoreMigration{Digest: hex.EncodeToString(hash[:]), Mode: mode, Document: string(data)}
	// Repeating a restore is the latest explicit choice, so move its marker last.
	if err := tx.Where("digest = ?", row.Digest).Delete(&attachmentRestoreMigration{}).Error; err != nil {
		return err
	}
	return tx.Create(&row).Error
}

// PrepareDataAttachmentRestore runs before InitDB. It preserves the target's
// cloud connections and reapplies durable attachment mappings to staged data,
// even when an older data archive follows later.
func PrepareDataAttachmentRestore(ctx context.Context, stage, currentDBPath string) error {
	if _, err := os.Stat(currentDBPath); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	current, err := backup.OpenOffline(currentDBPath, false)
	if err != nil {
		return err
	}
	currentSQL, err := current.DB()
	if err != nil {
		return err
	}
	defer currentSQL.Close()
	var rows []attachmentRestoreMigration
	if current.Migrator().HasTable(&attachmentRestoreMigration{}) {
		if err := current.WithContext(ctx).Order("id").Find(&rows).Error; err != nil {
			return err
		}
	}
	candidate, err := backup.OpenOffline(filepath.Join(stage, "database.db"), true)
	if err != nil {
		return err
	}
	candidateSQL, err := candidate.DB()
	if err != nil {
		return err
	}
	defer candidateSQL.Close()
	if err := candidate.Exec("PRAGMA journal_mode=DELETE").Error; err != nil {
		return err
	}
	if len(rows) > 0 {
		if err := candidate.WithContext(ctx).AutoMigrate(&models.AttachmentBlob{}, &models.AttachmentReference{}, &models.CloudAttachmentObject{}, &models.LocalAttachmentGrant{}, &attachmentRestoreMigration{}); err != nil {
			return err
		}
	}
	return candidate.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := preserveDataRestoreCloudConnections(tx, current.WithContext(ctx)); err != nil {
			return err
		}
		for _, row := range rows {
			if err := validateAttachmentMode(row.Mode); err != nil {
				return err
			}
			var doc attachmentArchiveDocument
			if err := json.Unmarshal([]byte(row.Document), &doc); err != nil {
				return err
			}
			if err := validateAttachmentRegistry(doc); err != nil {
				return err
			}
			if err := mergeAttachmentDocument(tx, doc, row.Mode); err != nil {
				return err
			}
			row.ID = 0
			if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "digest"}}, DoUpdates: clause.AssignmentColumns([]string{"mode", "document"})}).Create(&row).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// Only infrastructure connection fields belong to the target. Business settings
// and users remain those of the staged data archive, including zero values.
func preserveDataRestoreCloudConnections(tx, current *gorm.DB) error {
	if !current.Migrator().HasTable(&models.SiteConfig{}) || !tx.Migrator().HasTable(&models.SiteConfig{}) {
		return nil
	}
	var site models.SiteConfig
	if err := current.First(&site).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	} else if err != nil {
		return err
	}
	fields := map[string]interface{}{
		"storage_enabled":                    site.StorageEnabled,
		"storage_provider":                   site.StorageProvider,
		"storage_endpoint":                   site.StorageEndpoint,
		"storage_region":                     site.StorageRegion,
		"storage_bucket":                     site.StorageBucket,
		"storage_access_key":                 site.StorageAccessKey,
		"storage_secret_key":                 site.StorageSecretKey,
		"storage_use_path_style":             site.StorageUsePathStyle,
		"storage_public_base_url":            site.StoragePublicBaseURL,
		"attachment_storage_enabled":         site.AttachmentStorageEnabled,
		"attachment_storage_provider":        site.AttachmentStorageProvider,
		"attachment_storage_endpoint":        site.AttachmentStorageEndpoint,
		"attachment_storage_region":          site.AttachmentStorageRegion,
		"attachment_storage_bucket":          site.AttachmentStorageBucket,
		"attachment_storage_access_key":      site.AttachmentStorageAccessKey,
		"attachment_storage_secret_key":      site.AttachmentStorageSecretKey,
		"attachment_storage_use_path_style":  site.AttachmentStorageUsePathStyle,
		"attachment_storage_public_base_url": site.AttachmentStoragePublicBaseURL,
	}
	// Older archives may predate either storage channel. Do not migrate unrelated
	// site settings or manufacture connection values for columns absent on target.
	for column := range fields {
		if !current.Migrator().HasColumn(&models.SiteConfig{}, column) || !tx.Migrator().HasColumn(&models.SiteConfig{}, column) {
			delete(fields, column)
		}
	}
	if len(fields) == 0 {
		return nil
	}
	return tx.Model(&models.SiteConfig{}).Where("1 = 1").UpdateColumns(fields).Error
}
