package controllers

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupLoginExpireConfigTestDB(t *testing.T, cfg models.SiteConfig) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	if err := db.AutoMigrate(&models.SiteConfig{}); err != nil {
		t.Fatalf("migrate test db: %v", err)
	}
	if err := db.Create(&cfg).Error; err != nil {
		t.Fatalf("create site config: %v", err)
	}
	database.DB = db
	t.Cleanup(func() {
		database.DB = nil
	})
}

func TestNormalizeLoginExpireConfigSupportsHoursAndLimits(t *testing.T) {
	tests := []struct {
		name      string
		days      int
		hours     int
		wantDays  int
		wantHours int
	}{
		{name: "zero is explicitly permanent", days: 0, hours: 0, wantDays: 0, wantHours: 0},
		{name: "hours only is valid", days: 0, hours: 6, wantDays: 0, wantHours: 6},
		{name: "negative values are clamped", days: -1, hours: -2, wantDays: 0, wantHours: 0},
		{name: "hours capped at 24", days: 2, hours: 99, wantDays: 2, wantHours: 24},
		{name: "days above maximum caps to 31 days 24 hours", days: 90, hours: 0, wantDays: 31, wantHours: 24},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotDays, gotHours := normalizeLoginExpireConfig(tt.days, tt.hours)
			if gotDays != tt.wantDays || gotHours != tt.wantHours {
				t.Fatalf("normalizeLoginExpireConfig(%d, %d) = (%d, %d), want (%d, %d)", tt.days, tt.hours, gotDays, gotHours, tt.wantDays, tt.wantHours)
			}
		})
	}
}

func TestGetLoginExpireDurationUsesDaysAndHours(t *testing.T) {
	setupLoginExpireConfigTestDB(t, models.SiteConfig{LoginExpireDays: 1, LoginExpireHours: 6})

	got := getLoginExpireDuration()
	want := 30 * time.Hour
	if got != want {
		t.Fatalf("getLoginExpireDuration() = %v, want %v", got, want)
	}
}

func TestLoginExpirySeparatesOrdinaryDelegatedAndPrimaryAndPreservesPermanentZero(t *testing.T) {
	setupLoginExpireConfigTestDB(t, models.SiteConfig{
		LoginExpireDays: 1, LoginExpireHours: 2,
	})
	if err := database.DB.Model(&models.SiteConfig{}).Where("1 = 1").Updates(map[string]interface{}{
		"delegated_admin_login_expire_days":  0,
		"delegated_admin_login_expire_hours": 0,
	}).Error; err != nil {
		t.Fatalf("set delegated permanent duration: %v", err)
	}
	ordinary := &models.User{ID: 2, IsAdmin: false}
	delegated := &models.User{ID: 3, IsAdmin: true}
	primary := &models.User{ID: models.PrimaryAdminUserID, IsAdmin: true}

	if got := getLoginExpireDurationForUser(ordinary); got != 26*time.Hour {
		t.Fatalf("ordinary duration = %v, want 26h", got)
	}
	if got := getLoginExpireDurationForUser(delegated); got != 0 {
		t.Fatalf("delegated permanent duration = %v, want 0", got)
	}
	if got := getLoginExpireDurationForUser(primary); got != 0 {
		t.Fatalf("primary duration = %v, want 0", got)
	}
	if days, hours := normalizeLoginExpireConfig(0, 0); days != 0 || hours != 0 {
		t.Fatalf("zero duration normalized to (%d,%d), want (0,0)", days, hours)
	}
}
