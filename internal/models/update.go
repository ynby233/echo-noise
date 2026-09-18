package models

import "time"

// UpdatePreference is the single-instance channel preference. Changing it
// never creates an installation task.
type UpdatePreference struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	Channel    string    `gorm:"type:varchar(20);not null" json:"channel"`
	InstanceID string    `gorm:"type:varchar(32)" json:"instance_id"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// UpdateExecutorCredential stores only a verifier. The token is returned once
// when the primary administrator creates or rotates the credential.
type UpdateExecutorCredential struct {
	ID           uint       `gorm:"primaryKey" json:"id"`
	Name         string     `gorm:"type:varchar(100);not null" json:"name"`
	TokenHash    string     `gorm:"type:varchar(64);not null;uniqueIndex" json:"-"`
	TokenPrefix  string     `gorm:"type:varchar(16);not null" json:"token_prefix"`
	ExpiresAt    *time.Time `gorm:"index" json:"expires_at,omitempty"`
	LastSeenAt   *time.Time `json:"last_seen_at,omitempty"`
	SupersededAt *time.Time `gorm:"index" json:"-"`
	RevokedAt    *time.Time `gorm:"index" json:"revoked_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// UpdateTask pins one server-resolved image target. ActiveSlot is cleared on a
// terminal result; its unique index enforces at most one unfinished task.
type UpdateTask struct {
	ID                   uint       `gorm:"primaryKey" json:"-"`
	PublicID             string     `gorm:"type:varchar(64);not null;uniqueIndex" json:"id"`
	RequestedByUserID    uint       `gorm:"not null;index" json:"requested_by_user_id"`
	Channel              string     `gorm:"type:varchar(20);not null" json:"channel"`
	TargetImage          string     `gorm:"type:varchar(191);not null" json:"target_image"`
	TargetDigest         string     `gorm:"type:varchar(80);not null" json:"target_digest"`
	TargetRevision       string     `gorm:"type:varchar(40);not null" json:"target_revision"`
	TargetVersion        string     `gorm:"type:varchar(40)" json:"target_version,omitempty"`
	Status               string     `gorm:"type:varchar(30);not null;index" json:"status"`
	ExecutorCredentialID *uint      `gorm:"index" json:"-"`
	ActiveSlot           *uint      `gorm:"uniqueIndex" json:"-"`
	ErrorSummary         string     `gorm:"type:varchar(500)" json:"error_summary,omitempty"`
	ClaimedAt            *time.Time `json:"claimed_at,omitempty"`
	FinishedAt           *time.Time `json:"finished_at,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

type UpdateTaskEvent struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	TaskID    uint      `gorm:"not null;uniqueIndex:idx_update_task_status;index" json:"-"`
	Status    string    `gorm:"type:varchar(30);not null;uniqueIndex:idx_update_task_status" json:"status"`
	Summary   string    `gorm:"type:varchar(500)" json:"summary,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}
