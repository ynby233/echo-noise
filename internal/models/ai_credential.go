package models

import "time"

// AICredential stores only a SHA-256 digest. The bearer secret exists only in
// the creation response; deleting this row revokes the credential immediately.
type AICredential struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    uint      `gorm:"not null;index" json:"-"`
	Name      string    `gorm:"type:varchar(100);not null" json:"name"`
	TokenHash string    `gorm:"type:char(64);not null;uniqueIndex" json:"-"`
	CreatedAt time.Time `json:"created_at"`
}
