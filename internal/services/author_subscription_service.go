package services

import (
	"errors"
	"fmt"
	"time"

	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// authorSubscriptionSource excludes system carriers and lifecycle placeholders.
func authorSubscriptionSource(message models.Message) bool {
	return message.ID != 0 && message.UserID != 0 && message.DeletedAt == nil && !message.IsTombstone && !IsGuestbookMessage(message) && StoredMessageVisibility(message) != MessageVisibilityPrivate
}

// createAuthorSubscriptionNotifications runs inside the original note insert
// transaction. publishedAt is server insert time, independent of editable note dates.
// Contacts are refreshed before entering the transaction; only fresh cache rows
// are read here, avoiding network I/O and a second connection inside a write tx.
func createAuthorSubscriptionNotifications(tx *gorm.DB, message models.Message, publishedAt time.Time, contactsAllowed bool) ([]uint, error) {
	ids := []uint{}
	if !authorSubscriptionSource(message) {
		return ids, nil
	}
	var subscriptions []models.AuthorSubscription
	if err := tx.Where("author_user_id = ? AND subscriber_user_id <> ? AND created_at < ?", message.UserID, message.UserID, publishedAt).
		Where("subscriber_user_id IN (?)", tx.Model(&models.User{}).Select("id")).Find(&subscriptions).Error; err != nil {
		return nil, err
	}
	for _, subscription := range subscriptions {
		scope := ContentReadScope{actorID: subscription.SubscriberUserID, hasActor: true}
		allowed := false
		if StoredMessageVisibility(message) == MessageVisibilityContacts {
			if contactsAllowed {
				var count int64
				if err := tx.Model(&models.VoceChatContactCache{}).Where("user_id = ? AND contact_user_id = ? AND last_sync_status = ? AND expires_at > ?", message.UserID, subscription.SubscriberUserID, models.VoceChatContactSyncStatusOK, publishedAt).Count(&count).Error; err != nil {
					return nil, err
				}
				allowed = count > 0
			}
		} else {
			allowed = scope.CanInteractWithMessage(message)
		}
		if !allowed {
			continue
		}
		eventKey := fmt.Sprintf("author_subscription:%d:%d", message.ID, subscription.SubscriberUserID)
		authorID, messageID := message.UserID, message.ID
		notification := models.UserNotification{
			RecipientUserID:      subscription.SubscriberUserID,
			ActorUserID:          &authorID,
			Type:                 models.UserNotificationTypeAuthorSubscription,
			MessageID:            &messageID,
			AuthorSubscriptionID: &subscription.ID,
			EventKey:             &eventKey,
			CreatedAt:            publishedAt,
			UpdatedAt:            publishedAt,
		}
		result := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "event_key"}}, DoNothing: true}).Create(&notification)
		if result.Error != nil {
			return nil, result.Error
		}
		if result.RowsAffected == 0 {
			continue
		}
		if err := QueueWebPushForNotification(tx, notification); err != nil {
			return nil, err
		}
		ids = append(ids, notification.ID)
	}
	return ids, nil
}

// AuthorSubscriptionNotificationAllowed rechecks this type's current audience.
// It is deliberately a normal interaction check, without administrative hidden
// reads. An unsubscribe followed by a new subscription cannot revive old events.
func AuthorSubscriptionNotificationAllowed(db *gorm.DB, notification models.UserNotification) (bool, error) {
	if notification.Type != models.UserNotificationTypeAuthorSubscription {
		return true, nil
	}
	if db == nil || notification.RecipientUserID == 0 || notification.ActorUserID == nil || notification.MessageID == nil || notification.AuthorSubscriptionID == nil || *notification.ActorUserID == notification.RecipientUserID {
		return false, nil
	}
	var message models.Message
	if err := db.First(&message, *notification.MessageID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	if !authorSubscriptionSource(message) || message.UserID != *notification.ActorUserID {
		return false, nil
	}
	var users int64
	if err := db.Model(&models.User{}).Where("id IN ?", []uint{notification.RecipientUserID, message.UserID}).Count(&users).Error; err != nil {
		return false, err
	}
	if users != 2 {
		return false, nil
	}
	var active int64
	if err := db.Model(&models.AuthorSubscription{}).Where("subscriber_user_id = ? AND author_user_id = ? AND id = ?", notification.RecipientUserID, message.UserID, *notification.AuthorSubscriptionID).Count(&active).Error; err != nil || active == 0 {
		return false, err
	}
	scope := ContentReadScope{actorID: notification.RecipientUserID, hasActor: true}
	return scope.CanInteractWithMessage(message), nil
}
