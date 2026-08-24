package services

import (
	"errors"
	"time"

	"github.com/rcy1314/echo-noise/internal/authorization"
	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
)

// ContentReadScope is the single backend interface for content reads and
// normal-visibility interaction checks. It is resolved from current DB state.
type ContentReadScope struct {
	actorID       uint
	hasActor      bool
	administrator bool
	primaryAdmin  bool
	viewHidden    bool
}

func ResolveContentReadScope(db *gorm.DB, actorID *uint) (ContentReadScope, error) {
	if actorID == nil || *actorID == 0 {
		return ContentReadScope{}, nil
	}
	if db == nil {
		return ContentReadScope{}, errors.New("content read scope database is unavailable")
	}
	var actor models.User
	if err := db.Select("id", "is_admin").First(&actor, *actorID).Error; err != nil {
		return ContentReadScope{}, err
	}
	scope := ContentReadScope{
		actorID:       actor.ID,
		hasActor:      true,
		administrator: actor.IsAdmin,
		primaryAdmin:  actor.IsAdmin && actor.ID == models.PrimaryAdminUserID,
	}
	if !actor.IsAdmin {
		return scope, nil
	}
	capabilities, err := authorization.New(db).CapabilitiesFor(actor.ID)
	if err != nil {
		return ContentReadScope{}, err
	}
	for _, capability := range capabilities {
		if capability == authorization.CapabilityContentViewHidden {
			scope.viewHidden = true
			break
		}
	}
	return scope, nil
}

func (scope ContentReadScope) ActorID() (uint, bool) {
	return scope.actorID, scope.hasActor
}

func (scope ContentReadScope) IsAdministrator() bool {
	return scope.administrator
}

func (scope ContentReadScope) IsPrimaryAdministrator() bool {
	return scope.primaryAdmin
}

func (scope ContentReadScope) CanViewHiddenContent() bool {
	return scope.viewHidden
}

func AuthorizeMessageMutation(db *gorm.DB, actorID uint, message models.Message, capability authorization.Capability) authorization.Decision {
	if message.DeletedAt != nil {
		return authorization.Decision{Reason: authorization.DenialContentNotReadable}
	}
	scope, err := ResolveContentReadScope(db, &actorID)
	if err != nil || !scope.CanReadMessage(message) {
		return authorization.Decision{Reason: authorization.DenialContentNotReadable}
	}
	return authorization.New(db).Authorize(actorID, capability, &message.UserID)
}

func AuthorizeCommentMutation(db *gorm.DB, actorID uint, message models.Message, comment models.Comment, commentMap map[uint]models.Comment, capability authorization.Capability) authorization.Decision {
	scope, err := ResolveContentReadScope(db, &actorID)
	if err != nil || !scope.CanReadComment(message, comment, commentMap) {
		return authorization.Decision{Reason: authorization.DenialContentNotReadable}
	}
	return authorization.New(db).Authorize(actorID, capability, comment.UserID)
}

func AuthorizeRecycleBinMutation(db *gorm.DB, actorID, targetOwnerID uint, capability authorization.Capability) authorization.Decision {
	authorizer := authorization.New(db)
	if prerequisite := authorizer.Authorize(actorID, authorization.CapabilityNotesRecycleBinView, &targetOwnerID); !prerequisite.Allowed {
		return authorization.Decision{Reason: authorization.DenialMissingPrerequisite}
	}
	return authorizer.Authorize(actorID, capability, &targetOwnerID)
}

func (scope ContentReadScope) CanReadMessage(message models.Message) bool {
	if message.DeletedAt != nil {
		return false
	}
	return scope.canReadMessage(message)
}

// CanReadMessageInRecycleBin applies the original visibility rules while
// allowing a caller that is already in the recycle-bin context to inspect a
// trashed row. It never grants ordinary front-end access to deleted content.
func (scope ContentReadScope) CanReadMessageInRecycleBin(message models.Message) bool {
	return scope.canReadMessage(message)
}

func (scope ContentReadScope) canReadMessage(message models.Message) bool {
	if scope.canReadMessageNormally(message) {
		return true
	}
	if scope.primaryAdmin {
		return true
	}
	return scope.administrator && scope.viewHidden && message.UserID != models.PrimaryAdminUserID
}

