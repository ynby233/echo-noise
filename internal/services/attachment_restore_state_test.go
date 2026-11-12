package services

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/rcy1314/echo-noise/internal/models"
)

func dataRestoreCloudTestConfig(name string) models.SiteConfig {
	return models.SiteConfig{
		SiteTitle:                      name + " title",
		Description:                    name + " description",
		StorageEnabled:                 true,
		StorageProvider:                name + " provider",
		StorageEndpoint:                "https://" + name + ".example/backup",
		StorageRegion:                  name + " region",
		StorageBucket:                  name + " bucket",
		StorageAccessKey:               name + " access",
		StorageSecretKey:               name + " secret",
		StorageUsePathStyle:            true,
		StoragePublicBaseURL:           "https://" + name + ".example/public-backup",
		AttachmentStorageEnabled:       true,
		AttachmentStorageProvider:      name + " attachment provider",
		AttachmentStorageEndpoint:      "https://" + name + ".example/attachments",
		AttachmentStorageRegion:        name + " attachment region",
		AttachmentStorageBucket:        name + " attachment bucket",
		AttachmentStorageAccessKey:     name + " attachment access",
		AttachmentStorageSecretKey:     name + " attachment secret",
		AttachmentStorageUsePathStyle:  true,
		AttachmentStoragePublicBaseURL: "https://" + name + ".example/public-attachments",
	}
}

func TestPrepareDataAttachmentRestorePreservesTargetCloudConnections(t *testing.T) {
	for _, migrationTable := range []bool{false, true} {
		name := "no_migration_table"
		if migrationTable {
			name = "empty_migration_table"
		}
		t.Run(name, func(t *testing.T) {
			stage := t.TempDir()
			currentPath := filepath.Join(t.TempDir(), "current.db")
			current := attachmentArchiveTestDB(t, currentPath)
			candidate := attachmentArchiveTestDB(t, filepath.Join(stage, "database.db"))
			if migrationTable {
				if err := current.AutoMigrate(&attachmentRestoreMigration{}); err != nil {
					t.Fatal(err)
				}
			}
			target := dataRestoreCloudTestConfig("target")
			source := dataRestoreCloudTestConfig("source")
			source.EnableCompression = true
			if err := current.Create(&target).Error; err != nil {
				t.Fatal(err)
			}
			// Explicitly exercise false and empty target values: GORM struct updates
			// and default:true fields must not silently keep source values.
			if err := current.Model(&target).UpdateColumns(map[string]interface{}{
				"storage_enabled": false, "storage_use_path_style": false,
				"attachment_storage_enabled": false, "attachment_storage_use_path_style": false,
				"storage_public_base_url": "", "attachment_storage_public_base_url": "",
			}).Error; err != nil {
				t.Fatal(err)
			}
			if err := current.First(&target).Error; err != nil {
				t.Fatal(err)
			}
			if err := candidate.Create(&source).Error; err != nil {
				t.Fatal(err)
			}
			if err := candidate.First(&source).Error; err != nil {
				t.Fatal(err)
			}
			user := models.User{ID: 2, Username: "source-member", Password: "source-hash", IsAdmin: false}
			if err := candidate.Create(&user).Error; err != nil {
				t.Fatal(err)
			}
			if err := current.Create(&models.User{ID: 2, Username: "target-admin", Password: "target-hash", IsAdmin: true}).Error; err != nil {
				t.Fatal(err)
			}
			if err := candidate.AutoMigrate(&models.AdminCapabilityGrant{}); err != nil {
				t.Fatal(err)
			}
			grant := models.AdminCapabilityGrant{UserID: user.ID, Capability: "attachments.view", GrantedByUserID: 1}
			if err := candidate.Create(&grant).Error; err != nil {
				t.Fatal(err)
			}
			if err := candidate.First(&grant).Error; err != nil {
				t.Fatal(err)
			}
			if err := PrepareDataAttachmentRestore(context.Background(), stage, currentPath); err != nil {
				t.Fatal(err)
			}
			var got models.SiteConfig
			if err := candidate.First(&got).Error; err != nil {
				t.Fatal(err)
			}
			want := target
			want.Model = source.Model
			want.SiteTitle = source.SiteTitle
			want.Description = source.Description
			want.EnableCompression = source.EnableCompression
			// Compare all fields, including timestamps, to catch accidental writes
			// to unrelated site settings without printing credentials on failure.
			if !reflect.DeepEqual(got, want) {
				t.Fatal("restored site must retain target cloud connections and source business settings")
			}
			var gotUser models.User
			if err := candidate.First(&gotUser, user.ID).Error; err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(gotUser, user) {
				t.Fatal("data restore preparation changed user or admin status")
			}
			var grants []models.AdminCapabilityGrant
			if err := candidate.Order("id").Find(&grants).Error; err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(grants, []models.AdminCapabilityGrant{grant}) {
				t.Fatal("data restore preparation changed delegated permissions")
			}
			var unchanged models.SiteConfig
			if err := current.First(&unchanged).Error; err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(unchanged, target) {
				t.Fatal("preparation changed target database")
			}
		})
	}
}

func TestPrepareDataAttachmentRestoreGuardsMissingSiteConfig(t *testing.T) {
	for _, missing := range []string{"target_table", "candidate_table", "target_row"} {
		t.Run(missing, func(t *testing.T) {
			stage := t.TempDir()
			currentPath := filepath.Join(t.TempDir(), "current.db")
			current := attachmentArchiveTestDB(t, currentPath)
			candidate := attachmentArchiveTestDB(t, filepath.Join(stage, "database.db"))
			if missing == "target_table" {
				if err := current.Migrator().DropTable(&models.SiteConfig{}); err != nil {
					t.Fatal(err)
				}
			} else if missing != "target_row" {
				site := dataRestoreCloudTestConfig("target")
				if err := current.Create(&site).Error; err != nil {
					t.Fatal(err)
				}
			}
			if missing == "candidate_table" {
				if err := candidate.Migrator().DropTable(&models.SiteConfig{}); err != nil {
					t.Fatal(err)
				}
			} else {
				site := dataRestoreCloudTestConfig("source")
				if err := candidate.Create(&site).Error; err != nil {
					t.Fatal(err)
				}
			}
			if err := PrepareDataAttachmentRestore(context.Background(), stage, currentPath); err != nil {
				t.Fatal(err)
			}
			if missing == "candidate_table" {
				if candidate.Migrator().HasTable(&models.SiteConfig{}) {
					t.Fatal("preparation manufactured site config table")
				}
			} else {
				var site models.SiteConfig
				if err := candidate.First(&site).Error; err != nil {
					t.Fatal(err)
				}
				if site.StorageBucket != "source bucket" || site.SiteTitle != "source title" {
					t.Fatal("missing target configuration changed staged site")
				}
			}
		})
	}
}
