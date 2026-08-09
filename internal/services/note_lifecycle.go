package services

import (
	"errors"
	"strings"
	"time"

	"github.com/rcy1314/echo-noise/internal/authorization"
	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
)

var (
	ErrMessageNotFound       = errors.New("message not found")
	ErrMessageAlreadyTrashed = errors.New("message is already in recycle bin")
	ErrMessageNotTrashed     = errors.New("message is not in recycle bin")
	ErrMessageNotVisible     = errors.New("message is not visible")
	ErrMessageNotAuthorized  = errors.New("message action is not authorized")
	ErrMessageProtected      = errors.New("message is protected")
)

type NoteManagementFilter struct {
	Page          int
	PageSize      int
	Keyword       string
	MessageID     *uint
	AuthorID      *uint
	Username      string
	Visibility    string
	Tag           string
	CreatedFrom   *time.Time
	CreatedTo     *time.Time
	Pinned        *bool
	HasAttachment *bool
	Sort          string
}

type NoteManagementItem struct {
	ID              uint       `json:"id"`
	Content         string     `json:"content"`
	Username        string     `json:"username"`
	ImageURL        string     `json:"image_url,omitempty"`
	Private         bool       `json:"private"`
	Visibility      string     `json:"visibility"`
	UserID          uint       `json:"user_id"`
	IsGuestbook     bool       `json:"is_guestbook"`
	CreatedAt       time.Time  `json:"created_at"`
	Pinned          bool       `json:"pinned"`
	PersonalPinned  bool       `json:"personal_pinned"`
	LikeCount       int        `json:"like_count"`
	DeletedAt       *time.Time `json:"deleted_at,omitempty"`
	DeletedByUserID *uint      `json:"deleted_by_user_id,omitempty"`
	DeletedReason   string     `json:"deleted_reason,omitempty"`
}

type NoteManagementPageResult struct {
	Total int64                `json:"total"`
	Items []NoteManagementItem `json:"items"`
}

func noteManagementItem(message models.Message) NoteManagementItem {
	return NoteManagementItem{
		ID: message.ID, Content: message.Content, Username: message.Username, ImageURL: message.ImageURL,
		Private: message.Private, Visibility: StoredMessageVisibility(message), UserID: message.UserID,
		IsGuestbook: message.IsGuestbook, CreatedAt: message.CreatedAt, Pinned: message.Pinned,
		PersonalPinned: message.PersonalPinned, LikeCount: message.LikeCount, DeletedAt: message.DeletedAt,
		DeletedByUserID: message.DeletedByUserID, DeletedReason: message.DeletedReason,
	}
}

func normalizeNoteManagementPagination(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	return page, pageSize
}

func loadMessageForLifecycle(db *gorm.DB, messageID uint) (models.Message, error) {
	if db == nil || messageID == 0 {
		return models.Message{}, ErrMessageNotFound
	}
	var message models.Message
	if err := db.First(&message, messageID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.Message{}, ErrMessageNotFound
		}
		return models.Message{}, err
	}
	return message, nil
}

func ensureLifecycleActorCanRead(db *gorm.DB, actorID uint, message models.Message, recycleBin bool) error {
	if actorID == 0 {
		return ErrMessageNotAuthorized
	}
	scope, err := ResolveContentReadScope(db, &actorID)
	if err != nil {
		return err
	}
	if recycleBin {
		if !scope.CanReadMessageInRecycleBin(message) {
			return ErrMessageNotVisible
		}
	} else if !scope.CanReadMessage(message) {
		return ErrMessageNotVisible
	}
	return nil
}

// TrashMessage is the single state transition used by author and administrator
// deletion paths. It never removes the Message row or its related data.
func TrashMessage(db *gorm.DB, actorID, messageID uint, reason string) error {
	return TrashMessageWithAudit(db, actorID, messageID, reason, nil)
}

// TrashMessageWithAudit atomically commits the lifecycle transition and an
// optional success-audit callback on the same transaction.
func TrashMessageWithAudit(db *gorm.DB, actorID, messageID uint, reason string, audit func(*gorm.DB) error) error {
	message, err := loadMessageForLifecycle(db, messageID)
	if err != nil {
		return err
	}
	if IsGuestbookMessage(message) {
		return ErrMessageProtected
	}
	if actorID != message.UserID {
		if err := ensureLifecycleActorCanRead(db, actorID, message, false); err != nil {
			return err
		}
		if decision := authorization.New(db).Authorize(actorID, authorization.CapabilityNotesTrash, &message.UserID); !decision.Allowed {
			if decision.Reason == authorization.DenialContentNotReadable {
				return ErrMessageNotVisible
			}
			if decision.Reason == authorization.DenialProtectedContent {
				return ErrMessageProtected
			}
			return ErrMessageNotAuthorized
		}
		var primaryComments int64
		if err := db.Model(&models.Comment{}).Where("message_id = ? AND user_id = ?", message.ID, models.PrimaryAdminUserID).Count(&primaryComments).Error; err != nil {
			return err
		}
		if primaryComments > 0 {
			return ErrMessageProtected
		}
	}
	if message.DeletedAt != nil {
		return ErrMessageAlreadyTrashed
	}
	deletedAt := time.Now().UTC()
	deletedBy := actorID
	reason = strings.TrimSpace(reason)
	return db.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&models.Message{}).
			Where("id = ? AND deleted_at IS NULL", messageID).
			Updates(map[string]interface{}{"deleted_at": deletedAt, "deleted_by_user_id": deletedBy, "deleted_reason": reason})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrMessageAlreadyTrashed
		}
		if audit != nil {
			if err := audit(tx); err != nil {
				return err
			}
		}
		return nil
	})
}

