package services

import (
	"strings"
	"time"

	"github.com/rcy1314/echo-noise/internal/authorization"
	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
)

type CommentManagementFilter struct {
	Page       int
	PageSize   int
	Keyword    string
	Kind       string
	AuthorID   *uint
	ReasonCode string
}

type CommentContextNode struct {
	ID          uint   `json:"id"`
	Kind        string `json:"kind"`
	Content     string `json:"content,omitempty"`
	Placeholder string `json:"placeholder,omitempty"`
	IsTombstone bool   `json:"is_tombstone"`
}

type CommentManagementItem struct {
	ID                       uint                 `json:"id"`
	MessageID                uint                 `json:"message_id"`
	ParentID                 *uint                `json:"parent_id,omitempty"`
	UserID                   *uint                `json:"user_id,omitempty"`
	Username                 string               `json:"username,omitempty"`
	Kind                     string               `json:"kind"`
	Content                  string               `json:"content"`
	StoredVisibility         string               `json:"stored_visibility"`
	EffectiveVisibility      string               `json:"effective_visibility"`
	LimitedByAncestor        bool                 `json:"limited_by_ancestor"`
	CreatedAt                time.Time            `json:"created_at"`
	DeletedAt                *time.Time           `json:"deleted_at,omitempty"`
	DeletedByUserID          *uint                `json:"deleted_by_user_id,omitempty"`
	DeletionReasonCode       string               `json:"deletion_reason_code,omitempty"`
	DeletedAncestorCommentID *uint                `json:"deleted_ancestor_comment_id,omitempty"`
	DeletedAncestorMessageID *uint                `json:"deleted_ancestor_message_id,omitempty"`
	DeletionBatchID          string               `json:"deletion_batch_id,omitempty"`
	UserPurged               bool                 `json:"user_purged"`
	CanRestore               bool                 `json:"can_restore"`
	CanPermanentlyDelete     bool                 `json:"can_permanently_delete"`
	CanTrash                 bool                 `json:"can_trash"`
	CanOpenThread            bool                 `json:"can_open_thread"`
	CanEdit                  bool                 `json:"can_edit"`
	CanChangeVisibility      bool                 `json:"can_change_visibility"`
	Context                  []CommentContextNode `json:"context"`
	MessageContext           CommentContextNode   `json:"message_context"`
	RecycleDeadline          RecycleDeadline      `json:"recycle_deadline"`
}

type CommentManagementPage struct {
	Total    int64                   `json:"total"`
	Items    []CommentManagementItem `json:"items"`
	Page     int                     `json:"page"`
	PageSize int                     `json:"page_size"`
}

func commentKind(message models.Message, comment models.Comment) string {
	if comment.ParentID != nil {
		return "reply"
	}
	if IsGuestbookMessage(message) {
		return "guestbook"
	}
	return "comment"
}

func managementMessageForVisibility(message models.Message) models.Message {
	message.DeletedAt = nil
	return message
}

func canReadCommentInManagement(scope ContentReadScope, message models.Message, comment models.Comment, commentMap map[uint]models.Comment) bool {
	comment.DeletedAt = nil
	comment.IsTombstone = false
	return scope.CanReadComment(managementMessageForVisibility(message), comment, commentMap)
}

func commentAncestorsReady(message models.Message, target models.Comment, commentMap map[uint]models.Comment) bool {
	if message.DeletedAt != nil && !message.IsTombstone {
		return false
	}
	seen := map[uint]bool{target.ID: true}
	parentID := target.ParentID
	for parentID != nil {
		if seen[*parentID] {
			return false
		}
		parent, ok := commentMap[*parentID]
		if !ok || (parent.DeletedAt != nil && !parent.IsTombstone) {
			return false
		}
		seen[parent.ID] = true
		parentID = parent.ParentID
	}
	return true
}

