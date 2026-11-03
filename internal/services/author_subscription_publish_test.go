package services

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/repository"
	"gorm.io/gorm"
)

func TestAuthorSubscriptionPublishAudienceAndFutureOnly(t *testing.T) {
	db := setupUserServiceTestDB(t)
	author := mustCreateUser(t, models.User{Username: "subscription-author", Password: "unused"})
	subscriber := mustCreateUser(t, models.User{Username: "subscription-reader", Password: "unused", IsAdmin: true})
	late := mustCreateUser(t, models.User{Username: "late-reader", Password: "unused"})
	publishedAt := time.Now().UTC()
	for _, subscription := range []models.AuthorSubscription{
		{SubscriberUserID: subscriber.ID, AuthorUserID: author.ID, CreatedAt: publishedAt.Add(-time.Hour)},
		{SubscriberUserID: late.ID, AuthorUserID: author.ID, CreatedAt: publishedAt.Add(time.Hour)},
		{SubscriberUserID: author.ID, AuthorUserID: author.ID, CreatedAt: publishedAt.Add(-time.Hour)},
		{SubscriberUserID: 99999, AuthorUserID: author.ID, CreatedAt: publishedAt.Add(-time.Hour)},
	} {
		if err := db.Create(&subscription).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Create(&models.VoceChatContactCache{UserID: author.ID, ContactUserID: subscriber.ID, LastSyncStatus: models.VoceChatContactSyncStatusOK, ExpiresAt: publishedAt.Add(time.Hour)}).Error; err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name            string
		message         models.Message
		contactsAllowed bool
		want            int
	}{
		{"public", models.Message{Visibility: MessageVisibilityPublic}, false, 1},
		{"users", models.Message{Visibility: MessageVisibilityUsers}, false, 1},
		{"contacts", models.Message{Visibility: MessageVisibilityContacts}, true, 1},
		{"contacts_disabled", models.Message{Visibility: MessageVisibilityContacts}, false, 0},
		{"private_even_for_admin", models.Message{Visibility: MessageVisibilityPrivate}, false, 0},
		{"guestbook", models.Message{Visibility: MessageVisibilityPublic, IsGuestbook: true}, false, 0},
		{"legacy_guestbook", models.Message{Visibility: MessageVisibilityPublic, Content: models.CanonicalGuestbookContent}, false, 0},
		{"tombstone", models.Message{Visibility: MessageVisibilityPublic, IsTombstone: true}, false, 0},
		{"deleted", models.Message{Visibility: MessageVisibilityPublic, DeletedAt: &publishedAt}, false, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			message := test.message
			message.UserID = author.ID
			if message.Content == "" {
				message.Content = test.name
			}
			// Backdated display timestamps do not turn a new insert into an old event.
			message.CreatedAt = publishedAt.AddDate(-1, 0, 0)
			if err := db.Create(&message).Error; err != nil {
				t.Fatal(err)
			}
			var ids []uint
			if err := db.Transaction(func(tx *gorm.DB) error {
				var err error
				ids, err = createAuthorSubscriptionNotifications(tx, message, publishedAt, test.contactsAllowed)
				return err
			}); err != nil {
				t.Fatal(err)
			}
			if len(ids) != test.want {
				t.Fatalf("notifications=%d want=%d", len(ids), test.want)
			}
			for _, id := range ids {
				var notification models.UserNotification
				if err := db.First(&notification, id).Error; err != nil {
					t.Fatal(err)
				}
				var subscription models.AuthorSubscription
				if err := db.Where("subscriber_user_id = ? AND author_user_id = ?", subscriber.ID, author.ID).First(&subscription).Error; err != nil {
					t.Fatal(err)
				}
				if notification.AuthorSubscriptionID == nil || *notification.AuthorSubscriptionID != subscription.ID {
					t.Fatalf("notification not bound to publication subscription: %+v", notification)
				}
			}
			if err := db.Transaction(func(tx *gorm.DB) error {
				duplicate, err := createAuthorSubscriptionNotifications(tx, message, publishedAt, test.contactsAllowed)
				if len(duplicate) != 0 {
					t.Errorf("replayed event created %v", duplicate)
				}
				return err
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestAuthorSubscriptionPublicationIsAtomic(t *testing.T) {
	db := setupUserServiceTestDB(t)
	author := mustCreateUser(t, models.User{Username: "atomic-author", Password: "unused"})
	subscriber := mustCreateUser(t, models.User{Username: "atomic-reader", Password: "unused"})
	if err := db.Create(&models.AuthorSubscription{SubscriberUserID: subscriber.ID, AuthorUserID: author.ID, CreatedAt: time.Now().UTC().Add(-time.Hour)}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Callback().Create().Before("gorm:create").Register("test:reject-subscription-notification", func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Table == "user_notifications" {
			tx.AddError(errors.New("notification persistence failed"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Callback().Create().Remove("test:reject-subscription-notification") })
	message := models.Message{UserID: author.ID, Content: "must roll back", Visibility: MessageVisibilityPublic}
	if err := CreateMessage(&message); err == nil {
		t.Fatal("expected publish error")
	}
	var count int64
	if err := db.Model(&models.Message{}).Where("content = ?", message.Content).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("failed publication persisted a note")
	}
}

func TestAuthorSubscriptionEventKeyIsDatabaseUnique(t *testing.T) {
	db := setupUserServiceTestDB(t)
	key := "author_subscription:123:456"
	subscription := models.AuthorSubscription{SubscriberUserID: 456, AuthorUserID: 123}
	if err := db.Create(&subscription).Error; err != nil {
		t.Fatal(err)
	}
	first := models.UserNotification{RecipientUserID: 456, Type: models.UserNotificationTypeAuthorSubscription, EventKey: &key, AuthorSubscriptionID: &subscription.ID}
	if err := db.Create(&first).Error; err != nil {
		t.Fatal(err)
	}
	second := models.UserNotification{RecipientUserID: 456, Type: models.UserNotificationTypeAuthorSubscription, EventKey: &key, AuthorSubscriptionID: &subscription.ID}
	if err := db.Create(&second).Error; err == nil {
		t.Fatal("database accepted a duplicate event key")
	}
	for index := range 2 {
		if err := db.Create(&models.UserNotification{RecipientUserID: 456, Type: models.UserNotificationTypeComment}).Error; err != nil {
			t.Fatalf("nullable legacy event key %d: %v", index, err)
		}
	}
}

func TestAuthorSubscriptionEventsOnlyOnCreate(t *testing.T) {
	db := setupUserServiceTestDB(t)
	author := mustCreateUser(t, models.User{Username: "create-only-author", Password: "unused"})
	subscriber := mustCreateUser(t, models.User{Username: "create-only-reader", Password: "unused"})
	if err := db.Create(&models.AuthorSubscription{SubscriberUserID: subscriber.ID, AuthorUserID: author.ID, CreatedAt: time.Now().UTC().Add(-time.Hour)}).Error; err != nil {
		t.Fatal(err)
	}
	message := models.Message{UserID: author.ID, Content: "first publication", Visibility: MessageVisibilityPublic}
	if err := CreateMessage(&message); err != nil {
		t.Fatal(err)
	}
	if err := repository.UpdateMessageContent(message.ID, "edited publication"); err != nil {
		t.Fatal(err)
	}
	if err := TrashMessage(db, author.ID, message.ID, "subscription lifecycle test"); err != nil {
		t.Fatal(err)
	}
	if err := RestoreMessage(db, author.ID, message.ID); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := db.Model(&models.UserNotification{}).Where("type = ? AND message_id = ?", models.UserNotificationTypeAuthorSubscription, message.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatal(fmt.Sprintf("edit/restore notification count=%d", count))
	}
}

func TestAuthorSubscriptionCurrentEligibilityAndHydration(t *testing.T) {
	db := setupUserServiceTestDB(t)
	author := mustCreateUser(t, models.User{Username: "live-author", Password: "unused"})
	subscriber := mustCreateUser(t, models.User{Username: "live-reader", Password: "unused", IsAdmin: true})
	// Model persisted datetime(3) values that are equal despite subscription preceding publication.
	publishedAt := time.Now().UTC().Truncate(time.Millisecond)
	subscription := models.AuthorSubscription{SubscriberUserID: subscriber.ID, AuthorUserID: author.ID, CreatedAt: publishedAt}
	if err := db.Create(&subscription).Error; err != nil {
		t.Fatal(err)
	}
	message := models.Message{UserID: author.ID, Content: "secret preview", Visibility: MessageVisibilityPublic}
	if err := db.Create(&message).Error; err != nil {
		t.Fatal(err)
	}
	subscriptionID := subscription.ID
	notification := models.UserNotification{RecipientUserID: subscriber.ID, ActorUserID: &author.ID, Type: models.UserNotificationTypeAuthorSubscription, MessageID: &message.ID, AuthorSubscriptionID: &subscriptionID, CreatedAt: publishedAt}
	if err := db.Create(&notification).Error; err != nil {
		t.Fatal(err)
	}
	assertAllowed := func(wantDelivery, wantHistory bool) {
		t.Helper()
		allowed, err := AuthorSubscriptionNotificationAllowed(db, notification)
		if err != nil || allowed != wantDelivery {
			t.Fatalf("allowed=%v want=%v err=%v", allowed, wantDelivery, err)
		}
		page, err := QueryUserNotificationPage(db, subscriber.ID, 1, 20)
		if err != nil || page.Total != 1 || page.UnreadCount != 1 {
			t.Fatalf("history page counts changed: total=%d unread=%d err=%v", page.Total, page.UnreadCount, err)
		}
		for _, hydrated := range []UserNotificationHydration{
			HydrateUserNotifications(db, subscriber.ID, []models.UserNotification{notification}),
			page.Hydration,
		} {
			if len(hydrated.Items) != 1 {
				t.Fatalf("items=%d", len(hydrated.Items))
			}
			item := hydrated.Items[0]
			if wantHistory && (item.TargetStatus != UserNotificationTargetStatusAvailable || item.Message == nil || item.TargetURL == "") {
				t.Fatalf("missing available target: %+v", item)
			}
			if !wantHistory && (item.Message != nil || item.TargetStatus != UserNotificationTargetStatusUnavailable || item.TargetURL != "") {
				t.Fatalf("stale preview or jump leaked: %+v", item)
			}
		}
	}
	assertAllowed(true, true)
	notification.AuthorSubscriptionID = nil
	assertAllowed(false, true)
	notification.AuthorSubscriptionID = &subscriptionID
	if err := db.Model(&message).Updates(map[string]interface{}{"visibility": MessageVisibilityPrivate, "private": true}).Error; err != nil {
		t.Fatal(err)
	}
	assertAllowed(false, false)
	if err := db.Model(&message).Updates(map[string]interface{}{"visibility": MessageVisibilityPublic, "private": false}).Error; err != nil {
		t.Fatal(err)
	}
	assertAllowed(true, true)
	if err := db.Model(&message).Updates(map[string]interface{}{"visibility": MessageVisibilityUsers, "private": true}).Error; err != nil {
		t.Fatal(err)
	}
	assertAllowed(true, true)
	if err := db.Delete(&subscription).Error; err != nil {
		t.Fatal(err)
	}
	assertAllowed(false, true)
	// Resubscribing later must not revive a queued publication from the old subscription.
	subscription.ID = 0
	subscription.CreatedAt = publishedAt
	if err := db.Create(&subscription).Error; err != nil {
		t.Fatal(err)
	}
	assertAllowed(false, true)
	if err := db.Model(&subscription).Update("created_at", publishedAt.Add(-time.Hour)).Error; err != nil {
		t.Fatal(err)
	}
	assertAllowed(false, true)
	if err := db.Model(&notification).Update("author_subscription_id", subscription.ID).Error; err != nil {
		t.Fatal(err)
	}
	subscriptionID = subscription.ID
	assertAllowed(true, true)
	if err := db.Model(&message).Updates(map[string]interface{}{"visibility": MessageVisibilityContacts, "private": true}).Error; err != nil {
		t.Fatal(err)
	}
	assertAllowed(false, false)
	if err := db.Model(&message).Updates(map[string]interface{}{"visibility": MessageVisibilityPublic, "private": false}).Error; err != nil {
		t.Fatal(err)
	}
	assertAllowed(true, true)
	if err := db.Model(&message).Update("deleted_at", publishedAt).Error; err != nil {
		t.Fatal(err)
	}
	assertAllowed(false, false)
}
