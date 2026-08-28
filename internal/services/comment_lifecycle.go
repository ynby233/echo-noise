package services

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rcy1314/echo-noise/internal/authorization"
	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
)

const (
	CommentDeletionReasonSelf         = "self"
	CommentDeletionReasonModeration   = "moderation"
	CommentDeletionReasonOwnerCleanup = "owner_cleanup"
	CommentDeletionReasonAncestor     = "ancestor"
	CommentDeletionReasonSystem       = "system"
)

var (
	ErrCommentNotFound        = errors.New("comment not found")
	ErrCommentAlreadyTrashed  = errors.New("comment is already in recycle bin")
	ErrCommentNotAuthorized   = errors.New("comment action is not authorized")
	ErrCommentProtected       = errors.New("comment is protected")
	ErrCommentAncestorPending = errors.New("comment ancestor is still in recycle bin")
	ErrCommentUserPurged      = errors.New("comment was permanently deleted by its author")
	ErrCommentNotTrashed      = errors.New("comment is not in recycle bin")
)

type CommentTrashRequest struct {
	ReasonCode string
	BatchID    string
}

type CommentTrashResult struct {
	BatchID        string `json:"batch_id"`
	Trashed        int    `json:"trashed"`
	AlreadyTrashed int    `json:"already_trashed"`
}

func writeCommentLifecycleAudit(tx *gorm.DB, record models.AdminAuditLog) error {
	if tx == nil || !tx.Migrator().HasTable(&models.AdminAuditLog{}) || !tx.Migrator().HasTable(&models.AdminAuditConfig{}) {
		return nil
	}
	return authorization.New(tx).WriteAudit(record)
}

func newCommentLifecycleBatchID() string {
	var value [12]byte
	if _, err := rand.Read(value[:]); err == nil {
		return hex.EncodeToString(value[:])
	}
	return time.Now().UTC().Format("20060102T150405.000000000")
}

// NewCommentLifecycleBatchID groups several user-selected lifecycle changes
// without exposing how batch identifiers are generated.
func NewCommentLifecycleBatchID() string { return newCommentLifecycleBatchID() }

func normalizedCommentDeletionReason(value string) string {
	switch strings.TrimSpace(value) {
	case CommentDeletionReasonSelf, CommentDeletionReasonModeration, CommentDeletionReasonOwnerCleanup, CommentDeletionReasonAncestor, CommentDeletionReasonSystem:
		return strings.TrimSpace(value)
	default:
		return CommentDeletionReasonModeration
	}
}

func commentOwnerID(comment models.Comment) uint {
	if comment.UserID == nil {
		return 0
	}
	return *comment.UserID
}

func actorOwnsCommentAncestor(actorID uint, comment models.Comment, commentMap map[uint]models.Comment) bool {
	seen := map[uint]bool{comment.ID: true}
	parentID := comment.ParentID
	for parentID != nil {
		if seen[*parentID] {
			return false
		}
		parent, ok := commentMap[*parentID]
		if !ok {
			return false
		}
		seen[parent.ID] = true
		if commentOwnerID(parent) == actorID {
			return true
		}
		parentID = parent.ParentID
	}
	return false
}

func authorizeCommentTrash(db *gorm.DB, actorID uint, message models.Message, target models.Comment, commentMap map[uint]models.Comment) (string, error) {
	if actorID == 0 {
		return "", ErrCommentNotAuthorized
	}
	if commentOwnerID(target) == actorID {
		return CommentDeletionReasonSelf, nil
	}
	if message.UserID == actorID || actorOwnsCommentAncestor(actorID, target, commentMap) {
		return CommentDeletionReasonOwnerCleanup, nil
	}
	ownerID := commentOwnerID(target)
	if ownerID == 0 {
		return "", ErrCommentProtected
	}
	decision := authorization.New(db).Authorize(actorID, authorization.CapabilityCommentsTrash, &ownerID)
	if !decision.Allowed {
		if decision.Reason == authorization.DenialProtectedContent {
			return "", ErrCommentProtected
		}
		return "", ErrCommentNotAuthorized
	}
	scope, err := ResolveContentReadScope(db, &actorID)
	if err != nil || !scope.CanReadComment(message, target, commentMap) {
		return "", ErrCommentNotFound
	}
	return CommentDeletionReasonModeration, nil
}