func RestoreMessage(db *gorm.DB, actorID, messageID uint) error {
	return RestoreMessageWithAudit(db, actorID, messageID, nil)
}

func RestoreMessageWithAudit(db *gorm.DB, actorID, messageID uint, audit func(*gorm.DB) error) error {
	message, err := loadMessageForLifecycle(db, messageID)
	if err != nil {
		return err
	}
	if IsGuestbookMessage(message) {
		return ErrMessageProtected
	}
	if err := ensureLifecycleActorCanRead(db, actorID, message, true); err != nil {
		return err
	}
	if message.DeletedAt == nil {
		return ErrMessageNotTrashed
	}
	if decision := AuthorizeRecycleBinMutation(db, actorID, message.UserID, authorization.CapabilityNotesRestore); !decision.Allowed {
		if decision.Reason == authorization.DenialProtectedContent {
			return ErrMessageProtected
		}
		return ErrMessageNotAuthorized
	}
	return db.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&models.Message{}).Where("id = ? AND deleted_at IS NOT NULL", messageID).
			Updates(map[string]interface{}{"deleted_at": nil, "deleted_by_user_id": nil, "deleted_reason": ""})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrMessageNotTrashed
		}
		if audit != nil {
			if err := audit(tx); err != nil {
				return err
			}
		}
		return nil
	})
}

func PermanentlyDeleteMessage(db *gorm.DB, actorID, messageID uint, reason string) error {
	return PermanentlyDeleteMessageWithAudit(db, actorID, messageID, reason, nil)
}

