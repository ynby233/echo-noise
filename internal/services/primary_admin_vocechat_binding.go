package services

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/repository"
	"github.com/rcy1314/echo-noise/internal/vocechat"
)

// BindPrimaryAdminVoceChatEmail validates an existing VoceChat account through
// the independently configured management credential, then records the
// account as user ID 1's registration binding. It does not change the
// management credential or the primary administrator's local login method.
func BindPrimaryAdminVoceChatEmail(ctx context.Context, actorUserID uint, email string) (*models.User, error) {
	if actorUserID != models.PrimaryAdminUserID {
		return nil, errors.New("仅1号管理员可以修改自己的注册绑定 VoceChat 邮箱")
	}
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || !isValidVoceChatAdminEmail(email) {
		return nil, errors.New("VoceChat 邮箱格式无效")
	}

	db, err := database.GetDB()
	if err != nil {
		return nil, errors.New(models.DatabaseErrorMessage)
	}
	var config models.SiteConfig
	if err := db.Table("site_configs").First(&config).Error; err != nil {
		return nil, errors.New("VoceChat 配置不可用")
	}
	vcConfig := vocechat.FromSiteConfig(config)
	if !vcConfig.IsReady() {
		return nil, errors.New("VoceChat 当前未启用或管理凭据未配置完整")
	}

	client, err := vocechat.NewClient(vcConfig)
	if err != nil {
		return nil, errors.New("VoceChat 当前不可用，请检查注册配置")
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	apiKey, err := vocechat.NewAdminTokenManager(client, vcConfig).GetToken(ctx)
	if err != nil {
		return nil, errors.New("VoceChat 当前不可用，请检查注册配置")
	}
	remoteUsers, err := client.ListUsers(ctx, apiKey)
	if err != nil {
		return nil, errors.New("VoceChat 当前不可用，请检查注册配置")
	}

	var remoteUser *vocechat.User
	for i := range remoteUsers {
		if strings.EqualFold(strings.TrimSpace(remoteUsers[i].Email), email) {
			remoteUser = &remoteUsers[i]
			break
		}
	}
	if remoteUser == nil || remoteUser.UID <= 0 {
		return nil, errors.New("该 VoceChat 邮箱不存在")
	}
	remoteEmail := strings.ToLower(strings.TrimSpace(remoteUser.Email))
	remoteUID := strconv.FormatInt(remoteUser.UID, 10)
	var occupied int64
	if err := db.Model(&models.User{}).
		Where("id <> ? AND (LOWER(voce_chat_email) = ? OR voce_chat_user_id = ?)", models.PrimaryAdminUserID, remoteEmail, remoteUID).
		Count(&occupied).Error; err != nil {
		return nil, errors.New(models.DatabaseErrorMessage)
	}
	if occupied > 0 {
		return nil, errors.New("该 VoceChat 账户已绑定其他用户")
	}
	var reserved int64
	if err := db.Model(&models.RegistrationApplication{}).
		Where("status = ? AND (LOWER(voce_chat_email) = ? OR voce_chat_user_id = ?)", models.RegistrationApplicationStatusPending, remoteEmail, remoteUID).
		Count(&reserved).Error; err != nil {
		return nil, errors.New(models.DatabaseErrorMessage)
	}
	if reserved > 0 {
		return nil, errors.New("该 VoceChat 账户已由待处理的注册申请占用")
	}

	now := time.Now().UTC()
	updates := map[string]interface{}{
		"voce_chat_email":        remoteEmail,
		"voce_chat_user_id":      remoteUID,
		"voce_chat_username":     strings.TrimSpace(remoteUser.Name),
		"voce_chat_sync_status":  models.VoceChatSyncStatusLinked,
		"voce_chat_sync_error":   "",
		"voce_chat_linked_at":    now,
		"voce_chat_last_sync_at": now,
	}
	result := db.Model(&models.User{}).Where("id = ? AND is_admin = ?", models.PrimaryAdminUserID, true).Updates(updates)
	if result.Error != nil {
		return nil, fmt.Errorf("保存 VoceChat 绑定失败: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return nil, errors.New("1号管理员账户不存在")
	}
	repository.ClearUserCache()
	bound, err := repository.GetUserByID(models.PrimaryAdminUserID)
	if err != nil {
		return nil, errors.New(models.DatabaseErrorMessage)
	}
	updatePlainPasswordUserMetadata(bound)
	return bound, nil
}
