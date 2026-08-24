package repository

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
)

func CreateVoceChatProvisioningTaskWithPermanentNumber(task *models.VoceChatProvisioningTask, emailDomain string) error {
	if task == nil || task.UserID == 0 {
		return fmt.Errorf("provisioning task user is required")
	}
	emailDomain = strings.TrimPrefix(strings.TrimSpace(emailDomain), "@")
	if emailDomain == "" {
		emailDomain = "vc.com"
	}
	var lastErr error
	for attempt := 0; attempt < 8; attempt++ {
		candidate := *task
		err := database.DB.Transaction(func(tx *gorm.DB) error {
			sequence, err := incrementRegistrationApplicationSequence(tx)
			if err != nil {
				return err
			}
			candidate.ApplicationID = strconv.FormatUint(sequence, 10)
			candidate.CandidateEmail = candidate.ApplicationID + "@" + emailDomain
			return tx.Create(&candidate).Error
		})
		if err == nil {
			*task = candidate
			return nil
		}
		lastErr = err
		if !isRetryableAllocationError(err) {
			return err
		}
		time.Sleep(time.Duration(attempt+1) * 10 * time.Millisecond)
	}
	return lastErr
}

func incrementRegistrationApplicationSequence(tx *gorm.DB) (uint64, error) {
	result := tx.Model(&models.RegistrationApplicationSequence{}).
		Where("id = ?", 1).
		UpdateColumn("last_value", gorm.Expr("last_value + ?", 1))
	if result.Error != nil {
		return 0, result.Error
	}
	if result.RowsAffected == 0 {
		if err := tx.Create(&models.RegistrationApplicationSequence{ID: 1}).Error; err != nil {
			return 0, err
		}
		if err := tx.Model(&models.RegistrationApplicationSequence{}).
			Where("id = ?", 1).
			UpdateColumn("last_value", gorm.Expr("last_value + ?", 1)).Error; err != nil {
			return 0, err
		}
	}
	var sequence models.RegistrationApplicationSequence
	if err := tx.First(&sequence, 1).Error; err != nil {
		return 0, err
	}
	return sequence.LastValue, nil
}

func isRetryableAllocationError(err error) bool {
	message := strings.ToLower(fmt.Sprint(err))
	return strings.Contains(message, "locked") || strings.Contains(message, "busy") || strings.Contains(message, "deadlock") || strings.Contains(message, "serialization")
}
