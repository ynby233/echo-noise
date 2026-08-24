package services

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
)

// GuestbookDescriptor is the stable fact callers need about the canonical
// system guestbook. The implementation owns legacy discovery and repair.
type GuestbookDescriptor struct {
	MessageID       uint
	RecipientUserID uint
}

var guestbookEnsureMu sync.Mutex

func GuestbookRecipientID() uint {
	return models.PrimaryAdminUserID
}

func IsGuestbookMessage(message models.Message) bool {
	return message.IsGuestbook || models.IsCanonicalGuestbookContent(message.Content)
}

func guestbookDescriptor(message models.Message) GuestbookDescriptor {
	return GuestbookDescriptor{MessageID: message.ID, RecipientUserID: GuestbookRecipientID()}
}

// ResolveGuestbook reads the canonical marker without creating data. It also
// understands the strict historical marker so old rows remain readable before
// the next startup migration/EnsureGuestbook call.
func ResolveGuestbook(db *gorm.DB) (GuestbookDescriptor, error) {
	if db == nil {
		return GuestbookDescriptor{}, errors.New("guestbook database is unavailable")
	}
	var marked models.Message
	if err := db.Where("is_guestbook = ?", true).Order("id ASC").Limit(1).Find(&marked).Error; err == nil && marked.ID != 0 {
		return guestbookDescriptor(marked), nil
	} else if err != nil {
		return GuestbookDescriptor{}, err
	}

	var messages []models.Message
	if err := db.Order("id ASC").Find(&messages).Error; err != nil {
		return GuestbookDescriptor{}, err
	}
	for _, message := range messages {
		if IsGuestbookMessage(message) {
			return guestbookDescriptor(message), nil
		}
	}
	return GuestbookDescriptor{}, gorm.ErrRecordNotFound
}

// EnsureGuestbook returns one canonical guestbook, repairs a strict historical
// row to logical owner ID 1, and creates it exactly once when absent. The
// process mutex protects concurrent GET compatibility calls in one process;
// the transaction keeps the repair/create sequence atomic for the database.
func EnsureGuestbook(db *gorm.DB) (GuestbookDescriptor, error) {
	if db == nil {
		return GuestbookDescriptor{}, errors.New("guestbook database is unavailable")
	}
	guestbookEnsureMu.Lock()
	defer guestbookEnsureMu.Unlock()

	tx := db.Begin()
	if tx.Error != nil {
		return GuestbookDescriptor{}, tx.Error
	}
	commit := false
	defer func() {
		if !commit {
			tx.Rollback()
		}
	}()
	primary, err := loadPrimaryAdmin(tx)
	if err != nil {
		return GuestbookDescriptor{}, err
	}

	var messages []models.Message
	if err := tx.Order("id ASC").Find(&messages).Error; err != nil {
		return GuestbookDescriptor{}, err
	}
	var selected *models.Message
	for index := range messages {
		if !messages[index].IsGuestbook && !models.IsCanonicalGuestbookContent(messages[index].Content) {
			continue
		}
		selected = &messages[index]
		break
	}

	if selected == nil {
		message := models.Message{
			Content:     models.CanonicalGuestbookContent,
			Username:    primary.Username,
			UserID:      models.PrimaryAdminUserID,
			Private:     false,
			Visibility:  MessageVisibilityPublic,
			IsGuestbook: true,
		}
		if err := tx.Create(&message).Error; err != nil {
			return GuestbookDescriptor{}, fmt.Errorf("create guestbook: %w", err)
		}
		selected = &message
	}

	if err := tx.Model(&models.Message{}).Where("is_guestbook = ?", true).Update("is_guestbook", false).Error; err != nil {
		return GuestbookDescriptor{}, err
	}
	if err := tx.Model(&models.Message{}).Where("id = ?", selected.ID).Updates(map[string]interface{}{
		"is_guestbook": true,
		"user_id":      models.PrimaryAdminUserID,
		"username":     primary.Username,
		"private":      false,
		"visibility":   MessageVisibilityPublic,
	}).Error; err != nil {
		return GuestbookDescriptor{}, err
	}
	if err := tx.Commit().Error; err != nil {
		return GuestbookDescriptor{}, err
	}
	commit = true
	return GuestbookDescriptor{MessageID: selected.ID, RecipientUserID: GuestbookRecipientID()}, nil
}

// GuestbookSQLPredicate is used only for compatibility queries that must
// exclude the canonical system row after migration has marked it.
func GuestbookSQLPredicate(column string) string {
	column = strings.TrimSpace(column)
	if column == "" {
		column = "messages.is_guestbook"
	}
	return "COALESCE(" + column + ", false) = false"
}
