package services

import (
	"testing"

	"github.com/rcy1314/echo-noise/internal/models"
)

func TestSeedDefaultDataServerAndDesktopCreatePrimaryAdminDependencies(t *testing.T) {
	for _, startupPath := range []string{"server", "desktop-sidecar"} {
		t.Run(startupPath, func(t *testing.T) {
			db := setupUserServiceTestDB(t)
			t.Setenv("NOISE_MOBILE", "")

			if err := SeedDefaultData(); err != nil {
				t.Fatalf("SeedDefaultData: %v", err)
			}

			var primary models.User
			if err := db.First(&primary, models.PrimaryAdminUserID).Error; err != nil {
				t.Fatalf("load ID 1 site owner: %v", err)
			}
			if !primary.IsAdmin {
				t.Fatal("ID 1 site owner is not an administrator")
			}

			var messages []models.Message
			if err := db.Order("id ASC").Find(&messages).Error; err != nil {
				t.Fatalf("load seeded messages: %v", err)
			}
			var guestbookCount int
			var exampleCount int
			for _, message := range messages {
				if message.UserID != models.PrimaryAdminUserID {
					t.Fatalf("seeded message %d owner=%d, want ID 1", message.ID, message.UserID)
				}
				if message.IsGuestbook {
					guestbookCount++
				} else {
					exampleCount++
				}
			}
			if guestbookCount != 1 || exampleCount == 0 {
				t.Fatalf("seeded guestbooks=%d examples=%d, want one guestbook and at least one example", guestbookCount, exampleCount)
			}
		})
	}
}

func TestSeedDefaultDataMobileStopsBeforePrimaryAdminDependencies(t *testing.T) {
	db := setupUserServiceTestDB(t)
	t.Setenv("NOISE_MOBILE", "1")

	if err := SeedDefaultData(); err == nil {
		t.Fatal("SeedDefaultData succeeded on a mobile empty database without the ID 1 administrator")
	}

	var messageCount int64
	if err := db.Model(&models.Message{}).Count(&messageCount).Error; err != nil {
		t.Fatalf("count messages: %v", err)
	}
	if messageCount != 0 {
		t.Fatalf("mobile startup created %d primary-admin-dependent messages", messageCount)
	}
}
