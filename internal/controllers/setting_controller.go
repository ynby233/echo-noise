package controllers

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/internal/authorization"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/dto"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/services"
)

func hasAdminOnlySettingFields(setting dto.SettingDto) bool {
	return setting.AllowRegistration != nil ||
		setting.AutoApproveRegistration != nil ||
		setting.SmtpEnabled != nil ||
		setting.SmtpDriver != nil ||
		setting.SmtpHost != nil ||
		setting.SmtpPort != nil ||
		setting.SmtpUser != nil ||
		setting.SmtpPass != nil ||
		setting.ClearSmtpUser != nil ||
		setting.ClearSmtpPass != nil ||
		setting.SmtpFrom != nil ||
		setting.SmtpEncryption != nil ||
		setting.SmtpTLS != nil ||
		setting.StorageEnabled != nil ||
		setting.StorageConfig != nil ||
		setting.AttachmentStorageEnabled != nil ||
		setting.AttachmentStorageConfig != nil ||
		setting.RecycleBinRetentionDays != nil ||
		setting.CommentRecycleBinRetentionDays != nil ||
		setting.NotifyNoteDeletionByPrimary != nil ||
		setting.NotifyCommentDeletionByPrimary != nil ||
		setting.VoceChatConfig != nil
}

func hasRSSManagementSettings(frontendSettings map[string]interface{}) bool {
	for _, field := range []string{
		"rssEnabled",
		"rssMemberIDs",
		"rssTitle",
		"rssDescription",
		"rssAuthorName",
		"rssFaviconURL",
	} {
		if _, exists := frontendSettings[field]; exists {
			return true
		}
	}
	return false
}

func hasLoginExpirySettings(frontendSettings map[string]interface{}) bool {
	for _, field := range []string{
		"loginExpireDays", "loginExpireHours",
		"delegatedAdminLoginExpireDays", "delegatedAdminLoginExpireHours",
	} {
		if _, exists := frontendSettings[field]; exists {
			return true
		}
	}
	return false
}

func UpdateMusicSetting(c *gin.Context) {
	_, err := checkUser(c)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[any](err.Error()))
		return
	}
	var request widgetPreferencesRequest
	if err := c.ShouldBindJSON(&request); err != nil || !services.IsMusicSettingsOnly(request.FrontendSettings) {
		c.JSON(http.StatusOK, dto.Fail[any]("音乐配置格式无效"))
		return
	}
	if err := services.UpdateFrontendSetting(0, map[string]interface{}{"frontendSettings": request.FrontendSettings}); err != nil {
		c.JSON(http.StatusOK, dto.Fail[any]("保存音乐配置失败: "+err.Error()))
		return
	}
	c.JSON(http.StatusOK, dto.OK[any](nil, "音乐配置已保存"))
}

type widgetPreferencesRequest struct {
	FrontendSettings map[string]interface{} `json:"frontendSettings"`
}

func GetWidgetPreferences(c *gin.Context) {
	user, err := checkUser(c)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[any](err.Error()))
		return
	}
	config, err := services.GetFrontendConfig(user.ID)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[any]("获取小组件设置失败"))
		return
	}
	c.JSON(http.StatusOK, dto.OK(config["frontendSettings"], "获取小组件设置成功"))
}

func UpdateWidgetPreferences(c *gin.Context) {
	user, err := checkUser(c)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[any](err.Error()))
		return
	}
	var request widgetPreferencesRequest
	if err := c.ShouldBindJSON(&request); err != nil || !services.IsUserFrontendSettingsOnly(request.FrontendSettings) {
		c.JSON(http.StatusOK, dto.Fail[any]("小组件设置格式无效"))
		return
	}
	if err := services.UpdateUserWidgetPreferences(user.ID, request.FrontendSettings); err != nil {
		c.JSON(http.StatusOK, dto.Fail[any]("保存我的小组件失败: "+err.Error()))
		return
	}
	c.JSON(http.StatusOK, dto.OK[any](nil, "我的小组件已保存"))
}

func GetGuestWidgetPreferences(c *gin.Context) {
	if _, err := requirePrimaryAdmin(c); err != nil {
		c.JSON(http.StatusOK, dto.Fail[any](err.Error()))
		return
	}
	config, err := services.GetFrontendConfig(0)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[any]("获取访客默认失败"))
		return
	}
	c.JSON(http.StatusOK, dto.OK(config["frontendSettings"], "获取访客默认成功"))
}

