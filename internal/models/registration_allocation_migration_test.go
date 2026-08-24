package models

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestMigrateRegistrationApplicationAllocationDataBackfillsCandidatesAndMonotonicSequence(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.AutoMigrate(&SiteConfig{}, &RegistrationApplication{}, &RegistrationApplicationSequence{}); err != nil {
		t.Fatalf("create registration tables: %v", err)
	}
	if err := db.Create(&SiteConfig{VoceChatEmailDomain: "@vc.example"}).Error; err != nil {
		t.Fatalf("create site config: %v", err)
	}
	applications := []RegistrationApplication{
		{ApplicationID: "4", Username: "legacy-linked", PasswordHash: "hash", Status: RegistrationApplicationStatusApproved, VoceChatEmail: "legacy-actual@vc.example"},
		{ApplicationID: "9", Username: "legacy-pending", PasswordHash: "hash", Status: RegistrationApplicationStatusPending},
	}
	if err := db.Create(&applications).Error; err != nil {
		t.Fatalf("create legacy applications: %v", err)
	}

	if err := migrateRegistrationApplicationAllocationData(db); err != nil {
		t.Fatalf("migrate allocation data: %v", err)
	}
	var linked RegistrationApplication
	if err := db.Where("application_id = ?", "4").First(&linked).Error; err != nil {
		t.Fatalf("reload linked application: %v", err)
	}
	if linked.VoceChatCandidateEmail != "4@vc.example" {
		t.Fatalf("linked candidate = %q", linked.VoceChatCandidateEmail)
	}
	var pending RegistrationApplication
	if err := db.Where("application_id = ?", "9").First(&pending).Error; err != nil {
		t.Fatalf("reload pending application: %v", err)
	}
	if pending.VoceChatCandidateEmail != "9@vc.example" {
		t.Fatalf("pending candidate = %q", pending.VoceChatCandidateEmail)
	}
	var sequence RegistrationApplicationSequence
	if err := db.First(&sequence, 1).Error; err != nil {
		t.Fatalf("load sequence: %v", err)
	}
	if sequence.LastValue != 9 {
		t.Fatalf("sequence = %d, want 9", sequence.LastValue)
	}

	if err := db.Delete(&pending).Error; err != nil {
		t.Fatalf("delete highest application: %v", err)
	}
	if err := migrateRegistrationApplicationAllocationData(db); err != nil {
		t.Fatalf("repeat allocation migration: %v", err)
	}
	if err := db.First(&sequence, 1).Error; err != nil {
		t.Fatalf("reload sequence: %v", err)
	}
	if sequence.LastValue != 9 {
		t.Fatalf("repeated migration reused a deleted number: sequence=%d", sequence.LastValue)
	}
	if err := ensureRegistrationApplicationCandidateUniqueIndex(db); err != nil {
		t.Fatalf("create candidate unique index: %v", err)
	}
	duplicateCandidate := RegistrationApplication{
		ApplicationID:          "10",
		Username:               "duplicate-candidate",
		PasswordHash:           "hash",
		Status:                 RegistrationApplicationStatusPending,
		VoceChatCandidateEmail: linked.VoceChatCandidateEmail,
	}
	if err := db.Create(&duplicateCandidate).Error; err == nil {
		t.Fatal("database accepted a duplicate registration candidate email")
	}
}
