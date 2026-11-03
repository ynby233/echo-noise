package models

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type legacyAuthorSubscriptionNotification struct {
	ID              uint `gorm:"primaryKey"`
	RecipientUserID uint
	Type            string
	CreatedAt       time.Time
}

func (legacyAuthorSubscriptionNotification) TableName() string {
	return "user_notifications"
}

func TestAuthorSubscriptionSchemaAndNullableNotificationEventKeyUpgrade(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&legacyAuthorSubscriptionNotification{}); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := db.Create(&legacyAuthorSubscriptionNotification{RecipientUserID: 1, Type: UserNotificationTypeComment}).Error; err != nil {
			t.Fatal(err)
		}
	}
	// Exercise the production migration, including all pre-existing rows.
	if err := MigrateDB(db); err != nil {
		t.Fatalf("upgrade database: %v", err)
	}
	if err := MigrateDB(db); err != nil {
		t.Fatalf("repeat upgrade: %v", err)
	}
	var legacy []UserNotification
	if err := db.Order("id").Find(&legacy).Error; err != nil {
		t.Fatal(err)
	}
	if len(legacy) != 2 || legacy[0].EventKey != nil || legacy[1].EventKey != nil || legacy[0].AuthorSubscriptionID != nil || legacy[1].AuthorSubscriptionID != nil {
		t.Fatalf("legacy notifications must retain NULL event keys and subscription IDs: %+v", legacy)
	}
	for range 2 {
		if err := db.Create(&UserNotification{RecipientUserID: 1, Type: UserNotificationTypeComment}).Error; err != nil {
			t.Fatalf("multiple NULL event keys must remain valid: %v", err)
		}
	}
	key := "author_subscription:1:2"
	subscription := AuthorSubscription{SubscriberUserID: 1, AuthorUserID: 2}
	if err := db.Create(&subscription).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&UserNotification{RecipientUserID: 1, Type: UserNotificationTypeAuthorSubscription, EventKey: &key, AuthorSubscriptionID: &subscription.ID}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&UserNotification{RecipientUserID: 1, Type: UserNotificationTypeAuthorSubscription, EventKey: &key, AuthorSubscriptionID: &subscription.ID}).Error; err == nil {
		t.Fatal("duplicate non-NULL notification event key was accepted")
	}
	if err := db.Create(&AuthorSubscription{SubscriberUserID: 1, AuthorUserID: 2}).Error; err == nil {
		t.Fatal("duplicate subscriber/author pair was accepted")
	}
	for _, subscription := range []AuthorSubscription{
		{SubscriberUserID: 2, AuthorUserID: 1},
		{SubscriberUserID: 3, AuthorUserID: 2},
		{SubscriberUserID: 1, AuthorUserID: 3},
	} {
		if err := db.Create(&subscription).Error; err != nil {
			t.Fatalf("distinct subscriber/author pair rejected: %v", err)
		}
	}
}