func commentContext(scope ContentReadScope, message models.Message, target models.Comment, commentMap map[uint]models.Comment) []CommentContextNode {
	chain := make([]models.Comment, 0)
	seen := map[uint]bool{target.ID: true}
	parentID := target.ParentID
	for parentID != nil && !seen[*parentID] {
		seen[*parentID] = true
		parent, ok := commentMap[*parentID]
		if !ok {
			chain = append(chain, models.Comment{ID: *parentID, IsTombstone: true})
			break
		}
		chain = append(chain, parent)
		parentID = parent.ParentID
	}
	for left, right := 0, len(chain)-1; left < right; left, right = left+1, right-1 {
		chain[left], chain[right] = chain[right], chain[left]
	}
	out := make([]CommentContextNode, 0, len(chain))
	for _, ancestor := range chain {
		node := CommentContextNode{ID: ancestor.ID, Kind: commentKind(message, ancestor), IsTombstone: ancestor.IsTombstone}
		if ancestor.IsTombstone {
			node.Placeholder = "上级内容已永久删除"
		} else if canReadCommentInManagement(scope, message, ancestor, commentMap) {
			node.Content = ancestor.Content
		} else {
			node.Placeholder = "上级内容当前不可见"
		}
		out = append(out, node)
	}
	return out
}

func loadSiteRecycleRetention(db *gorm.DB) (noteDays, commentDays int) {
	var cfg models.SiteConfig
	if err := db.Table("site_configs").First(&cfg).Error; err == nil {
		return cfg.RecycleBinRetentionDays, cfg.CommentRecycleBinRetentionDays
	}
	return 0, 0
}