func (scope ContentReadScope) CanInteractWithMessage(message models.Message) bool {
	if message.DeletedAt != nil {
		return false
	}
	return scope.hasActor && scope.canReadMessageNormally(message)
}

func (scope ContentReadScope) canReadMessageNormally(message models.Message) bool {
	if scope.hasActor && message.UserID == scope.actorID {
		return true
	}
	switch StoredMessageVisibility(message) {
	case MessageVisibilityPublic:
		return true
	case MessageVisibilityUsers:
		return scope.hasActor
	case MessageVisibilityContacts:
		return scope.hasActor && CanViewVoceChatContactAudience(message.UserID, scope.actorID)
	case MessageVisibilityPrivate:
		return false
	default:
		return false
	}
}

func (scope ContentReadScope) ApplyMessageVisibility(query *gorm.DB) *gorm.DB {
	return scope.ApplyMessageVisibilityIncludingDeleted(query).Where("messages.deleted_at IS NULL")
}

func (scope ContentReadScope) ApplyMessageVisibilityIncludingDeleted(query *gorm.DB) *gorm.DB {
	if scope.primaryAdmin {
		return query
	}
	predicate, args := scope.normalMessageVisibilityPredicate()
	if scope.administrator && scope.viewHidden {
		return query.Where("(messages.user_id <> ? OR "+predicate+")", append([]interface{}{models.PrimaryAdminUserID}, args...)...)
	}
	return query.Where(predicate, args...)
}

func (scope ContentReadScope) normalMessageVisibilityPredicate() (string, []interface{}) {
	publicSQL := "(messages.private = ? AND (messages.visibility = ? OR messages.visibility = ? OR messages.visibility IS NULL))"
	publicArgs := []interface{}{false, MessageVisibilityPublic, ""}
	if !scope.hasActor {
		return publicSQL, publicArgs
	}
	if voceChatContactsVisibilityEnabled() {
		viewerID := scope.actorID
		// Delegated administrators without hidden-read access follow the same
		// contact audience rules as ordinary users, so their cache must also be
		// refreshed before applying the SQL predicate.
		EnsureVoceChatContactCachesForViewer(&viewerID, false)
		contactsSQL := "(messages.visibility = ? AND EXISTS (SELECT 1 FROM voce_chat_contact_caches AS vcc WHERE vcc.user_id = messages.user_id AND vcc.contact_user_id = ? AND vcc.last_sync_status = ? AND vcc.expires_at > ?))"
		contactsArgs := []interface{}{MessageVisibilityContacts, scope.actorID, models.VoceChatContactSyncStatusOK, time.Now().UTC()}
		if voceChatContactAuthorEligibilityRequired() {
			contactsSQL = "(messages.visibility = ? AND EXISTS (SELECT 1 FROM users AS contact_author WHERE contact_author.id = messages.user_id AND contact_author.voce_chat_email <> '' AND contact_author.voce_chat_user_id <> '' AND ((contact_author.id = ? AND contact_author.is_admin = ?) OR contact_author.voce_chat_sync_status = ?)) AND EXISTS (SELECT 1 FROM voce_chat_contact_caches AS vcc WHERE vcc.user_id = messages.user_id AND vcc.contact_user_id = ? AND vcc.last_sync_status = ? AND vcc.expires_at > ?))"
			contactsArgs = []interface{}{MessageVisibilityContacts, models.PrimaryAdminUserID, true, models.VoceChatSyncStatusLinked, scope.actorID, models.VoceChatContactSyncStatusOK, time.Now().UTC()}
		}
		args := []interface{}{
			scope.actorID,
			false, MessageVisibilityPublic, "",
			MessageVisibilityUsers,
		}
		args = append(args, contactsArgs...)
		return "(messages.user_id = ? OR " + publicSQL + " OR messages.visibility = ? OR " + contactsSQL + ")", args
	}
	return "(messages.user_id = ? OR " + publicSQL + " OR messages.visibility = ?)", []interface{}{
		scope.actorID,
		false, MessageVisibilityPublic, "",
		MessageVisibilityUsers,
	}
}
