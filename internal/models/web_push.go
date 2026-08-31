package models

import "time"

const (
	WebPushDeliveryPending    = "pending"
	WebPushDeliverySending    = "sending"
	WebPushDeliverySent       = "sent"
	WebPushDeliveryRetry      = "retry"
	WebPushDeliveryFailed     = "failed"
	WebPushDeliveryInvalid    = "invalid"
	WebPushDeliverySkipped    = "skipped"
	WebPushSourceNotification = "user_notification"
	WebPushSourceAnnouncement = "announcement"
	WebPushSourceTest         = "test"
)

// WebPushSubscription is one browser PushSubscription associated with one
// authenticated login session. Endpoint and keys are secrets and must never be
// returned by list/config responses or written to logs.
type WebPushSubscription struct {
	ID               uint       `gorm:"primaryKey" json:"id"`
	UserID           uint       `gorm:"not null;index" json:"user_id"`
	SessionID        string     `gorm:"type:varchar(64);not null;index" json:"-"`
	SessionIssuedAt  *time.Time `gorm:"index" json:"-"`
	SessionExpiresAt *time.Time `gorm:"index" json:"-"`
	Endpoint         string     `gorm:"type:text;not null" json:"-"`
	EndpointHash     string     `gorm:"type:varchar(64);not null;uniqueIndex" json:"-"`
	P256dh           string     `gorm:"type:text;not null" json:"-"`
	Auth             string     `gorm:"type:text;not null" json:"-"`
	Platform         string     `gorm:"type:varchar(32);index" json:"platform,omitempty"`
	UserAgent        string     `gorm:"type:varchar(255)" json:"-"`
	ExpiresAt        *time.Time `gorm:"index" json:"-"`
	DisabledAt       *time.Time `gorm:"index" json:"-"`
	FailureCount     uint       `gorm:"not null;default:0" json:"-"`
	LastSuccessAt    *time.Time `json:"-"`
	LastFailureAt    *time.Time `json:"-"`
	LastErrorCode    string     `gorm:"type:varchar(40)" json:"-"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

// WebPushPreference is account-wide. Every active subscribed login uses the
// same type and privacy preferences; per-device management is intentionally
// not part of the first PWA release.
type WebPushPreference struct {
	UserID                 uint       `gorm:"primaryKey;autoIncrement:false" json:"-"`
	Enabled                bool       `gorm:"not null;default:true" json:"enabled"`
	CommentEnabled         bool       `gorm:"not null;default:true" json:"comment_enabled"`
	ReplyEnabled           bool       `gorm:"not null;default:true" json:"reply_enabled"`
	GuestbookEnabled       bool       `gorm:"not null;default:true" json:"guestbook_enabled"`
	LikeEnabled            bool       `gorm:"not null;default:false" json:"like_enabled"`
	AnnouncementEnabled    bool       `gorm:"not null;default:true" json:"announcement_enabled"`
	AccountSecurityEnabled bool       `gorm:"not null;default:true" json:"account_security_enabled"`
	ShowPreview            bool       `gorm:"not null;default:false" json:"show_preview"`
	LastTestSentAt         *time.Time `json:"-"`
	CreatedAt              time.Time  `json:"-"`
	UpdatedAt              time.Time  `json:"-"`
}

// WebPushDelivery is a durable outbox row. PayloadJSON is the privacy-filtered
// snapshot that will be retried after process restarts without re-reading
// content that may later become hidden or deleted.
type WebPushDelivery struct {
	ID              uint       `gorm:"primaryKey" json:"id"`
	SourceKind      string     `gorm:"type:varchar(30);not null;uniqueIndex:idx_web_push_event_delivery,priority:1;index" json:"source_kind"`
	SourceID        uint       `gorm:"not null;uniqueIndex:idx_web_push_event_delivery,priority:2;index" json:"source_id"`
	SourceVersion   int64      `gorm:"not null;uniqueIndex:idx_web_push_event_delivery,priority:3" json:"source_version"`
	SubscriptionID  uint       `gorm:"not null;uniqueIndex:idx_web_push_event_delivery,priority:4;index" json:"subscription_id"`
	RecipientUserID uint       `gorm:"not null;index" json:"recipient_user_id"`
	PayloadJSON     string     `gorm:"type:text;not null" json:"-"`
	Status          string     `gorm:"type:varchar(20);not null;default:pending;index" json:"status"`
	AttemptCount    uint       `gorm:"not null;default:0" json:"attempt_count"`
	NextAttemptAt   *time.Time `gorm:"index" json:"next_attempt_at,omitempty"`
	LeaseUntil      *time.Time `gorm:"index" json:"-"`
	LastAttemptAt   *time.Time `json:"last_attempt_at,omitempty"`
	SentAt          *time.Time `json:"sent_at,omitempty"`
	LastStatusCode  int        `json:"last_status_code,omitempty"`
	LastError       string     `gorm:"type:varchar(255)" json:"-"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}
