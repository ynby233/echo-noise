package services

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/rcy1314/echo-noise/internal/models"
)

func TestTrashCommentCascadesWithoutCreatingOrphans(t *testing.T) {
	db := setupUserServiceTestDB(t)
	owner := mustCreateUser(t, models.User{Username: "thread-owner"})
	replier := mustCreateUser(t, models.User{Username: "thread-replier"})
	message := models.Message{Content: "thread", Username: owner.Username, UserID: owner.ID, Visibility: MessageVisibilityPublic}
	if err := db.Create(&message).Error; err != nil {
		t.Fatalf("create message: %v", err)
	}
	root := models.Comment{MessageID: message.ID, UserID: &owner.ID, Content: "root", Visibility: "public"}
	if err := db.Create(&root).Error; err != nil {
		t.Fatalf("create root: %v", err)
	}
	child := models.Comment{MessageID: message.ID, UserID: &replier.ID, Content: "child", Visibility: "public", ParentID: &root.ID}
	if err := db.Create(&child).Error; err != nil {
		t.Fatalf("create child: %v", err)
	}
	grandchild := models.Comment{MessageID: message.ID, UserID: &owner.ID, Content: "grandchild", Visibility: "public", ParentID: &child.ID}
	if err := db.Create(&grandchild).Error; err != nil {
		t.Fatalf("create grandchild: %v", err)
	}

	result, err := TrashCommentTree(db, owner.ID, root.ID, CommentTrashRequest{ReasonCode: CommentDeletionReasonSelf})
	if err != nil {
		t.Fatalf("trash root tree: %v", err)
	}
	if result.Trashed != 3 || result.AlreadyTrashed != 0 {
		t.Fatalf("trash result = %#v, want three newly trashed comments", result)
	}

	var stored []models.Comment
	if err := db.Where("message_id = ?", message.ID).Order("id ASC").Find(&stored).Error; err != nil {
		t.Fatalf("reload thread: %v", err)
	}
	if len(stored) != 3 {
		t.Fatalf("thread rows = %d, want 3", len(stored))
	}
	for _, comment := range stored {
		if comment.DeletedAt == nil {
			t.Fatalf("comment %d was not moved to recycle bin", comment.ID)
		}
		if comment.DeletedByUserID == nil || *comment.DeletedByUserID != owner.ID {
			t.Fatalf("comment %d deleted_by = %#v, want %d", comment.ID, comment.DeletedByUserID, owner.ID)
		}
	}
	if stored[0].DeletionReasonCode != CommentDeletionReasonSelf {
		t.Fatalf("root reason = %q", stored[0].DeletionReasonCode)
	}
	if stored[1].DeletionReasonCode != CommentDeletionReasonAncestor || stored[1].DeletedAncestorCommentID == nil || *stored[1].DeletedAncestorCommentID != root.ID {
		t.Fatalf("child lifecycle metadata = %#v", stored[1])
	}
	if stored[2].DeletionReasonCode != CommentDeletionReasonAncestor || stored[2].DeletedAncestorCommentID == nil || *stored[2].DeletedAncestorCommentID != root.ID {
		t.Fatalf("grandchild lifecycle metadata = %#v", stored[2])
	}
	if stored[1].ParentID == nil || *stored[1].ParentID != root.ID || stored[2].ParentID == nil || *stored[2].ParentID != child.ID {
		t.Fatalf("parent chain changed: %#v", stored)
	}
}

func TestRestoreCommentRequiresEveryRecoverableAncestor(t *testing.T) {
	db := setupUserServiceTestDB(t)
	owner := mustCreateUser(t, models.User{Username: "restore-owner"})
	replier := mustCreateUser(t, models.User{Username: "restore-replier"})
	message := models.Message{Content: "restore thread", Username: owner.Username, UserID: owner.ID, Visibility: MessageVisibilityPublic}
	if err := db.Create(&message).Error; err != nil {
		t.Fatalf("create message: %v", err)
	}
	root := models.Comment{MessageID: message.ID, UserID: &owner.ID, Content: "root", Visibility: "public"}
	if err := db.Create(&root).Error; err != nil {
		t.Fatalf("create root: %v", err)
	}
	child := models.Comment{MessageID: message.ID, UserID: &replier.ID, Content: "child", Visibility: "public", ParentID: &root.ID}
	if err := db.Create(&child).Error; err != nil {
		t.Fatalf("create child: %v", err)
	}
	if _, err := TrashCommentTree(db, owner.ID, root.ID, CommentTrashRequest{ReasonCode: CommentDeletionReasonSelf}); err != nil {
		t.Fatalf("trash tree: %v", err)
	}

	if err := RestoreComment(db, replier.ID, child.ID); !errors.Is(err, ErrCommentAncestorPending) {
		t.Fatalf("restore child with recoverable parent = %v, want %v", err, ErrCommentAncestorPending)
	}
	if err := RestoreComment(db, owner.ID, root.ID); err != nil {
		t.Fatalf("restore root: %v", err)
	}
	if err := RestoreComment(db, replier.ID, child.ID); err != nil {
		t.Fatalf("author restore child after parent: %v", err)
	}
}

