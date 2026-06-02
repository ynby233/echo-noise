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
}

type SecurityLoginAudit struct {
	gorm.Model
	UserID    uint   `gorm:"index;not null" json:"user_id"`
	Username  string `gorm:"type:varchar(191);index;not null" json:"username"`
	IP        string `gorm:"type:varchar(191);index" json:"ip"`
	UserAgent string `gorm:"type:text" json:"user_agent"`
}
