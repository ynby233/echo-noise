package services

import (
	"testing"
	"time"

	"github.com/rcy1314/echo-noise/internal/models"
)

func TestCommentManagementMarksContentOwnerCleanupAsAvailable(t *testing.T) {
	db := setupUserServiceTestDB(t)
	owner := mustCreateUser(t, models.User{Username: "management-owner"})
	commenter := mustCreateUser(t, models.User{Username: "management-commenter"})
	message := models.Message{Content: "owned note", Username: owner.Username, UserID: owner.ID, Visibility: MessageVisibilityPublic}
	if err := db.Create(&message).Error; err != nil {
		t.Fatal(err)
	}
	comment := models.Comment{MessageID: message.ID, UserID: &commenter.ID, Content: "other user's comment", Visibility: "public"}
	if err := db.Create(&comment).Error; err != nil {
		t.Fatal(err)
	}

	page, err := ListCommentManagement(db, owner.ID, CommentManagementFilter{Page: 1, PageSize: 20}, false, false, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || !page.Items[0].CanTrash {
		t.Fatalf("content owner management item = %#v, want can_trash", page.Items)
	}
}
