package services

import (
	"testing"
	"time"

	"github.com/rcy1314/echo-noise/internal/authorization"
	"github.com/rcy1314/echo-noise/internal/models"
)

func TestPrimaryHiddenInteractionsRespectDelegatedParticipationAcrossManagementLists(t *testing.T) {
	db := setupUserServiceTestDB(t)
	primary := mustCreateUser(t, models.User{ID: models.PrimaryAdminUserID, Username: "interaction-protection-primary", IsAdmin: true})
	delegated := mustCreateUser(t, models.User{Username: "interaction-protection-delegated", IsAdmin: true})
	ordinary := mustCreateUser(t, models.User{Username: "interaction-protection-ordinary"})
	if err := authorization.New(db).ReplaceGrants(primary.ID, delegated.ID, []authorization.Capability{
		authorization.CapabilityCommentsView,
		authorization.CapabilityCommentsViewHidden,
		authorization.CapabilityCommentsRecycleBinView,
	}); err != nil {
		t.Fatal(err)
	}

	ordinaryNote := models.Message{Content: "ordinary interaction context", UserID: ordinary.ID, Username: ordinary.Username, Visibility: MessageVisibilityPublic}
	delegatedNote := models.Message{Content: "delegated interaction context", UserID: delegated.ID, Username: delegated.Username, Visibility: MessageVisibilityPublic}
	if err := db.Create(&[]models.Message{ordinaryNote, delegatedNote}).Error; err != nil {
		t.Fatal(err)
	}
	var notes []models.Message
	if err := db.Where("content LIKE ?", "% interaction context").Order("id ASC").Find(&notes).Error; err != nil || len(notes) != 2 {
		t.Fatalf("load notes=%#v err=%v", notes, err)
	}
	ordinaryNote, delegatedNote = notes[0], notes[1]
	primaryID := primary.ID
	delegatedID := delegated.ID
	parent := models.Comment{MessageID: ordinaryNote.ID, UserID: &delegatedID, Content: "delegated public parent", Visibility: MessageVisibilityPublic}
	if err := db.Create(&parent).Error; err != nil {
		t.Fatal(err)
	}
	parentID := parent.ID
	now := time.Now().UTC()
	comments := []models.Comment{
		{MessageID: ordinaryNote.ID, UserID: &primaryID, Content: "unrelated primary private", Visibility: MessageVisibilityPrivate},
		{MessageID: delegatedNote.ID, UserID: &primaryID, Content: "primary private to note owner", Visibility: MessageVisibilityPrivate},
		{MessageID: ordinaryNote.ID, ParentID: &parentID, UserID: &primaryID, Content: "primary private reply to delegated", Visibility: MessageVisibilityPrivate},
		{MessageID: ordinaryNote.ID, UserID: &primaryID, Content: "trashed unrelated primary private", Visibility: MessageVisibilityPrivate, DeletedAt: &now},
		{MessageID: delegatedNote.ID, UserID: &primaryID, Content: "trashed primary private to note owner", Visibility: MessageVisibilityPrivate, DeletedAt: &now},
	}
	if err := db.Create(&comments).Error; err != nil {
		t.Fatal(err)
	}

	active, err := ListCommentManagement(db, delegated.ID, CommentManagementFilter{Page: 1, PageSize: 20}, false, false, now)
	if err != nil {
		t.Fatal(err)
	}
	activeContent := map[string]bool{}
	for _, item := range active.Items {
		activeContent[item.Content] = true
	}
	if activeContent["unrelated primary private"] {
		t.Fatal("delegated hidden reader must not see an unrelated primary-admin hidden interaction")
	}
	if !activeContent["primary private to note owner"] || !activeContent["primary private reply to delegated"] {
		t.Fatalf("delegated participant lost related primary hidden interactions: %#v", activeContent)
	}

	recycle, err := ListCommentManagement(db, delegated.ID, CommentManagementFilter{Page: 1, PageSize: 20}, true, false, now)
	if err != nil {
		t.Fatal(err)
	}
	recycleContent := map[string]bool{}
	for _, item := range recycle.Items {
		recycleContent[item.Content] = true
	}
	if recycleContent["trashed unrelated primary private"] {
		t.Fatal("comment recycle bin leaked an unrelated primary-admin hidden interaction")
	}
	if !recycleContent["trashed primary private to note owner"] {
		t.Fatalf("comment recycle bin hid a related primary interaction from its delegated participant: %#v", recycleContent)
	}
}
