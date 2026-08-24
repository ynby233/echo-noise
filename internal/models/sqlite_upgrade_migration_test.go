package models

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type legacyRegistrationApplicationV1 struct {
	ID                 uint   `gorm:"primaryKey"`
	ApplicationID      string `gorm:"type:varchar(64);not null;uniqueIndex"`
	Username           string `gorm:"type:varchar(191);not null;index"`
	PasswordHash       string `gorm:"type:varchar(191);not null"`
	Status             string `gorm:"type:varchar(20);not null;default:pending;index"`
	VoceChatUserID     string `gorm:"type:varchar(191);index"`
	VoceChatEmail      string `gorm:"type:varchar(191);index"`
	VoceChatSyncStatus string `gorm:"type:varchar(30);default:none;index"`
	VoceChatSyncError  string `gorm:"type:text"`
	LocalUserID        *uint  `gorm:"index"`
	ReviewerUserID     *uint  `gorm:"index"`
	ReviewNote         string `gorm:"type:text"`
	ReviewedAt         *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func (legacyRegistrationApplicationV1) TableName() string {
	return "registration_applications"
}

func TestMigrateDBUpgradesPopulatedLegacyRegistrationApplicationsOnSQLite(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "legacy-noise.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open legacy SQLite database: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get SQLite connection: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&legacyRegistrationApplicationV1{}); err != nil {
		t.Fatalf("create populated legacy registration table: %v", err)
	}
	if err := db.Create(&legacyRegistrationApplicationV1{
		ApplicationID:      "12",
		Username:           "legacy-applicant",
		PasswordHash:       "legacy-hash",
		Status:             RegistrationApplicationStatusApproved,
		VoceChatEmail:      "actual@vc.example",
		VoceChatSyncStatus: VoceChatSyncStatusLinked,
	}).Error; err != nil {
		t.Fatalf("seed legacy registration application: %v", err)
	}

	if err := MigrateDB(db); err != nil {
		if strings.Contains(err.Error(), "Cannot add a NOT NULL column with default value NULL") {
			t.Fatalf("SQLite upgrade reproduced fnOS startup failure: %v", err)
		}
		t.Fatalf("migrate populated legacy SQLite database: %v", err)
	}

	var application RegistrationApplication
	if err := db.Where("application_id = ?", "12").First(&application).Error; err != nil {
		t.Fatalf("reload migrated application: %v", err)
	}
	if application.Username != "legacy-applicant" || application.VoceChatEmail != "actual@vc.example" {
		t.Fatalf("migration changed existing application: %#v", application)
	}
	if application.VoceChatCandidateEmail != "12@vc.com" {
		t.Fatalf("candidate email = %q, want %q", application.VoceChatCandidateEmail, "12@vc.com")
	}
	if !db.Migrator().HasIndex(&RegistrationApplication{}, "idx_registration_applications_candidate_email_unique") {
		t.Fatal("candidate email unique index was not created")
	}
	var candidateColumn struct {
		Name    string `gorm:"column:name"`
		NotNull int    `gorm:"column:notnull"`
	}
	if err := db.Raw("SELECT name, `notnull` FROM pragma_table_info('registration_applications') WHERE name = ?", "voce_chat_candidate_email").Scan(&candidateColumn).Error; err != nil {
		t.Fatalf("inspect migrated candidate column: %v", err)
	}
	if candidateColumn.Name != "voce_chat_candidate_email" || candidateColumn.NotNull != 1 {
		t.Fatalf("candidate column schema = %#v, want final NOT NULL column", candidateColumn)
	}
	if err := MigrateDB(db); err != nil {
		t.Fatalf("repeat migrated SQLite startup: %v", err)
	}
}