func PermanentlyDeleteMessageWithAudit(db *gorm.DB, actorID, messageID uint, reason string, audit func(*gorm.DB) error) error {
	message, err := loadMessageForLifecycle(db, messageID)
	if err != nil {
		return err
	}
	if IsGuestbookMessage(message) {
		return ErrMessageProtected
	}
	if err := ensureLifecycleActorCanRead(db, actorID, message, true); err != nil {
		return err
	}
	if message.DeletedAt == nil {
		return ErrMessageNotTrashed
	}
	if decision := AuthorizeRecycleBinMutation(db, actorID, message.UserID, authorization.CapabilityNotesDelete); !decision.Allowed {
		if decision.Reason == authorization.DenialProtectedContent {
			return ErrMessageProtected
		}
		return ErrMessageNotAuthorized
	}
	return db.Transaction(func(tx *gorm.DB) error {
		var comments []models.Comment
		if err := tx.Where("message_id = ?", messageID).Find(&comments).Error; err != nil {
			return err
		}
		if err := RemoveUnreferencedMessageAttachmentReferences(tx, messageID, message, comments); err != nil {
			return err
		}
		if err := tx.Where("message_id = ?", messageID).Delete(&models.Comment{}).Error; err != nil {
			return err
		}
		if err := tx.Where("message_id = ?", messageID).Delete(&models.MessageLike{}).Error; err != nil {
			return err
		}
		result := tx.Where("id = ? AND deleted_at IS NOT NULL", messageID).Delete(&models.Message{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrMessageNotTrashed
		}
		if audit != nil {
			if err := audit(tx); err != nil {
				return err
			}
		}
		return nil
	})
}

func buildNoteManagementQuery(db *gorm.DB, actorID uint, filter NoteManagementFilter, recycleBin bool) (*gorm.DB, error) {
	scope, err := ResolveContentReadScope(db, &actorID)
	if err != nil {
		return nil, err
	}
	query := db.Model(&models.Message{})
	if recycleBin {
		query = scope.ApplyMessageVisibilityIncludingDeleted(query).Where("messages.deleted_at IS NOT NULL")
	} else {
		query = scope.ApplyMessageVisibility(query)
	}
	if filter.MessageID != nil && *filter.MessageID != 0 {
		query = query.Where("messages.id = ?", *filter.MessageID)
	}
	if filter.AuthorID != nil && *filter.AuthorID != 0 {
		query = query.Where("messages.user_id = ?", *filter.AuthorID)
	} else if strings.TrimSpace(filter.Username) != "" {
		query = query.Where("messages.username = ?", strings.TrimSpace(filter.Username))
	}
	if strings.TrimSpace(filter.Keyword) != "" {
		query = query.Where("messages.content LIKE ?", "%"+strings.TrimSpace(filter.Keyword)+"%")
	}
	if strings.TrimSpace(filter.Visibility) != "" {
		query = query.Where("messages.visibility = ?", strings.TrimSpace(filter.Visibility))
	}
	if strings.TrimSpace(filter.Tag) != "" {
		query = query.Where("messages.content LIKE ?", "%#"+strings.TrimPrefix(strings.TrimSpace(filter.Tag), "#")+"%")
	}
	if filter.CreatedFrom != nil {
		query = query.Where("messages.created_at >= ?", *filter.CreatedFrom)
	}
	if filter.CreatedTo != nil {
		query = query.Where("messages.created_at < ?", *filter.CreatedTo)
	}
	if filter.Pinned != nil {
		query = query.Where("messages.pinned = ?", *filter.Pinned)
	}
	if filter.HasAttachment != nil {
		if *filter.HasAttachment {
			query = query.Where("(messages.image_url <> '' OR messages.content LIKE '%/api/%attachments/%' OR messages.content LIKE '%/api/%/refs/%')")
		} else {
			query = query.Where("messages.image_url = '' AND messages.content NOT LIKE '%/api/%attachments/%' AND messages.content NOT LIKE '%/api/%/refs/%'")
		}
	}
	return query, nil
}

func ListNoteManagementMessages(db *gorm.DB, actorID uint, filter NoteManagementFilter) (NoteManagementPageResult, error) {
	filter.Page, filter.PageSize = normalizeNoteManagementPagination(filter.Page, filter.PageSize)
	query, err := buildNoteManagementQuery(db, actorID, filter, false)
	if err != nil {
		return NoteManagementPageResult{}, err
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return NoteManagementPageResult{}, err
	}
	var messages []models.Message
	order := noteManagementOrder(filter.Sort, false)
	if err := query.Order(order).Limit(filter.PageSize).Offset((filter.Page - 1) * filter.PageSize).Find(&messages).Error; err != nil {
		return NoteManagementPageResult{}, err
	}
	ApplyMessageViewerState(messages, &actorID)
	items := make([]NoteManagementItem, 0, len(messages))
	for _, message := range messages {
		items = append(items, noteManagementItem(message))
	}
	return NoteManagementPageResult{Total: total, Items: items}, nil
}

func ListRecycleBinMessages(db *gorm.DB, actorID uint, filter NoteManagementFilter) (NoteManagementPageResult, error) {
	filter.Page, filter.PageSize = normalizeNoteManagementPagination(filter.Page, filter.PageSize)
	query, err := buildNoteManagementQuery(db, actorID, filter, true)
	if err != nil {
		return NoteManagementPageResult{}, err
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return NoteManagementPageResult{}, err
	}
	var messages []models.Message
	if err := query.Order(noteManagementOrder(filter.Sort, true)).Limit(filter.PageSize).Offset((filter.Page - 1) * filter.PageSize).Find(&messages).Error; err != nil {
		return NoteManagementPageResult{}, err
	}
	items := make([]NoteManagementItem, 0, len(messages))
	for _, message := range messages {
		items = append(items, noteManagementItem(message))
	}
	return NoteManagementPageResult{Total: total, Items: items}, nil
}

func noteManagementOrder(sort string, recycleBin bool) string {
	if recycleBin {
		switch strings.TrimSpace(sort) {
		case "oldest":
			return "messages.deleted_at ASC, messages.id ASC"
		case "created_desc":
			return "messages.created_at DESC, messages.id DESC"
		case "created_asc":
			return "messages.created_at ASC, messages.id ASC"
		default:
			return "messages.deleted_at DESC, messages.id DESC"
		}
	}
	switch strings.TrimSpace(sort) {
	case "oldest":
		return "messages.created_at ASC, messages.id ASC"
	case "pinned":
		return "messages.pinned DESC, messages.pinned_at DESC, messages.created_at DESC, messages.id DESC"
	default:
		return "messages.created_at DESC, messages.id DESC"
	}
}

func GetRecycleBinMessageForViewer(db *gorm.DB, actorID, messageID uint) (*NoteManagementItem, error) {
	message, err := loadMessageForLifecycle(db, messageID)
	if err != nil {
		return nil, err
	}
	if message.DeletedAt == nil {
		return nil, ErrMessageNotTrashed
	}
	if err := ensureLifecycleActorCanRead(db, actorID, message, true); err != nil {
		return nil, err
	}
	item := noteManagementItem(message)
	return &item, nil
}

func GetNoteManagementMessageForViewer(db *gorm.DB, actorID, messageID uint) (*NoteManagementItem, error) {
	message, err := loadMessageForLifecycle(db, messageID)
	if err != nil {
		return nil, err
	}
	if err := ensureLifecycleActorCanRead(db, actorID, message, false); err != nil {
		return nil, err
	}
	item := noteManagementItem(message)
	return &item, nil
}
