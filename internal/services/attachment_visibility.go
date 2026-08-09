package services

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/rcy1314/echo-noise/internal/authorization"
	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
)

// AttachmentSource is the stable, viewer-independent provenance used by
// attachment administration and file transport checks.  A source is either a
// note (message), a top-level comment, a reply, or a guestbook entry/reply.
type AttachmentSource struct {
	SourceType      string          `json:"source_type"`
	SourceID        uint            `json:"source_id"`
	MessageID       uint            `json:"message_id"`
	CommentID       *uint           `json:"comment_id,omitempty"`
	ParentCommentID *uint           `json:"parent_comment_id,omitempty"`
	OwnerUserID     uint            `json:"owner_user_id"`
	Visibility      string          `json:"visibility"`
	Message         models.Message  `json:"-"`
	Comment         *models.Comment `json:"-"`
}

// LoadAttachmentSources returns every content row which references the given
// logical attachment.  Matching is deliberately done against the canonical
// managed URL prefix/public id, never against arbitrary external URLs.
func LoadAttachmentSources(db *gorm.DB, reference models.AttachmentReference, backend string) ([]AttachmentSource, error) {
	if db == nil {
		return nil, fmt.Errorf("attachment visibility database is unavailable")
	}
	publicID := strings.TrimSpace(reference.PublicID)
	if publicID == "" {
		return nil, nil
	}
	var messages []models.Message
	if err := db.Where("deleted_at IS NULL").Find(&messages).Error; err != nil {
		return nil, err
	}
	var comments []models.Comment
	if err := db.Find(&comments).Error; err != nil {
		return nil, err
	}
	commentsByMessage := make(map[uint][]models.Comment)
	for _, comment := range comments {
		commentsByMessage[comment.MessageID] = append(commentsByMessage[comment.MessageID], comment)
	}
	needle := attachmentURLNeedle(reference, backend)
	contains := func(value string) bool { return strings.Contains(value, needle) || strings.Contains(value, publicID) }
	out := make([]AttachmentSource, 0)
	for _, message := range messages {
		if message.UserID == reference.OwnerUserID && (contains(message.Content) || contains(message.ImageURL)) {
			out = append(out, AttachmentSource{SourceType: "message", SourceID: message.ID, MessageID: message.ID, OwnerUserID: message.UserID, Visibility: StoredMessageVisibility(message), Message: message})
		}
		thread := commentsByMessage[message.ID]
		commentMap := CommentMap(thread)
		for index := range thread {
			comment := thread[index]
			if !contains(comment.Content) || commentOwner(comment, message.UserID) != reference.OwnerUserID {
				continue
			}
			sourceType := "comment"
			if comment.ParentID != nil {
				sourceType = "reply"
			}
			if IsGuestbookMessage(message) {
				sourceType = "guestbook"
			}
			visibility := EffectiveCommentVisibilityInThread(comment, StoredMessageVisibility(message), commentMap)
			commentID := comment.ID
			out = append(out, AttachmentSource{SourceType: sourceType, SourceID: comment.ID, MessageID: message.ID, CommentID: &commentID, ParentCommentID: comment.ParentID, OwnerUserID: commentOwner(comment, message.UserID), Visibility: visibility, Message: message, Comment: &comment})
		}
	}
	return out, nil
}

func commentOwner(comment models.Comment, fallback uint) uint {
	if comment.UserID != nil && *comment.UserID != 0 {
		return *comment.UserID
	}
	return fallback
}

func attachmentURLNeedle(reference models.AttachmentReference, backend string) string {
	if backend == "cloud" {
		return "/api/cloud-attachments/" + strings.TrimSpace(reference.PublicID) + "/"
	}
	prefix := "/api/files/refs/"
	switch reference.Kind {
	case "image":
		prefix = "/api/images/refs/"
	case "video":
		prefix = "/api/video/refs/"
	case "audio":
		prefix = "/api/audio/refs/"
	}
	return prefix + strings.TrimSpace(reference.PublicID) + "/"
}

// AttachmentSourceVisible applies the same ContentReadScope and guestbook
// rules used by normal content pages.  content.view_hidden never widens
// primary-admin-owned content or canonical guestbook threads.
func AttachmentSourceVisible(db *gorm.DB, actorID *uint, source AttachmentSource) (bool, error) {
	scope, err := ResolveContentReadScope(db, actorID)
	if err != nil {
		return false, err
	}
	if source.Comment == nil {
		return scope.CanReadMessage(source.Message), nil
	}
	var comments []models.Comment
	if err := db.Where("message_id = ?", source.MessageID).Find(&comments).Error; err != nil {
		return false, err
	}
	return scope.CanReadComment(source.Message, *source.Comment, CommentMap(comments)), nil
}

// VisibleAttachmentSources returns the sources visible to the current actor;
// callers may safely expose only this result and its derived counts/labels.
func VisibleAttachmentSources(db *gorm.DB, actorID *uint, reference models.AttachmentReference, backend string) ([]AttachmentSource, error) {
	sources, err := LoadAttachmentSources(db, reference, backend)
	if err != nil {
		return nil, err
	}
	visible := make([]AttachmentSource, 0, len(sources))
	for _, source := range sources {
		ok, err := AttachmentSourceVisible(db, actorID, source)
		if err != nil {
			return nil, err
		}
		if ok {
			visible = append(visible, source)
		}
	}
	return visible, nil
}

