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

func TestMigrateDBCreatesUpdateStateAndRemovesOnlyDelegatedUpdateGrant(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := MigrateDB(db); err != nil {
		t.Fatal(err)
	}
	grants := []AdminCapabilityGrant{
		{UserID: 2, Capability: "version.view", GrantedByUserID: PrimaryAdminUserID},
		{UserID: 2, Capability: "version.update", GrantedByUserID: PrimaryAdminUserID},
		{UserID: 2, Capability: "notes.view", GrantedByUserID: PrimaryAdminUserID},
	}
	if err := db.Create(&grants).Error; err != nil {
		t.Fatal(err)
	}
	if err := MigrateDB(db); err != nil {
		t.Fatal(err)
	}
	var remaining []AdminCapabilityGrant
	if err := db.Order("capability").Find(&remaining).Error; err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 2 || remaining[0].Capability != "notes.view" || remaining[1].Capability != "version.view" {
		t.Fatalf("remaining grants=%#v", remaining)
	}
	for _, model := range []any{&UpdatePreference{}, &UpdateExecutorCredential{}, &UpdateTask{}, &UpdateTaskEvent{}} {
		if !db.Migrator().HasTable(model) {
			t.Fatalf("missing update table for %T", model)
		}
	}
}

// This is the credential schema shipped before check_requested_at existed.
type legacyUpdateExecutorCredential struct {
	ID                uint       `gorm:"primaryKey"`
	Name              string     `gorm:"type:varchar(100);not null"`
	TokenHash         string     `gorm:"type:varchar(64);not null;uniqueIndex"`
	TokenPrefix       string     `gorm:"type:varchar(16);not null"`
	ExpiresAt         *time.Time `gorm:"index"`
	LastSeenAt        *time.Time
	CheckedAt         *time.Time
	InstanceID        string `gorm:"type:varchar(32)"`
	ExecutorVersion   string `gorm:"type:varchar(20)"`
	Platform          string `gorm:"type:varchar(30)"`
	InstalledRevision string `gorm:"type:varchar(40)"`
	CheckOK           bool
	SupersededAt      *time.Time `gorm:"index"`
	RevokedAt         *time.Time `gorm:"index"`
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func (legacyUpdateExecutorCredential) TableName() string {
	return "update_executor_credentials"
}

func TestMigrateDBUpgradesPopulatedLegacyUpdateCredentialWithoutLosingTask(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "legacy-update.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&legacyUpdateExecutorCredential{}, &UpdateTask{}); err != nil {
		t.Fatal(err)
	}
	checked := time.Now().UTC().Truncate(time.Second)
	legacy := legacyUpdateExecutorCredential{
		Name: "original executor", TokenHash: strings.Repeat("a", 64), TokenPrefix: "enu_original",
		CheckedAt: &checked, LastSeenAt: &checked, InstanceID: strings.Repeat("b", 32),
		ExecutorVersion: "u6-1", Platform: "linux/amd64", InstalledRevision: strings.Repeat("c", 40), CheckOK: true,
	}
	if err := db.Create(&legacy).Error; err != nil {
		t.Fatal(err)
	}
	slot := uint(1)
	task := UpdateTask{
		PublicID: "original-task", RequestedByUserID: PrimaryAdminUserID, Channel: "edge",
		TargetImage: "ghcr.io/ynby233/echo-noise", TargetDigest: "sha256:" + strings.Repeat("d", 64),
		TargetRevision: strings.Repeat("e", 40), Status: "claimed", ExecutorCredentialID: &legacy.ID, ActiveSlot: &slot,
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	if db.Migrator().HasColumn(&legacyUpdateExecutorCredential{}, "check_requested_at") {
		t.Fatal("legacy fixture already has new request column")
	}
	for range 2 {
		if err := MigrateDB(db); err != nil {
			t.Fatal(err)
		}
		if !db.Migrator().HasColumn(&UpdateExecutorCredential{}, "check_requested_at") {
			t.Fatal("migration did not add request column")
		}
		var credential UpdateExecutorCredential
		if err := db.First(&credential, legacy.ID).Error; err != nil {
			t.Fatal(err)
		}
		if credential.CheckRequestedAt != nil || credential.TokenHash != legacy.TokenHash || credential.TokenPrefix != legacy.TokenPrefix || credential.CheckedAt == nil || !credential.CheckedAt.Equal(checked) || !credential.CheckOK || credential.InstalledRevision != legacy.InstalledRevision {
			t.Fatalf("migration changed original credential: %#v", credential)
		}
		var preserved UpdateTask
		if err := db.First(&preserved, task.ID).Error; err != nil {
			t.Fatal(err)
		}
		if preserved.PublicID != task.PublicID || preserved.Status != task.Status || preserved.TargetDigest != task.TargetDigest || preserved.TargetRevision != task.TargetRevision || preserved.ActiveSlot == nil || *preserved.ActiveSlot != slot || preserved.ExecutorCredentialID == nil || *preserved.ExecutorCredentialID != legacy.ID {
			t.Fatalf("migration changed original task: %#v", preserved)
		}
	}
}