func descendantCommentIDs(rootID uint, comments []models.Comment) []uint {
	children := make(map[uint][]uint)
	for _, comment := range comments {
		if comment.ParentID != nil {
			children[*comment.ParentID] = append(children[*comment.ParentID], comment.ID)
		}
	}
	result := []uint{rootID}
	seen := map[uint]bool{rootID: true}
	for cursor := 0; cursor < len(result); cursor++ {
		for _, childID := range children[result[cursor]] {
			if seen[childID] {
				continue
			}
			seen[childID] = true
			result = append(result, childID)
		}
	}
	return result
}

func trashActiveCommentsForMessage(tx *gorm.DB, actorID uint, message models.Message, batchID string, deletedAt time.Time) (int, error) {
	if strings.TrimSpace(batchID) == "" {
		batchID = newCommentLifecycleBatchID()
	}
	var active []models.Comment
	if err := tx.Where("message_id = ? AND deleted_at IS NULL AND is_tombstone = ?", message.ID, false).Find(&active).Error; err != nil {
		return 0, err
	}
	update := tx.Model(&models.Comment{}).
		Where("message_id = ? AND deleted_at IS NULL AND is_tombstone = ?", message.ID, false).
		Updates(map[string]interface{}{
			"deleted_at": deletedAt, "deleted_by_user_id": actorID,
			"deletion_reason_code":        CommentDeletionReasonAncestor,
			"deleted_ancestor_message_id": message.ID, "deletion_batch_id": batchID,
		})
	if update.Error != nil {
		return int(update.RowsAffected), update.Error
	}
	targets := make([]deletionNotificationTarget, 0, len(active))
	for _, comment := range active {
		targets = append(targets, deletionNotificationTarget{
			OwnerID: commentOwnerID(comment), TargetType: commentKind(message, comment), TargetID: comment.ID,
			Content: comment.Content, ContextText: fmt.Sprintf("因上级笔记 #%d 被移入回收站而一并回收", message.ID),
			ReasonCode: CommentDeletionReasonAncestor, DeletedAt: &deletedAt,
		})
	}
	if err := createDeletionNotificationsTx(tx, actorID, DeletionEventTrashed, batchID, targets, false); err != nil {
		return int(update.RowsAffected), err
	}
	return int(update.RowsAffected), nil
}

