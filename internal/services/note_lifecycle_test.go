package services

import (
	"errors"
	"fmt"
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

func TestPrimaryAdminCanTrashDelegatedPrivateNoteContainingPrimaryComment(t *testing.T) {
	db := setupUserServiceTestDB(t)
	primary := mustCreateUser(t, models.User{ID: models.PrimaryAdminUserID, Username: "primary-trash", IsAdmin: true})
	author := mustCreateUser(t, models.User{Username: "delegated-author", IsAdmin: true})
	message := models.Message{Content: "delegated private note", Username: author.Username, UserID: author.ID, Private: true, Visibility: MessageVisibilityPrivate}
	if err := db.Create(&message).Error; err != nil {
		t.Fatalf("create delegated private note: %v", err)
	}
	comment := models.Comment{MessageID: message.ID, UserID: &primary.ID, Content: "primary comment", Visibility: MessageVisibilityPrivate}
	if err := db.Create(&comment).Error; err != nil {
		t.Fatalf("create primary comment: %v", err)
	}

	if err := TrashMessage(db, primary.ID, message.ID, "primary moderation"); err != nil {
		t.Fatalf("primary administrator must be able to trash delegated private note containing their own comment: %v", err)
	}
	var stored models.Message
	if err := db.First(&stored, message.ID).Error; err != nil {
		t.Fatalf("reload trashed note: %v", err)
	}
	if stored.DeletedAt == nil || stored.DeletedByUserID == nil || *stored.DeletedByUserID != primary.ID {
		t.Fatalf("trash metadata = %#v, want primary administrator deletion", stored)
	}
}

func TestPrimaryCommentProtectionAppliesToDelegatedPermanentDelete(t *testing.T) {
	db := setupUserServiceTestDB(t)
	primary := mustCreateUser(t, models.User{ID: models.PrimaryAdminUserID, Username: "primary-protection", IsAdmin: true})
	author := mustCreateUser(t, models.User{Username: "delegated-owner", IsAdmin: true})
	moderator := mustCreateUser(t, models.User{Username: "delegated-moderator", IsAdmin: true})
	if err := authorization.New(db).ReplaceGrants(primary.ID, moderator.ID, []authorization.Capability{
		authorization.CapabilityContentViewHidden,
		authorization.CapabilityNotesRecycleBinView,
		authorization.CapabilityNotesDelete,
	}); err != nil {
		t.Fatalf("grant delegated recycle-bin capabilities: %v", err)
	}
	message := models.Message{Content: "protected delegated note", Username: author.Username, UserID: author.ID, Private: true, Visibility: MessageVisibilityPrivate}
	if err := db.Create(&message).Error; err != nil {
		t.Fatalf("create delegated private note: %v", err)
	}
	comment := models.Comment{MessageID: message.ID, UserID: &primary.ID, Content: "protected primary comment", Visibility: MessageVisibilityPrivate}
	if err := db.Create(&comment).Error; err != nil {
		t.Fatalf("create primary comment: %v", err)
	}
	if err := TrashMessage(db, author.ID, message.ID, "author request"); err != nil {
		t.Fatalf("author must be able to trash own note: %v", err)
	}

	if err := PermanentlyDeleteMessage(db, moderator.ID, message.ID, "delegated moderation"); !errors.Is(err, ErrMessageProtected) {
		t.Fatalf("delegated administrator permanent delete error = %v, want %v", err, ErrMessageProtected)
	}
	if err := PermanentlyDeleteMessage(db, primary.ID, message.ID, "primary moderation"); err != nil {
		t.Fatalf("primary administrator must be able to permanently delete protected note: %v", err)
	}
}

func TestPrimaryCommentProtectionAllowsAuthorTrashAndBlocksDelegatedTrash(t *testing.T) {
	db := setupUserServiceTestDB(t)
	primary := mustCreateUser(t, models.User{ID: models.PrimaryAdminUserID, Username: "primary-thread", IsAdmin: true})
	author := mustCreateUser(t, models.User{Username: "ordinary-author"})
	moderator := mustCreateUser(t, models.User{Username: "delegated-trash-moderator", IsAdmin: true})
	if err := authorization.New(db).ReplaceGrants(primary.ID, moderator.ID, []authorization.Capability{
		authorization.CapabilityContentViewHidden,
		authorization.CapabilityNotesTrash,
	}); err != nil {
		t.Fatalf("grant delegated trash capabilities: %v", err)
	}
	message := models.Message{Content: "ordinary private note with primary reply", Username: author.Username, UserID: author.ID, Private: true, Visibility: MessageVisibilityPrivate}
	if err := db.Create(&message).Error; err != nil {
		t.Fatalf("create ordinary private note: %v", err)
	}
	comment := models.Comment{MessageID: message.ID, UserID: &primary.ID, Content: "primary reply", Visibility: MessageVisibilityPrivate}
	if err := db.Create(&comment).Error; err != nil {
		t.Fatalf("create primary reply: %v", err)
	}

	if err := TrashMessage(db, moderator.ID, message.ID, "delegated moderation"); !errors.Is(err, ErrMessageProtected) {
		t.Fatalf("delegated administrator trash error = %v, want %v", err, ErrMessageProtected)
	}
	if err := TrashMessage(db, author.ID, message.ID, "author request"); err != nil {
		t.Fatalf("author must be able to trash own note containing primary reply: %v", err)
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

func TestRunRecycleBinAutoCleanupProcessesMoreThanOneThousandExpiredNotes(t *testing.T) {
	db := setupUserServiceTestDB(t)
	primary := mustCreateUser(t, models.User{ID: models.PrimaryAdminUserID, Username: "auto-large-primary", IsAdmin: true})
	if err := db.Create(&models.SiteConfig{RecycleBinRetentionDays: 7}).Error; err != nil {
		t.Fatal(err)
	}
	deletedAt := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	guestbook := models.Message{Content: models.CanonicalGuestbookContent, Username: primary.Username, UserID: primary.ID, Visibility: MessageVisibilityPublic, IsGuestbook: true, DeletedAt: &deletedAt}
	if err := db.Create(&guestbook).Error; err != nil {
		t.Fatal(err)
	}
	messages := make([]models.Message, 1001)
	for i := range messages {
		messages[i] = models.Message{Content: "expired large batch", Username: primary.Username, UserID: primary.ID, Visibility: MessageVisibilityPublic, DeletedAt: &deletedAt}
	}
	if err := db.CreateInBatches(&messages, 200).Error; err != nil {
		t.Fatal(err)
	}

	succeeded, failed, skipped, err := RunRecycleBinAutoCleanup(db, deletedAt.AddDate(0, 0, 8))
	if err != nil || succeeded != 1001 || failed != 0 || skipped != 1 {
		t.Fatalf("cleanup = %d,%d,%d,%v", succeeded, failed, skipped, err)
	}
	var remaining int64
	if err := db.Unscoped().Model(&models.Message{}).Where("deleted_at IS NOT NULL").Count(&remaining).Error; err != nil {
		t.Fatal(err)
	}
	if remaining != 1 {
		t.Fatalf("remaining expired notes = %d, want only guestbook", remaining)
	}
}

func TestRunRecycleBinAutoCleanupCardinalitiesAndCutoff(t *testing.T) {
	for _, count := range []int{0, 1, 1000} {
		t.Run(fmt.Sprintf("count_%d", count), func(t *testing.T) {
			db := setupUserServiceTestDB(t)
			primary := mustCreateUser(t, models.User{ID: models.PrimaryAdminUserID, Username: fmt.Sprintf("auto-count-%d", count), IsAdmin: true})
			if err := db.Create(&models.SiteConfig{RecycleBinRetentionDays: 7}).Error; err != nil {
				t.Fatal(err)
			}
			deletedAt := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
			messages := make([]models.Message, count)
			for i := range messages {
				messages[i] = models.Message{Content: "cleanup cardinality", Username: primary.Username, UserID: primary.ID, Visibility: MessageVisibilityPublic, DeletedAt: &deletedAt}
			}
			if count > 0 {
				if err := db.CreateInBatches(&messages, 200).Error; err != nil {
					t.Fatal(err)
				}
			}
			succeeded, failed, skipped, err := RunRecycleBinAutoCleanup(db, deletedAt.AddDate(0, 0, 7))
			if err != nil || succeeded != count || failed != 0 || skipped != 0 {
				t.Fatalf("cleanup = %d,%d,%d,%v", succeeded, failed, skipped, err)
			}
		})
	}

	t.Run("cutoff boundary and never", func(t *testing.T) {
		db := setupUserServiceTestDB(t)
		primary := mustCreateUser(t, models.User{ID: models.PrimaryAdminUserID, Username: "auto-cutoff-primary", IsAdmin: true})
		cfg := models.SiteConfig{RecycleBinRetentionDays: 7}
		if err := db.Create(&cfg).Error; err != nil {
			t.Fatal(err)
		}
		now := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
		cutoff := now.AddDate(0, 0, -7)
		before, equal, after := cutoff.Add(-time.Nanosecond), cutoff, cutoff.Add(time.Nanosecond)
		messages := []models.Message{
			{Content: "before cutoff", Username: primary.Username, UserID: primary.ID, Visibility: MessageVisibilityPublic, DeletedAt: &before},
			{Content: "equal cutoff", Username: primary.Username, UserID: primary.ID, Visibility: MessageVisibilityPublic, DeletedAt: &equal},
			{Content: "after cutoff", Username: primary.Username, UserID: primary.ID, Visibility: MessageVisibilityPublic, DeletedAt: &after},
		}
		if err := db.Create(&messages).Error; err != nil {
			t.Fatal(err)
		}
		succeeded, failed, skipped, err := RunRecycleBinAutoCleanup(db, now)
		if err != nil || succeeded != 2 || failed != 0 || skipped != 0 {
			t.Fatalf("boundary cleanup = %d,%d,%d,%v", succeeded, failed, skipped, err)
		}
		if err := db.Model(&models.SiteConfig{}).Where("id = ?", cfg.ID).Update("recycle_bin_retention_days", 0).Error; err != nil {
			t.Fatal(err)
		}
		if succeeded, failed, skipped, err = RunRecycleBinAutoCleanup(db, now.AddDate(0, 0, 365)); err != nil || succeeded != 0 || failed != 0 || skipped != 0 {
			t.Fatalf("never cleanup = %d,%d,%d,%v", succeeded, failed, skipped, err)
		}
		var retained models.Message
		if err := db.Unscoped().First(&retained, messages[2].ID).Error; err != nil || retained.DeletedAt == nil {
			t.Fatalf("after-cutoff note should remain, err=%v row=%#v", err, retained)
		}
	})
}

func TestRunRecycleBinAutoCleanupIsolatesFailureAndRetriesItNextRun(t *testing.T) {
	db := setupUserServiceTestDB(t)
	primary := mustCreateUser(t, models.User{ID: models.PrimaryAdminUserID, Username: "auto-retry-primary", IsAdmin: true})
	if err := db.Create(&models.SiteConfig{RecycleBinRetentionDays: 7}).Error; err != nil {
		t.Fatal(err)
	}
	deletedAt := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	messages := make([]models.Message, 1001)
	for i := range messages {
		content := "expired retry batch"
		if i == 10 {
			content = "expired forced failure"
		}
		messages[i] = models.Message{Content: content, Username: primary.Username, UserID: primary.ID, Visibility: MessageVisibilityPublic, DeletedAt: &deletedAt}
	}
	if err := db.CreateInBatches(&messages, 200).Error; err != nil {
		t.Fatal(err)
	}
	failedID := messages[10].ID
	if err := db.Exec(`CREATE TRIGGER fail_one_expired_note BEFORE DELETE ON messages
		WHEN OLD.id = ` + fmt.Sprint(failedID) + ` BEGIN SELECT RAISE(FAIL, 'forced cleanup failure'); END`).Error; err != nil {
		t.Fatal(err)
	}

	succeeded, failed, skipped, err := RunRecycleBinAutoCleanup(db, deletedAt.AddDate(0, 0, 8))
	if err != nil || succeeded != 1000 || failed != 1 || skipped != 0 {
		t.Fatalf("first cleanup = %d,%d,%d,%v", succeeded, failed, skipped, err)
	}
	var retained models.Message
	if err := db.Unscoped().First(&retained, failedID).Error; err != nil || retained.DeletedAt == nil {
		t.Fatalf("failed note should remain for retry, err=%v row=%#v", err, retained)
	}
	var failureAudit int64
	if err := db.Model(&models.AdminAuditLog{}).Where("action = ? AND target_id = ? AND result = ?", "auto_permanent_delete", fmt.Sprint(failedID), "failure").Count(&failureAudit).Error; err != nil {
		t.Fatal(err)
	}
	if failureAudit != 1 {
		t.Fatalf("failure audit count = %d, want 1", failureAudit)
	}
	if err := db.Exec("DROP TRIGGER fail_one_expired_note").Error; err != nil {
		t.Fatal(err)
	}
	succeeded, failed, skipped, err = RunRecycleBinAutoCleanup(db, deletedAt.AddDate(0, 0, 8))
	if err != nil || succeeded != 1 || failed != 0 || skipped != 0 {
		t.Fatalf("retry cleanup = %d,%d,%d,%v", succeeded, failed, skipped, err)
	}
	if err := db.Unscoped().First(&models.Message{}, failedID).Error; !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("failed note was not deleted on retry: %v", err)
	}
}

func TestBatchNoteLifecycleByFilterRebuildsScopeAcrossPages(t *testing.T) {
	db := setupUserServiceTestDB(t)
	primary := mustCreateUser(t, models.User{ID: models.PrimaryAdminUserID, Username: "filtered-primary", IsAdmin: true})
	for i := 0; i < 25; i++ {
		message := models.Message{Content: "cross-page-filter-target", Username: primary.Username, UserID: primary.ID, Visibility: MessageVisibilityPublic}
		if err := db.Create(&message).Error; err != nil {
			t.Fatal(err)
		}
	}
	result, err := BatchNoteLifecycleByFilter(db, primary.ID, NoteManagementFilter{Keyword: "cross-page-filter-target", Sort: "created_asc"}, false, "trash", "filtered test")
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 25 || result.Succeeded != 25 || result.Failed != 0 || len(result.Items) != 25 {
		t.Fatalf("filtered batch result=%#v", result)
	}
	var trashed int64
	if err := db.Unscoped().Model(&models.Message{}).Where("deleted_at IS NOT NULL").Count(&trashed).Error; err != nil {
		t.Fatal(err)
	}
	if trashed != 25 {
		t.Fatalf("trashed=%d, want 25", trashed)
	}
}

func TestBatchNoteLifecycleByFilterProcessesMoreThanOneThousandMatches(t *testing.T) {
	db := setupUserServiceTestDB(t)
	primary := mustCreateUser(t, models.User{ID: models.PrimaryAdminUserID, Username: "filtered-large-primary", IsAdmin: true})
	messages := make([]models.Message, 1001)
	for i := range messages {
		messages[i] = models.Message{Content: "filtered-large-target", Username: primary.Username, UserID: primary.ID, Visibility: MessageVisibilityPublic}
	}
	if err := db.CreateInBatches(&messages, 200).Error; err != nil {
		t.Fatal(err)
	}

	result, err := BatchNoteLifecycleByFilter(db, primary.ID, NoteManagementFilter{Keyword: "filtered-large-target"}, false, "trash", "filtered large test")
	if err != nil {
		t.Fatalf("filtered batch returned an error: %v", err)
	}
	if result.Total != 1001 || result.Succeeded != 1001 || result.Failed != 0 || len(result.Items) != 1001 {
		t.Fatalf("filtered batch result=%#v", result)
	}
}

func TestBatchNoteLifecycleByFilterCardinalities(t *testing.T) {
	for _, count := range []int{0, 1, 20, 21, 1000} {
		t.Run(fmt.Sprintf("count_%d", count), func(t *testing.T) {
			db := setupUserServiceTestDB(t)
			primary := mustCreateUser(t, models.User{ID: models.PrimaryAdminUserID, Username: fmt.Sprintf("filtered-count-%d", count), IsAdmin: true})
			messages := make([]models.Message, count)
			for i := range messages {
				messages[i] = models.Message{Content: "filtered-cardinality-target", Username: primary.Username, UserID: primary.ID, Visibility: MessageVisibilityPublic}
			}
			if count > 0 {
				if err := db.CreateInBatches(&messages, 200).Error; err != nil {
					t.Fatal(err)
				}
			}
			result, err := BatchNoteLifecycleByFilter(db, primary.ID, NoteManagementFilter{Keyword: "filtered-cardinality-target"}, false, "trash", "cardinality test")
			if err != nil {
				t.Fatal(err)
			}
			if result.Total != count || result.Processed != count || result.Succeeded != count || result.Failed != 0 || len(result.Items) != count {
				t.Fatalf("filtered batch result=%#v", result)
			}
		})
	}
}

func TestBatchNoteLifecycleByFilterRechecksRevokedCapabilityPerItem(t *testing.T) {
	db := setupUserServiceTestDB(t)
	primary := mustCreateUser(t, models.User{ID: models.PrimaryAdminUserID, Username: "filtered-revoke-primary", IsAdmin: true})
	delegated := mustCreateUser(t, models.User{Username: "filtered-revoke-delegated", IsAdmin: true})
	author := mustCreateUser(t, models.User{Username: "filtered-revoke-author"})
	if err := authorization.New(db).ReplaceGrants(primary.ID, delegated.ID, []authorization.Capability{authorization.CapabilityNotesTrash}); err != nil {
		t.Fatal(err)
	}
	messages := []models.Message{
		{Content: "filtered-revoke-target", Username: author.Username, UserID: author.ID, Visibility: MessageVisibilityPublic},
		{Content: "filtered-revoke-target", Username: author.Username, UserID: author.ID, Visibility: MessageVisibilityPublic},
	}
	if err := db.Create(&messages).Error; err != nil {
		t.Fatal(err)
	}
	revoked := false
	if err := db.Callback().Update().After("gorm:update").Register("test:revoke_note_trash", func(tx *gorm.DB) {
		if revoked || tx.Statement.Table != "messages" {
			return
		}
		revoked = true
		if err := tx.Session(&gorm.Session{NewDB: true}).Exec("DELETE FROM admin_capability_grants WHERE user_id = ? AND capability = ?", delegated.ID, authorization.CapabilityNotesTrash).Error; err != nil {
			tx.AddError(err)
		}
	}); err != nil {
		t.Fatal(err)
	}

	result, err := BatchNoteLifecycleByFilter(db, delegated.ID, NoteManagementFilter{Keyword: "filtered-revoke-target"}, false, "trash", "filtered revoke test")
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 2 || result.Processed != 2 || result.Succeeded != 1 || result.Failed != 1 {
		t.Fatalf("filtered revoke result=%#v", result)
	}
	if len(result.Items) != 2 || result.Items[1].Reason != "操作失败" {
		t.Fatalf("filtered revoke items=%#v", result.Items)
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
