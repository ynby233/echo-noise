package services

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/rcy1314/echo-noise/internal/authorization"
	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
)

func TestVisibleRecycleBinAttachmentSourcesRequiresViewAndHiddenCapabilities(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.Message{}, &models.Comment{}, &models.AdminCapabilityGrant{}, &models.AdminAuditLog{}, &models.AdminAuditConfig{}); err != nil {
		t.Fatal(err)
	}
	primary := models.User{ID: models.PrimaryAdminUserID, Username: "primary", IsAdmin: true}
	owner := models.User{ID: 2, Username: "owner"}
	delegated := models.User{ID: 3, Username: "delegated", IsAdmin: true}
	for _, user := range []*models.User{&primary, &owner, &delegated} {
		if err := db.Create(user).Error; err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now().UTC()
	message := models.Message{UserID: owner.ID, Visibility: MessageVisibilityPrivate, Private: true, DeletedAt: &now, Content: "[file](/api/files/refs/recycle-ref/file.txt)"}
	if err := db.Create(&message).Error; err != nil {
		t.Fatal(err)
	}
	ref := models.AttachmentReference{PublicID: "recycle-ref", OwnerUserID: owner.ID, Kind: "file"}
	if _, err := VisibleRecycleBinAttachmentSources(db, &delegated.ID, ref, "local"); err == nil {
		t.Fatal("missing recycle-bin capability must fail")
	}
	authorizer := authorization.New(db)
	if err := authorizer.ReplaceGrants(primary.ID, delegated.ID, []authorization.Capability{authorization.CapabilityNotesRecycleBinView}); err != nil {
		t.Fatal(err)
	}
	if sources, err := VisibleRecycleBinAttachmentSources(db, &delegated.ID, ref, "local"); err != nil || len(sources) != 0 {
		t.Fatalf("hidden source leaked: %#v err=%v", sources, err)
	}
	if err := authorizer.ReplaceGrants(primary.ID, delegated.ID, []authorization.Capability{authorization.CapabilityNotesRecycleBinView, authorization.CapabilityContentViewHidden}); err != nil {
		t.Fatal(err)
	}
	if sources, err := VisibleRecycleBinAttachmentSources(db, &delegated.ID, ref, "local"); err != nil || len(sources) != 1 {
		t.Fatalf("authorized recycle source missing: %#v err=%v", sources, err)
	}
}

func TestVisibleAttachmentSourcesCoversMessageCommentReplyAndGuestbook(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.Message{}, &models.Comment{}, &models.AdminCapabilityGrant{}, &models.AdminAuditLog{}, &models.AdminAuditConfig{}); err != nil {
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

func TestVisibleLegacyAttachmentSourcesScansDiscussionWhenParentHasNoAttachment(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.Message{}, &models.Comment{}, &models.AdminCapabilityGrant{}); err != nil {
		t.Fatal(err)
	}
	owner := models.User{ID: 2, Username: "owner"}
	if err := db.Create(&owner).Error; err != nil {
		t.Fatal(err)
	}
	message := models.Message{ID: 10, UserID: owner.ID, Content: "parent without attachment", Visibility: "public"}
	if err := db.Create(&message).Error; err != nil {
		t.Fatal(err)
	}
	comment := models.Comment{ID: 11, MessageID: message.ID, UserID: &owner.ID, Content: "comment /api/files/comment-only.txt", Visibility: "public"}
	if err := db.Create(&comment).Error; err != nil {
		t.Fatal(err)
	}
	reply := models.Comment{ID: 12, MessageID: message.ID, UserID: &owner.ID, ParentID: &comment.ID, Content: "reply /api/files/reply-only.txt", Visibility: "public"}
	if err := db.Create(&reply).Error; err != nil {
		t.Fatal(err)
	}
	sources, err := VisibleLegacyAttachmentSources(db, nil, "file", "comment-only.txt")
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 1 || sources[0].SourceType != "comment" || sources[0].SourceID != comment.ID {
		t.Fatalf("comment-only source = %#v", sources)
	}
	sources, err = VisibleLegacyAttachmentSources(db, nil, "file", "reply-only.txt")
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 1 || sources[0].SourceType != "reply" || sources[0].SourceID != reply.ID {
		t.Fatalf("reply-only source = %#v", sources)
	}
}