// TrashCommentTree is the single comment-to-recycle-bin transition. The root
// and every active descendant are updated atomically so a parent can never be
// removed while its replies still point at it. Existing recycle-bin metadata
// is deliberately preserved for descendants which were already trashed.
func TrashCommentTree(db *gorm.DB, actorID, commentID uint, request CommentTrashRequest) (CommentTrashResult, error) {
	result := CommentTrashResult{}
	if db == nil || commentID == 0 {
		return result, ErrCommentNotFound
	}
	batchID := strings.TrimSpace(request.BatchID)
	if batchID == "" {
		batchID = newCommentLifecycleBatchID()
	}
	result.BatchID = batchID
	err := db.Transaction(func(tx *gorm.DB) error {
		var target models.Comment
		if err := tx.First(&target, commentID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrCommentNotFound
			}
			return err
		}
		if target.IsTombstone {
			return ErrCommentNotFound
		}
		if target.DeletedAt != nil {
			return ErrCommentAlreadyTrashed
		}
		var message models.Message
		if err := tx.First(&message, target.MessageID).Error; err != nil {
			return ErrMessageNotFound
		}
		var comments []models.Comment
		if err := tx.Where("message_id = ?", target.MessageID).Find(&comments).Error; err != nil {
			return err
		}
		commentMap := CommentMap(comments)
		reason, err := authorizeCommentTrash(tx, actorID, message, target, commentMap)
		if err != nil {
			return err
		}
		requestedReason := normalizedCommentDeletionReason(request.ReasonCode)
		if reason == CommentDeletionReasonSelf && requestedReason == CommentDeletionReasonSelf {
			reason = requestedReason
		}
		deletedAt := time.Now().UTC()
		notificationTargets := make([]deletionNotificationTarget, 0)
		for index, id := range descendantCommentIDs(target.ID, comments) {
			comment := commentMap[id]
			if comment.DeletedAt != nil || comment.IsTombstone {
				result.AlreadyTrashed++
				continue
			}
			values := map[string]interface{}{
				"deleted_at": deletedAt, "deleted_by_user_id": actorID,
				"deletion_batch_id": batchID, "user_purged_at": nil,
			}
			if index == 0 {
				values["deletion_reason_code"] = reason
				values["deleted_ancestor_comment_id"] = nil
				values["deleted_ancestor_message_id"] = nil
			} else {
				values["deletion_reason_code"] = CommentDeletionReasonAncestor
				values["deleted_ancestor_comment_id"] = target.ID
				values["deleted_ancestor_message_id"] = nil
			}
			update := tx.Model(&models.Comment{}).Where("id = ? AND deleted_at IS NULL AND is_tombstone = ?", id, false).Updates(values)
			if update.Error != nil {
				return update.Error
			}
			if update.RowsAffected == 1 {
				result.Trashed++
				ownerID := commentOwnerID(comment)
				contextText := ""
				itemReason := reason
				if index > 0 {
					itemReason = CommentDeletionReasonAncestor
					contextText = fmt.Sprintf("因上级互动 #%d 被移入回收站而一并回收", target.ID)
				}
				notificationTargets = append(notificationTargets, deletionNotificationTarget{
					OwnerID: ownerID, TargetType: commentKind(message, comment), TargetID: comment.ID,
					Content: comment.Content, ContextText: contextText, ReasonCode: itemReason, DeletedAt: &deletedAt,
				})
				ownerPointer := (*uint)(nil)
				if ownerID != 0 {
					value := ownerID
					ownerPointer = &value
				}
				if err := writeCommentLifecycleAudit(tx, models.AdminAuditLog{
					ActorUserID: actorID, Capability: string(authorization.CapabilityCommentsTrash),
					Module: "comments", Action: "trash", TargetType: commentKind(message, comment),
					TargetID: fmt.Sprint(comment.ID), TargetOwnerUserID: ownerPointer, Result: "success",
					Summary: "moved interaction to recycle bin", ChangesJSON: fmt.Sprintf(`{"batch_id":%q,"reason":%q,"cascade":%t}`, batchID, itemReason, index > 0),
				}); err != nil {
					return err
				}
			} else {
				result.AlreadyTrashed++
			}
		}
		return createDeletionNotificationsTx(tx, actorID, DeletionEventTrashed, batchID, notificationTargets, false)
	})
	return result, err
}

