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
	"github.com/rcy1314/echo-noise/internal/vocechat"
)

type registrationVoceChatProvisionResult struct {
	Email      string
	UserID     string
	Username   string
	SyncStatus string
	SyncError  string
}

type registrationVoceChatProvisionFunc func(applicationID, username, password string) registrationVoceChatProvisionResult

var registrationVoceChatProvision = provisionRegistrationUserWithVoceChat

func buildVoceChatApplicationEmail(applicationID, domain string) string {
	applicationID = strings.ToLower(strings.TrimSpace(applicationID))
	domain = vocechat.NormalizeEmailDomain(domain)
	return applicationID + "@" + domain
}

func provisionRegistrationUserWithVoceChat(applicationID, username, password string) registrationVoceChatProvisionResult {
	config := models.SiteConfig{}
	if database.DB != nil {
		_ = database.DB.First(&config).Error
	}
	cfg := vocechat.FromSiteConfig(config)
	result := registrationVoceChatProvisionResult{
		Email:      buildVoceChatApplicationEmail(applicationID, cfg.EmailDomain),
		Username:   strings.TrimSpace(username),
		SyncStatus: models.VoceChatSyncStatusNone,
	}

	if !cfg.Enabled {
		return result
	}
	if !cfg.IsReady() {
		result.SyncStatus = models.VoceChatSyncStatusPending
		result.SyncError = "VoceChat 未配置完成，等待审核时重试"
		return result
	}

	client, err := vocechat.NewClient(cfg)
	if err != nil {
		result.SyncStatus = models.VoceChatSyncStatusPending
		result.SyncError = err.Error()
		return result
	}
	tokenManager := vocechat.NewAdminTokenManager(client, cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	apiKey, err := tokenManager.GetToken(ctx)
	if err != nil {
		result.SyncStatus = models.VoceChatSyncStatusPending
		result.SyncError = err.Error()
		return result
	}

	created, err := client.CreateUser(ctx, apiKey, vocechat.CreateUserRequest{
		Email:    result.Email,
		Password: password,
		Name:     strings.TrimSpace(username),
		Gender:   0,
		IsAdmin:  false,
		Language: "zh-CN",
	})
	if err != nil {
		if isVoceChatUserConflict(err) {
			result.SyncStatus = models.VoceChatSyncStatusConflicted
			result.SyncError = "VoceChat 用户名或邮箱已被占用"
			return result
		}
		result.SyncStatus = models.VoceChatSyncStatusPending
		result.SyncError = err.Error()
		return result
	}
	if created == nil || created.UID == 0 {
		result.SyncStatus = models.VoceChatSyncStatusPending
		result.SyncError = "VoceChat 创建用户未返回有效 UID"
		return result
	}

	result.SyncStatus = models.VoceChatSyncStatusCreated
	result.SyncError = ""
	result.UserID = strconv.FormatInt(created.UID, 10)
	result.Username = strings.TrimSpace(created.Name)
	if strings.TrimSpace(created.Email) != "" {
		result.Email = strings.TrimSpace(created.Email)
	}
	return result
}

func isVoceChatUserConflict(err error) bool {
	var apiErr *vocechat.APIError
	if errors.As(err, &apiErr) {
		if apiErr.StatusCode == 409 {
			return true
		}
		body := strings.ToLower(apiErr.Body)
		return apiErr.StatusCode == 400 && (strings.Contains(body, "conflict") || strings.Contains(body, "already") || strings.Contains(body, "exist") || strings.Contains(body, "duplicate"))
	}
	return strings.Contains(strings.ToLower(fmt.Sprint(err)), "conflict")
}
