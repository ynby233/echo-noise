package services

import (
	"errors"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
)

func TestMessagePinScopesUseIndependentOrderingAndLocateTheSamePage(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.Message{}); err != nil {
		t.Fatalf("migrate test database: %v", err)
	}
	database.DB = db
	models.SetDB(db)
	t.Cleanup(func() {
		database.DB = nil
		models.SetDB(nil)
	})

	owner := models.User{ID: 42, Username: "owner"}
	if err := db.Create(&owner).Error; err != nil {
		t.Fatalf("create owner: %v", err)
	}
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	globalFirst := base.Add(3 * time.Hour)
	globalSecond := base.Add(1 * time.Hour)
	personalFirst := base.Add(5 * time.Hour)
	personalSecond := base.Add(2 * time.Hour)
	messages := []models.Message{
		{Content: "global only #alpha", Username: owner.Username, UserID: owner.ID, Visibility: MessageVisibilityPublic, Pinned: true, PinnedAt: &globalFirst, CreatedAt: base},
		{Content: "personal only #alpha", Username: owner.Username, UserID: owner.ID, Visibility: MessageVisibilityPublic, PersonalPinned: true, PersonalPinnedAt: &personalFirst, CreatedAt: base.Add(24 * time.Hour)},
		{Content: "both #alpha", Username: owner.Username, UserID: owner.ID, Visibility: MessageVisibilityPublic, Pinned: true, PinnedAt: &globalSecond, PersonalPinned: true, PersonalPinnedAt: &personalSecond, CreatedAt: base.Add(48 * time.Hour)},
		{Content: "normal #alpha", Username: owner.Username, UserID: owner.ID, Visibility: MessageVisibilityPublic, CreatedAt: base.Add(72 * time.Hour)},
	}
	if err := db.Create(&messages).Error; err != nil {
		t.Fatalf("create messages: %v", err)
	}

	ownerID := owner.ID
	latest, err := GetMessagesByPage(1, 10, &ownerID, nil, nil, nil, nil, nil, MessagePinScopeLatest, nil)
	if err != nil {
		t.Fatalf("load latest scope: %v", err)
	}
	assertMessageIDs(t, latest.Items, messages[0].ID, messages[2].ID, messages[3].ID, messages[1].ID)

	personal, err := GetMessagesByPage(1, 10, &ownerID, &ownerID, nil, nil, nil, nil, MessagePinScopePersonal, nil)
	if err != nil {
		t.Fatalf("load personal scope: %v", err)
	}
	assertMessageIDs(t, personal.Items, messages[1].ID, messages[2].ID, messages[3].ID, messages[0].ID)

	latestTag, err := GetMessagesByPage(1, 2, &ownerID, nil, nil, nil, nil, stringPtr("alpha"), MessagePinScopeLatest, nil)
	if err != nil {
		t.Fatalf("load latest tag scope: %v", err)
	}
	assertMessageIDs(t, latestTag.Items, messages[0].ID, messages[2].ID)

	personalTag, err := GetMessagesByPage(1, 2, &ownerID, &ownerID, nil, nil, nil, stringPtr("alpha"), MessagePinScopePersonal, nil)
	if err != nil {
		t.Fatalf("load personal tag scope: %v", err)
	}
	assertMessageIDs(t, personalTag.Items, messages[1].ID, messages[2].ID)

	latestLocation, err := LocateMessagePage(messages[3].ID, 2, &ownerID, nil, nil, nil, nil, nil, MessagePinScopeLatest, nil)
	if err != nil {
		t.Fatalf("locate latest message: %v", err)
	}
	if latestLocation.Page != 2 || latestLocation.Total != 4 {
		t.Fatalf("latest location must match latest ordering: %#v", latestLocation)
	}

	personalLocation, err := LocateMessagePage(messages[0].ID, 2, &ownerID, &ownerID, nil, nil, nil, nil, MessagePinScopePersonal, nil)
	if err != nil {
		t.Fatalf("locate personal message: %v", err)
	}
	if personalLocation.Page != 2 || personalLocation.Total != 4 {
		t.Fatalf("personal location must match personal ordering: %#v", personalLocation)
	}
}

