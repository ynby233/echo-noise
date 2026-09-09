package services

import (
	"context"
	"fmt"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/vocechat"
	"gorm.io/gorm"
	"net/mail"
	"strings"
	"time"
)

// VoceChat settings own credential patch semantics, health-affecting fields and health persistence.
func parseBoolSetting(value interface{}) (bool, bool) {
	switch v := value.(type) {
	case bool:
		return v, true
	case string:
		text := strings.TrimSpace(v)
		if strings.EqualFold(text, "true") {
			return true, true
		}
		if strings.EqualFold(text, "false") {
			return false, true
		}
	}
	return false, false
}

func applySensitiveStringSetting(raw map[string]interface{}, valueKey, clearKey string, target *string) {
	if target == nil {
		return
	}
	if clear, exists := raw[clearKey]; exists && parseBoolLike(clear, false) {
		*target = ""
		return
	}
	if v, ok := raw[valueKey].(string); ok {
		v = strings.TrimSpace(v)
		if v != "" {
			*target = v
		}
	}
}

func isValidVoceChatAdminEmail(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || !strings.Contains(value, "@") {
		return false
	}
	addr, err := mail.ParseAddress(value)
	return err == nil && addr.Address == value
}

func applyVoceChatConfigUpdate(config *models.SiteConfig, raw map[string]interface{}) error {
	if raw == nil {
		return nil
	}

	legacyModeSwitches := config.RuntimeModeMigrationVersion < models.RuntimeModeMigrationVersionCurrent
	if legacyModeSwitches {
		if v, exists := raw["enabled"]; exists {
			config.VoceChatEnabled = parseBoolLike(v, config.VoceChatEnabled)
		}
	}
	if v, ok := raw["baseURL"].(string); ok {
		config.VoceChatBaseURL = vocechat.NormalizeBaseURL(v)
	}
	if v, ok := raw["adminUsername"].(string); ok {
		config.VoceChatAdminUsername = strings.TrimSpace(v)
	}
	applySensitiveStringSetting(raw, "adminPassword", "clearAdminPassword", &config.VoceChatAdminPassword)
	applySensitiveStringSetting(raw, "adminToken", "clearAdminToken", &config.VoceChatAdminToken)
	applySensitiveStringSetting(raw, "thirdPartySecret", "clearThirdPartySecret", &config.VoceChatThirdPartySecret)
	applySensitiveStringSetting(raw, "botApiKey", "clearBotApiKey", &config.VoceChatBotAPIKey)
	if legacyModeSwitches {
		if v, exists := raw["notificationEnabled"]; exists {
			config.VoceChatNotificationEnabled = parseBoolLike(v, config.VoceChatNotificationEnabled)
		}
	}
	if v, ok := raw["emailDomain"].(string); ok {
		config.VoceChatEmailDomain = vocechat.NormalizeEmailDomain(v)
	}
	if legacyModeSwitches {
		if v, exists := raw["loginVerificationEnabled"]; exists {
			config.VoceChatLoginVerificationEnabled = parseBoolLike(v, config.VoceChatLoginVerificationEnabled)
		}
		if v, exists := raw["localFallbackEnabled"]; exists {
			config.VoceChatLocalFallbackEnabled = parseBoolLike(v, config.VoceChatLocalFallbackEnabled)
		}
		if v, exists := raw["contactsEnabled"]; exists {
			config.VoceChatContactsEnabled = parseBoolLike(v, config.VoceChatContactsEnabled)
		}
	}
	if v, exists := raw["contactsCacheTTLSeconds"]; exists {
		if ttl, ok := parsePositiveIntSetting(v); ok {
			config.VoceChatContactsCacheTTLSeconds = ttl
		}
	}
	if config.VoceChatContactsCacheTTLSeconds <= 0 {
		config.VoceChatContactsCacheTTLSeconds = vocechat.DefaultContactsCacheTTLSeconds
	}
	if strings.TrimSpace(config.VoceChatEmailDomain) == "" {
		config.VoceChatEmailDomain = vocechat.DefaultEmailDomain
	}
	if legacyModeSwitches && !config.VoceChatEnabled {
		config.VoceChatLoginVerificationEnabled = false
		config.VoceChatContactsEnabled = false
		config.VoceChatNotificationEnabled = false
	}
	if strings.TrimSpace(config.VoceChatAdminToken) == "" && strings.TrimSpace(config.VoceChatAdminPassword) != "" && strings.TrimSpace(config.VoceChatAdminUsername) != "" {
		if !isValidVoceChatAdminEmail(config.VoceChatAdminUsername) {
			return fmt.Errorf("管理员邮箱格式无效，请填写 VoceChat 管理员邮箱，不要填写显示名")
		}
	}
	if voceChatHealthAffectingConfigChanged(raw, legacyModeSwitches) {
		config.VoceChatLastHealthStatus = ""
		config.VoceChatLastHealthError = ""
		config.VoceChatLastHealthCheckAt = nil
	}
	return nil
}

