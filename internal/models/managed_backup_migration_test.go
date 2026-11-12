package models

import (
	"reflect"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestMigrateDBManagedBackupsRevokesUnsafeGrantsAndPreservesHistoryIdempotently(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	connection, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	connection.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = connection.Close() })
	if err := db.AutoMigrate(&AdminCapabilityGrant{}, &AdminAuditLog{}); err != nil {
		t.Fatal(err)
	}
	retiredOrOwnerOnly := []string{
		"database.view", "database.backup", "database.restore",
		"backup.data.download", "backup.data.restore",
		"backup.attachments.download", "backup.attachments.restore",
		"backup.server_config.view", "backup.server_config.history", "backup.server_config.create",
		"backup.server_config.plan", "backup.server_config.download", "backup.server_config.restore",
	}
	retained := []string{
		"storage.view", "storage.manage", "users.view",
		"backup.data.view", "backup.data.history", "backup.data.create", "backup.data.plan",
		"backup.attachments.view", "backup.attachments.history", "backup.attachments.create", "backup.attachments.plan",
	}
	for _, userID := range []uint{PrimaryAdminUserID, 7, 8} {
		for _, capability := range retiredOrOwnerOnly {
			if err := db.Create(&AdminCapabilityGrant{UserID: userID, Capability: capability, GrantedByUserID: PrimaryAdminUserID}).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, capability := range retained {
		if err := db.Create(&AdminCapabilityGrant{UserID: 7, Capability: capability, GrantedByUserID: PrimaryAdminUserID}).Error; err != nil {
			t.Fatal(err)
		}
	}
	// User 8 has only legacy/illegal grants: migration must not create replacement
	// creation, viewing, download or restore rights for this administrator.
	for _, capability := range retiredOrOwnerOnly {
		if err := db.Create(&AdminAuditLog{
			ActorUserID: 7, ActorUsername: "delegated", Capability: capability,
			Module: "database", Action: "legacy-backup", Result: "success",
			Summary: "historical operation", ChangesJSON: `{"preserve":"original"}`,
		}).Error; err != nil {
			t.Fatal(err)
		}
	}
	var originalGrants []AdminCapabilityGrant
	if err := db.Where("user_id = ? AND capability IN ?", 7, retained).Order("id").Find(&originalGrants).Error; err != nil {
		t.Fatal(err)
	}
	var originalAudit []AdminAuditLog
	if err := db.Order("id").Find(&originalAudit).Error; err != nil {
		t.Fatal(err)
	}
	var originalPlans []ManagedBackupPlan
	var originalRecords []ManagedBackupRecord
	for pass := range 2 {
		if err := MigrateDB(db); err != nil {
			t.Fatalf("migration pass %d: %v", pass, err)
		}
		for _, model := range []interface{}{&ManagedBackupPlan{}, &ManagedBackupRecord{}} {
			if !db.Migrator().HasTable(model) {
				t.Fatalf("migration did not create %T", model)
			}
		}
		if db.Migrator().HasColumn(&ManagedBackupPlan{}, "NextRun") {
			t.Fatal("computed next run must not be persisted")
		}
		var grants []AdminCapabilityGrant
		if err := db.Order("id").Find(&grants).Error; err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(grants, originalGrants) {
			t.Fatalf("migration changed legitimate grants or retained unsafe grants: got %#v, want %#v", grants, originalGrants)
		}
		var audit []AdminAuditLog
		if err := db.Order("id").Find(&audit).Error; err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(audit, originalAudit) {
			t.Fatalf("migration rewrote or removed audit history: got %#v, want %#v", audit, originalAudit)
		}
		if pass == 0 {
			now := time.Date(2026, 11, 12, 1, 2, 3, 0, time.UTC)
			for _, kind := range []string{"data", "attachments", "server_config"} {
				plan := ManagedBackupPlan{
					Kind: kind, Enabled: true, Time: "03:45", Timezone: "Asia/Shanghai",
					KeepLocal: 3, KeepCloud: 5, Target: "both", Scope: "all", LastRun: &now, Revision: 9,
				}
				if err := db.Create(&plan).Error; err != nil {
					t.Fatal(err)
				}
				// A record whose archives were removed is still durable history.
				record := ManagedBackupRecord{
					ID: "history-" + kind, Kind: kind, CreatedAt: now, FinishedAt: &now,
					Trigger: "scheduled", Target: "both", Scope: "all", Status: "failed",
					Size: 1234, Local: false, Cloud: false, Error: "archive expired", CloudKey: "managed/" + kind,
				}
				if err := db.Create(&record).Error; err != nil {
					t.Fatal(err)
				}
			}
			if err := db.Order("kind").Find(&originalPlans).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Order("id").Find(&originalRecords).Error; err != nil {
				t.Fatal(err)
			}
		} else {
			var plans []ManagedBackupPlan
			var records []ManagedBackupRecord
			if err := db.Order("kind").Find(&plans).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Order("id").Find(&records).Error; err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(plans, originalPlans) || !reflect.DeepEqual(records, originalRecords) {
				t.Fatalf("repeat migration changed schedules, slot claims, revisions or backup history: plans=%#v records=%#v", plans, records)
			}
		}
	}
}
