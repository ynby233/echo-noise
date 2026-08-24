package models

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestMigrateRuntimePolicyDataMapsLegacyModeOnceAndBackfillsAccountStates(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.AutoMigrate(&SiteConfig{}, &User{}); err != nil {
		t.Fatalf("create legacy-shaped tables: %v", err)
	}
	config := SiteConfig{
		RuntimeMode:                 "local",
		RuntimeModeMigrationVersion: 0,
		VoceChatEnabled:             true,
	}
	if err := db.Create(&config).Error; err != nil {
		t.Fatalf("create legacy config: %v", err)
	}
	users := []User{
		{ID: PrimaryAdminUserID, Username: "primary", Password: HashPassword("primary"), IsAdmin: true},
		{ID: 2, Username: "unbound", Password: HashPassword("unbound"), VoceChatSyncStatus: VoceChatSyncStatusNone},
		{ID: 3, Username: "linked", Password: HashPassword("linked"), VoceChatEmail: "linked@vc.example", VoceChatUserID: "33", VoceChatSyncStatus: VoceChatSyncStatusNone},
	}
	if err := db.Create(&users).Error; err != nil {
		t.Fatalf("create users: %v", err)
	}

	if err := migrateRuntimePolicyData(db); err != nil {
		t.Fatalf("migrate runtime policy: %v", err)
	}
	var migrated SiteConfig
	if err := db.First(&migrated, config.ID).Error; err != nil {
		t.Fatalf("reload config: %v", err)
	}
	if migrated.RuntimeMode != "vocechat" || migrated.RuntimeModeMigrationVersion != 1 {
		t.Fatalf("migrated mode = %q version=%d, want vocechat/1", migrated.RuntimeMode, migrated.RuntimeModeMigrationVersion)
	}
	var unbound User
	if err := db.First(&unbound, 2).Error; err != nil {
		t.Fatalf("reload unbound user: %v", err)
	}
	if unbound.VoceChatSyncStatus != VoceChatSyncStatusPending {
		t.Fatalf("unbound status = %q, want %q", unbound.VoceChatSyncStatus, VoceChatSyncStatusPending)
	}
	var linked User
	if err := db.First(&linked, 3).Error; err != nil {
		t.Fatalf("reload linked user: %v", err)
	}
	if linked.VoceChatSyncStatus != VoceChatSyncStatusLinked {
		t.Fatalf("linked status = %q, want %q", linked.VoceChatSyncStatus, VoceChatSyncStatusLinked)
	}

	if err := db.Model(&SiteConfig{}).Where("id = ?", config.ID).Update("voce_chat_enabled", false).Error; err != nil {
		t.Fatalf("change retired legacy flag: %v", err)
	}
	if err := migrateRuntimePolicyData(db); err != nil {
		t.Fatalf("repeat runtime policy migration: %v", err)
	}
	if err := db.First(&migrated, config.ID).Error; err != nil {
		t.Fatalf("reload repeated config: %v", err)
	}
	if migrated.RuntimeMode != "vocechat" || migrated.RuntimeModeMigrationVersion != 1 {
		t.Fatalf("repeated migration changed authoritative mode: %q version=%d", migrated.RuntimeMode, migrated.RuntimeModeMigrationVersion)
	}
}

func TestMigrateRuntimePolicyDataKeepsLegacyLocalUsersUnbound(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.AutoMigrate(&SiteConfig{}, &User{}); err != nil {
		t.Fatalf("create legacy-shaped tables: %v", err)
	}
	if err := db.Create(&SiteConfig{RuntimeModeMigrationVersion: 0, VoceChatEnabled: false}).Error; err != nil {
		t.Fatalf("create local legacy config: %v", err)
	}
	user := User{ID: 2, Username: "local-user", Password: HashPassword("local"), VoceChatSyncStatus: VoceChatSyncStatusNone}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create local user: %v", err)
	}

	if err := migrateRuntimePolicyData(db); err != nil {
		t.Fatalf("migrate local runtime policy: %v", err)
	}
	if err := db.First(&user, 2).Error; err != nil {
		t.Fatalf("reload local user: %v", err)
	}
	if user.VoceChatSyncStatus != VoceChatSyncStatusUnbound {
		t.Fatalf("local user status = %q, want %q", user.VoceChatSyncStatus, VoceChatSyncStatusUnbound)
	}
}
