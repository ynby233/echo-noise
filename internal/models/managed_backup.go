package models

import "time"

// ManagedBackupPlan schedules one daily backup in an IANA timezone. LastRun is
// the durable scheduled slot claim, including failed or interrupted attempts.
// Clients cannot reset that claim through SavePlan.
type ManagedBackupPlan struct {
	Kind      string     `gorm:"primaryKey;size:32" json:"kind"`
	Enabled   bool       `json:"enabled"`
	Time      string     `gorm:"size:5" json:"time"`
	Timezone  string     `gorm:"size:128" json:"timezone"`
	KeepLocal int        `json:"keepLocal"`
	KeepCloud int        `json:"keepCloud"`
	Target    string     `gorm:"size:16" json:"target"`
	Scope     string     `gorm:"size:16" json:"scope"`
	LastRun   *time.Time `json:"lastRun,omitempty"`
	NextRun   *time.Time `gorm:"-" json:"nextRun,omitempty"`
	Revision  uint64     `json:"-"`
	CreatedAt time.Time  `json:"-"`
	UpdatedAt time.Time  `json:"-"`
}

// ManagedBackupRecord is an audit entry, retained even when retention removes
// its archive. Local and Cloud describe copies currently available to open.
type ManagedBackupRecord struct {
	ID         string     `gorm:"primaryKey;size:64" json:"id"`
	Kind       string     `gorm:"size:32;index" json:"kind"`
	CreatedAt  time.Time  `gorm:"index" json:"createdAt"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
	Trigger    string     `gorm:"size:16" json:"trigger"`
	Target     string     `gorm:"size:16" json:"target"`
	Scope      string     `gorm:"size:16" json:"scope"`
	Status     string     `gorm:"size:16;index" json:"status"`
	Size       int64      `json:"size"`
	Local      bool       `json:"local"`
	Cloud      bool       `json:"cloud"`
	Error      string     `json:"error,omitempty"`
	CloudKey   string     `gorm:"size:512" json:"-"`
}
