package services

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/repository"
	"github.com/rcy1314/echo-noise/internal/vocechat"
	"gorm.io/gorm"
)

type RegistrationApplicationView struct {
	ID                 uint       `json:"id"`
	ApplicationID      string     `json:"application_id"`
	Username           string     `json:"username"`
	Status             string     `json:"status"`
	VoceChatUserID     string     `json:"voce_chat_user_id,omitempty"`
	VoceChatEmail      string     `json:"voce_chat_email,omitempty"`
	VoceChatSyncStatus string     `json:"voce_chat_sync_status,omitempty"`
	VoceChatSyncError  string     `json:"voce_chat_sync_error,omitempty"`
	LocalUserID        *uint      `json:"local_user_id,omitempty"`
	ReviewerUserID     *uint      `json:"reviewer_user_id,omitempty"`
	ReviewNote         string     `json:"review_note,omitempty"`
	ReviewedAt         *time.Time `json:"reviewed_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

type RegistrationApplicationListResult struct {
	Items []RegistrationApplicationView `json:"items"`
	Total int64                         `json:"total"`
}

type registrationVoceChatDeleteFunc func(application models.RegistrationApplication) error
type registrationVoceChatVerifyFunc func(application models.RegistrationApplication) (bool, error)

var registrationVoceChatDelete = deleteRegistrationUserWithVoceChat
var registrationVoceChatVerify = verifyRegistrationUserWithVoceChat

func ListRegistrationApplications(status string, limit int, offset int) (RegistrationApplicationListResult, error) {
	applications, total, err := repository.ListRegistrationApplications(status, limit, offset)
	if err != nil {
		return RegistrationApplicationListResult{}, err
	}
	views := make([]RegistrationApplicationView, 0, len(applications))
	for _, application := range applications {
		views = append(views, buildRegistrationApplicationView(application))
	}
	return RegistrationApplicationListResult{Items: views, Total: total}, nil
}

func ApproveRegistrationApplication(id uint, reviewerUserID uint, reviewNote string) (*models.User, error) {
	if id == 0 {
		return nil, errors.New(models.InvalidIDMessage)
	}
	application, err := repository.GetRegistrationApplicationByID(id)
	if err != nil {
		return nil, errors.New("注册申请不存在")
	}
	if application.Status != models.RegistrationApplicationStatusPending {
		return nil, errors.New("该注册申请已处理")
	}

	exists, err := existingUser(application.Username)
	if err != nil {
		return nil, errors.New(models.DatabaseErrorMessage)
	}
	if exists {
		return nil, errors.New(models.UsernameAlreadyExistsMessage)
	}

	plainStore := vocechat.DefaultPlainPasswordStore()
	plainRecord, ok, err := plainStore.GetApplicationPassword(application.ApplicationID)
	if err != nil {
		return nil, errors.New("读取注册申请明文密码失败")
	}
	plainPassword := plainRecord.VoceChatPasswordValue()
	if !ok || plainPassword == "" {
		return nil, errors.New("注册申请明文密码备份不存在，无法通过审核")
	}

	if err := ensureRegistrationVoceChatUser(application, plainPassword); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	linkedAt := now
	localUser := models.User{
		Username:           application.Username,
		Password:           application.PasswordHash,
		IsAdmin:            false,
		Token:              models.GenerateToken(32),
		VoceChatUserID:     strings.TrimSpace(application.VoceChatUserID),
		VoceChatEmail:      strings.TrimSpace(application.VoceChatEmail),
		VoceChatUsername:   strings.TrimSpace(application.Username),
		VoceChatLinkedAt:   &linkedAt,
		VoceChatSyncStatus: models.VoceChatSyncStatusLinked,
		VoceChatLastSyncAt: &now,
	}
	reviewerID := reviewerUserID
	note := strings.TrimSpace(reviewNote)

	var createdUser models.User
	err = database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&localUser).Error; err != nil {
			return err
		}
		createdUser = localUser
		if err := plainStore.UpsertUserVoceChatPassword(createdUser.ID, createdUser.Username, plainPassword, createdUser.VoceChatEmail, createdUser.VoceChatUserID); err != nil {
			return err
		}
		application.Status = models.RegistrationApplicationStatusApproved
		application.LocalUserID = &createdUser.ID
		application.ReviewerUserID = &reviewerID
		application.ReviewNote = note
		application.ReviewedAt = &now
		application.VoceChatSyncStatus = models.VoceChatSyncStatusLinked
		application.VoceChatSyncError = ""
		return tx.Save(application).Error
	})
	if err != nil {
		if createdUser.ID != 0 {
			_ = plainStore.DeleteUserPassword(createdUser.ID)
		}
		return nil, fmt.Errorf("通过注册申请失败: %w", err)
	}
	_ = plainStore.DeleteApplicationPassword(application.ApplicationID)
	return &createdUser, nil
}

func RejectRegistrationApplication(id uint, reviewerUserID uint, reviewNote string) error {
	if id == 0 {
		return errors.New(models.InvalidIDMessage)
	}
	application, err := repository.GetRegistrationApplicationByID(id)
	if err != nil {
		return errors.New("注册申请不存在")
	}
	if application.Status != models.RegistrationApplicationStatusPending {
		return errors.New("该注册申请已处理")
	}

	if strings.TrimSpace(application.VoceChatUserID) != "" {
		if err := registrationVoceChatDelete(*application); err != nil {
			_ = repository.UpdateRegistrationApplicationFields(application.ID, map[string]interface{}{
				"voce_chat_sync_status": models.VoceChatSyncStatusFailed,
				"voce_chat_sync_error":  strings.TrimSpace(err.Error()),
			})
			return fmt.Errorf("删除 VoceChat 预创建用户失败: %w", err)
		}
	}

	now := time.Now().UTC()
	reviewerID := reviewerUserID
	application.Status = models.RegistrationApplicationStatusRejected
	application.ReviewerUserID = &reviewerID
	application.ReviewNote = strings.TrimSpace(reviewNote)
	application.ReviewedAt = &now
	if strings.TrimSpace(application.VoceChatUserID) != "" || application.VoceChatSyncStatus == models.VoceChatSyncStatusCreated || application.VoceChatSyncStatus == models.VoceChatSyncStatusFailed {
		application.VoceChatSyncStatus = models.VoceChatSyncStatusNone
		application.VoceChatSyncError = ""
	}
	if err := repository.UpdateRegistrationApplication(application); err != nil {
		return fmt.Errorf("拒绝注册申请失败: %w", err)
	}
	_ = vocechat.DefaultPlainPasswordStore().DeleteApplicationPassword(application.ApplicationID)
	return nil
}

func ensureRegistrationVoceChatUser(application *models.RegistrationApplication, password string) error {
	if strings.TrimSpace(application.VoceChatUserID) != "" && application.VoceChatSyncStatus == models.VoceChatSyncStatusCreated {
		exists, err := registrationVoceChatVerify(*application)
		if err != nil {
			return fmt.Errorf("校验 VoceChat 预创建用户失败: %w", err)
		}
		if exists {
			return nil
		}
		application.VoceChatUserID = ""
		application.VoceChatSyncStatus = models.VoceChatSyncStatusPending
		application.VoceChatSyncError = "VoceChat 预创建用户不存在，已重新尝试创建"
	}

	provision := registrationVoceChatProvision(application.ApplicationID, application.Username, password)
	if strings.TrimSpace(provision.Email) != "" {
		application.VoceChatEmail = strings.TrimSpace(provision.Email)
	}
	if strings.TrimSpace(provision.UserID) != "" {
		application.VoceChatUserID = strings.TrimSpace(provision.UserID)
	}
	if strings.TrimSpace(provision.SyncStatus) != "" {
		application.VoceChatSyncStatus = strings.TrimSpace(provision.SyncStatus)
	}
	application.VoceChatSyncError = strings.TrimSpace(provision.SyncError)
	_ = repository.UpdateRegistrationApplicationFields(application.ID, map[string]interface{}{
		"voce_chat_user_id":     application.VoceChatUserID,
		"voce_chat_email":       application.VoceChatEmail,
		"voce_chat_sync_status": application.VoceChatSyncStatus,
		"voce_chat_sync_error":  application.VoceChatSyncError,
	})

	if strings.TrimSpace(application.VoceChatUserID) == "" || application.VoceChatSyncStatus != models.VoceChatSyncStatusCreated {
		if application.VoceChatSyncError != "" {
			return fmt.Errorf("VoceChat 账号未创建成功，暂不能通过审核: %s", application.VoceChatSyncError)
		}
		return errors.New("VoceChat 账号未创建成功，暂不能通过审核")
	}
	return nil
}

func verifyRegistrationUserWithVoceChat(application models.RegistrationApplication) (bool, error) {
	uid, err := strconv.ParseInt(strings.TrimSpace(application.VoceChatUserID), 10, 64)
	if err != nil || uid <= 0 {
		return false, nil
	}
	config := models.SiteConfig{}
	if database.DB != nil {
		_ = database.DB.First(&config).Error
	}
	cfg := vocechat.FromSiteConfig(config)
	if !cfg.Enabled || !cfg.IsReady() {
		return false, errors.New("VoceChat 未配置完成")
	}
	client, err := vocechat.NewClient(cfg)
	if err != nil {
		return false, err
	}
	tokenManager := vocechat.NewAdminTokenManager(client, cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	apiKey, err := tokenManager.GetToken(ctx)
	if err != nil {
		return false, err
	}
	user, err := client.GetUser(ctx, apiKey, uid)
	if err != nil {
		var apiErr *vocechat.APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
			return false, nil
		}
		return false, err
	}
	return user != nil && user.UID > 0, nil
}

func deleteRegistrationUserWithVoceChat(application models.RegistrationApplication) error {
	uid, err := strconv.ParseInt(strings.TrimSpace(application.VoceChatUserID), 10, 64)
	if err != nil || uid <= 0 {
		return errors.New("VoceChat 用户ID无效")
	}
	config := models.SiteConfig{}
	if database.DB != nil {
		_ = database.DB.First(&config).Error
	}
	cfg := vocechat.FromSiteConfig(config)
	if !cfg.Enabled || !cfg.IsReady() {
		return errors.New("VoceChat 未配置完成")
	}
	client, err := vocechat.NewClient(cfg)
	if err != nil {
		return err
	}
	tokenManager := vocechat.NewAdminTokenManager(client, cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	apiKey, err := tokenManager.GetToken(ctx)
	if err != nil {
		return err
	}
	return client.DeleteUser(ctx, apiKey, uid)
}

func buildRegistrationApplicationView(application models.RegistrationApplication) RegistrationApplicationView {
	return RegistrationApplicationView{
		ID:                 application.ID,
		ApplicationID:      application.ApplicationID,
		Username:           application.Username,
		Status:             application.Status,
		VoceChatUserID:     application.VoceChatUserID,
		VoceChatEmail:      application.VoceChatEmail,
		VoceChatSyncStatus: application.VoceChatSyncStatus,
		VoceChatSyncError:  application.VoceChatSyncError,
		LocalUserID:        application.LocalUserID,
		ReviewerUserID:     application.ReviewerUserID,
		ReviewNote:         application.ReviewNote,
		ReviewedAt:         application.ReviewedAt,
		CreatedAt:          application.CreatedAt,
		UpdatedAt:          application.UpdatedAt,
	}
}
