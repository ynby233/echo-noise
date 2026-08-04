package models

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type legacyPinMessage struct {
	ID         uint   `gorm:"primaryKey"`
	Content    string `gorm:"type:text;not null"`
	Username   string `gorm:"type:varchar(100)"`
	ImageURL   string `gorm:"type:text"`
	Private    bool   `gorm:"default:false"`
	Visibility string `gorm:"type:varchar(20);not null;default:public"`
	UserID     uint   `gorm:"not null"`
	CreatedAt  time.Time
	Notify     bool `gorm:"default:false"`
	Pinned     bool `gorm:"default:false"`
	LikeCount  int  `gorm:"default:0"`
}

func (legacyPinMessage) TableName() string { return "messages" }

func TestMigrateDBPreservesHistoricalGlobalPinAndDefaultsPersonalPinFalse(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	if err := db.AutoMigrate(&legacyPinMessage{}); err != nil {
		t.Fatalf("create legacy messages table: %v", err)
	}
	if err := db.Create(&legacyPinMessage{Content: "legacy", Username: "owner", Visibility: "public", UserID: 42, Pinned: true}).Error; err != nil {
		t.Fatalf("insert legacy pinned message: %v", err)
	}

	if err := MigrateDB(db); err != nil {
		t.Fatalf("migrate legacy database: %v", err)
	}
	if !db.Migrator().HasColumn(&Message{}, "personal_pinned") {
		t.Fatal("migration did not add personal_pinned")
	}
	if !db.Migrator().HasIndex(&Message{}, "idx_msg_global_pin_order") || !db.Migrator().HasIndex(&Message{}, "idx_msg_personal_pin_order") {
		t.Fatal("migration did not create both pin ordering indexes")
	}

	var legacy Message
	if err := db.First(&legacy, "content = ?", "legacy").Error; err != nil {
		t.Fatalf("reload migrated message: %v", err)
	}
	if !legacy.Pinned {
		t.Fatal("historical pinned=true must remain the global pin")
	}
	if legacy.PersonalPinned {
		t.Fatal("historical messages must default personal_pinned to false")
	}

	newMessage := Message{Content: "new", UserID: 42, Visibility: "public"}
	if err := db.Create(&newMessage).Error; err != nil {
		t.Fatalf("create migrated message: %v", err)
	}
	if newMessage.Pinned || newMessage.PersonalPinned {
		t.Fatalf("new messages must default both pin fields to false: %#v", newMessage)
	}
}