func UpdateGuestWidgetPreferences(c *gin.Context) {
	if _, err := requirePrimaryAdmin(c); err != nil {
		c.JSON(http.StatusOK, dto.Fail[any](err.Error()))
		return
	}
	var request widgetPreferencesRequest
	if err := c.ShouldBindJSON(&request); err != nil || !services.IsGuestWidgetSettingsOnly(request.FrontendSettings) {
		c.JSON(http.StatusOK, dto.Fail[any]("访客默认小组件设置格式无效"))
		return
	}
	if err := services.UpdateGuestWidgetPreferences(request.FrontendSettings); err != nil {
		c.JSON(http.StatusOK, dto.Fail[any]("保存访客默认失败: "+err.Error()))
		return
	}
	c.JSON(http.StatusOK, dto.OK[any](nil, "访客默认已保存"))
}

func UpdateSetting(c *gin.Context) {
	user, err := checkUser(c)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](err.Error()))
		return
	}

	var setting dto.SettingDto
	if err := c.ShouldBindJSON(&setting); err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](models.InvalidRequestBodyMessage))
		return
	}

	frontendSettings := setting.FrontendSettings
	if services.HasMusicSettings(frontendSettings) {
		db, _ := database.GetDB()
		if db == nil || !authorization.New(db).Authorize(user.ID, authorization.CapabilityMusicManage, nil).Allowed {
			c.JSON(http.StatusOK, dto.Fail[string]("需要音乐配置权限"))
			return
		}
	}
	if hasRSSManagementSettings(frontendSettings) && user.ID != models.PrimaryAdminUserID {
		c.JSON(http.StatusOK, dto.Fail[string]("仅站长可管理 RSS"))
		return
	}
	if hasLoginExpirySettings(frontendSettings) && user.ID != models.PrimaryAdminUserID {
		c.JSON(http.StatusOK, dto.Fail[string]("仅站长可管理登录过期时间"))
		return
	}
	if !user.IsAdmin {
		if hasAdminOnlySettingFields(setting) || frontendSettings == nil || !services.IsUserFrontendSettingsOnly(frontendSettings) {
			c.JSON(http.StatusOK, dto.Fail[string]("需要管理员权限"))
			return
		}
		if err := services.UpdateUserLifeCountdownConfig(user.ID, frontendSettings); err != nil {
			c.JSON(http.StatusOK, dto.Fail[string]("保存人生倒计时配置失败: "+err.Error()))
			return
		}
		if err := services.UpdateUserFrontendPreferenceConfig(user.ID, frontendSettings); err != nil {
			c.JSON(http.StatusOK, dto.Fail[string]("保存个人界面配置失败: "+err.Error()))
			return
		}
		c.JSON(http.StatusOK, dto.OK[any](nil, models.UpdateSettingSuccessMessage))
		return
	}

	db, _ := database.GetDB()
	var oldSetting models.Setting
	if err := db.Table("settings").First(&oldSetting).Error; err != nil {
		c.JSON(http.StatusOK, dto.Fail[string]("读取原有配置失败"))
		return
	}
	var oldSiteConfig models.SiteConfig
	_ = db.Table("site_configs").First(&oldSiteConfig).Error
	oldRetention := oldSiteConfig.RecycleBinRetentionDays
	oldCommentRetention := oldSiteConfig.CommentRecycleBinRetentionDays
	oldNotifyNoteDeletion := oldSiteConfig.NotifyNoteDeletionByPrimary
	oldNotifyCommentDeletion := oldSiteConfig.NotifyCommentDeletionByPrimary

	if setting.AllowRegistration != nil {
		oldSetting.AllowRegistration = *setting.AllowRegistration
	}
	if setting.AutoApproveRegistration != nil {
		oldSetting.AutoApproveRegistration = *setting.AutoApproveRegistration
	}

	settingMap := map[string]interface{}{}
	hasSiteConfigUpdate := false
	if setting.RecycleBinRetentionDays != nil {
		if user.ID != models.PrimaryAdminUserID {
			c.JSON(http.StatusOK, dto.Fail[string]("仅站长可管理回收站自动清理"))
			return
		}
		allowed := map[int]bool{0: true, 7: true, 30: true, 90: true, 180: true, 365: true}
		if !allowed[*setting.RecycleBinRetentionDays] {
			c.JSON(http.StatusOK, dto.Fail[string]("回收站保留期限无效"))
			return
		}
		settingMap["recycleBinRetentionDays"] = *setting.RecycleBinRetentionDays
		hasSiteConfigUpdate = true
	}
	if setting.CommentRecycleBinRetentionDays != nil {
		if user.ID != models.PrimaryAdminUserID {
			c.JSON(http.StatusOK, dto.Fail[string]("仅站长可管理互动回收站自动清理"))
			return
		}
		allowed := map[int]bool{0: true, 7: true, 30: true, 90: true, 180: true, 365: true}
		if !allowed[*setting.CommentRecycleBinRetentionDays] {
			c.JSON(http.StatusOK, dto.Fail[string]("互动回收站保留期限无效"))
			return
		}
		settingMap["commentRecycleBinRetentionDays"] = *setting.CommentRecycleBinRetentionDays
		hasSiteConfigUpdate = true
	}
	if setting.NotifyNoteDeletionByPrimary != nil || setting.NotifyCommentDeletionByPrimary != nil {
		if user.ID != models.PrimaryAdminUserID {
			c.JSON(http.StatusOK, dto.Fail[string]("仅站长可管理删除通知策略"))
			return
		}
		if setting.NotifyNoteDeletionByPrimary != nil {
			settingMap["notifyNoteDeletionByPrimary"] = *setting.NotifyNoteDeletionByPrimary
		}
		if setting.NotifyCommentDeletionByPrimary != nil {
			settingMap["notifyCommentDeletionByPrimary"] = *setting.NotifyCommentDeletionByPrimary
		}
		hasSiteConfigUpdate = true
	}
	if frontendSettings != nil {
		if services.HasLifeCountdownSettings(frontendSettings) {
			if err := services.UpdateUserLifeCountdownConfig(user.ID, frontendSettings); err != nil {
				c.JSON(http.StatusOK, dto.Fail[string]("保存人生倒计时配置失败: "+err.Error()))
				return
			}
			frontendSettings = services.StripLifeCountdownSettings(frontendSettings)
		}
		if len(frontendSettings) > 0 {
			settingMap["frontendSettings"] = frontendSettings
			hasSiteConfigUpdate = true
		}
	}
	if setting.AllowRegistration != nil {
		settingMap["allowRegistration"] = *setting.AllowRegistration
	}
	if setting.AutoApproveRegistration != nil {
		settingMap["autoApproveRegistration"] = *setting.AutoApproveRegistration
	}
	if setting.SmtpEnabled != nil {
		settingMap["smtpEnabled"] = *setting.SmtpEnabled
		hasSiteConfigUpdate = true
	}
	if setting.SmtpDriver != nil {
		settingMap["smtpDriver"] = *setting.SmtpDriver
		hasSiteConfigUpdate = true
	}
	if setting.SmtpHost != nil {
		settingMap["smtpHost"] = *setting.SmtpHost
		hasSiteConfigUpdate = true
	}
	if setting.SmtpPort != nil {
		settingMap["smtpPort"] = *setting.SmtpPort
		hasSiteConfigUpdate = true
	}
	if setting.SmtpUser != nil {
		settingMap["smtpUser"] = *setting.SmtpUser
		hasSiteConfigUpdate = true
	}
	if setting.SmtpPass != nil {
		settingMap["smtpPass"] = *setting.SmtpPass
		hasSiteConfigUpdate = true
	}
	if setting.ClearSmtpUser != nil {
		settingMap["clearSmtpUser"] = *setting.ClearSmtpUser
		hasSiteConfigUpdate = true
	}
	if setting.ClearSmtpPass != nil {
		settingMap["clearSmtpPass"] = *setting.ClearSmtpPass
		hasSiteConfigUpdate = true
	}
	if setting.SmtpFrom != nil {
		settingMap["smtpFrom"] = *setting.SmtpFrom
		hasSiteConfigUpdate = true
	}
	if setting.SmtpEncryption != nil {
		settingMap["smtpEncryption"] = *setting.SmtpEncryption
		hasSiteConfigUpdate = true
	}
	if setting.SmtpTLS != nil {
		settingMap["smtpTLS"] = *setting.SmtpTLS
		hasSiteConfigUpdate = true
	}

	if setting.StorageEnabled != nil {
		settingMap["storageEnabled"] = *setting.StorageEnabled
		hasSiteConfigUpdate = true
	}
	if setting.StorageConfig != nil {
		settingMap["storageConfig"] = setting.StorageConfig
		hasSiteConfigUpdate = true
	}

	if setting.AttachmentStorageEnabled != nil {
		settingMap["attachmentStorageEnabled"] = *setting.AttachmentStorageEnabled
		hasSiteConfigUpdate = true
	}
	if setting.AttachmentStorageConfig != nil {
		settingMap["attachmentStorageConfig"] = setting.AttachmentStorageConfig
		hasSiteConfigUpdate = true
	}
	if setting.VoceChatConfig != nil {
		if user.ID != models.PrimaryAdminUserID {
			c.JSON(http.StatusOK, dto.Fail[string]("仅站长可管理 VoceChat 配置"))
			return
		}
		settingMap["voceChatConfig"] = setting.VoceChatConfig
		hasSiteConfigUpdate = true
	}

	if hasSiteConfigUpdate {
		if err := services.UpdateFrontendSetting(0, settingMap); err != nil {
			c.JSON(http.StatusOK, dto.Fail[string]("保存前端配置失败: "+err.Error()))
			return
		}
		if setting.RecycleBinRetentionDays != nil {
			changes, _ := json.Marshal(map[string]int{"from": oldRetention, "to": *setting.RecycleBinRetentionDays})
			authorization.New(db).WriteAuditBestEffort(models.AdminAuditLog{
				ActorUserID: user.ID,
				Capability:  string(authorization.CapabilitySiteSettingsManage),
				Module:      "notes",
				Action:      "update_recycle_retention",
				TargetType:  "recycle_bin_policy",
				TargetID:    "retention_days",
				Result:      "success",
				Summary:     "updated recycle-bin retention policy",
				ChangesJSON: string(changes),
			})
		}
		if setting.CommentRecycleBinRetentionDays != nil {
			changes, _ := json.Marshal(map[string]int{"from": oldCommentRetention, "to": *setting.CommentRecycleBinRetentionDays})
			authorization.New(db).WriteAuditBestEffort(models.AdminAuditLog{
				ActorUserID: user.ID, Capability: string(authorization.CapabilitySiteSettingsManage),
				Module: "comments", Action: "update_recycle_retention", TargetType: "recycle_bin_policy",
				TargetID: "retention_days", Result: "success", Summary: "updated comment recycle-bin retention policy", ChangesJSON: string(changes),
			})
		}
		if setting.NotifyNoteDeletionByPrimary != nil || setting.NotifyCommentDeletionByPrimary != nil {
			changes, _ := json.Marshal(map[string]any{
				"note_from": oldNotifyNoteDeletion, "note_to": setting.NotifyNoteDeletionByPrimary,
				"comment_from": oldNotifyCommentDeletion, "comment_to": setting.NotifyCommentDeletionByPrimary,
			})
			authorization.New(db).WriteAuditBestEffort(models.AdminAuditLog{
				ActorUserID: user.ID, Capability: string(authorization.CapabilitySiteSettingsManage),
				Module: "notifications", Action: "update_primary_deletion_notification_policy",
				TargetType: "deletion_notification_policy", TargetID: "primary_admin", Result: "success",
				Summary: "updated primary-admin deletion notification policy", ChangesJSON: string(changes),
			})
		}
	}

	if err := db.Table("settings").Save(&oldSetting).Error; err != nil {
		c.JSON(http.StatusOK, dto.Fail[string]("保存配置失败"))
		return
	}

	if setting.AttachmentStorageEnabled != nil || setting.AttachmentStorageConfig != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
			defer cancel()
			if err := services.MigrateLegacyCloudAttachments(ctx); err != nil {
				log.Printf("历史云附件安全迁移暂未完成，将自动重试: %v", err)
			}
		}()
	}
	c.JSON(http.StatusOK, dto.OK[any](nil, models.UpdateSettingSuccessMessage))
}

func GetFrontendConfig(c *gin.Context) {
	viewerUserID := uint(0)
	if user, ok := currentReadUser(c); ok {
		viewerUserID = user.ID
	}

	config, err := services.GetFrontendConfig(viewerUserID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "获取配置失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 1, "data": config})
}

func CheckVoceChatHealth(c *gin.Context) {
	if _, err := requirePrimaryAdmin(c); err != nil {
		c.JSON(http.StatusOK, dto.Fail[map[string]interface{}](err.Error()))
		return
	}

	config, err := services.CheckVoceChatHealth(c.Request.Context())
	if err != nil {
		if config != nil {
			c.JSON(http.StatusOK, gin.H{"code": 0, "msg": err.Error(), "data": config})
			return
		}
		c.JSON(http.StatusOK, dto.Fail[map[string]interface{}](err.Error()))
		return
	}
	c.JSON(http.StatusOK, dto.OK(config, "VoceChat 健康检查完成"))
}

// SubmitFriendLinkApply 提交友链申请（公开）