func TestVisibleLegacyAttachmentSourcesMatchesHistoricalCloudURL(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.Message{}, &models.Comment{}, &models.CloudAttachmentObject{}, &models.SiteConfig{}); err != nil {
		t.Fatal(err)
	}
	owner := models.User{ID: 2, Username: "owner"}
	if err := db.Create(&owner).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.SiteConfig{AttachmentStoragePublicBaseURL: "https://public.example.test/note"}).Error; err != nil {
		t.Fatal(err)
	}
	object := models.CloudAttachmentObject{PublicID: "cloud-public-id", ObjectKey: "note/legacy/report.txt", LegacyObjectKey: "note/legacy/report.txt", OriginalName: "report.txt"}
	if err := db.Create(&object).Error; err != nil {
		t.Fatal(err)
	}
	message := models.Message{ID: 10, UserID: owner.ID, Content: "cloud parent", Visibility: "public"}
	if err := db.Create(&message).Error; err != nil {
		t.Fatal(err)
	}
	comment := models.Comment{ID: 11, MessageID: message.ID, UserID: &owner.ID, Content: "legacy https://public.example.test/note/legacy/report.txt", Visibility: "private"}
	if err := db.Create(&comment).Error; err != nil {
		t.Fatal(err)
	}
	sources, err := VisibleLegacyAttachmentSources(db, nil, "cloud", object.PublicID)
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 1 || sources[0].SourceType != "comment" || sources[0].SourceID != comment.ID {
		t.Fatalf("historical cloud source = %#v", sources)
	}
	visible, err := VisibleLegacyAttachmentSourcesForViewer(db, nil, "cloud", object.PublicID)
	if err != nil {
		t.Fatal(err)
	}
	if len(visible) != 0 {
		t.Fatalf("anonymous viewer saw hidden historical cloud source = %#v", visible)
	}
	visible, err = VisibleLegacyAttachmentSourcesForViewer(db, &owner.ID, "cloud", object.PublicID)
	if err != nil {
		t.Fatal(err)
	}
	if len(visible) != 1 {
		t.Fatalf("owner could not see hidden historical cloud source = %#v", visible)
	}
}

func TestVisibleRecycleBinLegacyAttachmentSourcesExcludesActiveSources(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.Message{}, &models.Comment{}, &models.AdminCapabilityGrant{}, &models.AdminAuditLog{}, &models.AdminAuditConfig{}); err != nil {
		t.Fatal(err)
	}
	primary := models.User{ID: 1, Username: "primary", IsAdmin: true}
	owner := models.User{ID: 2, Username: "owner"}
	for _, u := range []*models.User{&primary, &owner} {
		if err := db.Create(u).Error; err != nil {
			t.Fatal(err)
		}
	}
	deletedAt := time.Now().UTC()
	deleted := models.Message{ID: 10, UserID: owner.ID, Content: "[file](/api/files/old.txt)", Visibility: "public", DeletedAt: &deletedAt}
	active := models.Message{ID: 11, UserID: owner.ID, Content: "[file](/api/files/active.txt)", Visibility: "public"}
	if err := db.Create(&deleted).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&active).Error; err != nil {
		t.Fatal(err)
	}
	sources, err := VisibleRecycleBinLegacyAttachmentSources(db, &primary.ID, "file", "active.txt")
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 0 {
		t.Fatalf("active source leaked into recycle context: %#v", sources)
	}
	sources, err = VisibleRecycleBinLegacyAttachmentSources(db, &primary.ID, "file", "old.txt")
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 1 || sources[0].MessageID != deleted.ID {
		t.Fatalf("deleted source missing: %#v", sources)
	}
}