func TestSetPinMaintainsPinTimestampsAndClearsThemOnUnpin(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.Message{}); err != nil {
		t.Fatalf("migrate test database: %v", err)
	}
	message := models.Message{Content: "pin timestamp", UserID: 42, Visibility: MessageVisibilityPublic}
	if err := db.Create(&message).Error; err != nil {
		t.Fatalf("create message: %v", err)
	}

	if err := SetGlobalPin(db, message.ID, true); err != nil {
		t.Fatalf("set global pin: %v", err)
	}
	var first models.Message
	if err := db.First(&first, message.ID).Error; err != nil {
		t.Fatalf("reload first global pin: %v", err)
	}
	if !first.Pinned || first.PinnedAt == nil {
		t.Fatalf("global pin must set its timestamp: %#v", first)
	}

	time.Sleep(2 * time.Millisecond)
	if err := SetGlobalPin(db, message.ID, false); err != nil {
		t.Fatalf("unset global pin: %v", err)
	}
	first = models.Message{}
	if err := db.First(&first, message.ID).Error; err != nil {
		t.Fatalf("reload unset global pin: %v", err)
	}
	if first.Pinned || first.PinnedAt != nil {
		t.Fatalf("unsetting global pin must clear its timestamp: %#v", first)
	}

	if err := SetPersonalPin(db, message.ID, true); err != nil {
		t.Fatalf("set personal pin: %v", err)
	}
	var second models.Message
	if err := db.First(&second, message.ID).Error; err != nil {
		t.Fatalf("reload personal pin: %v", err)
	}
	if !second.PersonalPinned || second.PersonalPinnedAt == nil || second.Pinned {
		t.Fatalf("personal pin must only set its own timestamp and state: %#v", second)
	}
	if err := SetPersonalPin(db, message.ID, false); err != nil {
		t.Fatalf("unset personal pin: %v", err)
	}
	second = models.Message{}
	if err := db.First(&second, message.ID).Error; err != nil {
		t.Fatalf("reload unset personal pin: %v", err)
	}
	if second.PersonalPinned || second.PersonalPinnedAt != nil {
		t.Fatalf("unsetting personal pin must clear its timestamp: %#v", second)
	}
}

func TestSetPinRejectsRecycledMessages(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.Message{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	deletedAt := time.Now().UTC()
	message := models.Message{Content: "recycled", UserID: 42, Visibility: MessageVisibilityPublic, DeletedAt: &deletedAt}
	if err := db.Create(&message).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := SetGlobalPin(db, message.ID, true); !errors.Is(err, ErrMessageNotVisible) {
		t.Fatalf("global pin must reject recycled message, got %v", err)
	}
	if err := SetPersonalPin(db, message.ID, true); !errors.Is(err, ErrMessageNotVisible) {
		t.Fatalf("personal pin must reject recycled message, got %v", err)
	}
}

func TestPersonalPinScopeRejectsMissingOrForeignAuthor(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.Message{}); err != nil {
		t.Fatalf("migrate test database: %v", err)
	}
	database.DB = db
	models.SetDB(db)
	t.Cleanup(func() {
		database.DB = nil
		models.SetDB(nil)
	})

	ownerID := uint(42)
	otherID := uint(43)
	for _, user := range []models.User{{ID: ownerID, Username: "owner"}, {ID: otherID, Username: "other"}} {
		if err := db.Create(&user).Error; err != nil {
			t.Fatalf("create user: %v", err)
		}
	}
	if err := db.Create(&models.Message{Content: "private scope", UserID: ownerID, Visibility: MessageVisibilityPublic}).Error; err != nil {
		t.Fatalf("create message: %v", err)
	}

	if _, err := GetMessagesByPage(1, 10, &ownerID, nil, nil, nil, nil, nil, MessagePinScopePersonal, nil); err == nil {
		t.Fatal("personal scope without authorId must be rejected")
	}
	if _, err := GetMessagesByPage(1, 10, &otherID, &ownerID, nil, nil, nil, nil, MessagePinScopePersonal, nil); err == nil {
		t.Fatal("personal scope for another author must be rejected")
	}
}

func assertMessageIDs(t *testing.T, items []models.Message, expected ...uint) {
	t.Helper()
	if len(items) != len(expected) {
		t.Fatalf("expected message ids %#v, got %d items: %#v", expected, len(items), items)
	}
	for index, item := range items {
		if item.ID != expected[index] {
			t.Fatalf("expected message ids %#v, got %#v", expected, messageIDs(items))
		}
	}
}

func messageIDs(items []models.Message) []uint {
	ids := make([]uint, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	return ids
}

func stringPtr(value string) *string { return &value }