// ListCommentManagement is shared by the normal administration list, the
// administrator recycle bin, and the user's own interaction surfaces.
func ListCommentManagement(db *gorm.DB, actorID uint, filter CommentManagementFilter, recycleBin, personalOnly bool, now time.Time) (CommentManagementPage, error) {
	page := filter.Page
	if page < 1 {
		page = 1
	}
	pageSize := filter.PageSize
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	result := CommentManagementPage{Items: []CommentManagementItem{}, Page: page, PageSize: pageSize}
	scope, err := ResolveContentReadScope(db, &actorID)
	if err != nil {
		return result, err
	}
	query := db.Model(&models.Comment{}).Where("is_tombstone = ?", false)
	if recycleBin {
		query = query.Where("deleted_at IS NOT NULL")
	} else {
		query = query.Where("deleted_at IS NULL")
	}
	if personalOnly {
		query = query.Where("user_id = ?", actorID)
		if recycleBin {
			query = query.Where("user_purged_at IS NULL")
		}
	}
	if filter.AuthorID != nil {
		query = query.Where("user_id = ?", *filter.AuthorID)
	}
	if strings.TrimSpace(filter.Keyword) != "" {
		query = query.Where("content LIKE ?", "%"+strings.TrimSpace(filter.Keyword)+"%")
	}
	if strings.TrimSpace(filter.ReasonCode) != "" {
		query = query.Where("deletion_reason_code = ?", strings.TrimSpace(filter.ReasonCode))
	}
	var candidates []models.Comment
	if err := query.Order("created_at DESC, id DESC").Find(&candidates).Error; err != nil {
		return result, err
	}
	messageIDs := make([]uint, 0)
	seenMessages := map[uint]bool{}
	for _, candidate := range candidates {
		if !seenMessages[candidate.MessageID] {
			seenMessages[candidate.MessageID] = true
			messageIDs = append(messageIDs, candidate.MessageID)
		}
	}
	var messages []models.Message
	if len(messageIDs) > 0 {
		if err := db.Where("id IN ?", messageIDs).Find(&messages).Error; err != nil {
			return result, err
		}
	}
	messageMap := map[uint]models.Message{}
	for _, message := range messages {
		messageMap[message.ID] = message
	}
	var allComments []models.Comment
	if len(messageIDs) > 0 {
		if err := db.Where("message_id IN ?", messageIDs).Find(&allComments).Error; err != nil {
			return result, err
		}
	}
	commentsByMessage := map[uint][]models.Comment{}
	for _, comment := range allComments {
		commentsByMessage[comment.MessageID] = append(commentsByMessage[comment.MessageID], comment)
	}
	userIDs := make([]uint, 0)
	for _, candidate := range candidates {
		if candidate.UserID != nil {
			userIDs = append(userIDs, *candidate.UserID)
		}
	}
	var users []models.User
	if len(userIDs) > 0 {
		_ = db.Select("id,username").Where("id IN ?", userIDs).Find(&users).Error
	}
	usernames := map[uint]string{}
	for _, user := range users {
		usernames[user.ID] = user.Username
	}
	_, commentRetention := loadSiteRecycleRetention(db)
	visible := make([]CommentManagementItem, 0, len(candidates))
	for _, candidate := range candidates {
		message, ok := messageMap[candidate.MessageID]
		if !ok {
			continue
		}
		commentMap := CommentMap(commentsByMessage[candidate.MessageID])
		if !personalOnly && !canReadCommentInManagement(scope, message, candidate, commentMap) {
			continue
		}
		kind := commentKind(message, candidate)
		if strings.TrimSpace(filter.Kind) != "" && filter.Kind != kind {
			continue
		}
		stored := NormalizedCommentVisibilityOrPublic(candidate.Visibility)
		effective := EffectiveCommentVisibilityInThread(candidate, StoredMessageVisibility(message), commentMap)
		messageContext := CommentContextNode{ID: message.ID, Kind: "note"}
		if message.IsTombstone {
			messageContext.IsTombstone = true
			messageContext.Placeholder = "原笔记已永久删除"
		} else if scope.canReadMessage(managementMessageForVisibility(message)) {
			messageContext.Content = message.Content
		} else {
			messageContext.Placeholder = "上级笔记当前不可见"
		}
		ownerID := uint(0)
		if candidate.UserID != nil {
			ownerID = *candidate.UserID
		}
		canRestore := candidate.DeletedAt != nil && candidate.UserPurgedAt == nil && commentAncestorsReady(message, candidate, commentMap)
		canPermanentlyDelete := false
		canTrash := false
		canEdit := false
		canChangeVisibility := false
		if personalOnly {
			canRestore = canRestore && ownerID == actorID
			canPermanentlyDelete = candidate.DeletedAt != nil && ownerID == actorID
			canTrash = candidate.DeletedAt == nil && ownerID == actorID
		} else if ownerID != 0 {
			canRestore = canRestore && authorization.New(db).Authorize(actorID, authorization.CapabilityCommentsRestore, &ownerID).Allowed
			canPermanentlyDelete = candidate.DeletedAt != nil && authorization.New(db).Authorize(actorID, authorization.CapabilityCommentsDeletePermanently, &ownerID).Allowed
			canTrash = candidate.DeletedAt == nil && authorization.New(db).Authorize(actorID, authorization.CapabilityCommentsTrash, &ownerID).Allowed
			canEdit = candidate.DeletedAt == nil && authorization.New(db).Authorize(actorID, authorization.CapabilityCommentsEdit, &ownerID).Allowed
			canChangeVisibility = candidate.DeletedAt == nil && authorization.New(db).Authorize(actorID, authorization.CapabilityCommentsChangeVisibility, &ownerID).Allowed
		}
		item := CommentManagementItem{
			ID: candidate.ID, MessageID: candidate.MessageID, ParentID: candidate.ParentID, UserID: candidate.UserID,
			Username: func() string {
				if candidate.UserID == nil {
					return ""
				}
				return usernames[*candidate.UserID]
			}(),
			Kind: kind, Content: candidate.Content, StoredVisibility: stored, EffectiveVisibility: effective,
			LimitedByAncestor: stored != effective, CreatedAt: candidate.CreatedAt, DeletedAt: candidate.DeletedAt,
			DeletedByUserID: candidate.DeletedByUserID, DeletionReasonCode: candidate.DeletionReasonCode,
			DeletedAncestorCommentID: candidate.DeletedAncestorCommentID, DeletedAncestorMessageID: candidate.DeletedAncestorMessageID,
			DeletionBatchID: candidate.DeletionBatchID, UserPurged: candidate.UserPurgedAt != nil,
			CanRestore: canRestore, CanPermanentlyDelete: canPermanentlyDelete, CanTrash: canTrash,
			CanOpenThread: candidate.DeletedAt == nil && canReadCommentInManagement(scope, message, candidate, commentMap),
			CanEdit:       canEdit, CanChangeVisibility: canChangeVisibility,
			Context: commentContext(scope, message, candidate, commentMap), MessageContext: messageContext,
			RecycleDeadline: CalculateRecycleDeadline(candidate.DeletedAt, commentRetention, now),
		}
		if personalOnly {
			// Personal surfaces intentionally do not expose the concrete identity
			// of a content administrator who performed the deletion.
			item.DeletedByUserID = nil
		}
		visible = append(visible, item)
	}
	result.Total = int64(len(visible))
	start := (page - 1) * pageSize
	if start > len(visible) {
		start = len(visible)
	}
	end := start + pageSize
	if end > len(visible) {
		end = len(visible)
	}
	result.Items = visible[start:end]
	return result, nil
}