func RestoreComment(db *gorm.DB, actorID, commentID uint) error {
	if db == nil || actorID == 0 || commentID == 0 {
		return ErrCommentNotAuthorized
	}
	return db.Transaction(func(tx *gorm.DB) error {
		var target models.Comment
		if err := tx.First(&target, commentID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrCommentNotFound
			}
			return err
		}
		if target.IsTombstone || target.DeletedAt == nil {
			return ErrCommentNotFound
		}
		if target.UserPurgedAt != nil {
			return ErrCommentUserPurged
		}
		var message models.Message
		if err := tx.First(&message, target.MessageID).Error; err != nil {
			return ErrCommentAncestorPending
		}
		if message.DeletedAt != nil && !message.IsTombstone {
			return ErrCommentAncestorPending
		}
		var comments []models.Comment
		if err := tx.Where("message_id = ?", target.MessageID).Find(&comments).Error; err != nil {
			return err
		}
		commentMap := CommentMap(comments)
		seen := map[uint]bool{target.ID: true}
		parentID := target.ParentID
		for parentID != nil {
			if seen[*parentID] {
				return ErrCommentAncestorPending
			}
			parent, ok := commentMap[*parentID]
			if !ok {
				return ErrCommentAncestorPending
			}
			seen[parent.ID] = true
			if parent.DeletedAt != nil && !parent.IsTombstone {
				return ErrCommentAncestorPending
			}
			parentID = parent.ParentID
		}
		ownerID := commentOwnerID(target)
		if ownerID != actorID {
			if ownerID == 0 {
				return ErrCommentProtected
			}
			decision := authorization.New(tx).Authorize(actorID, authorization.CapabilityCommentsRestore, &ownerID)
			if !decision.Allowed {
				if decision.Reason == authorization.DenialProtectedContent {
					return ErrCommentProtected
				}
				return ErrCommentNotAuthorized
			}
		}
		update := tx.Model(&models.Comment{}).
			Where("id = ? AND deleted_at IS NOT NULL AND user_purged_at IS NULL AND is_tombstone = ?", target.ID, false).
			Updates(map[string]interface{}{
				"deleted_at": nil, "deleted_by_user_id": nil, "deletion_reason_code": "",
				"deleted_ancestor_comment_id": nil, "deleted_ancestor_message_id": nil, "deletion_batch_id": "",
			})
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected != 1 {
			return ErrCommentNotFound
		}
		return writeCommentLifecycleAudit(tx, models.AdminAuditLog{
			ActorUserID: actorID, Capability: string(authorization.CapabilityCommentsRestore), Module: "comments",
			Action: "restore", TargetType: commentKind(message, target), TargetID: fmt.Sprint(target.ID),
			TargetOwnerUserID: target.UserID, Result: "success", Summary: "restored interaction from recycle bin",
		})
	})
}

