package repository

import (
	"strings"
	"time"

	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
)

func ReplaceVoceChatContacts(userID uint, contacts []models.VoceChatContactCache) error {
	return database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ?", userID).Delete(&models.VoceChatContactCache{}).Error; err != nil {
			return err
		}
		if len(contacts) == 0 {
			return nil
		}
		return tx.Create(&contacts).Error
	})
}

func MarkVoceChatContactsSyncFailure(userID uint, voceChatUserID string, syncErr error, syncedAt time.Time, expiresAt time.Time) error {
	errorText := ""
	if syncErr != nil {
		errorText = strings.TrimSpace(syncErr.Error())
	}
	marker := models.VoceChatContactCache{
		UserID:         userID,
		ContactUserID:  0,
		VoceChatUserID: strings.TrimSpace(voceChatUserID),
		Source:         "vocechat",
		SyncedAt:       syncedAt,
		ExpiresAt:      expiresAt,
		LastSyncStatus: models.VoceChatContactSyncStatusFailed,
		LastSyncError:  errorText,
	}
	return ReplaceVoceChatContacts(userID, []models.VoceChatContactCache{marker})
}

func HasFreshVoceChatContactSyncRecord(userID uint, now time.Time) (bool, error) {
	var count int64
	err := database.DB.Model(&models.VoceChatContactCache{}).
		Where("user_id = ? AND expires_at > ?", userID, now).
		Count(&count).Error
	return count > 0, err
}

func ListFreshVoceChatContacts(userID uint, now time.Time) ([]models.VoceChatContactCache, error) {
	var contacts []models.VoceChatContactCache
	err := database.DB.
		Where("user_id = ? AND contact_user_id > 0 AND last_sync_status = ? AND expires_at > ?", userID, models.VoceChatContactSyncStatusOK, now).
		Find(&contacts).Error
	return contacts, err
}

func IsFreshVoceChatContact(userID uint, contactUserID uint, now time.Time) (bool, error) {
	var count int64
	err := database.DB.Model(&models.VoceChatContactCache{}).
		Where("user_id = ? AND contact_user_id = ? AND last_sync_status = ? AND expires_at > ?", userID, contactUserID, models.VoceChatContactSyncStatusOK, now).
		Count(&count).Error
	return count > 0, err
}

func DeleteVoceChatContactsForUser(userID uint) error {
	return database.DB.Where("user_id = ? OR contact_user_id = ?", userID, userID).Delete(&models.VoceChatContactCache{}).Error
}
