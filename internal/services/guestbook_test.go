package services

import (
	"sync"
	"testing"

	"github.com/rcy1314/echo-noise/internal/models"
)

func TestEnsureGuestbookDoesNotCreateWithoutPrimaryAdmin(t *testing.T) {
	db := setupUserServiceTestDB(t)
	mustCreateUser(t, models.User{ID: 2, Username: "delegated-only", IsAdmin: true})

	if _, err := EnsureGuestbook(db); err == nil {
		t.Fatal("EnsureGuestbook succeeded without the ID 1 administrator")
	}

	var count int64
	if err := db.Model(&models.Message{}).Count(&count).Error; err != nil {
		t.Fatalf("count messages: %v", err)
	}
	if count != 0 {
		t.Fatalf("messages created without the ID 1 administrator: %d", count)
	}
}

func TestEnsureGuestbookDoesNotCreateWhenPrimaryUserIsNotAdmin(t *testing.T) {
	db := setupUserServiceTestDB(t)
	mustCreateUser(t, models.User{ID: models.PrimaryAdminUserID, Username: "ordinary-id-one", IsAdmin: false})
	mustCreateUser(t, models.User{ID: 2, Username: "delegated-fallback", IsAdmin: true})

	if _, err := EnsureGuestbook(db); err == nil {
		t.Fatal("EnsureGuestbook used a delegated administrator while user ID 1 was not an administrator")
	}

	var count int64
	if err := db.Model(&models.Message{}).Count(&count).Error; err != nil {
		t.Fatalf("count messages: %v", err)
	}
	if count != 0 {
		t.Fatalf("messages created while the ID 1 user was not an administrator: %d", count)
	}
}

func TestEnsureGuestbookRepairsLegacyOwnerAndIgnoresDecoy(t *testing.T) {
	db := setupUserServiceTestDB(t)
	if !models.IsCanonicalGuestbookContent("留言板\n\n#guestbook") {
		t.Fatal("the historical title plus #guestbook marker must remain migratable")
	}
	primary := mustCreateUser(t, models.User{ID: models.PrimaryAdminUserID, Username: "guestbook-owner", IsAdmin: true})
	legacyOwner := mustCreateUser(t, models.User{Username: "legacy-owner", IsAdmin: true})
	decoy := models.Message{Content: "普通笔记：我在留言板记录今天的安排", UserID: legacyOwner.ID, Visibility: MessageVisibilityPublic}
	legacy := models.Message{Content: models.CanonicalGuestbookContent, UserID: legacyOwner.ID, Username: legacyOwner.Username, Visibility: MessageVisibilityPublic}
	if err := db.Create(&decoy).Error; err != nil {
		t.Fatalf("create decoy: %v", err)
	}
	if err := db.Create(&legacy).Error; err != nil {
		t.Fatalf("create legacy guestbook: %v", err)
	}

	descriptor, err := EnsureGuestbook(db)
	if err != nil {
		t.Fatalf("ensure guestbook: %v", err)
	}
	if descriptor.MessageID != legacy.ID || descriptor.RecipientUserID != models.PrimaryAdminUserID {
		t.Fatalf("descriptor=%#v, want legacy message and recipient 1", descriptor)
	}
	var repaired models.Message
	if err := db.First(&repaired, legacy.ID).Error; err != nil {
		t.Fatalf("load repaired guestbook: %v", err)
	}
	if !repaired.IsGuestbook || repaired.UserID != primary.ID || repaired.Username != primary.Username {
		t.Fatalf("repaired guestbook=%#v, want marker and owner 1", repaired)
	}
	var decoyAfter models.Message
	if err := db.First(&decoyAfter, decoy.ID).Error; err != nil {
		t.Fatalf("load decoy: %v", err)
	}
	if decoyAfter.IsGuestbook {
		t.Fatal("ordinary note mentioning 留言板 must remain a normal message")
	}
	repeat, err := EnsureGuestbook(db)
	if err != nil || repeat.MessageID != legacy.ID {
		t.Fatalf("repeat ensure=%#v, err=%v; ensure must be idempotent", repeat, err)
	}
	var marked int64
	if err := db.Model(&models.Message{}).Where("is_guestbook = ?", true).Count(&marked).Error; err != nil {
		t.Fatalf("count canonical guestbooks: %v", err)
	}
	if marked != 1 {
		t.Fatalf("canonical guestbook rows=%d, want 1", marked)
	}
}

func TestEnsureGuestbookConcurrentCallsShareOneMessage(t *testing.T) {
	db := setupUserServiceTestDB(t)
	mustCreateUser(t, models.User{ID: models.PrimaryAdminUserID, Username: "guestbook-concurrent", IsAdmin: true})
	const callers = 8
	results := make(chan GuestbookDescriptor, callers)
	errs := make(chan error, callers)
	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			descriptor, err := EnsureGuestbook(db)
			results <- descriptor
			errs <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	var messageID uint
	for descriptor := range results {
		if descriptor.MessageID == 0 {
			t.Fatal("concurrent ensure returned empty message ID")
		}
		if messageID == 0 {
			messageID = descriptor.MessageID
		} else if messageID != descriptor.MessageID {
			t.Fatalf("concurrent ensure IDs differ: %d vs %d", messageID, descriptor.MessageID)
		}
	}
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent ensure failed: %v", err)
		}
	}
	var count int64
	if err := db.Model(&models.Message{}).Where("is_guestbook = ?", true).Count(&count).Error; err != nil {
		t.Fatalf("count guestbooks: %v", err)
	}
	if count != 1 {
		t.Fatalf("concurrent canonical rows=%d, want 1", count)
	}
}