func hasCommentDescendants(db *gorm.DB, commentID uint) (bool, error) {
	var count int64
	if err := db.Model(&models.Comment{}).Where("parent_id = ?", commentID).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func permanentlyDeleteCommentInTx(tx *gorm.DB, target models.Comment) error {
	hasChildren, err := hasCommentDescendants(tx, target.ID)
	if err != nil {
		return err
	}
	if err := RemoveUnreferencedCommentAttachmentReferences(tx, target); err != nil {
		return err
	}
	if hasChildren {
		now := time.Now().UTC()
		visibility := target.Visibility
		if strings.TrimSpace(visibility) == "" {
			visibility = MessageVisibilityPrivate
		}
		update := tx.Model(&models.Comment{}).Where("id = ?", target.ID).Updates(map[string]interface{}{
			"user_id": nil, "content": "", "visibility": MessageVisibilityPrivate,
			"deleted_at": nil, "deleted_by_user_id": nil, "deletion_reason_code": "",
			"deleted_ancestor_comment_id": nil, "deleted_ancestor_message_id": nil,
			"deletion_batch_id": "", "user_purged_at": nil,
			"is_tombstone": true, "tombstoned_at": now, "tombstone_visibility": visibility,
		})
		return update.Error
	}
	parentID := target.ParentID
	if err := tx.Delete(&models.Comment{}, target.ID).Error; err != nil {
		return err
	}
	return pruneEmptyCommentTombstones(tx, parentID)
}

func pruneEmptyCommentTombstones(tx *gorm.DB, commentID *uint) error {
	for commentID != nil && *commentID != 0 {
		var candidate models.Comment
		if err := tx.First(&candidate, *commentID).Error; err != nil {
			return nil
		}
		if !candidate.IsTombstone {
			return nil
		}
		hasChildren, err := hasCommentDescendants(tx, candidate.ID)
		if err != nil || hasChildren {
			return err
		}
		parentID := candidate.ParentID
		if err := tx.Delete(&models.Comment{}, candidate.ID).Error; err != nil {
			return err
		}
		commentID = parentID
	}
	return nil
}

// UserPurgeComment hides an author's interaction permanently from that author.
// Ordinary users leave an administrator-only retention copy. A delegated
// administrator may truly delete their own interaction only when both recycle
// bin view and permanent-delete capabilities are currently effective.
func UserPurgeComment(db *gorm.DB, actorID, commentID uint) (retainedForAdministration bool, err error) {
	if db == nil || actorID == 0 || commentID == 0 {
		return false, ErrCommentNotAuthorized
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		var target models.Comment
		if err := tx.First(&target, commentID).Error; err != nil {
			return ErrCommentNotFound
		}
		if target.UserID == nil || *target.UserID != actorID {
			return ErrCommentNotAuthorized
		}
		if target.DeletedAt == nil || target.IsTombstone {
			return ErrCommentNotTrashed
		}
		if target.UserPurgedAt != nil {
			return ErrCommentUserPurged
		}
		var actor models.User
		if err := tx.Select("id,is_admin").First(&actor, actorID).Error; err != nil {
			return ErrCommentNotAuthorized
		}
		mayDeleteDirectly := actor.ID == models.PrimaryAdminUserID
		if actor.IsAdmin && actor.ID != models.PrimaryAdminUserID {
			mayDeleteDirectly = authorization.New(tx).Authorize(actorID, authorization.CapabilityCommentsDeletePermanently, &actorID).Allowed
		}
		if mayDeleteDirectly {
			if err := permanentlyDeleteCommentInTx(tx, target); err != nil {
				return err
			}
			return writeCommentLifecycleAudit(tx, models.AdminAuditLog{
				ActorUserID: actorID, Capability: string(authorization.CapabilityCommentsDeletePermanently), Module: "comments",
				Action: "self_permanent_delete", TargetType: "interaction", TargetID: fmt.Sprint(target.ID),
				TargetOwnerUserID: target.UserID, Result: "success", Summary: "permanently deleted own interaction",
			})
		}
		now := time.Now().UTC()
		update := tx.Model(&models.Comment{}).Where("id = ? AND user_purged_at IS NULL", target.ID).Update("user_purged_at", now)
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected != 1 {
			return ErrCommentUserPurged
		}
		retainedForAdministration = true
		return writeCommentLifecycleAudit(tx, models.AdminAuditLog{
			ActorUserID: actorID, Capability: string(authorization.CapabilityCommentsTrash), Module: "comments",
			Action: "user_purge", TargetType: "interaction", TargetID: fmt.Sprint(target.ID),
			TargetOwnerUserID: target.UserID, Result: "success", Summary: "author removed interaction from personal recycle bin; administrative copy retained",
		})
	})
	return retainedForAdministration, err
}

