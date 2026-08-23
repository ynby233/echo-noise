package models

import (
	"encoding/json"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestMigrateDBAddsPasswordAlertCleanupTasksWithoutLosingNotifications(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.AutoMigrate(&User{}, &UserNotification{}); err != nil {
		t.Fatalf("create legacy notification schema: %v", err)
	}
	user := User{Username: "migration-user", Password: HashPassword("migration-password")}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create legacy user: %v", err)
	}
	if err := db.Create(&UserNotification{RecipientUserID: user.ID, Type: UserNotificationTypeVoceChatPasswordChanged}).Error; err != nil {
		t.Fatalf("create legacy password alert: %v", err)
	}

	if err := MigrateDB(db); err != nil {
		t.Fatalf("migrate database: %v", err)
	}
	if !db.Migrator().HasTable(&PasswordAlertCleanupTask{}) {
		t.Fatal("password alert cleanup task table was not created")
	}
	var notificationCount int64
	if err := db.Model(&UserNotification{}).Where("recipient_user_id = ?", user.ID).Count(&notificationCount).Error; err != nil {
		t.Fatalf("count legacy password alerts: %v", err)
	}
	if notificationCount != 1 {
		t.Fatalf("migration changed legacy password alerts: got %d, want 1", notificationCount)
	}
	if err := db.Create(&PasswordAlertCleanupTask{UserID: user.ID}).Error; err != nil {
		t.Fatalf("create migrated cleanup task: %v", err)
	}
}

func TestPasswordAlertCleanupTaskHasNoPublicJSONFields(t *testing.T) {
	encoded, err := json.Marshal(PasswordAlertCleanupTask{UserID: 42})
	if err != nil {
		t.Fatalf("marshal cleanup task: %v", err)
	}
	if string(encoded) != "{}" {
		t.Fatalf("cleanup task exposed public JSON fields: %s", encoded)
	}
}
