package models

import "time"

const (
	AnnouncementStatusDraft     = "draft"
	AnnouncementStatusPublished = "published"
	AnnouncementStatusWithdrawn = "withdrawn"

	AnnouncementReaderDevice = "device"
	AnnouncementReaderUser   = "user"

	AnnouncementPushPending = "pending"
	AnnouncementPushSending = "sending"
	AnnouncementPushSent    = "sent"
	AnnouncementPushFailed  = "failed"
	AnnouncementPushSkipped = "skipped"
)

type Announcement struct {
	ID           uint       `gorm:"primaryKey" json:"id"`
	Title        string     `gorm:"type:varchar(191);not null" json:"title"`
	Content      string     `gorm:"type:text;not null" json:"content"`
	Status       string     `gorm:"type:varchar(20);not null;default:draft;index" json:"status"`
	Revision     uint       `gorm:"not null;default:1" json:"revision"`
	PushEnabled  bool       `gorm:"not null;default:false" json:"push_enabled"`
	AuthorUserID uint       `gorm:"not null;index" json:"author_user_id"`
	PublishedAt  *time.Time `gorm:"index" json:"published_at,omitempty"`
	WithdrawnAt  *time.Time `json:"withdrawn_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

type AnnouncementRead struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	AnnouncementID uint      `gorm:"not null;index;uniqueIndex:idx_announcement_reader" json:"announcement_id"`
	ReaderType     string    `gorm:"type:varchar(20);not null;uniqueIndex:idx_announcement_reader" json:"reader_type"`
	ReaderKey      string    `gorm:"type:varchar(64);not null;uniqueIndex:idx_announcement_reader" json:"-"`
	Revision       uint      `gorm:"not null;default:1" json:"revision"`
	ReadAt         time.Time `gorm:"not null;index" json:"read_at"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type AnnouncementPushDelivery struct {
	ID                      uint       `gorm:"primaryKey" json:"id"`
	AnnouncementID          uint       `gorm:"not null;index;uniqueIndex:idx_announcement_push_recipient" json:"announcement_id"`
	RecipientUserID         uint       `gorm:"not null;index;uniqueIndex:idx_announcement_push_recipient" json:"recipient_user_id"`
	RecipientVoceChatUserID string     `gorm:"type:varchar(191)" json:"recipient_voce_chat_user_id,omitempty"`
	Status                  string     `gorm:"type:varchar(20);not null;default:pending;index" json:"status"`
	AttemptCount            int        `gorm:"not null;default:0" json:"attempt_count"`
	LastError               string     `gorm:"type:text" json:"last_error,omitempty"`
	LastAttemptAt           *time.Time `json:"last_attempt_at,omitempty"`
	SentAt                  *time.Time `json:"sent_at,omitempty"`
	CreatedAt               time.Time  `json:"created_at"`
	UpdatedAt               time.Time  `json:"updated_at"`
}