func voceChatHealthAffectingConfigChanged(raw map[string]interface{}, includeLegacyModeSwitches bool) bool {
	for _, key := range []string{
		"baseURL",
		"adminUsername",
		"adminPassword",
		"clearAdminPassword",
		"adminToken",
		"clearAdminToken",
		"thirdPartySecret",
		"clearThirdPartySecret",
		"botApiKey",
		"clearBotApiKey",
	} {
		if _, exists := raw[key]; exists {
			return true
		}
	}
	if includeLegacyModeSwitches {
		for _, key := range []string{"enabled", "notificationEnabled", "loginVerificationEnabled", "contactsEnabled"} {
			if _, exists := raw[key]; exists {
				return true
			}
		}
	}
	return false
}

func CheckVoceChatHealth(ctx context.Context) (map[string]interface{}, error) {
	db, err := database.GetDB()
	if err != nil {
		return nil, fmt.Errorf("获取数据库连接失败: %w", err)
	}

	var config models.SiteConfig
	if err := db.Table("site_configs").First(&config).Error; err != nil {
		return nil, fmt.Errorf("读取 VoceChat 配置失败: %w", err)
	}

	now := time.Now().UTC()
	fail := func(healthErr error) (map[string]interface{}, error) {
		writeVoceChatHealth(db, config.ID, "failed", healthErr, now)
		config.VoceChatLastHealthStatus = "failed"
		config.VoceChatLastHealthError = strings.TrimSpace(healthErr.Error())
		config.VoceChatLastHealthCheckAt = &now
		return vocechat.PublicConfigFromSiteConfig(config, true), healthErr
	}

	vcConfig := vocechat.FromSiteConfig(config)
	if strings.TrimSpace(vcConfig.BaseURL) == "" {
		return fail(fmt.Errorf("VoceChat 服务地址未配置"))
	}
	client, err := vocechat.NewClient(vcConfig)
	if err != nil {
		return fail(err)
	}

	token := strings.TrimSpace(vcConfig.AdminToken)
	var adminLogin *vocechat.LoginResponse
	if strings.TrimSpace(vcConfig.AdminUsername) != "" || strings.TrimSpace(vcConfig.AdminPassword) != "" {
		if strings.TrimSpace(vcConfig.AdminUsername) == "" || strings.TrimSpace(vcConfig.AdminPassword) == "" {
			return fail(fmt.Errorf("VoceChat 管理员邮箱或密码未配置完整"))
		}
		if !isValidVoceChatAdminEmail(vcConfig.AdminUsername) {
			return fail(fmt.Errorf("管理员邮箱格式无效，请填写 VoceChat 管理员邮箱，不要填写显示名"))
		}
		adminLogin, err = client.LoginWithPassword(ctx, vcConfig.AdminUsername, vcConfig.AdminPassword, "echo-noise-health-check")
		if err != nil {
			return fail(err)
		}
		if adminLogin == nil || strings.TrimSpace(adminLogin.Token) == "" {
			return fail(fmt.Errorf("VoceChat 管理员登录未返回 token"))
		}
		if !adminLogin.User.IsAdmin {
			return fail(fmt.Errorf("VoceChat 管理员邮箱对应账号不是管理员"))
		}
		token = strings.TrimSpace(adminLogin.Token)
	}
	if token == "" {
		return fail(fmt.Errorf("VoceChat 管理员凭据未配置"))
	}
	if err := client.CheckHealth(ctx, token); err != nil {
		return fail(err)
	}
	writeVoceChatHealth(db, config.ID, "ok", nil, now)
	config.VoceChatLastHealthStatus = "ok"
	config.VoceChatLastHealthError = ""
	config.VoceChatLastHealthCheckAt = &now
	return vocechat.PublicConfigFromSiteConfig(config, true), nil
}

func writeVoceChatHealth(db *gorm.DB, configID uint, status string, healthErr error, checkedAt time.Time) {
	if db == nil || configID == 0 {
		return
	}
	errorText := ""
	if healthErr != nil {
		errorText = strings.TrimSpace(healthErr.Error())
	}
	_ = db.Model(&models.SiteConfig{}).Where("id = ?", configID).Updates(map[string]interface{}{
		"voce_chat_last_health_status":   status,
		"voce_chat_last_health_error":    errorText,
		"voce_chat_last_health_check_at": checkedAt,
	}).Error
}
