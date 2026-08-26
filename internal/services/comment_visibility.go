package services

import (
	"strings"

	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
)

func NormalizeCommentVisibility(value string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "public":
		return "public", true
	case "users", "members", "member", "all_users", "logged_in", "logged-in":
		return "users", true
	case "contacts", "contact", "mutual":
		return "contacts", true
	case "private", "only_me":
		return "private", true
	default:
		return "", false
	}
}

func NormalizedCommentVisibilityOrPublic(value string) string {
	visibility, ok := NormalizeCommentVisibility(value)
	if ok {
		return visibility
	}
	return "public"
}

func CommentVisibilityRank(value string) int {
	switch NormalizedCommentVisibilityOrPublic(value) {
	case "public":
		return 4
	case "users":
		return 3
	case "contacts":
		return 2
	case "private":
		return 1
	default:
		return 0
	}
}

func DefaultCommentVisibilityForMessage(messageVisibility string) string {
	switch messageVisibility {
	case MessageVisibilityPublic:
		return "public"
	case MessageVisibilityUsers:
		return "users"
	case MessageVisibilityContacts:
		return "contacts"
	case MessageVisibilityPrivate:
		return "private"
	default:
		return "public"
	}
}

func CommentVisibilityAllowedForMessage(visibility string, messageVisibility string) bool {
	visibility = NormalizedCommentVisibilityOrPublic(visibility)
	switch messageVisibility {
	case MessageVisibilityPublic:
		return true
	case MessageVisibilityUsers:
		return visibility == "users" || visibility == "contacts" || visibility == "private"
	case MessageVisibilityContacts:
		return visibility == "contacts" || visibility == "private"
	case MessageVisibilityPrivate:
		return visibility == "private"
	default:
		return visibility == "public"
	}
}

func EffectiveCommentVisibilityForMessage(visibility string, messageVisibility string) string {
	visibility = NormalizedCommentVisibilityOrPublic(visibility)
	if CommentVisibilityAllowedForMessage(visibility, messageVisibility) {
		return visibility
	}
	return DefaultCommentVisibilityForMessage(messageVisibility)
}

func CommentMap(comments []models.Comment) map[uint]models.Comment {
	commentMap := make(map[uint]models.Comment, len(comments))
	for _, comment := range comments {
		commentMap[comment.ID] = comment
	}
	return commentMap
}

func LoadCommentMapForMessage(messageID uint) (map[uint]models.Comment, error) {
	var comments []models.Comment
	if err := database.DB.Where("message_id = ?", messageID).Find(&comments).Error; err != nil {
		return nil, err
	}
	return CommentMap(comments), nil
}

func EffectiveCommentVisibilityInThread(comment models.Comment, messageVisibility string, commentMap map[uint]models.Comment) string {
	visibility := EffectiveCommentVisibilityForMessage(comment.Visibility, messageVisibility)
	seen := map[uint]bool{comment.ID: true}
	parentID := comment.ParentID
	for parentID != nil {
		if seen[*parentID] {
			break
		}
		parent, ok := commentMap[*parentID]
		if !ok {
			break
		}
		seen[parent.ID] = true
		storedParentVisibility := parent.Visibility
		if parent.IsTombstone && strings.TrimSpace(parent.TombstoneVisibility) != "" {
			storedParentVisibility = parent.TombstoneVisibility
		}
		parentVisibility := EffectiveCommentVisibilityForMessage(storedParentVisibility, messageVisibility)
		if CommentVisibilityRank(visibility) > CommentVisibilityRank(parentVisibility) {
			visibility = parentVisibility
		}
		parentID = parent.ParentID
	}
	return visibility
}

