package services

import (
	"errors"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestAuthorSubscriptionStateValidationAndDeletedUsers(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&models.User{}, &models.AuthorSubscription{}); err != nil {
		t.Fatal(err)
	}
	subscriber := models.User{Username: "subscriber"}
	author := models.User{Username: "author"}
	for _, user := range []*models.User{&subscriber, &author} {
		if err := db.Create(user).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, test := range []struct {
		name         string
		subscriberID uint
		authorID     uint
		want         error
	}{
		{"guest", 0, author.ID, ErrAuthorSubscriptionUnauthorized},
		{"missing subscriber", 999999, author.ID, ErrAuthorSubscriptionUnauthorized},
		{"missing author", subscriber.ID, 999999, ErrAuthorSubscriptionNotFound},
		{"zero author", subscriber.ID, 0, ErrAuthorSubscriptionNotFound},
		{"self", subscriber.ID, subscriber.ID, ErrAuthorSubscriptionSelf},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := GetAuthorSubscriptionState(db, test.subscriberID, test.authorID); !errors.Is(err, test.want) {
				t.Fatalf("GET error=%v want=%v", err, test.want)
			}
			for _, subscribed := range []bool{true, false} {
				if _, err := SetAuthorSubscriptionState(db, test.subscriberID, test.authorID, subscribed); !errors.Is(err, test.want) {
					t.Fatalf("set %t error=%v want=%v", subscribed, err, test.want)
				}
			}
		})
	}
	if state, err := SetAuthorSubscriptionState(db, subscriber.ID, author.ID, true); err != nil || !state.Subscribed {
		t.Fatalf("subscribe: %+v %v", state, err)
	}
	for _, deleted := range []models.User{subscriber, author} {
		if err := db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Delete(&models.User{}, deleted.ID).Error; err != nil {
				return err
			}
			var active int64
			if err := ActiveAuthorSubscriptions(tx).Count(&active).Error; err != nil {
				return err
			}
			if active != 0 {
				t.Errorf("deleted participant %d still has %d active subscriptions", deleted.ID, active)
			}
			// Roll back so the other participant can be tested independently.
			return gorm.ErrInvalidTransaction
		}); !errors.Is(err, gorm.ErrInvalidTransaction) {
			t.Fatal(err)
		}
	}
	if err := db.Migrator().DropTable(&models.AuthorSubscription{}); err != nil {
		t.Fatal(err)
	}
	if _, err := GetAuthorSubscriptionState(db, subscriber.ID, author.ID); err == nil {
		t.Fatal("missing subscription table must not return a successful unsubscribed state")
	}
	for _, subscribed := range []bool{true, false} {
		if _, err := SetAuthorSubscriptionState(db, subscriber.ID, author.ID, subscribed); err == nil {
			t.Fatalf("set %t swallowed database failure", subscribed)
		}
	}
}
