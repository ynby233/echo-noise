package repository

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestDeleteUserRemovesWebPushSecretsAndPendingDeliveries(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open repository test db: %v", err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.WebPushSubscription{}, &models.WebPushPreference{}, &models.WebPushDelivery{}); err != nil {
		t.Fatalf("migrate repository test db: %v", err)
	}
	database.DB = db
	t.Cleanup(func() { database.DB = nil })
	user := models.User{Username: "deleted-push-user", Password: "hashed"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	subscription := models.WebPushSubscription{
		UserID: user.ID, SessionID: "session", Endpoint: "https://push.example/delete",
		EndpointHash: "delete-endpoint", P256dh: "secret-p256dh", Auth: "secret-auth",
	}
	if err := db.Create(&subscription).Error; err != nil {
		t.Fatalf("create subscription: %v", err)
	}
	preference := models.WebPushPreference{UserID: user.ID}
	if err := db.Create(&preference).Error; err != nil {
		t.Fatalf("create preference: %v", err)
	}
	delivery := models.WebPushDelivery{
		SourceKind: models.WebPushSourceTest, SourceID: user.ID, SourceVersion: 1,
		SubscriptionID: subscription.ID, RecipientUserID: user.ID, PayloadJSON: `{}`, Status: models.WebPushDeliveryPending,
	}
	if err := db.Create(&delivery).Error; err != nil {
		t.Fatalf("create delivery: %v", err)
	}

	if err := DeleteUser(user.ID); err != nil {
		t.Fatalf("delete user: %v", err)
	}
	for name, model := range map[string]any{
		"user": &models.User{}, "subscription": &models.WebPushSubscription{},
		"preference": &models.WebPushPreference{}, "delivery": &models.WebPushDelivery{},
	} {
		var count int64
		query := db.Model(model)
		if name == "user" {
			query = query.Where("id = ?", user.ID)
		} else if name == "subscription" || name == "preference" {
			query = query.Where("user_id = ?", user.ID)
		} else {
			query = query.Where("recipient_user_id = ?", user.ID)
		}
		if err := query.Count(&count).Error; err != nil {
			t.Fatalf("count %s: %v", name, err)
		}
		if count != 0 {
			t.Fatalf("%s rows after deletion = %d, want 0", name, count)
		}
	}
}