func TestRestoreCommentAllowsTombstoneAncestor(t *testing.T) {
	db := setupUserServiceTestDB(t)
	owner := mustCreateUser(t, models.User{Username: "tombstone-owner"})
	replier := mustCreateUser(t, models.User{Username: "tombstone-replier"})
	message := models.Message{Content: "tombstone thread", Username: owner.Username, UserID: owner.ID, Visibility: MessageVisibilityPublic}
	if err := db.Create(&message).Error; err != nil {
		t.Fatalf("create message: %v", err)
	}
	now := time.Now().UTC()
	root := models.Comment{MessageID: message.ID, Content: "", Visibility: "private", IsTombstone: true, TombstonedAt: &now}
	if err := db.Create(&root).Error; err != nil {
		t.Fatalf("create tombstone: %v", err)
	}
	child := models.Comment{MessageID: message.ID, UserID: &replier.ID, Content: "child", Visibility: "public", ParentID: &root.ID, DeletedAt: &now, DeletedByUserID: &owner.ID, DeletionReasonCode: CommentDeletionReasonAncestor}
	if err := db.Create(&child).Error; err != nil {
		t.Fatalf("create trashed child: %v", err)
	}

	if err := RestoreComment(db, replier.ID, child.ID); err != nil {
		t.Fatalf("author restore below tombstone: %v", err)
	}
	var stored models.Comment
	if err := db.First(&stored, child.ID).Error; err != nil || stored.DeletedAt != nil {
		t.Fatalf("restored child = %#v err=%v", stored, err)
	}
}

func TestOrdinaryUserPurgeLeavesHiddenAdministrativeCopy(t *testing.T) {
	db := setupUserServiceTestDB(t)
	mustCreateUser(t, models.User{ID: models.PrimaryAdminUserID, Username: "purge-primary", IsAdmin: true})
	user := mustCreateUser(t, models.User{Username: "ordinary-purge"})
	message := models.Message{Content: "purge", Username: user.Username, UserID: user.ID, Visibility: MessageVisibilityPublic}
	if err := db.Create(&message).Error; err != nil {
		t.Fatal(err)
	}
	comment := models.Comment{MessageID: message.ID, UserID: &user.ID, Content: "retain for admin", Visibility: "public"}
	if err := db.Create(&comment).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := TrashCommentTree(db, user.ID, comment.ID, CommentTrashRequest{ReasonCode: CommentDeletionReasonSelf}); err != nil {
		t.Fatal(err)
	}
	retained, err := UserPurgeComment(db, user.ID, comment.ID)
	if err != nil || !retained {
		t.Fatalf("purge retained=%v err=%v", retained, err)
	}
	var stored models.Comment
	if err := db.First(&stored, comment.ID).Error; err != nil {
		t.Fatalf("administrative copy missing: %v", err)
	}
	if stored.UserPurgedAt == nil || stored.Content != comment.Content {
		t.Fatalf("administrative copy = %#v", stored)
	}
	if err := RestoreComment(db, user.ID, comment.ID); !errors.Is(err, ErrCommentUserPurged) {
		t.Fatalf("restore user-purged = %v", err)
	}
	personal, err := ListCommentManagement(db, user.ID, CommentManagementFilter{Page: 1, PageSize: 20}, true, true, time.Now().UTC())
	if err != nil || personal.Total != 0 {
		t.Fatalf("user-purged content leaked into personal recycle bin: total=%d err=%v", personal.Total, err)
	}
	admin, err := ListCommentManagement(db, models.PrimaryAdminUserID, CommentManagementFilter{Page: 1, PageSize: 20}, true, false, time.Now().UTC())
	if err != nil || admin.Total != 1 || !admin.Items[0].UserPurged || admin.Items[0].CanRestore {
		t.Fatalf("administrative retention copy = %#v err=%v", admin, err)
	}
}

