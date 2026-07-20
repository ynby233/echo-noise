package models

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestMigrateDBBackfillsLocalAttachmentVisibilityGrants(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	if err := db.AutoMigrate(&User{}, &Message{}); err != nil {
		t.Fatalf("migrate legacy tables: %v", err)
	}
	user := User{Username: "legacy-owner", Password: HashPassword("password")}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create legacy owner: %v", err)
	}
	message := Message{
		Content:    "[report](https://example.test/api/files/legacy%20report.pdf)",
		ImageURL:   "/api/images/legacy-image.png",
		UserID:     user.ID,
		Private:    true,
		Visibility: "private",
	}
	if err := db.Create(&message).Error; err != nil {
		t.Fatalf("create legacy message: %v", err)
	}

	if err := MigrateDB(db); err != nil {
		t.Fatalf("migrate current database: %v", err)
	}

	var grants []LocalAttachmentGrant
	if err := db.Where("message_id = ?", message.ID).Order("kind, name").Find(&grants).Error; err != nil {
		t.Fatalf("load backfilled grants: %v", err)
	}
	if len(grants) != 2 {
		t.Fatalf("backfilled grants = %#v, want file and image grants", grants)
	}
	got := map[string]LocalAttachmentGrant{}
	for _, grant := range grants {
		got[grant.Kind+"/"+grant.Name] = grant
	}
	for _, key := range []string{"file/legacy report.pdf", "image/legacy-image.png"} {
		grant, ok := got[key]
		if !ok {
			t.Fatalf("missing backfilled grant %q in %#v", key, grants)
		}
		if grant.OwnerUserID != user.ID || grant.Visibility != "private" {
			t.Fatalf("grant %q = %#v", key, grant)
		}
	}
}
