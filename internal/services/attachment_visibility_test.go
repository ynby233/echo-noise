package services

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
)

func TestVisibleAttachmentSourcesCoversMessageCommentReplyAndGuestbook(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.Message{}, &models.Comment{}, &models.AdminCapabilityGrant{}); err != nil {
		t.Fatal(err)
	}
	primary := models.User{ID: 1, Username: "primary", IsAdmin: true}
	owner := models.User{ID: 2, Username: "owner"}
	delegated := models.User{ID: 3, Username: "delegated", IsAdmin: true}
	for _, user := range []*models.User{&primary, &owner, &delegated} {
		if err := db.Create(user).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Create(&models.AdminCapabilityGrant{UserID: delegated.ID, Capability: "content.view_hidden"}).Error; err != nil {
		t.Fatal(err)
	}
	message := models.Message{ID: 10, UserID: owner.ID, Content: "note /api/files/refs/ref-note/a.txt", Visibility: "private", Private: true}
	if err := db.Create(&message).Error; err != nil {
		t.Fatal(err)
	}
	root := models.Comment{ID: 11, MessageID: message.ID, UserID: &owner.ID, Content: "comment /api/files/refs/ref-comment/a.txt", Visibility: "private"}
	if err := db.Create(&root).Error; err != nil {
		t.Fatal(err)
	}
	reply := models.Comment{ID: 12, MessageID: message.ID, UserID: &owner.ID, ParentID: &root.ID, Content: "reply /api/files/refs/ref-reply/a.txt", Visibility: "private"}
	if err := db.Create(&reply).Error; err != nil {
		t.Fatal(err)
	}
	guestbook := models.Message{ID: 20, UserID: models.PrimaryAdminUserID, Content: models.CanonicalGuestbookContent, IsGuestbook: true, Visibility: "public"}
	if err := db.Create(&guestbook).Error; err != nil {
		t.Fatal(err)
	}
	guestbookComment := models.Comment{ID: 21, MessageID: guestbook.ID, UserID: &owner.ID, Content: "guestbook /api/files/refs/ref-guest/a.txt", Visibility: "private"}
	if err := db.Create(&guestbookComment).Error; err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		id   string
		want string
	}{{"ref-note", "message"}, {"ref-comment", "comment"}, {"ref-reply", "reply"}} {
		ref := models.AttachmentReference{PublicID: tc.id, OwnerUserID: owner.ID, Kind: "file"}
		sources, err := VisibleAttachmentSources(db, &delegated.ID, ref, "local")
		if err != nil {
			t.Fatal(err)
		}
		if len(sources) != 1 || sources[0].SourceType != tc.want {
			all, _ := LoadAttachmentSources(db, ref, "local")
			scope, _ := ResolveContentReadScope(db, &delegated.ID)
			for _, source := range all {
				ok, _ := AttachmentSourceVisible(db, &delegated.ID, source)
				t.Logf("source=%s visible=%v msg=%v", source.SourceType, ok, scope.CanReadMessage(source.Message))
			}
			t.Logf("all=%#v scopeHidden=%v", all, scope.CanViewHiddenContent())
			t.Fatalf("%s => %#v", tc.id, sources)
		}
	}
	guestRef := models.AttachmentReference{PublicID: "ref-guest", OwnerUserID: owner.ID, Kind: "file"}
	if sources, err := VisibleAttachmentSources(db, &delegated.ID, guestRef, "local"); err != nil {
		t.Fatal(err)
	} else if len(sources) != 0 {
		t.Fatalf("delegated hidden-read must not widen hidden guestbook: %#v", sources)
	}
	if sources, err := VisibleAttachmentSources(db, &primary.ID, guestRef, "local"); err != nil {
		t.Fatal(err)
	} else if len(sources) != 1 {
		t.Fatalf("primary should see the complete guestbook thread: %#v", sources)
	}
}
