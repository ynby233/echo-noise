package services

import (
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
	"strconv"
	"strings"
	"time"
)

// Login-expiry settings normalize configured values and evaluate a user's issued session.
func normalizeLoginExpireConfig(days int, hours int) (int, int) {
	if days < 0 {
		days = 0
	}
	if hours < 0 {
		hours = 0
	}
	if days > maxLoginExpireDays {
		return maxLoginExpireDays, maxLoginExpireHours
	}
	if hours > maxLoginExpireHours {
		hours = maxLoginExpireHours
	}
	return days, hours
}

// LoginExpireDurationForUser resolves the current database policy on every
// authentication check. A zero duration is intentional and means permanent;
// ID 1 is always permanent regardless of either configurable policy.
func LoginExpireDurationForUser(user *models.User) time.Duration {
	if user != nil && user.ID == models.PrimaryAdminUserID {
		return 0
	}
	db, err := database.GetDB()
	if err != nil {
		return time.Duration(defaultLoginExpireDays) * 24 * time.Hour
	}
	var config models.SiteConfig
	if err := db.Table("site_configs").First(&config).Error; err != nil {
		return time.Duration(defaultLoginExpireDays) * 24 * time.Hour
	}
	days, hours := normalizeLoginExpireConfig(config.LoginExpireDays, config.LoginExpireHours)
	if user != nil && user.IsAdmin {
		days, hours = normalizeLoginExpireConfig(config.DelegatedAdminLoginExpireDays, config.DelegatedAdminLoginExpireHours)
	}
	return (time.Duration(days)*24 + time.Duration(hours)) * time.Hour
}

func IsUserLoginExpired(user *models.User, issuedAt int64, now time.Time) bool {
	duration := LoginExpireDurationForUser(user)
	if duration <= 0 {
		return false
	}
	if issuedAt <= 0 {
		// Database migration stamps legacy accounts once. An in-memory or
		// externally managed test database without that migration remains
		// compatible until it is restarted through the normal bootstrap path.
		return false
	}
	return now.Unix() > issuedAt+int64(duration/time.Second)
}

func parsePositiveIntSetting(raw interface{}) (int, bool) {
	switch v := raw.(type) {
	case float64:
		return int(v), true
	case float32:
		return int(v), true
	case int:
		return v, true
	case int64:
		return int(v), true
	case uint:
		return int(v), true
	case string:
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return n, true
		}
	}
	return 0, false
}
