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
	AutoBanEnabled       bool `gorm:"default:false" json:"autoBanEnabled"`
	AutoBanWindowSeconds int  `gorm:"default:600" json:"autoBanWindowSeconds"`
	AutoBanThreshold     int  `gorm:"default:10" json:"autoBanThreshold"`
	AutoBanMinutes       int  `gorm:"default:60" json:"autoBanMinutes"`
	AccessLogEnabled     bool `gorm:"default:false" json:"accessLogEnabled"`
	SiteVisitLogEnabled  bool `gorm:"default:false" json:"siteVisitLogEnabled"`
}

type SecurityLoginAudit struct {
	gorm.Model
	UserID    uint   `gorm:"index;not null" json:"user_id"`
	Username  string `gorm:"type:varchar(191);index;not null" json:"username"`
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
