package services

import (
	"errors"

	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
)

const MaxCommentSummaryIDs = 100

var ErrInvalidCommentSummaryIDs = errors.New("comment summaries require 1 to 100 IDs")

type CommentSummary struct {
	ID                 uint  `json:"id"`
	Count              int64 `json:"count"`
	HasHiddenComments  bool  `json:"hasHiddenComments"`
	HasVisibleComments bool  `json:"hasVisibleComments"`
}

// LoadCommentSummaries counts active, readable top-level comments. Replies only
// affect HasVisibleComments, preserving expansion of threads below tombstones.
// HasHiddenComments stays false: the list has no hidden-comment hint contract,
// and an unreadable comment's existence must not be disclosed by this endpoint.
func LoadCommentSummaries(db *gorm.DB, scope ContentReadScope, ids []uint) ([]CommentSummary, error) {
	if len(ids) == 0 || len(ids) > MaxCommentSummaryIDs {
		return nil, ErrInvalidCommentSummaryIDs
	}
	uniqueIDs := make([]uint, 0, len(ids))
	seen := make(map[uint]bool, len(ids))
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			uniqueIDs = append(uniqueIDs, id)
		}
	}
	var messages []models.Message
	if err := db.Select("id", "user_id", "visibility", "private", "deleted_at", "is_tombstone", "tombstone_visibility").
		Where("id IN ?", uniqueIDs).Find(&messages).Error; err != nil {
		return nil, err
	}
	readable := make(map[uint]models.Message, len(messages))
	readableIDs := make([]uint, 0, len(messages))
	for _, message := range messages {
		// Match GetComments: a message tombstone may carry readable replies.
		if message.IsTombstone {
			message.DeletedAt = nil
		}
		if scope.CanReadMessage(message) {
			readable[message.ID] = message
			readableIDs = append(readableIDs, message.ID)
		}
	}
	result := make([]CommentSummary, 0, len(readable))
	if len(readableIDs) == 0 {
		return result, nil
	}
	var comments []models.Comment
	// Retain ancestors, including deleted rows and tombstones, for the shared
	// read policy; never load bodies, user profiles, or timestamp ordering.
	if err := db.Select("id", "message_id", "user_id", "parent_id", "visibility", "deleted_at", "is_tombstone", "tombstone_visibility").
		Where("message_id IN ?", readableIDs).Find(&comments).Error; err != nil {
		return nil, err
	}
	commentMap := CommentMap(comments)
	summaries := make(map[uint]CommentSummary, len(readable))
	for _, comment := range comments {
		message, ok := readable[comment.MessageID]
		if !ok || !scope.CanReadComment(message, comment, commentMap) {
			continue
		}
		summary := summaries[message.ID]
		summary.HasVisibleComments = true
		if comment.ParentID == nil {
			summary.Count++
		}
		summaries[message.ID] = summary
	}
	for _, id := range uniqueIDs {
		if _, ok := readable[id]; ok {
			summary := summaries[id]
			summary.ID = id
			result = append(result, summary)
		}
	}
	return result, nil
}
