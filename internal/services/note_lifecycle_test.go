package services

import (
	"errors"
	"testing"
	"time"

	"github.com/rcy1314/echo-noise/internal/authorization"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
)

func TestNoteLifecycleTrashRestoreIsMutuallyExclusive(t *testing.T) {
	db := setupUserServiceTestDB(t)
	primary := mustCreateUser(t, models.User{ID: models.PrimaryAdminUserID, Username: "lifecycle-primary", IsAdmin: true})
	author := mustCreateUser(t, models.User{Username: "lifecycle-author"})
	message := models.Message{
		Content:    "lifecycle note",
		Username:   author.Username,
		UserID:     author.ID,
		Visibility: MessageVisibilityPublic,
	}
	if err := db.Create(&message).Error; err != nil {
		t.Fatalf("create message: %v", err)
	}

	if err := TrashMessage(db, author.ID, message.ID, "author request"); err != nil {
		t.Fatalf("trash message: %v", err)
	}
	var stored models.Message
	if err := db.Unscoped().First(&stored, message.ID).Error; err != nil {
		t.Fatalf("load trashed message: %v", err)
	}
	if stored.DeletedAt == nil || stored.DeletedByUserID == nil || *stored.DeletedByUserID != author.ID || stored.DeletedReason != "author request" {
		t.Fatalf("trash metadata = %#v, want deleted metadata", stored)
	}
	var active []models.Message
	if err := ApplyMessageVisibilityScope(database.DB.Model(&models.Message{}), &author.ID, false).Find(&active).Error; err != nil {
		t.Fatalf("query active messages: %v", err)
	}
	if len(active) != 0 {
		t.Fatalf("trashed message remains in active query: %#v", active)
	}
	activeResult, err := ListNoteManagementMessages(db, primary.ID, NoteManagementFilter{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("list active notes: %v", err)
	}
	if activeResult.Total != 0 {
		t.Fatalf("active management total = %d, want 0", activeResult.Total)
	}
	recycleResult, err := ListRecycleBinMessages(db, primary.ID, NoteManagementFilter{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("list recycle bin: %v", err)
	}
	if recycleResult.Total != 1 || len(recycleResult.Items) != 1 || recycleResult.Items[0].DeletedAt == nil {
		t.Fatalf("recycle result = %#v, want one deleted note", recycleResult)
	}
	if err := TrashMessage(db, author.ID, message.ID, "duplicate"); !errors.Is(err, ErrMessageAlreadyTrashed) {
		t.Fatalf("duplicate trash error = %v, want %v", err, ErrMessageAlreadyTrashed)
	}
	if err := RestoreMessage(db, primary.ID, message.ID); err != nil {
		t.Fatalf("restore message: %v", err)
	}
	if err := RestoreMessage(db, primary.ID, message.ID); !errors.Is(err, ErrMessageNotTrashed) {
		t.Fatalf("duplicate restore error = %v, want %v", err, ErrMessageNotTrashed)
	}
}

func TestRunRecycleBinAutoCleanupUsesDeletedAtAndSystemIdentity(t *testing.T) {
	db := setupUserServiceTestDB(t)
	primary := mustCreateUser(t, models.User{ID: models.PrimaryAdminUserID, Username: "auto-primary", IsAdmin: true})
	author := mustCreateUser(t, models.User{Username: "auto-author"})
	if err := db.Create(&models.SiteConfig{RecycleBinRetentionDays: 7}).Error; err != nil {
		t.Fatal(err)
	}
	old := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	m := models.Message{Content: "expired", Username: author.Username, UserID: author.ID, Visibility: MessageVisibilityPublic, DeletedAt: &old}
	if err := db.Create(&m).Error; err != nil {
		t.Fatal(err)
	}
	newer := old.AddDate(0, 0, 8)
	if succeeded, failed, skipped, err := RunRecycleBinAutoCleanup(db, newer); err != nil || succeeded != 1 || failed != 0 || skipped != 0 {
		t.Fatalf("cleanup = %d,%d,%d,%v", succeeded, failed, skipped, err)
	}
	if err := db.Unscoped().First(&models.Message{}, m.ID).Error; !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("expired message still exists: %v", err)
	}
	var audit models.AdminAuditLog
	if err := db.Where("action = ? AND target_id = ?", "auto_permanent_delete", m.ID).First(&audit).Error; err != nil {
		t.Fatal(err)
	}
	if audit.ActorUserID != primary.ID || audit.Result != "success" {
		t.Fatalf("audit = %#v", audit)
	}
}

func TestRunRecycleBinAutoCleanupIncludesPrimaryNotesAndLeavesGuestbook(t *testing.T) {
	db := setupUserServiceTestDB(t)
	primary := mustCreateUser(t, models.User{ID: models.PrimaryAdminUserID, Username: "auto-primary-all", IsAdmin: true})
	if err := db.Create(&models.SiteConfig{RecycleBinRetentionDays: 7}).Error; err != nil {
		t.Fatal(err)
	}
	old := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	primaryNote := models.Message{Content: "primary ordinary expired", Username: primary.Username, UserID: primary.ID, Visibility: MessageVisibilityPublic, DeletedAt: &old}
	guestbook := models.Message{Content: "guestbook must remain", Username: primary.Username, UserID: primary.ID, Visibility: MessageVisibilityPublic, IsGuestbook: true, DeletedAt: &old}
	if err := db.Create(&primaryNote).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&guestbook).Error; err != nil {
		t.Fatal(err)
	}
	succeeded, failed, skipped, err := RunRecycleBinAutoCleanup(db, old.AddDate(0, 0, 8))
	if err != nil || succeeded != 1 || failed != 0 || skipped != 1 {
		t.Fatalf("cleanup = %d,%d,%d,%v", succeeded, failed, skipped, err)
	}
	if err := db.Unscoped().First(&models.Message{}, primaryNote.ID).Error; !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("primary ordinary note remains: %v", err)
	}
	var retained models.Message
	if err := db.Unscoped().First(&retained, guestbook.ID).Error; err != nil || retained.DeletedAt == nil {
		t.Fatalf("guestbook should remain trashed, err=%v row=%#v", err, retained)
	}
}