// VisibleRecycleBinAttachmentSources is restricted to an explicit recycle-bin
// context. It includes deleted messages only when the actor's independent
// recycle-bin capability and hidden-content/primary-admin protections allow
// that source; normal attachment listing must continue using the function
// above.
func VisibleRecycleBinAttachmentSources(db *gorm.DB, actorID *uint, reference models.AttachmentReference, backend string) ([]AttachmentSource, error) {
	if db == nil || actorID == nil || *actorID == 0 {
		return nil, fmt.Errorf("recycle-bin attachment context requires an actor")
	}
	if decision := authorization.New(db).Authorize(*actorID, authorization.CapabilityNotesRecycleBinView, &reference.OwnerUserID); !decision.Allowed {
		return nil, errors.New("recycle-bin attachment access denied")
	}
	scope, err := ResolveContentReadScope(db, actorID)
	if err != nil {
		return nil, err
	}
	var messages []models.Message
	if err := db.Where("deleted_at IS NOT NULL").Find(&messages).Error; err != nil {
		return nil, err
	}
	var comments []models.Comment
	if err := db.Find(&comments).Error; err != nil {
		return nil, err
	}
	needle := attachmentURLNeedle(reference, backend)
	contains := func(value string) bool {
		return strings.Contains(value, needle) || strings.Contains(value, strings.TrimSpace(reference.PublicID))
	}
	byMessage := make(map[uint][]models.Comment)
	for _, comment := range comments {
		byMessage[comment.MessageID] = append(byMessage[comment.MessageID], comment)
	}
	out := make([]AttachmentSource, 0)
	for _, message := range messages {
		if !scope.CanReadMessageInRecycleBin(message) {
			continue
		}
		if message.UserID == reference.OwnerUserID && (contains(message.Content) || contains(message.ImageURL)) {
			out = append(out, AttachmentSource{SourceType: "message", SourceID: message.ID, MessageID: message.ID, OwnerUserID: message.UserID, Visibility: StoredMessageVisibility(message), Message: message})
		}
		thread := byMessage[message.ID]
		commentMap := CommentMap(thread)
		for i := range thread {
			comment := thread[i]
			if !contains(comment.Content) || !scope.CanReadComment(message, comment, commentMap) {
				continue
			}
			cid := comment.ID
			typeName := "comment"
			if comment.ParentID != nil {
				typeName = "reply"
			}
			if IsGuestbookMessage(message) {
				typeName = "guestbook"
			}
			out = append(out, AttachmentSource{SourceType: typeName, SourceID: comment.ID, MessageID: message.ID, CommentID: &cid, ParentCommentID: comment.ParentID, OwnerUserID: commentOwner(comment, message.UserID), Visibility: EffectiveCommentVisibilityInThread(comment, StoredMessageVisibility(message), commentMap), Message: message, Comment: &comment})
		}
	}
	return out, nil
}

// AttachmentReferenceURLName is shared by tests and legacy scanners when
// validating managed attachment URLs.
func AttachmentReferenceURLName(raw string) string {
	decoded, err := url.PathUnescape(strings.TrimSpace(raw))
	if err != nil {
		return strings.TrimSpace(raw)
	}
	return decoded
}

// VisibleLegacyAttachmentSources applies the same source model to pre-registry
// local/cloud URLs whose logical identity is a filename or cloud public id.
func VisibleLegacyAttachmentSources(db *gorm.DB, actorID *uint, kind, name string) ([]AttachmentSource, error) {
	return visibleLegacyAttachmentSourcesContext(db, actorID, kind, name, false)
}

// VisibleRecycleBinLegacyAttachmentSources is the explicit deleted-content
// context for pre-registry local URLs. It never falls back to owner visibility
// and only considers deleted note sources.
func VisibleRecycleBinLegacyAttachmentSources(db *gorm.DB, actorID *uint, kind, name string) ([]AttachmentSource, error) {
	if db == nil || actorID == nil || *actorID == 0 {
		return nil, fmt.Errorf("recycle-bin attachment context requires an actor")
	}
	if decision := authorization.New(db).Authorize(*actorID, authorization.CapabilityNotesRecycleBinView, nil); !decision.Allowed {
		return nil, errors.New("recycle-bin attachment access denied")
	}
	return visibleLegacyAttachmentSourcesContext(db, actorID, kind, name, true)
}