func (scope ContentReadScope) CanReadComment(message models.Message, comment models.Comment, commentMap map[uint]models.Comment) bool {
	if comment.DeletedAt != nil || comment.IsTombstone {
		return false
	}
	if !scope.CanReadMessage(message) {
		return false
	}
	if scope.canViewCommentInThread(message, comment, commentMap) {
		return true
	}
	if scope.primaryAdmin {
		return true
	}
	if !scope.administrator || !scope.viewHiddenInteractions {
		return false
	}
	if comment.UserID != nil && *comment.UserID == models.PrimaryAdminUserID {
		return false
	}
	if comment.ParentID != nil {
		parent, ok := commentMap[*comment.ParentID]
		return ok && scope.CanReadComment(message, parent, commentMap)
	}
	return true
}

func (scope ContentReadScope) CanInteractWithComment(message models.Message, comment models.Comment, commentMap map[uint]models.Comment) bool {
	return scope.hasActor && scope.canReadMessageNormally(message) && scope.canViewCommentInThread(message, comment, commentMap)
}

func (scope ContentReadScope) canViewCommentInThread(message models.Message, comment models.Comment, commentMap map[uint]models.Comment) bool {
	viewerID, hasViewer := scope.ActorID()
	if !scope.CanReadMessage(message) {
		return false
	}
	messageVisibility := StoredMessageVisibility(message)
	if messageVisibility == MessageVisibilityPrivate {
		return hasViewer && viewerID == message.UserID
	}

	var parent *models.Comment
	if comment.ParentID != nil {
		loaded, ok := commentMap[*comment.ParentID]
		if !ok {
			return false
		}
		if !scope.canViewCommentInThread(message, loaded, commentMap) {
			return false
		}
		parent = &loaded
	}

	visibility := EffectiveCommentVisibilityInThread(comment, messageVisibility, commentMap)
	if comment.ParentID != nil {
		switch visibility {
		case "public":
			return true
		case "users":
			return hasViewer
		case "contacts":
			if !hasViewer {
				return false
			}
			if messageVisibility == MessageVisibilityContacts {
				return true
			}
			if viewerID == message.UserID {
				return true
			}
			if comment.UserID != nil && *comment.UserID == viewerID {
				return true
			}
			if comment.UserID != nil && CanViewVoceChatContactAudience(*comment.UserID, viewerID) {
				return true
			}
			return parent != nil && parent.UserID != nil && *parent.UserID == viewerID
		case "private":
			if !hasViewer {
				return false
			}
			if viewerID == message.UserID {
				return true
			}
			if comment.UserID != nil && *comment.UserID == viewerID {
				return true
			}
			return parent != nil && parent.UserID != nil && *parent.UserID == viewerID
		default:
			return false
		}
	}

	switch visibility {
	case "public":
		return true
	case "users":
		return hasViewer
	case "contacts":
		if !hasViewer {
			return false
		}
		if messageVisibility == MessageVisibilityContacts {
			return true
		}
		if viewerID == message.UserID {
			return true
		}
		if comment.UserID != nil && *comment.UserID == viewerID {
			return true
		}
		return comment.UserID != nil && CanViewVoceChatContactAudience(*comment.UserID, viewerID)
	case "private":
		if !hasViewer {
			return false
		}
		if viewerID == message.UserID {
			return true
		}
		return comment.UserID != nil && *comment.UserID == viewerID
	default:
		return false
	}
}

func CanViewCommentInThread(message models.Message, comment models.Comment, commentMap map[uint]models.Comment, viewerID uint, hasViewer bool, _ bool) bool {
	var actorID *uint
	if hasViewer && viewerID != 0 {
		actorID = &viewerID
	}
	scope, err := ResolveContentReadScope(database.DB, actorID)
	return err == nil && scope.CanReadComment(message, comment, commentMap)
}

func CanUserViewCommentInThread(message models.Message, comment models.Comment, viewerID uint) bool {
	commentMap, err := LoadCommentMapForMessage(message.ID)
	if err != nil {
		return false
	}
	return CanViewCommentInThread(message, comment, commentMap, viewerID, viewerID != 0, false)
}
