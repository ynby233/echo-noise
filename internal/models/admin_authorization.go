package models

import "time"

// AdminCapabilityGrant is an explicit capability granted to a delegated
// administrator. Primary administrator access remains implicit.
type AdminCapabilityGrant struct {
	ID              uint      `gorm:"primaryKey" json:"id"`
	UserID          uint      `gorm:"not null;uniqueIndex:idx_admin_capability_grant;index" json:"user_id"`
	Capability      string    `gorm:"type:varchar(100);not null;uniqueIndex:idx_admin_capability_grant;index" json:"capability"`
	GrantedByUserID uint      `gorm:"not null;index" json:"granted_by_user_id"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// AdminAuditLog is append-only administration history. Retention cleanup is a
// system policy; no request endpoint may mutate individual audit rows.
type AdminAuditLog struct {
	ID                uint      `gorm:"primaryKey" json:"id"`
	ActorUserID       uint      `gorm:"index" json:"actor_user_id"`
	ActorUsername     string    `gorm:"type:varchar(191);not null" json:"actor_username"`
	ActorIsPrimary    bool      `gorm:"not null;index" json:"actor_is_primary"`
	ActorType         string    `gorm:"type:varchar(20);not null;default:user;index" json:"actor_type"`
	Capability        string    `gorm:"type:varchar(100);index" json:"capability"`
	Module            string    `gorm:"type:varchar(80);index" json:"module"`
	Action            string    `gorm:"type:varchar(80);index" json:"action"`
	TargetType        string    `gorm:"type:varchar(80);index" json:"target_type"`
	TargetID          string    `gorm:"type:varchar(191);index" json:"target_id"`
	TargetOwnerUserID *uint     `gorm:"index" json:"target_owner_user_id,omitempty"`
	Result            string    `gorm:"type:varchar(20);not null;index" json:"result"`
	Summary           string    `gorm:"type:text" json:"summary"`
	ChangesJSON       string    `gorm:"type:text" json:"changes_json"`
	Reason            string    `gorm:"type:text" json:"reason"`
	IP                string    `gorm:"type:varchar(64)" json:"ip"`
	UserAgent         string    `gorm:"type:varchar(512)" json:"user_agent"`
	AuthVia           string    `gorm:"type:varchar(20);index" json:"auth_via"`
	RequestID         string    `gorm:"type:varchar(64);index" json:"request_id"`
	CreatedAt         time.Time `gorm:"index" json:"created_at"`
}

// AdminAuditConfig has a single ID=1 row and defaults audit logging to enabled.
type AdminAuditConfig struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	Enabled       bool      `gorm:"not null;default:true" json:"enabled"`
	RetentionDays int       `gorm:"not null;default:730" json:"retention_days"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}