func visibleLegacyAttachmentSourcesContext(db *gorm.DB, actorID *uint, kind, name string, recycleBin bool) ([]AttachmentSource, error) {
	if db == nil {
		return nil, fmt.Errorf("attachment visibility database is unavailable")
	}
	var messages []models.Message
	messageQuery := db
	if recycleBin {
		messageQuery = messageQuery.Where("deleted_at IS NOT NULL")
	} else {
		messageQuery = messageQuery.Where("deleted_at IS NULL")
	}
	if err := messageQuery.Find(&messages).Error; err != nil {
		return nil, err
	}
	var comments []models.Comment
	if err := db.Find(&comments).Error; err != nil {
		return nil, err
	}
	commentsByMessage := make(map[uint][]models.Comment)
	for _, c := range comments {
		commentsByMessage[c.MessageID] = append(commentsByMessage[c.MessageID], c)
	}
	cloudNeedles := legacyCloudAttachmentNeedles(db, name)
	match := func(content string) bool {
		if kind == "cloud" {
			for _, needle := range cloudNeedles {
				if strings.Contains(content, needle) {
					return true
				}
			}
			return false
		}
		for _, ref := range models.ExtractLocalAttachmentReferences(models.Message{Content: content}) {
			if ref.Kind == kind && ref.Name == name {
				return true
			}
		}
		return false
	}
	all := make([]AttachmentSource, 0)
	for _, message := range messages {
		if recycleBin {
			scope, err := ResolveContentReadScope(db, actorID)
			if err != nil {
				return nil, err
			}
			if !scope.CanReadMessageInRecycleBin(message) {
				continue
			}
		}
		// Scan the note and its discussion independently. A note without an
		// attachment can still own comment/reply attachments, so the parent
		// match must never short-circuit the child scan.
		if match(message.Content) || match(message.ImageURL) {
			all = append(all, AttachmentSource{SourceType: "message", SourceID: message.ID, MessageID: message.ID, OwnerUserID: message.UserID, Visibility: StoredMessageVisibility(message), Message: message})
		}
		thread := commentsByMessage[message.ID]
		commentMap := CommentMap(thread)
		for i := range thread {
			comment := thread[i]
			if !match(comment.Content) {
				continue
			}
			if recycleBin {
				scope, err := ResolveContentReadScope(db, actorID)
				if err != nil {
					return nil, err
				}
				if !scope.CanReadComment(message, comment, commentMap) {
					continue
				}
			}
			t := "comment"
			if comment.ParentID != nil {
				t = "reply"
			}
			if IsGuestbookMessage(message) {
				t = "guestbook"
			}
			cid := comment.ID
			all = append(all, AttachmentSource{SourceType: t, SourceID: comment.ID, MessageID: message.ID, CommentID: &cid, ParentCommentID: comment.ParentID, OwnerUserID: commentOwner(comment, message.UserID), Visibility: EffectiveCommentVisibilityInThread(comment, StoredMessageVisibility(message), commentMap), Message: message, Comment: &comment})
		}
	}
	if actorID == nil || recycleBin {
		return all, nil
	}
	visible := make([]AttachmentSource, 0, len(all))
	for _, source := range all {
		ok, err := AttachmentSourceVisible(db, actorID, source)
		if err != nil {
			return nil, err
		}
		if ok {
			visible = append(visible, source)
		}
	}
	return visible, nil
}

// VisibleLegacyAttachmentSourcesForViewer is the transport-safe variant for
// legacy file URLs. Unlike the administration compatibility scanner, it always
// applies ContentReadScope, including for anonymous viewers.
func VisibleLegacyAttachmentSourcesForViewer(db *gorm.DB, actorID *uint, kind, name string) ([]AttachmentSource, error) {
	all, err := VisibleLegacyAttachmentSources(db, nil, kind, name)
	if err != nil {
		return nil, err
	}
	visible := make([]AttachmentSource, 0, len(all))
	for _, source := range all {
		ok, err := AttachmentSourceVisible(db, actorID, source)
		if err != nil {
			return nil, err
		}
		if ok {
			visible = append(visible, source)
		}
	}
	return visible, nil
}

// legacyCloudAttachmentNeedles covers both the managed application URL and
// historical public-bucket URLs. The latter are constrained to the configured
// public base and the object's persisted key; arbitrary external URLs never
// become attachment sources.
func legacyCloudAttachmentNeedles(db *gorm.DB, publicID string) []string {
	publicID = strings.TrimSpace(publicID)
	if publicID == "" {
		return nil
	}
	needles := []string{"/api/cloud-attachments/" + publicID + "/"}
	var object models.CloudAttachmentObject
	if db != nil && db.Migrator().HasTable(&models.CloudAttachmentObject{}) && db.Where("public_id = ?", publicID).First(&object).Error == nil {
		var cfg models.SiteConfig
		if db.Table("site_configs").First(&cfg).Error == nil {
			base := strings.TrimRight(strings.TrimSpace(cfg.AttachmentStoragePublicBaseURL), "/")
			if base != "" {
				basePath := ""
				if parsed, err := url.Parse(base); err == nil {
					basePath = strings.Trim(parsed.EscapedPath(), "/")
				}
				for _, key := range []string{object.ObjectKey, object.LegacyObjectKey} {
					key = strings.TrimLeft(strings.TrimSpace(key), "/")
					if key == "" {
						continue
					}
					relativeKey := strings.TrimPrefix(key, basePath+"/")
					needles = append(needles, base+"/"+relativeKey)
					needles = append(needles, base+"/"+url.PathEscape(relativeKey))
				}
			}
		}
	}
	return dedupeStrings(needles)
}

func dedupeStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