func TestLifecycleMutationRollsBackWhenSuccessAuditFails(t *testing.T) {
	db := setupUserServiceTestDB(t)
	author := mustCreateUser(t, models.User{Username: "audit-rollback-author"})
	message := models.Message{Content: "audit rollback", Username: author.Username, UserID: author.ID, Visibility: MessageVisibilityPublic}
	if err := db.Create(&message).Error; err != nil {
		t.Fatalf("create message: %v", err)
	}
	auditErr := errors.New("audit write failed")
	if err := TrashMessageWithAudit(db, author.ID, message.ID, "test", func(*gorm.DB) error { return auditErr }); !errors.Is(err, auditErr) {
		t.Fatalf("expected audit error, got %v", err)
	}
	var stored models.Message
	if err := db.First(&stored, message.ID).Error; err != nil {
		t.Fatalf("reload message: %v", err)
	}
	if stored.DeletedAt != nil {
		t.Fatalf("message changed despite audit rollback: %#v", stored)
	}
}

func TestPermanentlyDeleteMessageClearsLocalRelationsButKeepsNotifications(t *testing.T) {
	db := setupUserServiceTestDB(t)
	primary := mustCreateUser(t, models.User{ID: models.PrimaryAdminUserID, Username: "lifecycle-permanent-primary", IsAdmin: true})
	delegated := mustCreateUser(t, models.User{Username: "lifecycle-permanent-delegated", IsAdmin: true})
	author := mustCreateUser(t, models.User{Username: "lifecycle-permanent-author"})
	if err := authorization.New(db).ReplaceGrants(primary.ID, delegated.ID, []authorization.Capability{
		authorization.CapabilityNotesTrash,
		authorization.CapabilityNotesRecycleBinView,
		authorization.CapabilityNotesDelete,
	}); err != nil {
		t.Fatalf("grant lifecycle capabilities: %v", err)
	}
	message := models.Message{Content: "permanent lifecycle note", Username: author.Username, UserID: author.ID, Visibility: MessageVisibilityPublic}
	if err := db.Create(&message).Error; err != nil {
		t.Fatalf("create message: %v", err)
	}
	comment := models.Comment{MessageID: message.ID, UserID: &author.ID, Content: "comment", Visibility: MessageVisibilityPublic}
	if err := db.Create(&comment).Error; err != nil {
		t.Fatalf("create comment: %v", err)
	}
	like := models.MessageLike{MessageID: message.ID, UserID: &delegated.ID}
	if err := db.Create(&like).Error; err != nil {
		t.Fatalf("create like: %v", err)
	}
	notification := models.UserNotification{RecipientUserID: author.ID, ActorUserID: &delegated.ID, Type: models.UserNotificationTypeLike, MessageID: &message.ID}
	if err := db.Create(&notification).Error; err != nil {
		t.Fatalf("create notification: %v", err)
	}
	if err := TrashMessage(db, delegated.ID, message.ID, "moderation"); err != nil {
		t.Fatalf("trash message: %v", err)
	}
	if err := PermanentlyDeleteMessage(db, delegated.ID, message.ID, "moderation"); err != nil {
		t.Fatalf("permanently delete message: %v", err)
	}
	var count int64
	if err := db.Unscoped().Model(&models.Message{}).Where("id = ?", message.ID).Count(&count).Error; err != nil {
		t.Fatalf("count message: %v", err)
	}
	if count != 0 {
		t.Fatalf("message remains after permanent delete: %d", count)
	}
	if err := db.Model(&models.Comment{}).Where("message_id = ?", message.ID).Count(&count).Error; err != nil {
		t.Fatalf("count comments: %v", err)
	}
	if count != 0 {
		t.Fatalf("comments remain after permanent delete: %d", count)
	}
	if err := db.Model(&models.MessageLike{}).Where("message_id = ?", message.ID).Count(&count).Error; err != nil {
		t.Fatalf("count likes: %v", err)
	}
	if count != 0 {
		t.Fatalf("likes remain after permanent delete: %d", count)
	}
	if err := db.Model(&models.UserNotification{}).Where("id = ?", notification.ID).Count(&count).Error; err != nil {
		t.Fatalf("count notification: %v", err)
	}
	if count != 1 {
		t.Fatalf("notification history count = %d, want 1", count)
	}
}

