package models

import "time"

// AuthorSubscription records an explicit opt-in to an author's future notes.
// The pair is unique so repeated subscriptions cannot duplicate deliveries.
type AuthorSubscription struct {
	ID               uint      `gorm:"primaryKey" json:"id"`
	SubscriberUserID uint      `gorm:"not null;uniqueIndex:idx_author_subscription_pair,priority:1;index" json:"subscriber_user_id"`
	AuthorUserID     uint      `gorm:"not null;uniqueIndex:idx_author_subscription_pair,priority:2;index" json:"author_user_id"`
	CreatedAt        time.Time `json:"created_at"`
}
