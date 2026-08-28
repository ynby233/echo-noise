package models

import (
	"time"

	"gorm.io/gorm"
)

type SecurityAttackLog struct {
	gorm.Model
	IP     string `gorm:"type:varchar(191);index" json:"ip"`
	Method string `gorm:"type:varchar(20)" json:"method"`
	Path   string `gorm:"type:text" json:"path"`
	UA     string `gorm:"type:text" json:"ua"`
	// CreatedAt from gorm.Model is used as event time
}

type SecurityIPBan struct {
	gorm.Model
	IP     string     `gorm:"type:varchar(191);uniqueIndex" json:"ip"`
	Reason string     `gorm:"type:varchar(255)" json:"reason"`
	Until  *time.Time `json:"until"`
}

type SecurityConfig struct {
	gorm.Model
	AutoBanEnabled          bool `gorm:"default:false" json:"autoBanEnabled"`
	AutoBanWindowSeconds    int  `gorm:"default:600" json:"autoBanWindowSeconds"`
	AutoBanThreshold        int  `gorm:"default:10" json:"autoBanThreshold"`
	AutoBanMinutes          int  `gorm:"default:60" json:"autoBanMinutes"`
	AccessLogEnabled        bool `gorm:"default:false" json:"accessLogEnabled"`
	SiteVisitLogEnabled     bool `gorm:"default:false" json:"siteVisitLogEnabled"`
	AttackLogRetentionDays  int  `gorm:"not null;default:90" json:"attackLogRetentionDays"`
	AccessLogRetentionDays  int  `gorm:"not null;default:30" json:"accessLogRetentionDays"`
	SiteVisitRetentionDays  int  `gorm:"not null;default:90" json:"siteVisitRetentionDays"`
	LoginAuditRetentionDays int  `gorm:"not null;default:365" json:"loginAuditRetentionDays"`
}

// LoginAuditConfig is deliberately separate from the general security
// configuration: only the site owner may decide whether their own login and
// logout events are recorded. Delegated administrators are always audited.
type LoginAuditConfig struct {
	ID                 uint      `gorm:"primaryKey" json:"id"`
	RecordPrimaryAdmin bool      `gorm:"not null;default:false" json:"recordPrimaryAdmin"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type SecurityLoginAudit struct {
	gorm.Model
	UserID    uint   `gorm:"index;not null" json:"user_id"`
	Username  string `gorm:"type:varchar(191);index;not null" json:"username"`
	IsAdmin   bool   `gorm:"not null;default:false;index" json:"is_admin"`
	IsPrimary bool   `gorm:"not null;default:false;index" json:"is_primary"`
	Action    string `gorm:"type:varchar(20);index;default:login" json:"action"`
	IP        string `gorm:"type:varchar(191);index" json:"ip"`
	UserAgent string `gorm:"type:text" json:"user_agent"`
}

type SecurityAccessLog struct {
	gorm.Model
	IP         string `gorm:"type:varchar(191);index" json:"ip"`
	Method     string `gorm:"type:varchar(20);index" json:"method"`
	Path       string `gorm:"type:varchar(1024)" json:"path"`
	Status     int    `gorm:"index" json:"status"`
	UserID     uint   `gorm:"index" json:"user_id"`
	Username   string `gorm:"type:varchar(191);index" json:"username"`
	IsAdmin    bool   `gorm:"index" json:"is_admin"`
	UserAgent  string `gorm:"type:text" json:"user_agent"`
	Referer    string `gorm:"type:text" json:"referer"`
	DurationMS int64  `json:"duration_ms"`
}

type SecuritySiteVisitLog struct {
	gorm.Model
	IP        string `gorm:"type:varchar(191);index" json:"ip"`
	UserID    uint   `gorm:"index" json:"user_id"`
	Username  string `gorm:"type:varchar(191);index" json:"username"`
	IsAdmin   bool   `gorm:"index" json:"is_admin"`
	UserAgent string `gorm:"type:text" json:"user_agent"`
	Referer   string `gorm:"type:text" json:"referer"`
}