func TestRecycleBinVisibilitySeparatesHiddenOrdinaryAndPrimaryContent(t *testing.T) {
	db := setupUserServiceTestDB(t)
	primary := mustCreateUser(t, models.User{ID: models.PrimaryAdminUserID, Username: "recycle-visibility-primary", IsAdmin: true})
	delegated := mustCreateUser(t, models.User{Username: "recycle-visibility-delegated", IsAdmin: true})
	ordinary := mustCreateUser(t, models.User{Username: "recycle-visibility-ordinary"})
	authorizer := authorization.New(db)
	baseGrants := []authorization.Capability{authorization.CapabilityNotesRecycleBinView, authorization.CapabilityNotesRestore, authorization.CapabilityNotesDelete}
	if err := authorizer.ReplaceGrants(primary.ID, delegated.ID, baseGrants); err != nil {
		t.Fatalf("grant recycle-bin capabilities: %v", err)
	}
	now := time.Now().UTC()
	ordinaryHidden := models.Message{Content: "ordinary hidden recycle note", Username: ordinary.Username, UserID: ordinary.ID, Private: true, Visibility: MessageVisibilityPrivate, DeletedAt: &now}
	primaryHidden := models.Message{Content: "primary hidden recycle note", Username: primary.Username, UserID: primary.ID, Private: true, Visibility: MessageVisibilityPrivate, DeletedAt: &now}
	for _, message := range []*models.Message{&ordinaryHidden, &primaryHidden} {
		if err := db.Create(message).Error; err != nil {
			t.Fatalf("create hidden recycle note: %v", err)
		}
	}
	withoutHidden, err := ListRecycleBinMessages(db, delegated.ID, NoteManagementFilter{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("list recycle bin without hidden grant: %v", err)
	}
	if withoutHidden.Total != 0 {
		t.Fatalf("without content.view_hidden total=%d, want 0", withoutHidden.Total)
	}
	if _, err := GetRecycleBinMessageForViewer(db, delegated.ID, ordinaryHidden.ID); !errors.Is(err, ErrMessageNotVisible) {
		t.Fatalf("hidden ordinary detail without grant=%v, want %v", err, ErrMessageNotVisible)
	}
	if err := authorizer.ReplaceGrants(primary.ID, delegated.ID, append(baseGrants, authorization.CapabilityContentViewHidden)); err != nil {
		t.Fatalf("grant hidden read: %v", err)
	}
	withHidden, err := ListRecycleBinMessages(db, delegated.ID, NoteManagementFilter{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("list recycle bin with hidden grant: %v", err)
	}
	if withHidden.Total != 1 || len(withHidden.Items) != 1 || withHidden.Items[0].ID != ordinaryHidden.ID {
		t.Fatalf("with content.view_hidden result=%#v, want only ordinary hidden note", withHidden)
	}
	item, err := GetRecycleBinMessageForViewer(db, delegated.ID, ordinaryHidden.ID)
	if err != nil || item.DeletedAt == nil {
		t.Fatalf("ordinary hidden recycle detail=%#v err=%v, want deletion metadata", item, err)
	}
	if _, err := GetRecycleBinMessageForViewer(db, delegated.ID, primaryHidden.ID); !errors.Is(err, ErrMessageNotVisible) {
		t.Fatalf("primary hidden detail=%v, want %v", err, ErrMessageNotVisible)
	}
	if err := RestoreMessage(db, delegated.ID, primaryHidden.ID); !errors.Is(err, ErrMessageNotVisible) {
		t.Fatalf("primary hidden restore=%v, want %v", err, ErrMessageNotVisible)
	}
	if err := PermanentlyDeleteMessage(db, delegated.ID, primaryHidden.ID, "test"); !errors.Is(err, ErrMessageNotVisible) {
		t.Fatalf("primary hidden permanent delete=%v, want %v", err, ErrMessageNotVisible)
	}
}

func TestNoteLifecycleRejectsInvalidAndRepeatedTransitions(t *testing.T) {
	db := setupUserServiceTestDB(t)
	primary := mustCreateUser(t, models.User{ID: models.PrimaryAdminUserID, Username: "transition-primary", IsAdmin: true})
	author := mustCreateUser(t, models.User{Username: "transition-author"})
	message := models.Message{Content: "transition note", Username: author.Username, UserID: author.ID, Visibility: MessageVisibilityPublic}
	if err := db.Create(&message).Error; err != nil {
		t.Fatalf("create transition note: %v", err)
	}
	if err := TrashMessage(db, author.ID, 999999, "missing"); !errors.Is(err, ErrMessageNotFound) {
		t.Fatalf("missing trash=%v, want %v", err, ErrMessageNotFound)
	}
	if err := RestoreMessage(db, primary.ID, message.ID); !errors.Is(err, ErrMessageNotTrashed) {
		t.Fatalf("active restore=%v, want %v", err, ErrMessageNotTrashed)
	}
	if err := PermanentlyDeleteMessage(db, primary.ID, message.ID, "active"); !errors.Is(err, ErrMessageNotTrashed) {
		t.Fatalf("active permanent delete=%v, want %v", err, ErrMessageNotTrashed)
	}
	if err := TrashMessage(db, author.ID, message.ID, "first"); err != nil {
		t.Fatalf("first trash=%v", err)
	}
	if err := TrashMessage(db, author.ID, message.ID, "second"); !errors.Is(err, ErrMessageAlreadyTrashed) {
		t.Fatalf("repeated trash=%v, want %v", err, ErrMessageAlreadyTrashed)
	}
	if err := RestoreMessage(db, primary.ID, message.ID); err != nil {
		t.Fatalf("restore=%v", err)
	}
	if err := RestoreMessage(db, primary.ID, message.ID); !errors.Is(err, ErrMessageNotTrashed) {
		t.Fatalf("repeated restore=%v, want %v", err, ErrMessageNotTrashed)
	}
	if err := TrashMessage(db, author.ID, message.ID, "second lifecycle"); err != nil {
		t.Fatalf("second trash=%v", err)
	}
	if err := PermanentlyDeleteMessage(db, primary.ID, message.ID, "final"); err != nil {
		t.Fatalf("permanent delete=%v", err)
	}
	if err := PermanentlyDeleteMessage(db, primary.ID, message.ID, "again"); !errors.Is(err, ErrMessageNotFound) {
		t.Fatalf("repeated permanent delete=%v, want %v", err, ErrMessageNotFound)
	}
}
