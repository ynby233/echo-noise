package repository

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestDeleteUserRemovesBothDirectionsOfAuthorSubscriptions(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.AuthorSubscription{}); err != nil {
		t.Fatal(err)
	}
	previousDB := database.DB
	database.DB = db
	t.Cleanup(func() {
		database.DB = previousDB
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	deleted := models.User{Username: "deleted"}
	first := models.User{Username: "first"}
	second := models.User{Username: "second"}
	for _, user := range []*models.User{&deleted, &first, &second} {
		if err := db.Create(user).Error; err != nil {
			t.Fatal(err)
		}
	}
	subscriptions := []models.AuthorSubscription{
		{SubscriberUserID: deleted.ID, AuthorUserID: first.ID},
		{SubscriberUserID: second.ID, AuthorUserID: deleted.ID},
		{SubscriberUserID: first.ID, AuthorUserID: second.ID},
	}
	if err := db.Create(&subscriptions).Error; err != nil {
		t.Fatal(err)
	}
	if err := DeleteUser(deleted.ID); err != nil {
		t.Fatalf("delete account: %v", err)
	}
	var remaining []models.AuthorSubscription
	if err := db.Find(&remaining).Error; err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 1 || remaining[0].SubscriberUserID != first.ID || remaining[0].AuthorUserID != second.ID {
		t.Fatalf("account deletion retained orphaned or removed unrelated subscriptions: %+v", remaining)
	}
	var userCount int64
	if err := db.Model(&models.User{}).Where("id = ?", deleted.ID).Count(&userCount).Error; err != nil {
		t.Fatal(err)
	}
	if userCount != 0 {
		t.Fatal("account deletion did not remove the user")
	}
}