func PermanentlyDeleteComment(db *gorm.DB, actorID, commentID uint) error {
	if db == nil || actorID == 0 || commentID == 0 {
		return ErrCommentNotAuthorized
	}
	return db.Transaction(func(tx *gorm.DB) error {
		var target models.Comment
		if err := tx.First(&target, commentID).Error; err != nil {
			return ErrCommentNotFound
		}
		if target.DeletedAt == nil || target.IsTombstone {
			return ErrCommentNotTrashed
		}
		ownerID := commentOwnerID(target)
		if ownerID == models.PrimaryAdminUserID && actorID != models.PrimaryAdminUserID {
			return ErrCommentProtected
		}
		if ownerID == 0 {
			return ErrCommentProtected
		}
		decision := authorization.New(tx).Authorize(actorID, authorization.CapabilityCommentsDeletePermanently, &ownerID)
		if !decision.Allowed {
			if decision.Reason == authorization.DenialProtectedContent {
				return ErrCommentProtected
			}
			return ErrCommentNotAuthorized
		}
		var message models.Message
		if err := tx.First(&message, target.MessageID).Error; err != nil {
			return err
		}
		if target.UserPurgedAt == nil {
			if err := createDeletionNotificationsTx(tx, actorID, DeletionEventPermanentlyDeleted, newCommentLifecycleBatchID(), []deletionNotificationTarget{{
				OwnerID: ownerID, TargetType: commentKind(message, target), TargetID: target.ID, Content: target.Content,
				ContextText: "互动已从回收站永久删除", ReasonCode: target.DeletionReasonCode,
			}}, false); err != nil {
				return err
			}
		}
		if err := permanentlyDeleteCommentInTx(tx, target); err != nil {
			return err
		}
		return writeCommentLifecycleAudit(tx, models.AdminAuditLog{
			ActorUserID: actorID, Capability: string(authorization.CapabilityCommentsDeletePermanently), Module: "comments",
			Action: "permanent_delete", TargetType: commentKind(message, target), TargetID: fmt.Sprint(target.ID),
			TargetOwnerUserID: target.UserID, Result: "success", Summary: "permanently deleted interaction from recycle bin",
		})
	})
}

func RunCommentRecycleBinAutoCleanup(db *gorm.DB, now time.Time) (succeeded, failed int, err error) {
	if db == nil {
		return 0, 0, errors.New("database is nil")
	}
	var cfg models.SiteConfig
	if err = db.Table("site_configs").First(&cfg).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return
	}
	if cfg.CommentRecycleBinRetentionDays <= 0 {
		return 0, 0, nil
	}
	cutoff := now.UTC().AddDate(0, 0, -cfg.CommentRecycleBinRetentionDays)
	const pageSize = 250
	var cursorDeletedAt time.Time
	var cursorID uint
	for {
		query := db.Where("deleted_at IS NOT NULL AND deleted_at <= ? AND is_tombstone = ?", cutoff, false)
		if !cursorDeletedAt.IsZero() {
			query = query.Where("deleted_at > ? OR (deleted_at = ? AND id > ?)", cursorDeletedAt, cursorDeletedAt, cursorID)
		}
		var comments []models.Comment
		if err = query.Order("deleted_at ASC, id ASC").Limit(pageSize).Find(&comments).Error; err != nil {
			return
		}
		if len(comments) == 0 {
			break
		}
		for _, comment := range comments {
			cursorDeletedAt = comment.DeletedAt.UTC()
			cursorID = comment.ID
			itemErr := db.Transaction(func(tx *gorm.DB) error {
				if comment.UserPurgedAt == nil && comment.UserID != nil {
					if err := createDeletionNotificationsTx(tx, models.PrimaryAdminUserID, DeletionEventPermanentlyDeleted, newCommentLifecycleBatchID(), []deletionNotificationTarget{{
						OwnerID: *comment.UserID, TargetType: "interaction", TargetID: comment.ID, Content: comment.Content,
						ContextText: "系统按互动回收站保留期限自动永久删除", ReasonCode: CommentDeletionReasonSystem,
					}}, true); err != nil {
						return err
					}
				}
				if err := permanentlyDeleteCommentInTx(tx, comment); err != nil {
					return err
				}
				ownerID := commentOwnerID(comment)
				return writeCommentLifecycleAudit(tx, models.AdminAuditLog{
					ActorUserID: models.PrimaryAdminUserID, Capability: string(authorization.CapabilityCommentsDeletePermanently),
					Module: "comments", Action: "auto_permanent_delete", TargetType: "comment", TargetID: fmt.Sprint(comment.ID),
					TargetOwnerUserID: func() *uint {
						if ownerID == 0 {
							return nil
						}
						value := ownerID
						return &value
					}(),
					Result: "success", Summary: "system comment recycle-bin retention cleanup", AuthVia: "system",
				})
			})
			if itemErr != nil {
				failed++
				continue
			}
			succeeded++
		}
	}
	return
}