func TestPermanentDeleteKeepsVisibilityCeilingTombstoneForChildren(t *testing.T) {
	db := setupUserServiceTestDB(t)
	primary := mustCreateUser(t, models.User{ID: models.PrimaryAdminUserID, Username: "primary-tombstone", IsAdmin: true})
	user := mustCreateUser(t, models.User{Username: "child-owner"})
	message := models.Message{Content: "thread", Username: primary.Username, UserID: primary.ID, Visibility: MessageVisibilityPublic}
	if err := db.Create(&message).Error; err != nil {
		t.Fatal(err)
	}
	root := models.Comment{MessageID: message.ID, UserID: &primary.ID, Content: "private root", Visibility: "private"}
	if err := db.Create(&root).Error; err != nil {
		t.Fatal(err)
	}
	child := models.Comment{MessageID: message.ID, UserID: &user.ID, Content: "child", Visibility: "public", ParentID: &root.ID}
	if err := db.Create(&child).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := TrashCommentTree(db, primary.ID, root.ID, CommentTrashRequest{ReasonCode: CommentDeletionReasonSelf}); err != nil {
		t.Fatal(err)
	}
	if err := PermanentlyDeleteComment(db, primary.ID, root.ID); err != nil {
		t.Fatal(err)
	}
	var tombstone models.Comment
	if err := db.First(&tombstone, root.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !tombstone.IsTombstone || tombstone.Content != "" || tombstone.UserID != nil || tombstone.TombstoneVisibility != "private" {
		t.Fatalf("tombstone = %#v", tombstone)
	}
	commentMap := CommentMap([]models.Comment{tombstone, child})
	if got := EffectiveCommentVisibilityInThread(child, MessageVisibilityPublic, commentMap); got != "private" {
		t.Fatalf("child effective visibility = %q, want private", got)
	}
}

func TestTrashAndPermanentDeleteNoteCascadeCommentsThenKeepThreadTombstone(t *testing.T) {
	db := setupUserServiceTestDB(t)
	owner := mustCreateUser(t, models.User{ID: models.PrimaryAdminUserID, Username: "note-tombstone-owner", IsAdmin: true})
	other := mustCreateUser(t, models.User{Username: "note-commenter"})
	message := models.Message{Content: "private note", Username: owner.Username, UserID: owner.ID, Visibility: MessageVisibilityPrivate, Private: true}
	if err := db.Create(&message).Error; err != nil {
		t.Fatal(err)
	}
	comment := models.Comment{MessageID: message.ID, UserID: &other.ID, Content: "surviving interaction", Visibility: "private"}
	if err := db.Create(&comment).Error; err != nil {
		t.Fatal(err)
	}
	if err := TrashMessage(db, owner.ID, message.ID, "author request"); err != nil {
		t.Fatal(err)
	}
	var trashed models.Comment
	if err := db.First(&trashed, comment.ID).Error; err != nil {
		t.Fatal(err)
	}
	if trashed.DeletedAt == nil || trashed.DeletedAncestorMessageID == nil || *trashed.DeletedAncestorMessageID != message.ID {
		t.Fatalf("cascaded comment = %#v", trashed)
	}
	if err := PermanentlyDeleteMessage(db, owner.ID, message.ID, "author request"); err != nil {
		t.Fatal(err)
	}
	var tombstone models.Message
	if err := db.First(&tombstone, message.ID).Error; err != nil {
		t.Fatalf("thread tombstone missing: %v", err)
	}
	if !tombstone.IsTombstone || tombstone.Content != "" || tombstone.TombstoneVisibility != MessageVisibilityPrivate {
		t.Fatalf("message tombstone = %#v", tombstone)
	}
	if err := db.First(&trashed, comment.ID).Error; err != nil {
		t.Fatalf("child interaction was deleted: %v", err)
	}
	if err := RestoreComment(db, other.ID, comment.ID); err != nil {
		t.Fatalf("restore interaction beneath note tombstone: %v", err)
	}
	viewerID := owner.ID
	thread, err := GetMessageByIDForViewer(message.ID, &viewerID, true)
	if err != nil {
		t.Fatalf("load structural thread tombstone: %v", err)
	}
	if !thread.IsTombstone || thread.UserID != 0 || thread.Username != "" || thread.Content != "" || thread.CanInteract {
		t.Fatalf("thread tombstone response leaked original note data: %#v", thread)
	}
}

func TestOwnerCleanupNotifiesEveryAffectedAuthorWithSafeSnapshotAndDeadline(t *testing.T) {
	db := setupUserServiceTestDB(t)
	mustCreateUser(t, models.User{ID: models.PrimaryAdminUserID, Username: "notify-primary", IsAdmin: true})
	owner := mustCreateUser(t, models.User{Username: "cleanup-owner"})
	bob := mustCreateUser(t, models.User{Username: "cleanup-bob"})
	charlie := mustCreateUser(t, models.User{Username: "cleanup-charlie"})
	if err := db.Create(&models.SiteConfig{CommentRecycleBinRetentionDays: 7}).Error; err != nil {
		t.Fatal(err)
	}
	message := models.Message{Content: "owner note", Username: owner.Username, UserID: owner.ID, Visibility: MessageVisibilityPublic}
	if err := db.Create(&message).Error; err != nil {
		t.Fatal(err)
	}
	root := models.Comment{MessageID: message.ID, UserID: &bob.ID, Content: "root <script>alert(1)</script> [file](/api/files/refs/secret/report.pdf)", Visibility: "public"}
	if err := db.Create(&root).Error; err != nil {
		t.Fatal(err)
	}
	reply := models.Comment{MessageID: message.ID, UserID: &charlie.ID, Content: "reply", Visibility: "public", ParentID: &root.ID}
	if err := db.Create(&reply).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := TrashCommentTree(db, owner.ID, root.ID, CommentTrashRequest{}); err != nil {
		t.Fatal(err)
	}
	var notifications []models.UserNotification
	if err := db.Where("type = ?", models.UserNotificationTypeContentDeletion).Order("recipient_user_id").Find(&notifications).Error; err != nil {
		t.Fatal(err)
	}
	if len(notifications) != 2 {
		t.Fatalf("notifications = %#v", notifications)
	}
	for _, notification := range notifications {
		if notification.DeletionActorLabel != owner.Username || notification.ScheduledDeletionAt == nil {
			t.Fatalf("notification metadata = %#v", notification)
		}
		if strings.Contains(notification.DeletionSnapshotJSON, "<script") || strings.Contains(notification.DeletionSnapshotJSON, "/api/files/refs/") {
			t.Fatalf("unsafe snapshot = %s", notification.DeletionSnapshotJSON)
		}
		var items []DeletionSnapshotItem
		if err := json.Unmarshal([]byte(notification.DeletionSnapshotJSON), &items); err != nil || len(items) != 1 {
			t.Fatalf("snapshot items=%#v err=%v", items, err)
		}
		if notification.RecipientUserID == charlie.ID && (!strings.Contains(items[0].ContextText, "上级互动") || items[0].ReasonCode != CommentDeletionReasonAncestor) {
			t.Fatalf("reply notification = %#v", items[0])
		}
	}
}

func TestCommentAutoCleanupNotifiesAuthorAsSystem(t *testing.T) {
	db := setupUserServiceTestDB(t)
	primary := mustCreateUser(t, models.User{ID: models.PrimaryAdminUserID, Username: "auto-primary", IsAdmin: true})
	user := mustCreateUser(t, models.User{Username: "auto-comment-owner"})
	if err := db.Create(&models.SiteConfig{CommentRecycleBinRetentionDays: 7}).Error; err != nil {
		t.Fatal(err)
	}
	message := models.Message{Content: "auto", Username: primary.Username, UserID: primary.ID, Visibility: MessageVisibilityPublic}
	if err := db.Create(&message).Error; err != nil {
		t.Fatal(err)
	}
	deletedAt := time.Now().UTC().AddDate(0, 0, -8)
	comment := models.Comment{MessageID: message.ID, UserID: &user.ID, Content: "expired", Visibility: "public", DeletedAt: &deletedAt, DeletedByUserID: &user.ID, DeletionReasonCode: CommentDeletionReasonSelf}
	if err := db.Create(&comment).Error; err != nil {
		t.Fatal(err)
	}
	succeeded, failed, err := RunCommentRecycleBinAutoCleanup(db, time.Now().UTC())
	if err != nil || succeeded != 1 || failed != 0 {
		t.Fatalf("cleanup succeeded=%d failed=%d err=%v", succeeded, failed, err)
	}
	var notification models.UserNotification
	if err := db.Where("recipient_user_id = ? AND type = ?", user.ID, models.UserNotificationTypeContentDeletion).First(&notification).Error; err != nil {
		t.Fatal(err)
	}
	if notification.DeletionEvent != DeletionEventPermanentlyDeleted || notification.DeletionActorLabel != "系统定时清理" {
		t.Fatalf("notification = %#v", notification)
	}
}
