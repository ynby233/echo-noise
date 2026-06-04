package repository

import (
	"strings"

	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
)

func CreateRegistrationApplication(application *models.RegistrationApplication) error {
	return database.DB.Create(application).Error
}

func GetRegistrationApplicationByID(id uint) (*models.RegistrationApplication, error) {
	var application models.RegistrationApplication
	if err := database.DB.First(&application, id).Error; err != nil {
		return nil, err
	}
	return &application, nil
}

func GetRegistrationApplicationByApplicationID(applicationID string) (*models.RegistrationApplication, error) {
	var application models.RegistrationApplication
	if err := database.DB.Where("application_id = ?", strings.TrimSpace(applicationID)).First(&application).Error; err != nil {
		return nil, err
	}
	return &application, nil
}

func GetPendingRegistrationApplicationByUsername(username string) (*models.RegistrationApplication, error) {
	var application models.RegistrationApplication
	err := database.DB.Where("username = ? AND status = ?", strings.TrimSpace(username), models.RegistrationApplicationStatusPending).First(&application).Error
	if err != nil {
		return nil, err
	}
	return &application, nil
}

func HasPendingRegistrationApplication(username string) (bool, error) {
	_, err := GetPendingRegistrationApplicationByUsername(username)
	if err == nil {
		return true, nil
	}
	if err == gorm.ErrRecordNotFound {
		return false, nil
	}
	return false, err
}

func CountPendingRegistrationApplications() (int64, error) {
	var count int64
	if err := database.DB.Model(&models.RegistrationApplication{}).Where("status = ?", models.RegistrationApplicationStatusPending).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func ListRegistrationApplications(status string, limit int, offset int) ([]models.RegistrationApplication, int64, error) {
	query := database.DB.Model(&models.RegistrationApplication{})
	if trimmed := strings.TrimSpace(status); trimmed != "" {
		query = query.Where("status = ?", trimmed)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	var applications []models.RegistrationApplication
	if err := query.Order("created_at DESC, id DESC").Limit(limit).Offset(offset).Find(&applications).Error; err != nil {
		return nil, 0, err
	}
	return applications, total, nil
}

func UpdateRegistrationApplication(application *models.RegistrationApplication) error {
	return database.DB.Save(application).Error
}

func UpdateRegistrationApplicationFields(id uint, updates map[string]interface{}) error {
	return database.DB.Model(&models.RegistrationApplication{}).Where("id = ?", id).Updates(updates).Error
}
