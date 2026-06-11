package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/mail"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/syncmanager"
	"github.com/rcy1314/echo-noise/internal/vocechat"
	"github.com/rcy1314/echo-noise/pkg"
	"gorm.io/gorm"
)

type lifeCountdownSettings struct {
	Enabled             bool
	BirthDate           string
	LifeExpectancyYears int
}

type RSSConfig struct {
	Enabled          bool
	MemberIDs        []uint
	AvailableMembers []map[string]interface{}
	Title            string
	Description      string
	AuthorName       string
	FaviconURL       string
}

var lifeCountdownSettingKeys = map[string]struct{}{
	"lifeCountdownEnabled":   {},
	"lifeCountdownBirthDate": {},
	"lifeExpectancyYears":    {},
}

const defaultHeaderImageURL = "https://s2.loli.net/2025/03/26/d7iyuPYA8cRqD1K.jpg"

const (
	defaultLoginExpireDays  = 3
	defaultLoginExpireHours = 0
	maxLoginExpireDays      = 31
	maxLoginExpireHours     = 24
)

func normalizeLoginExpireConfig(days int, hours int) (int, int) {
	if days < 0 {
		days = 0
	}
	if hours < 0 {
		hours = 0
	}
	if days > maxLoginExpireDays {
		return maxLoginExpireDays, maxLoginExpireHours
	}
	if hours > maxLoginExpireHours {
		hours = maxLoginExpireHours
	}
	if days == 0 && hours == 0 {
		return defaultLoginExpireDays, defaultLoginExpireHours
	}
	return days, hours
}

func parsePositiveIntSetting(raw interface{}) (int, bool) {
	switch v := raw.(type) {
	case float64:
		return int(v), true
	case float32:
		return int(v), true
	case int:
		return v, true
	case int64:
		return int(v), true
	case uint:
		return int(v), true
	case string:
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return n, true
		}
	}
	return 0, false
}

var legacyDefaultHeaderImageURLs = map[string]struct{}{
	"https://s2.loli.net/2025/03/27/KJ1trnU2ksbFEYM.jpg": {},
	"https://s2.loli.net/2025/03/27/MZqaLczCvwjSmW7.jpg": {},
	"https://s2.loli.net/2025/03/27/UMijKXwJ9yTqSeE.jpg": {},
	"https://s2.loli.net/2025/03/27/WJQIlkXvBg2afcR.jpg": {},
	"https://s2.loli.net/2025/03/27/oHNQtf4spkq2iln.jpg": {},
	"https://s2.loli.net/2025/03/27/PMRuX5loc6Uaimw.jpg": {},
	"https://s2.loli.net/2025/03/27/U2WIslbNyTLt4rD.jpg": {},
	"https://s2.loli.net/2025/03/27/xu1jZL5Og4pqT9d.jpg": {},
	"https://s2.loli.net/2025/03/27/OXqwzZ6v3PVIns9.jpg": {},
	"https://s2.loli.net/2025/03/27/HGuqlE6apgNywbh.jpg": {},
	"https://s2.loli.net/2025/03/27/7Zck3y6XTzhYPs5.jpg": {},
	"https://s2.loli.net/2025/03/27/wYy12qDMH6bGJOI.jpg": {},
	"https://s2.loli.net/2025/03/27/y67m2k5xcSdTsHN.jpg": {},
	defaultHeaderImageURL:                                {},
}

func defaultHeaderImages() []string {
	return []string{defaultHeaderImageURL}
}

func defaultHeaderImagesJSON() string {
	data, err := json.Marshal(defaultHeaderImages())
	if err != nil {
		return `["` + defaultHeaderImageURL + `"]`
	}
	return string(data)
}

func defaultRSSConfigValues() RSSConfig {
	return RSSConfig{
		Enabled:     false,
		MemberIDs:   []uint{},
		Title:       "Noise的说说笔记",
		Description: "一个说说笔记~",
		AuthorName:  "Noise",
		FaviconURL:  "/favicon-32x32.png",
	}
}

func parseRSSMemberIDString(raw string) ([]uint, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, false
	}

	var ids []uint
	if err := json.Unmarshal([]byte(trimmed), &ids); err == nil {
		return ids, true
	}

	return []uint{}, true
}

func parseRSSMemberIDValue(raw interface{}) ([]uint, bool) {
	switch v := raw.(type) {
	case []uint:
		return v, true
	case []int:
		ids := make([]uint, 0, len(v))
		for _, id := range v {
			if id > 0 {
				ids = append(ids, uint(id))
			}
		}
		return ids, true
	case []float64:
		ids := make([]uint, 0, len(v))
		for _, id := range v {
			if id > 0 {
				ids = append(ids, uint(id))
			}
		}
		return ids, true
	case []interface{}:
		ids := make([]uint, 0, len(v))
		for _, item := range v {
			switch id := item.(type) {
			case float64:
				if id > 0 {
					ids = append(ids, uint(id))
				}
			case int:
				if id > 0 {
					ids = append(ids, uint(id))
				}
			case uint:
				if id > 0 {
					ids = append(ids, id)
				}
			case string:
				if parsed, err := strconv.ParseUint(strings.TrimSpace(id), 10, 64); err == nil && parsed > 0 {
					ids = append(ids, uint(parsed))
				}
			}
		}
		return ids, true
	case string:
		return parseRSSMemberIDString(v)
	default:
		return []uint{}, true
	}
}

func normalizeRSSMemberIDs(db *gorm.DB, ids []uint) ([]uint, error) {
	requested := make([]uint, 0, len(ids))
	seen := map[uint]struct{}{}
	for _, id := range ids {
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		requested = append(requested, id)
	}
	if len(requested) == 0 {
		return []uint{}, nil
	}

	var users []models.User
	if err := db.Select("id").Where("id IN ?", requested).Find(&users).Error; err != nil {
		return nil, err
	}
	valid := map[uint]struct{}{}
	for _, user := range users {
		valid[user.ID] = struct{}{}
	}

	normalized := make([]uint, 0, len(requested))
	for _, id := range requested {
		if _, ok := valid[id]; ok {
			normalized = append(normalized, id)
		}
	}
	return normalized, nil
}

func defaultRSSMemberIDs(db *gorm.DB) ([]uint, error) {
	var ids []uint
	if err := db.Model(&models.User{}).Where("is_admin = ?", true).Order("id ASC").Pluck("id", &ids).Error; err != nil {
		return nil, err
	}
	return ids, nil
}

func resolveRSSMemberIDs(db *gorm.DB, raw string) ([]uint, error) {
	ids, explicit := parseRSSMemberIDString(raw)
	if !explicit {
		return defaultRSSMemberIDs(db)
	}
	return normalizeRSSMemberIDs(db, ids)
}

func getRSSAvailableMembers(db *gorm.DB) ([]map[string]interface{}, error) {
	var users []models.User
	if err := db.Select("id, username, is_admin, avatar_url").Order("is_admin DESC, id ASC").Find(&users).Error; err != nil {
		return nil, err
	}

	members := make([]map[string]interface{}, 0, len(users))
	for _, user := range users {
		members = append(members, map[string]interface{}{
			"id":        user.ID,
			"username":  user.Username,
			"isAdmin":   user.IsAdmin,
			"avatarURL": strings.TrimSpace(user.AvatarURL),
		})
	}
	return members, nil
}

func buildRSSConfig(db *gorm.DB, config models.SiteConfig) (RSSConfig, error) {
	defaults := defaultRSSConfigValues()
	memberIDs, err := resolveRSSMemberIDs(db, config.RSSMemberIDs)
	if err != nil {
		return defaults, err
	}
	availableMembers, err := getRSSAvailableMembers(db)
	if err != nil {
		return defaults, err
	}

	return RSSConfig{
		Enabled:          config.RSSEnabled && len(memberIDs) > 0,
		MemberIDs:        memberIDs,
		AvailableMembers: availableMembers,
		Title:            choose(config.RSSTitle, defaults.Title),
		Description:      choose(config.RSSDescription, defaults.Description),
		AuthorName:       choose(config.RSSAuthorName, defaults.AuthorName),
		FaviconURL:       choose(config.RSSFaviconURL, defaults.FaviconURL),
	}, nil
}

func GetRSSConfig() (RSSConfig, error) {
	db, err := database.GetDB()
	if err != nil {
		return RSSConfig{}, err
	}

	var config models.SiteConfig
	if err := db.Table("site_configs").First(&config).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return defaultRSSConfigValues(), nil
		}
		return RSSConfig{}, err
	}

	return buildRSSConfig(db, config)
}

func shouldCollapseLegacyBackgrounds(backgrounds []string) bool {
	hasBackground := false
	for _, raw := range backgrounds {
		url := strings.TrimSpace(raw)
		if url == "" {
			continue
		}
		hasBackground = true
		if _, ok := legacyDefaultHeaderImageURLs[url]; !ok {
			return false
		}
	}
	return hasBackground
}

func HasLifeCountdownSettings(frontendSettings map[string]interface{}) bool {
	for key := range lifeCountdownSettingKeys {
		if _, ok := frontendSettings[key]; ok {
			return true
		}
	}
	return false
}

func IsLifeCountdownSettingsOnly(frontendSettings map[string]interface{}) bool {
	if len(frontendSettings) == 0 {
		return false
	}
	for key := range frontendSettings {
		if _, ok := lifeCountdownSettingKeys[key]; !ok {
			return false
		}
	}
	return true
}

func StripLifeCountdownSettings(frontendSettings map[string]interface{}) map[string]interface{} {
	stripped := make(map[string]interface{}, len(frontendSettings))
	for key, value := range frontendSettings {
		if _, ok := lifeCountdownSettingKeys[key]; ok {
			continue
		}
		stripped[key] = value
	}
	return stripped
}

func UpdateUserLifeCountdownConfig(userID uint, frontendSettings map[string]interface{}) error {
	if userID == 0 || !HasLifeCountdownSettings(frontendSettings) {
		return nil
	}

	db, err := database.GetDB()
	if err != nil {
		return err
	}

	var config models.UserLifeCountdownConfig
	err = db.Where("user_id = ?", userID).First(&config).Error
	if err != nil {
		if err != gorm.ErrRecordNotFound {
			return err
		}
		config = models.UserLifeCountdownConfig{UserID: userID}
	}

	if v, ok := frontendSettings["lifeCountdownEnabled"]; ok {
		if parsed, ok := parseBoolSetting(v); ok {
			config.Enabled = parsed
		}
	}
	if v, ok := frontendSettings["lifeCountdownBirthDate"]; ok {
		birthDate, err := normalizeLifeCountdownBirthDate(v)
		if err != nil {
			return err
		}
		config.BirthDate = birthDate
	}
	if v, ok := frontendSettings["lifeExpectancyYears"]; ok {
		years, err := normalizeLifeExpectancyYears(v)
		if err != nil {
			return err
		}
		config.LifeExpectancyYears = years
	}

	if config.ID == 0 {
		return db.Create(&config).Error
	}
	return db.Save(&config).Error
}

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

	if v, exists := raw["enabled"]; exists {
		config.VoceChatEnabled = parseBoolLike(v, config.VoceChatEnabled)
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
	if v, exists := raw["notificationEnabled"]; exists {
		config.VoceChatNotificationEnabled = parseBoolLike(v, config.VoceChatNotificationEnabled)
	}
	if v, ok := raw["emailDomain"].(string); ok {
		config.VoceChatEmailDomain = vocechat.NormalizeEmailDomain(v)
	}
	if v, exists := raw["loginVerificationEnabled"]; exists {
		config.VoceChatLoginVerificationEnabled = parseBoolLike(v, config.VoceChatLoginVerificationEnabled)
	}
	if v, exists := raw["localFallbackEnabled"]; exists {
		config.VoceChatLocalFallbackEnabled = parseBoolLike(v, config.VoceChatLocalFallbackEnabled)
	}
	if v, exists := raw["contactsEnabled"]; exists {
		config.VoceChatContactsEnabled = parseBoolLike(v, config.VoceChatContactsEnabled)
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
	if !config.VoceChatEnabled {
		config.VoceChatLoginVerificationEnabled = false
		config.VoceChatContactsEnabled = false
		config.VoceChatNotificationEnabled = false
	}
	if strings.TrimSpace(config.VoceChatAdminToken) == "" && strings.TrimSpace(config.VoceChatAdminPassword) != "" && strings.TrimSpace(config.VoceChatAdminUsername) != "" {
		if !isValidVoceChatAdminEmail(config.VoceChatAdminUsername) {
			return fmt.Errorf("管理员邮箱格式无效，请填写 VoceChat 管理员邮箱，不要填写显示名")
		}
	}
	if voceChatHealthAffectingConfigChanged(raw) {
		config.VoceChatLastHealthStatus = ""
		config.VoceChatLastHealthError = ""
		config.VoceChatLastHealthCheckAt = nil
	}
	return nil
}

func voceChatHealthAffectingConfigChanged(raw map[string]interface{}) bool {
	for _, key := range []string{
		"enabled",
		"baseURL",
		"adminUsername",
		"adminPassword",
		"clearAdminPassword",
		"adminToken",
		"clearAdminToken",
		"thirdPartySecret",
		"clearThirdPartySecret",
		"notificationEnabled",
		"botApiKey",
		"clearBotApiKey",
		"loginVerificationEnabled",
		"contactsEnabled",
	} {
		if _, exists := raw[key]; exists {
			return true
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

	if !config.VoceChatEnabled {
		return fail(fmt.Errorf("VoceChat 集成未启用"))
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

func normalizeLifeCountdownBirthDate(value interface{}) (string, error) {
	text := strings.TrimSpace(fmt.Sprintf("%v", value))
	if text == "" || text == "<nil>" {
		return "", nil
	}
	if _, err := time.Parse("2006-01-02", text); err != nil {
		return "", fmt.Errorf("生日格式无效，请使用 YYYY-MM-DD")
	}
	return text, nil
}

func normalizeLifeExpectancyYears(value interface{}) (int, error) {
	var years int
	switch v := value.(type) {
	case int:
		years = v
	case int64:
		years = int(v)
	case float64:
		years = int(v)
		if float64(years) != v {
			return 0, fmt.Errorf("预期寿命必须是整数")
		}
	case string:
		text := strings.TrimSpace(v)
		if text == "" {
			return 0, nil
		}
		parsed, err := strconv.Atoi(text)
		if err != nil {
			return 0, fmt.Errorf("预期寿命必须是整数")
		}
		years = parsed
	default:
		return 0, fmt.Errorf("预期寿命格式无效")
	}
	if years != 0 && (years < 1 || years > 150) {
		return 0, fmt.Errorf("预期寿命必须在 1-150 年之间")
	}
	return years, nil
}

func resolveLifeCountdownSettings(db *gorm.DB, viewerUserID uint, siteConfig models.SiteConfig) lifeCountdownSettings {
	targetUserID := viewerUserID
	fallbackToSiteConfig := false

	if targetUserID == 0 {
		var admin models.User
		if err := db.Where("is_admin = ?", true).Order("id ASC").First(&admin).Error; err == nil {
			targetUserID = admin.ID
		}
		fallbackToSiteConfig = true
	} else {
		var user models.User
		if err := db.Select("id, is_admin").First(&user, targetUserID).Error; err == nil && user.IsAdmin {
			fallbackToSiteConfig = true
		}
	}

	if targetUserID != 0 {
		var config models.UserLifeCountdownConfig
		if err := db.Where("user_id = ?", targetUserID).First(&config).Error; err == nil {
			return lifeCountdownSettings{
				Enabled:             config.Enabled,
				BirthDate:           strings.TrimSpace(config.BirthDate),
				LifeExpectancyYears: config.LifeExpectancyYears,
			}
		}
	}

	if fallbackToSiteConfig {
		return lifeCountdownSettings{
			Enabled:             siteConfig.LifeCountdownEnabled,
			BirthDate:           strings.TrimSpace(siteConfig.LifeCountdownBirthDate),
			LifeExpectancyYears: siteConfig.LifeExpectancyYears,
		}
	}

	return lifeCountdownSettings{}
}

// GetFrontendConfig 获取前端配置
func GetFrontendConfig(viewerUserIDs ...uint) (map[string]interface{}, error) {
	viewerUserID := uint(0)
	if len(viewerUserIDs) > 0 {
		viewerUserID = viewerUserIDs[0]
	}

	db, err := database.GetDB()
	if err != nil {
		return getDefaultConfig(), nil
	}

	var config models.SiteConfig
	if err := db.Table("site_configs").First(&config).Error; err != nil {
		return getDefaultConfig(), nil
	}

	// 新增：读取Setting表的AllowRegistration
	var setting models.Setting
	allowReg := true
	if err := db.Table("settings").First(&setting).Error; err == nil {
		allowReg = setting.AllowRegistration
	}

	// 读取 DB 类型
	dbType := os.Getenv("DB_TYPE")
	if dbType == "" {
		dbType = "sqlite"
	}

	var leftAdsListRaw []map[string]interface{}
	if strings.TrimSpace(config.LeftAds) != "" {
		_ = json.Unmarshal([]byte(config.LeftAds), &leftAdsListRaw)
	}
	normalizedAds := make([]map[string]string, 0, len(leftAdsListRaw))
	for _, m := range leftAdsListRaw {
		img := strings.TrimSpace(fmt.Sprintf("%v", pickAny(m, "imageURL", "ImageURL")))
		link := strings.TrimSpace(fmt.Sprintf("%v", pickAny(m, "linkURL", "LinkURL")))
		desc := strings.TrimSpace(fmt.Sprintf("%v", pickAny(m, "description", "Description")))
		if img == "" {
			continue
		}
		normalizedAds = append(normalizedAds, map[string]string{
			"imageURL":    img,
			"linkURL":     link,
			"description": desc,
		})
	}
	if len(normalizedAds) == 0 {
		if defFrontend, ok := getDefaultConfig()["frontendSettings"].(map[string]interface{}); ok {
			if defAds, ok := defFrontend["leftAds"].([]map[string]string); ok {
				normalizedAds = append(normalizedAds, defAds...)
			} else if defAds2, ok := defFrontend["leftAds"].([]map[string]interface{}); ok {
				for _, m := range defAds2 {
					img := strings.TrimSpace(fmt.Sprintf("%v", pickAny(m, "imageURL", "ImageURL")))
					if img == "" {
						continue
					}
					link := strings.TrimSpace(fmt.Sprintf("%v", pickAny(m, "linkURL", "LinkURL")))
					desc := strings.TrimSpace(fmt.Sprintf("%v", pickAny(m, "description", "Description")))
					normalizedAds = append(normalizedAds, map[string]string{"imageURL": img, "linkURL": link, "description": desc})
				}
			}
		}
	}

	var friendLinks []models.FriendLink
	_ = db.Order("created_at DESC").Find(&friendLinks).Error
	normalizedLinks := make([]map[string]string, 0, len(friendLinks))
	for _, fl := range friendLinks {
		link := strings.TrimSpace(fl.Link)
		if link == "" {
			continue
		}
		normalizedLinks = append(normalizedLinks, map[string]string{
			"title":       strings.TrimSpace(fl.Title),
			"link":        link,
			"icon":        strings.TrimSpace(fl.Icon),
			"description": strings.TrimSpace(fl.Description),
		})
	}
	if len(normalizedLinks) == 0 {
		if defFrontend, ok := getDefaultConfig()["frontendSettings"].(map[string]interface{}); ok {
			if defLinks, ok := defFrontend["friendLinks"].([]map[string]string); ok {
				normalizedLinks = append(normalizedLinks, defLinks...)
			} else if defLinks2, ok := defFrontend["friendLinks"].([]map[string]interface{}); ok {
				for _, m := range defLinks2 {
					title := strings.TrimSpace(fmt.Sprintf("%v", pickAny(m, "title", "Title")))
					link := strings.TrimSpace(fmt.Sprintf("%v", pickAny(m, "link", "Link")))
					icon := strings.TrimSpace(fmt.Sprintf("%v", pickAny(m, "icon", "Icon")))
					desc := strings.TrimSpace(fmt.Sprintf("%v", pickAny(m, "description", "Description")))
					if link == "" {
						continue
					}
					normalizedLinks = append(normalizedLinks, map[string]string{"title": title, "link": link, "icon": icon, "description": desc})
				}
			}
		}
	}

	// 读取社交链接（JSON 字符串）
	var socialLinksRaw []map[string]interface{}
	if strings.TrimSpace(config.SocialLinks) != "" {
		_ = json.Unmarshal([]byte(config.SocialLinks), &socialLinksRaw)
	}
	normalizedSocialLinks := make([]map[string]string, 0, len(socialLinksRaw))
	for _, m := range socialLinksRaw {
		name := strings.TrimSpace(fmt.Sprintf("%v", m["name"]))
		url := strings.TrimSpace(fmt.Sprintf("%v", m["url"]))
		icon := strings.TrimSpace(fmt.Sprintf("%v", m["icon"]))
		if url == "" {
			continue
		}
		normalizedSocialLinks = append(normalizedSocialLinks, map[string]string{
			"name": name,
			"url":  url,
			"icon": icon,
		})
	}

	var feedSourcesRaw []map[string]interface{}
	if strings.TrimSpace(config.FeedSources) != "" {
		_ = json.Unmarshal([]byte(config.FeedSources), &feedSourcesRaw)
	}
	normalizedFeedSources := normalizeFeedSources(feedSourcesRaw)
	if len(normalizedFeedSources) == 0 {
		if defFrontend, ok := getDefaultConfig()["frontendSettings"].(map[string]interface{}); ok {
			if defFeeds, ok := defFrontend["feedSources"].([]map[string]interface{}); ok {
				normalizedFeedSources = append(normalizedFeedSources, normalizeFeedSources(defFeeds)...)
			} else if defFeeds2, ok := defFrontend["feedSources"].([]map[string]interface{}); ok {
				normalizedFeedSources = append(normalizedFeedSources, normalizeFeedSources(defFeeds2)...)
			}
		}
	}
	feedLimit := config.FeedLimit
	if feedLimit < 0 {
		feedLimit = 0
	}
	feedRefreshSeconds := config.FeedRefreshSeconds
	if feedRefreshSeconds <= 0 {
		feedRefreshSeconds = 7200
	}
	if len(normalizedSocialLinks) == 0 {
		if defFrontend, ok := getDefaultConfig()["frontendSettings"].(map[string]interface{}); ok {
			if defLinks, ok := defFrontend["socialLinks"].([]map[string]string); ok {
				normalizedSocialLinks = append(normalizedSocialLinks, defLinks...)
			} else if defLinks2, ok := defFrontend["socialLinks"].([]map[string]interface{}); ok {
				for _, m := range defLinks2 {
					name := strings.TrimSpace(fmt.Sprintf("%v", m["name"]))
					url := strings.TrimSpace(fmt.Sprintf("%v", m["url"]))
					icon := strings.TrimSpace(fmt.Sprintf("%v", m["icon"]))
					if url == "" {
						continue
					}
					normalizedSocialLinks = append(normalizedSocialLinks, map[string]string{"name": name, "url": url, "icon": icon})
				}
			}
		}
	}

	leftAdsInterval := config.LeftAdsIntervalMs
	if leftAdsInterval <= 0 {
		if defFrontend, ok := getDefaultConfig()["frontendSettings"].(map[string]interface{}); ok {
			if v, ok := defFrontend["leftAdsIntervalMs"].(int); ok {
				leftAdsInterval = v
			} else if v2, ok := defFrontend["leftAdsIntervalMs"].(float64); ok {
				leftAdsInterval = int(v2)
			}
		}
	}

	lifeCountdown := resolveLifeCountdownSettings(db, viewerUserID, config)
	rssConfig, err := buildRSSConfig(db, config)
	if err != nil {
		rssConfig = defaultRSSConfigValues()
	}
	rssMemberIDsForViewer := []uint{}
	rssAvailableMembersForViewer := []map[string]interface{}{}
	if viewerUserID > 0 {
		var viewer models.User
		if err := db.Select("id, is_admin").First(&viewer, viewerUserID).Error; err == nil && viewer.IsAdmin {
			rssMemberIDsForViewer = rssConfig.MemberIDs
			rssAvailableMembersForViewer = rssConfig.AvailableMembers
		}
	}
	effectiveSyncConfirmed := config.StorageSyncConfirmed && syncmanager.IsStorageSyncConfirmedLocal()

	configMap := map[string]interface{}{
		"allowRegistration": allowReg,
		"dbType":            dbType,
		"frontendSettings": map[string]interface{}{
			"siteTitle":           config.SiteTitle,
			"subtitleText":        config.SubtitleText,
			"avatarURL":           config.AvatarURL,
			"username":            config.Username,
			"description":         config.Description,
			"backgrounds":         config.GetBackgroundsList(),
			"cardFooterTitle":     config.CardFooterTitle,
			"cardFooterLink":      config.CardFooterLink,
			"pageFooterHTML":      config.PageFooterHTML,
			"rssTitle":            rssConfig.Title,
			"rssDescription":      rssConfig.Description,
			"rssAuthorName":       rssConfig.AuthorName,
			"rssFaviconURL":       rssConfig.FaviconURL,
			"rssEnabled":          rssConfig.Enabled,
			"rssMemberIDs":        rssMemberIDsForViewer,
			"rssAvailableMembers": rssAvailableMembersForViewer,
			"walineServerURL":     config.WalineServerURL,
			"enableGithubCard":    config.EnableGithubCard,
			"notifyEnabled":       config.NotifyEnabled,
			// 页面文案与关于页内容
			"linksTitle":       choose(config.LinksTitle, getDefaultConfig()["frontendSettings"].(map[string]interface{})["linksTitle"].(string)),
			"linksDescription": choose(config.LinksDescription, getDefaultConfig()["frontendSettings"].(map[string]interface{})["linksDescription"].(string)),
			"linksApplyTitle":  choose(config.LinksApplyTitle, "申请友链须知"),
			"linksApplyText":   choose(config.LinksApplyText, "请提供站点名称、网址、图标（可选）、简介与有效邮箱。提交后需管理员审核，审核通过后展示。"),
			"loginExpireDays": func() int {
				days, _ := normalizeLoginExpireConfig(config.LoginExpireDays, config.LoginExpireHours)
				return days
			}(),
			"loginExpireHours": func() int {
				_, hours := normalizeLoginExpireConfig(config.LoginExpireDays, config.LoginExpireHours)
				return hours
			}(),
			"commentPageTitle":       choose(config.CommentPageTitle, getDefaultConfig()["frontendSettings"].(map[string]interface{})["commentPageTitle"].(string)),
			"commentPageDescription": choose(config.CommentPageDescription, getDefaultConfig()["frontendSettings"].(map[string]interface{})["commentPageDescription"].(string)),
			"aboutPageTitle":         choose(config.AboutPageTitle, getDefaultConfig()["frontendSettings"].(map[string]interface{})["aboutPageTitle"].(string)),
			"aboutPageDescription":   choose(config.AboutPageDescription, getDefaultConfig()["frontendSettings"].(map[string]interface{})["aboutPageDescription"].(string)),
			"aboutMarkdown":          choose(config.AboutMarkdown, getDefaultConfig()["frontendSettings"].(map[string]interface{})["aboutMarkdown"].(string)),
			// 信息流
			"feedEnabled":         config.FeedEnabled,
			"feedPageTitle":       choose(config.FeedPageTitle, getDefaultConfig()["frontendSettings"].(map[string]interface{})["feedPageTitle"].(string)),
			"feedPageDescription": choose(config.FeedPageDescription, getDefaultConfig()["frontendSettings"].(map[string]interface{})["feedPageDescription"].(string)),
			"feedSources":         normalizedFeedSources,
			"feedLimit":           feedLimit,
			"feedRefreshSeconds":  feedRefreshSeconds,
			// 系统欢迎组件（与用户资料解耦；若未设置则回退默认）
			"welcomeAvatarURL":   choose(config.WelcomeAvatarURL, getDefaultConfig()["frontendSettings"].(map[string]interface{})["welcomeAvatarURL"].(string)),
			"welcomeName":        choose(config.WelcomeName, getDefaultConfig()["frontendSettings"].(map[string]interface{})["welcomeName"].(string)),
			"welcomeDescription": choose(config.WelcomeDescription, getDefaultConfig()["frontendSettings"].(map[string]interface{})["welcomeDescription"].(string)),
			"welcomeUseAdmin":    config.WelcomeUseAdmin,
			// GitHub OAuth
			"githubOAuthEnabled": config.GithubOAuthEnabled,
			"githubClientId":     config.GithubClientId,
			"githubClientSecret": config.GithubClientSecret,
			"githubCallbackURL":  config.GithubCallbackURL,
			// PWA 设置
			"pwaEnabled":     config.PwaEnabled,
			"pwaTitle":       choose(config.PwaTitle, config.SiteTitle),
			"pwaDescription": choose(config.PwaDescription, config.Description),
			"pwaIconURL":     choose(config.PwaIconURL, config.RSSFaviconURL),
			// 默认内容主题
			"defaultContentTheme": choose(config.ContentThemeDefault, "dark"),
			"homeLayoutDefault":   choose(config.HomeLayoutDefault, "three"),
			// 公告栏
			"announcementText":    choose(config.AnnouncementText, "欢迎访问我的说说笔记！"),
			"announcementEnabled": config.AnnouncementEnabled,
			// 音乐播放器
			"musicEnabled":          config.MusicEnabled,
			"musicPlaylistId":       choose(config.MusicPlaylistId, ""),
			"musicSongId":           choose(config.MusicSongId, ""),
			"musicPosition":         choose(config.MusicPosition, "bottom-left"),
			"musicTheme":            choose(config.MusicTheme, "auto"),
			"musicLyric":            config.MusicLyric,
			"musicAutoplay":         config.MusicAutoplay,
			"musicDefaultMinimized": config.MusicDefaultMinimized,
			"musicEmbed":            config.MusicEmbed,
			"musicHideOnMobile":     config.MusicHideOnMobile,
			"musicCssCdnURL":        choose(config.MusicCssCdnURL, ""),
			"musicJsCdnURL":         choose(config.MusicJsCdnURL, ""),
			// 评论系统
			"commentEnabled":             config.CommentEnabled,
			"commentSystem":              choose(config.CommentSystem, "builtin"),
			"commentEmailEnabled":        config.CommentEmailEnabled,
			"commentEmailAdminNotifyAll": config.CommentEmailAdminNotifyAll,
			"commentLoginRequired":       config.CommentLoginRequired,
			// 扩展组件开关
			"calendarEnabled":        config.CalendarEnabled,
			"timeEnabled":            config.TimeEnabled,
			"hitokotoEnabled":        config.HitokotoEnabled,
			"lifeCountdownEnabled":   lifeCountdown.Enabled,
			"lifeCountdownBirthDate": choose(lifeCountdown.BirthDate, ""),
			"lifeExpectancyYears": func() int {
				if lifeCountdown.LifeExpectancyYears > 0 {
					return lifeCountdown.LifeExpectancyYears
				}
				return 0
			}(),

			"leftAdEnabled":          config.LeftAdEnabled,
			"leftAds":                normalizedAds,
			"leftAdsIntervalMs":      leftAdsInterval,
			"friendLinks":            normalizedLinks,
			"friendLinkEmailEnabled": config.FriendLinkEmailEnabled,
			// 社交链接
			"socialLinksEnabled": config.SocialLinksEnabled,
			"socialLinks":        normalizedSocialLinks,
		},
		"voceChatConfig": vocechat.PublicConfigFromSiteConfig(config, viewerUserID == models.PrimaryAdminUserID),
		"storageEnabled": config.StorageEnabled,
		"storageConfig": map[string]interface{}{
			"provider":      choose(config.StorageProvider, ""),
			"endpoint":      choose(config.StorageEndpoint, ""),
			"region":        choose(config.StorageRegion, ""),
			"bucket":        choose(config.StorageBucket, ""),
			"accessKey":     choose(config.StorageAccessKey, ""),
			"secretKey":     choose(config.StorageSecretKey, ""),
			"usePathStyle":  config.StorageUsePathStyle,
			"publicBaseURL": choose(config.StoragePublicBaseURL, ""),
			"syncRole": func() string {
				if config.StorageSyncRole == "" {
					return "primary"
				}
				return config.StorageSyncRole
			}(),
			"autoSyncEnabled": config.StorageAutoSyncEnabled,
			"syncConfirmed":   effectiveSyncConfirmed,
			"needsConfirm":    config.StorageEnabled && !effectiveSyncConfirmed,
			"syncMode":        choose(config.StorageSyncMode, "instant"),
			"syncIntervalMinute": func() int {
				if config.StorageSyncIntervalMinute > 0 {
					return config.StorageSyncIntervalMinute
				}
				return 15
			}(),
			"lastSyncTime": func() string {
				if config.StorageLastSyncTime != nil {
					return config.StorageLastSyncTime.Format(time.RFC3339)
				}
				return ""
			}(),
		},
		"attachmentStorageEnabled": config.AttachmentStorageEnabled,
		"attachmentStorageConfig": map[string]interface{}{
			"provider":          choose(config.AttachmentStorageProvider, ""),
			"endpoint":          choose(config.AttachmentStorageEndpoint, ""),
			"region":            choose(config.AttachmentStorageRegion, ""),
			"bucket":            choose(config.AttachmentStorageBucket, ""),
			"accessKey":         choose(config.AttachmentStorageAccessKey, ""),
			"secretKey":         choose(config.AttachmentStorageSecretKey, ""),
			"usePathStyle":      config.AttachmentStorageUsePathStyle,
			"publicBaseURL":     choose(config.AttachmentStoragePublicBaseURL, ""),
			"enableCompression": config.EnableCompression,
			"ffmpegInstalled":   pkg.CheckFFmpegInstalled(),
		},
		"smtpEnabled":    config.SmtpEnabled,
		"smtpDriver":     config.SmtpDriver,
		"smtpHost":       config.SmtpHost,
		"smtpPort":       config.SmtpPort,
		"smtpUser":       config.SmtpUser,
		"smtpPass":       config.SmtpPass,
		"smtpFrom":       config.SmtpFrom,
		"smtpEncryption": config.SmtpEncryption,
		"smtpTLS":        config.SmtpTLS,
	}
	return configMap, nil
}

// UpdateSetting 更新站点配置
func UpdateFrontendSetting(userID uint, settingMap map[string]interface{}) error {
	db, err := database.GetDB()
	if err != nil {
		return fmt.Errorf("数据库连接失败: %v", err)
	}

	frontendSettings := map[string]interface{}{}
	if raw, exists := settingMap["frontendSettings"]; exists {
		parsed, ok := raw.(map[string]interface{})
		if !ok {
			return fmt.Errorf("无效的前端配置格式")
		}
		frontendSettings = parsed
	}

	// 开启事务
	tx := db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	var config models.SiteConfig
	// 先尝试获取现有配置
	if err := tx.Table("site_configs").First(&config).Error; err != nil {
		config.ID = 1 // 设置默认ID
	}

	// 更新配置字段
	if v, ok := frontendSettings["siteTitle"].(string); ok {
		config.SiteTitle = v
	}
	if v, ok := frontendSettings["subtitleText"].(string); ok {
		config.SubtitleText = v
	}
	if v, ok := frontendSettings["avatarURL"].(string); ok {
		config.AvatarURL = v
	}
	if v, ok := frontendSettings["username"].(string); ok {
		config.Username = v
	}
	if v, ok := frontendSettings["description"].(string); ok {
		config.Description = v
	}
	if v, ok := frontendSettings["cardFooterTitle"].(string); ok {
		config.CardFooterTitle = v
	}
	if v, ok := frontendSettings["cardFooterLink"].(string); ok {
		config.CardFooterLink = v
	}
	if v, ok := frontendSettings["pageFooterHTML"].(string); ok {
		config.PageFooterHTML = v
	}
	if v, ok := frontendSettings["rssTitle"].(string); ok {
		config.RSSTitle = v
	}
	if v, ok := frontendSettings["rssDescription"].(string); ok {
		config.RSSDescription = v
	}
	if v, ok := frontendSettings["rssAuthorName"].(string); ok {
		config.RSSAuthorName = v
	}
	if v, ok := frontendSettings["rssFaviconURL"].(string); ok {
		config.RSSFaviconURL = v
	}
	if vb, ok := frontendSettings["rssEnabled"].(bool); ok {
		config.RSSEnabled = vb
	} else if vs, ok := frontendSettings["rssEnabled"].(string); ok {
		config.RSSEnabled = strings.EqualFold(strings.TrimSpace(vs), "true")
	}
	if rawIDs, exists := frontendSettings["rssMemberIDs"]; exists {
		ids, _ := parseRSSMemberIDValue(rawIDs)
		normalizedIDs, err := normalizeRSSMemberIDs(tx, ids)
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("RSS 成员配置无效: %v", err)
		}
		if len(normalizedIDs) == 0 {
			config.RSSEnabled = false
		}
		memberIDsJSON, err := json.Marshal(normalizedIDs)
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("RSS 成员配置序列化失败: %v", err)
		}
		config.RSSMemberIDs = string(memberIDsJSON)
	}
	if strings.TrimSpace(config.RSSMemberIDs) == "[]" {
		config.RSSEnabled = false
	}
	if v, ok := frontendSettings["walineServerURL"].(string); ok {
		config.WalineServerURL = v
	}
	// 页面文案与关于页内容
	if v, ok := frontendSettings["linksTitle"].(string); ok {
		config.LinksTitle = v
	}
	if v, ok := frontendSettings["linksDescription"].(string); ok {
		config.LinksDescription = v
	}
	if v, ok := frontendSettings["linksApplyTitle"].(string); ok {
		config.LinksApplyTitle = v
	}
	if v, ok := frontendSettings["linksApplyText"].(string); ok {
		config.LinksApplyText = v
	}
	if v, ok := frontendSettings["commentPageTitle"].(string); ok {
		config.CommentPageTitle = v
	}
	if v, ok := frontendSettings["commentPageDescription"].(string); ok {
		config.CommentPageDescription = v
	}
	if v, ok := frontendSettings["aboutPageTitle"].(string); ok {
		config.AboutPageTitle = v
	}
	if v, ok := frontendSettings["aboutPageDescription"].(string); ok {
		config.AboutPageDescription = v
	}
	if v, ok := frontendSettings["aboutMarkdown"].(string); ok {
		config.AboutMarkdown = v
	}
	loginExpireDays := config.LoginExpireDays
	loginExpireHours := config.LoginExpireHours
	if n, ok := parsePositiveIntSetting(frontendSettings["loginExpireDays"]); ok {
		loginExpireDays = n
	}
	if n, ok := parsePositiveIntSetting(frontendSettings["loginExpireHours"]); ok {
		loginExpireHours = n
	}
	config.LoginExpireDays, config.LoginExpireHours = normalizeLoginExpireConfig(loginExpireDays, loginExpireHours)
	if vb, ok := frontendSettings["calendarEnabled"].(bool); ok {
		config.CalendarEnabled = vb
	} else if vs, ok := frontendSettings["calendarEnabled"].(string); ok {
		config.CalendarEnabled = (vs == "true")
	}
	if vb, ok := frontendSettings["timeEnabled"].(bool); ok {
		config.TimeEnabled = vb
	} else if vs, ok := frontendSettings["timeEnabled"].(string); ok {
		config.TimeEnabled = (vs == "true")
	}
	if vb, ok := frontendSettings["hitokotoEnabled"].(bool); ok {
		config.HitokotoEnabled = vb
	} else if vs, ok := frontendSettings["hitokotoEnabled"].(string); ok {
		config.HitokotoEnabled = (vs == "true")
	}
	if vb, ok := frontendSettings["lifeCountdownEnabled"].(bool); ok {
		config.LifeCountdownEnabled = vb
	} else if vs, ok := frontendSettings["lifeCountdownEnabled"].(string); ok {
		config.LifeCountdownEnabled = (vs == "true")
	}
	if v, ok := frontendSettings["lifeCountdownBirthDate"].(string); ok {
		config.LifeCountdownBirthDate = strings.TrimSpace(v)
	}
	if vi, ok := frontendSettings["lifeExpectancyYears"].(float64); ok {
		config.LifeExpectancyYears = int(vi)
	} else if vi2, ok := frontendSettings["lifeExpectancyYears"].(int); ok {
		config.LifeExpectancyYears = vi2
	} else if vs, ok := frontendSettings["lifeExpectancyYears"].(string); ok {
		if n, err := strconv.Atoi(strings.TrimSpace(vs)); err == nil {
			config.LifeExpectancyYears = n
		}
	}
	if config.LifeExpectancyYears < 0 {
		config.LifeExpectancyYears = 0
	}
	// 评论系统设置
	if vb, ok := frontendSettings["commentEnabled"].(bool); ok {
		config.CommentEnabled = vb
	} else if vs, ok := frontendSettings["commentEnabled"].(string); ok {
		if vs == "true" {
			config.CommentEnabled = true
		} else if vs == "false" {
			config.CommentEnabled = false
		}
	}

	// 推送模块总开关（允许开启但所有渠道都未启用）
	if vb, ok := frontendSettings["notifyEnabled"].(bool); ok {
		config.NotifyEnabled = vb
	} else if vs, ok := frontendSettings["notifyEnabled"].(string); ok {
		if strings.EqualFold(strings.TrimSpace(vs), "true") {
			config.NotifyEnabled = true
		} else if strings.EqualFold(strings.TrimSpace(vs), "false") {
			config.NotifyEnabled = false
		}
	}

	// 广告位设置（与评论系统无关，独立保存）
	if vb, ok := frontendSettings["leftAdEnabled"].(bool); ok {
		config.LeftAdEnabled = vb
	} else if vs, ok := frontendSettings["leftAdEnabled"].(string); ok {
		config.LeftAdEnabled = (vs == "true")
	}
	// 轮播间隔
	if vi, ok := frontendSettings["leftAdsIntervalMs"].(float64); ok {
		config.LeftAdsIntervalMs = int(vi)
	} else if vi2, ok := frontendSettings["leftAdsIntervalMs"].(int); ok {
		config.LeftAdsIntervalMs = vi2
	} else if vs, ok := frontendSettings["leftAdsIntervalMs"].(string); ok {
		if n, err := strconv.Atoi(vs); err == nil {
			config.LeftAdsIntervalMs = n
		}
	}
	// 多广告列表
	if arr, ok := frontendSettings["leftAds"].([]interface{}); ok {
		list := make([]map[string]string, 0, len(arr))
		for _, it := range arr {
			m, ok := it.(map[string]interface{})
			if !ok {
				continue
			}
			img := strings.TrimSpace(fmt.Sprintf("%v", m["imageURL"]))
			if img == "" {
				continue
			}
			link := strings.TrimSpace(fmt.Sprintf("%v", m["linkURL"]))
			desc := strings.TrimSpace(fmt.Sprintf("%v", m["description"]))
			list = append(list, map[string]string{
				"imageURL":    img,
				"linkURL":     link,
				"description": desc,
			})
		}
		bs, _ := json.Marshal(list)
		config.LeftAds = string(bs)
	} else if arr2, ok := frontendSettings["leftAds"].([]map[string]string); ok {
		bs, _ := json.Marshal(arr2)
		config.LeftAds = string(bs)
	}

	// 社交链接（首页左栏）
	if arr, ok := frontendSettings["socialLinks"].([]interface{}); ok {
		list := make([]map[string]string, 0, len(arr))
		for _, it := range arr {
			m, ok := it.(map[string]interface{})
			if !ok {
				continue
			}
			name := strings.TrimSpace(fmt.Sprintf("%v", m["name"]))
			url := strings.TrimSpace(fmt.Sprintf("%v", m["url"]))
			icon := strings.TrimSpace(fmt.Sprintf("%v", m["icon"]))
			if url == "" {
				continue
			}
			list = append(list, map[string]string{"name": name, "url": url, "icon": icon})
		}
		bs, _ := json.Marshal(list)
		config.SocialLinks = string(bs)
	} else if arr2, ok := frontendSettings["socialLinks"].([]map[string]interface{}); ok {
		list := make([]map[string]string, 0, len(arr2))
		for _, m := range arr2 {
			name := strings.TrimSpace(fmt.Sprintf("%v", m["name"]))
			url := strings.TrimSpace(fmt.Sprintf("%v", m["url"]))
			icon := strings.TrimSpace(fmt.Sprintf("%v", m["icon"]))
			if url == "" {
				continue
			}
			list = append(list, map[string]string{"name": name, "url": url, "icon": icon})
		}
		bs, _ := json.Marshal(list)
		config.SocialLinks = string(bs)
	}
	if vb, ok := frontendSettings["socialLinksEnabled"].(bool); ok {
		config.SocialLinksEnabled = vb
	} else if vs, ok := frontendSettings["socialLinksEnabled"].(string); ok {
		config.SocialLinksEnabled = (vs == "true")
	}
	// 信息流设置
	if vb, ok := frontendSettings["feedEnabled"].(bool); ok {
		config.FeedEnabled = vb
	} else if vs, ok := frontendSettings["feedEnabled"].(string); ok {
		config.FeedEnabled = strings.EqualFold(strings.TrimSpace(vs), "true")
	}
	if v, ok := frontendSettings["feedPageTitle"].(string); ok {
		config.FeedPageTitle = strings.TrimSpace(v)
	}
	if v, ok := frontendSettings["feedPageDescription"].(string); ok {
		config.FeedPageDescription = strings.TrimSpace(v)
	}
	if vi, ok := frontendSettings["feedLimit"].(float64); ok {
		config.FeedLimit = int(vi)
	} else if vi2, ok := frontendSettings["feedLimit"].(int); ok {
		config.FeedLimit = vi2
	} else if vs, ok := frontendSettings["feedLimit"].(string); ok {
		if n, err := strconv.Atoi(strings.TrimSpace(vs)); err == nil {
			config.FeedLimit = n
		}
	}
	if config.FeedLimit <= 0 {
		config.FeedLimit = 0
	}
	if vi, ok := frontendSettings["feedRefreshSeconds"].(float64); ok {
		config.FeedRefreshSeconds = int(vi)
	} else if vi2, ok := frontendSettings["feedRefreshSeconds"].(int); ok {
		config.FeedRefreshSeconds = vi2
	} else if vs, ok := frontendSettings["feedRefreshSeconds"].(string); ok {
		if n, err := strconv.Atoi(strings.TrimSpace(vs)); err == nil {
			config.FeedRefreshSeconds = n
		}
	}
	if config.FeedRefreshSeconds <= 0 {
		config.FeedRefreshSeconds = 7200
	}
	if arr, ok := frontendSettings["feedSources"].([]interface{}); ok {
		list := normalizeFeedSources(arr)
		bs, _ := json.Marshal(list)
		config.FeedSources = string(bs)
	} else if arr2, ok := frontendSettings["feedSources"].([]map[string]interface{}); ok {
		list := normalizeFeedSources(arr2)
		bs, _ := json.Marshal(list)
		config.FeedSources = string(bs)
	} else if arr3, ok := frontendSettings["feedSources"].([]map[string]string); ok {
		bs, _ := json.Marshal(arr3)
		config.FeedSources = string(bs)
	}

	// 友链列表（管理员直接配置）
	if arr, ok := frontendSettings["friendLinks"].([]interface{}); ok {
		links := make([]models.FriendLink, 0, len(arr))
		for _, it := range arr {
			m, ok := it.(map[string]interface{})
			if !ok {
				continue
			}
			title := strings.TrimSpace(fmt.Sprintf("%v", m["title"]))
			link := strings.TrimSpace(fmt.Sprintf("%v", m["link"]))
			icon := strings.TrimSpace(fmt.Sprintf("%v", m["icon"]))
			desc := strings.TrimSpace(fmt.Sprintf("%v", m["description"]))
			if link == "" {
				continue
			}
			links = append(links, models.FriendLink{Title: title, Link: link, Icon: icon, Description: desc})
		}
		if err := tx.Where("1 = 1").Delete(&models.FriendLink{}).Error; err != nil {
			tx.Rollback()
			return fmt.Errorf("更新友链失败: %v", err)
		}
		for _, l := range links {
			if err := tx.Create(&l).Error; err != nil {
				tx.Rollback()
				return fmt.Errorf("保存友链失败: %v", err)
			}
		}
	} else if arr2, ok := frontendSettings["friendLinks"].([]map[string]interface{}); ok {
		links := make([]models.FriendLink, 0, len(arr2))
		for _, m := range arr2 {
			title := strings.TrimSpace(fmt.Sprintf("%v", m["title"]))
			link := strings.TrimSpace(fmt.Sprintf("%v", m["link"]))
			icon := strings.TrimSpace(fmt.Sprintf("%v", m["icon"]))
			desc := strings.TrimSpace(fmt.Sprintf("%v", m["description"]))
			if link == "" {
				continue
			}
			links = append(links, models.FriendLink{Title: title, Link: link, Icon: icon, Description: desc})
		}
		if err := tx.Where("1 = 1").Delete(&models.FriendLink{}).Error; err != nil {
			tx.Rollback()
			return fmt.Errorf("更新友链失败: %v", err)
		}
		for _, l := range links {
			if err := tx.Create(&l).Error; err != nil {
				tx.Rollback()
				return fmt.Errorf("保存友链失败: %v", err)
			}
		}
	}

	// 系统欢迎组件（与用户资料解耦）
	if v, ok := frontendSettings["welcomeAvatarURL"].(string); ok {
		config.WelcomeAvatarURL = strings.TrimSpace(v)
	}
	if v, ok := frontendSettings["welcomeName"].(string); ok {
		config.WelcomeName = strings.TrimSpace(v)
	}
	if v, ok := frontendSettings["welcomeDescription"].(string); ok {
		config.WelcomeDescription = strings.TrimSpace(v)
	}
	if vb, ok := frontendSettings["welcomeUseAdmin"].(bool); ok {
		config.WelcomeUseAdmin = vb
	} else if vs, ok := frontendSettings["welcomeUseAdmin"].(string); ok {
		config.WelcomeUseAdmin = (strings.EqualFold(strings.TrimSpace(vs), "true"))
	}

	// 音乐播放器设置
	if vb, ok := frontendSettings["musicEnabled"].(bool); ok {
		config.MusicEnabled = vb
	} else if vs, ok := frontendSettings["musicEnabled"].(string); ok {
		config.MusicEnabled = (vs == "true")
	}
	if v, ok := frontendSettings["musicPlaylistId"].(string); ok {
		config.MusicPlaylistId = v
	}
	if v, ok := frontendSettings["musicSongId"].(string); ok {
		config.MusicSongId = v
	}
	if v, ok := frontendSettings["musicPosition"].(string); ok {
		config.MusicPosition = v
	}
	if v, ok := frontendSettings["musicTheme"].(string); ok {
		config.MusicTheme = v
	}
	if vb, ok := frontendSettings["musicLyric"].(bool); ok {
		config.MusicLyric = vb
	} else if vs, ok := frontendSettings["musicLyric"].(string); ok {
		config.MusicLyric = (vs == "true")
	}
	if vb, ok := frontendSettings["musicAutoplay"].(bool); ok {
		config.MusicAutoplay = vb
	} else if vs, ok := frontendSettings["musicAutoplay"].(string); ok {
		config.MusicAutoplay = (vs == "true")
	}
	if vb, ok := frontendSettings["musicDefaultMinimized"].(bool); ok {
		config.MusicDefaultMinimized = vb
	} else if vs, ok := frontendSettings["musicDefaultMinimized"].(string); ok {
		config.MusicDefaultMinimized = (vs == "true")
	}
	if vb, ok := frontendSettings["musicEmbed"].(bool); ok {
		config.MusicEmbed = vb
	} else if vs, ok := frontendSettings["musicEmbed"].(string); ok {
		config.MusicEmbed = (vs == "true")
	}
	if vb, ok := frontendSettings["musicHideOnMobile"].(bool); ok {
		config.MusicHideOnMobile = vb
	} else if vs, ok := frontendSettings["musicHideOnMobile"].(string); ok {
		config.MusicHideOnMobile = (vs == "true")
	}
	if v, ok := frontendSettings["musicCssCdnURL"].(string); ok {
		config.MusicCssCdnURL = v
	}
	if v, ok := frontendSettings["musicJsCdnURL"].(string); ok {
		config.MusicJsCdnURL = v
	}
	if v, ok := frontendSettings["commentSystem"].(string); ok {
		config.CommentSystem = v
	}
	if vb, ok := frontendSettings["commentLoginRequired"].(bool); ok {
		config.CommentLoginRequired = vb
	} else if vs, ok := frontendSettings["commentLoginRequired"].(string); ok {
		config.CommentLoginRequired = (vs == "true")
	}
	if vb, ok := frontendSettings["commentEmailEnabled"].(bool); ok {
		config.CommentEmailEnabled = vb
	} else if vs, ok := frontendSettings["commentEmailEnabled"].(string); ok {
		if vs == "true" {
			config.CommentEmailEnabled = true
		} else if vs == "false" {
			config.CommentEmailEnabled = false
		}
	}
	if vb, ok := frontendSettings["commentEmailAdminNotifyAll"].(bool); ok {
		config.CommentEmailAdminNotifyAll = vb
	} else if vs, ok := frontendSettings["commentEmailAdminNotifyAll"].(string); ok {
		if vs == "true" {
			config.CommentEmailAdminNotifyAll = true
		} else if vs == "false" {
			config.CommentEmailAdminNotifyAll = false
		}
	}
	// GitHub OAuth 设置
	if vb, ok := frontendSettings["githubOAuthEnabled"].(bool); ok {
		config.GithubOAuthEnabled = vb
	} else if vs, ok := frontendSettings["githubOAuthEnabled"].(string); ok {
		if vs == "true" {
			config.GithubOAuthEnabled = true
		} else if vs == "false" {
			config.GithubOAuthEnabled = false
		}
	}
	if v, ok := frontendSettings["githubClientId"].(string); ok {
		config.GithubClientId = v
	}
	if v, ok := frontendSettings["githubClientSecret"].(string); ok {
		config.GithubClientSecret = v
	}
	if v, ok := frontendSettings["githubCallbackURL"].(string); ok {
		config.GithubCallbackURL = v
	}
	if v, ok := frontendSettings["enableGithubCard"].(bool); ok {
		config.EnableGithubCard = v
	} else if vs, ok := frontendSettings["enableGithubCard"].(string); ok {
		if vs == "true" {
			config.EnableGithubCard = true
		} else if vs == "false" {
			config.EnableGithubCard = false
		}
	}
	// 公告栏
	if v, ok := frontendSettings["announcementText"].(string); ok {
		config.AnnouncementText = v
	}
	if vb, ok := frontendSettings["announcementEnabled"].(bool); ok {
		config.AnnouncementEnabled = vb
	} else if vs, ok := frontendSettings["announcementEnabled"].(string); ok {
		if vs == "true" {
			config.AnnouncementEnabled = true
		} else if vs == "false" {
			config.AnnouncementEnabled = false
		}
	}
	// PWA 设置
	if v, ok := frontendSettings["pwaEnabled"].(bool); ok {
		config.PwaEnabled = v
	}
	if v, ok := frontendSettings["pwaTitle"].(string); ok {
		config.PwaTitle = v
	}
	if v, ok := frontendSettings["pwaDescription"].(string); ok {
		config.PwaDescription = v
	}
	if v, ok := frontendSettings["pwaIconURL"].(string); ok {
		config.PwaIconURL = v
	}

	// 默认内容主题
	if v, ok := frontendSettings["defaultContentTheme"].(string); ok {
		if v == "dark" || v == "light" {
			config.ContentThemeDefault = v
		}
	}
	if v, ok := frontendSettings["homeLayoutDefault"].(string); ok {
		if v == "three" || v == "two" || v == "single" {
			config.HomeLayoutDefault = v
		}
	}

	// 处理背景图片列表
	if backgrounds, ok := frontendSettings["backgrounds"].([]interface{}); ok {
		backgroundsList := make([]string, 0, len(backgrounds))
		for _, bg := range backgrounds {
			if bgStr, ok := bg.(string); ok && bgStr != "" {
				backgroundsList = append(backgroundsList, bgStr)
			}
		}
		// 确保至少保留一个默认背景
		if len(backgroundsList) == 0 {
			backgroundsList = getDefaultConfig()["frontendSettings"].(map[string]interface{})["backgrounds"].([]string)
		}
		backgroundsJSON, err := json.Marshal(backgroundsList)
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("背景图片列表序列化失败: %v", err)
		}
		config.Backgrounds = string(backgroundsJSON)
	} else if backgrounds, ok := frontendSettings["backgrounds"].([]string); ok {
		// 直接处理字符串数组
		if len(backgrounds) == 0 {
			backgrounds = getDefaultConfig()["frontendSettings"].(map[string]interface{})["backgrounds"].([]string)
		}
		backgroundsJSON, err := json.Marshal(backgrounds)
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("背景图片列表序列化失败: %v", err)
		}
		config.Backgrounds = string(backgroundsJSON)
	}

	// 保存或更新配置
	if config.ID == 0 {
		if err := tx.Table("site_configs").Create(&config).Error; err != nil {
			tx.Rollback()
			return fmt.Errorf("创建配置失败: %v", err)
		}
	} else {
		if err := tx.Table("site_configs").Save(&config).Error; err != nil {
			tx.Rollback()
			return fmt.Errorf("更新配置失败: %v", err)
		}
	}

	if v, ok := settingMap["storageEnabled"].(bool); ok {
		config.StorageEnabled = v
	}
	if sc, ok := settingMap["storageConfig"].(map[string]interface{}); ok {
		if pv, ok := sc["provider"].(string); ok {
			config.StorageProvider = pv
		}
		if v, ok := sc["endpoint"].(string); ok {
			v = strings.TrimSpace(v)
			if v != "" {
				if u, err := url.Parse(v); err == nil {
					v = strings.TrimRight(u.Scheme+"://"+u.Host, "/")
				}
			}
			config.StorageEndpoint = v
		}
		if v, ok := sc["region"].(string); ok {
			if config.StorageProvider == "r2" {
				config.StorageRegion = "auto"
			} else {
				config.StorageRegion = v
			}
		}
		if v, ok := sc["bucket"].(string); ok {
			config.StorageBucket = v
		}
		if v, ok := sc["accessKey"].(string); ok {
			config.StorageAccessKey = v
		}
		if v, ok := sc["secretKey"].(string); ok {
			config.StorageSecretKey = v
		}
		if v, ok := sc["usePathStyle"].(bool); ok {
			config.StorageUsePathStyle = v
		}
		if v, ok := sc["publicBaseURL"].(string); ok {
			config.StoragePublicBaseURL = v
		}
		if v, ok := sc["syncRole"].(string); ok {
			if v == "primary" || v == "secondary" {
				config.StorageSyncRole = v
			}
		}
		if vb, ok := sc["autoSyncEnabled"].(bool); ok {
			config.StorageAutoSyncEnabled = vb
		} else if vs, ok := sc["autoSyncEnabled"].(string); ok {
			config.StorageAutoSyncEnabled = (vs == "true")
		}
		if v, ok := sc["syncMode"].(string); ok {
			if v == "instant" || v == "scheduled" {
				config.StorageSyncMode = v
			}
		}
		if vi, ok := sc["syncIntervalMinute"].(float64); ok {
			config.StorageSyncIntervalMinute = int(vi)
		} else if vi2, ok := sc["syncIntervalMinute"].(int); ok {
			config.StorageSyncIntervalMinute = vi2
		} else if vs, ok := sc["syncIntervalMinute"].(string); ok {
			if n, err := strconv.Atoi(vs); err == nil {
				config.StorageSyncIntervalMinute = n
			}
		}
		// 若未显式传入 autoSyncEnabled，则在云存储配置完整且启用时自动开启
		if _, exists := sc["autoSyncEnabled"]; !exists {
			if config.StorageEnabled &&
				config.StorageProvider != "" &&
				config.StorageEndpoint != "" &&
				config.StorageBucket != "" &&
				config.StorageAccessKey != "" &&
				config.StorageSecretKey != "" {
				config.StorageAutoSyncEnabled = true
			}
		}

		// 若用户在后台明确保存了云存储参数（配置完整），则认为已人工确认同步
		// 这样“首次确认”仅针对旧数据库中已存在云端参数但未确认的情况。
		if config.StorageEnabled &&
			strings.TrimSpace(config.StorageProvider) != "" &&
			strings.TrimSpace(config.StorageEndpoint) != "" &&
			strings.TrimSpace(config.StorageBucket) != "" &&
			strings.TrimSpace(config.StorageAccessKey) != "" &&
			strings.TrimSpace(config.StorageSecretKey) != "" {
			// no-op: confirmation must be explicit via /api/backup/storage/sync-confirm
		}
	}

	if config.StorageProvider == "r2" {
		config.StorageUsePathStyle = true
	}

	// 附件存储设置
	if v, ok := settingMap["attachmentStorageEnabled"].(bool); ok {
		config.AttachmentStorageEnabled = v
	}
	if sc, ok := settingMap["attachmentStorageConfig"].(map[string]interface{}); ok {
		if pv, ok := sc["provider"].(string); ok {
			config.AttachmentStorageProvider = pv
		}
		if v, ok := sc["endpoint"].(string); ok {
			v = strings.TrimSpace(v)
			if v != "" {
				if u, err := url.Parse(v); err == nil {
					v = strings.TrimRight(u.Scheme+"://"+u.Host, "/")
				}
			}
			config.AttachmentStorageEndpoint = v
		}
		if v, ok := sc["region"].(string); ok {
			if config.AttachmentStorageProvider == "r2" {
				config.AttachmentStorageRegion = "auto"
			} else {
				config.AttachmentStorageRegion = v
			}
		}
		if v, ok := sc["bucket"].(string); ok {
			config.AttachmentStorageBucket = v
		}
		if v, ok := sc["accessKey"].(string); ok {
			config.AttachmentStorageAccessKey = v
		}
		if v, ok := sc["secretKey"].(string); ok {
			config.AttachmentStorageSecretKey = v
		}
		if v, ok := sc["usePathStyle"].(bool); ok {
			config.AttachmentStorageUsePathStyle = v
		}
		if v, ok := sc["publicBaseURL"].(string); ok {
			config.AttachmentStoragePublicBaseURL = v
		}
		if v, ok := sc["enableCompression"].(bool); ok {
			config.EnableCompression = v
		}
	}

	if config.AttachmentStorageProvider == "r2" {
		config.AttachmentStorageUsePathStyle = true
	}

	if sc, ok := settingMap["voceChatConfig"].(map[string]interface{}); ok {
		if err := applyVoceChatConfigUpdate(&config, sc); err != nil {
			tx.Rollback()
			return fmt.Errorf("VoceChat 配置错误: %v", err)
		}
	}

	// 邮件设置
	if v, ok := settingMap["smtpEnabled"].(bool); ok {
		config.SmtpEnabled = v
	}
	// 友链邮件通知开关
	if vb, ok := frontendSettings["friendLinkEmailEnabled"].(bool); ok {
		config.FriendLinkEmailEnabled = vb
	} else if vs, ok := frontendSettings["friendLinkEmailEnabled"].(string); ok {
		config.FriendLinkEmailEnabled = (vs == "true")
	}
	if v, ok := settingMap["smtpDriver"].(string); ok {
		config.SmtpDriver = v
	}
	if v, ok := settingMap["smtpHost"].(string); ok {
		config.SmtpHost = v
	}
	if v, ok := settingMap["smtpPort"].(float64); ok {
		config.SmtpPort = int(v)
	} else if vi, ok := settingMap["smtpPort"].(int); ok {
		config.SmtpPort = vi
	} else if vs, ok := settingMap["smtpPort"].(string); ok {
		if p, err := strconv.Atoi(vs); err == nil {
			config.SmtpPort = p
		}
	}
	if v, ok := settingMap["smtpUser"].(string); ok {
		config.SmtpUser = v
	}
	if v, ok := settingMap["smtpPass"].(string); ok {
		config.SmtpPass = v
	}
	if v, ok := settingMap["smtpFrom"].(string); ok {
		config.SmtpFrom = v
	}
	if v, ok := settingMap["smtpEncryption"].(string); ok {
		config.SmtpEncryption = v
	}
	if v, ok := settingMap["smtpTLS"].(bool); ok {
		config.SmtpTLS = v
	}

	// 自动启用：当必填项齐全时，强制启用
	if !config.SmtpEnabled {
		if config.SmtpHost != "" && config.SmtpPort > 0 && config.SmtpUser != "" && config.SmtpPass != "" &&
			(config.SmtpEncryption == "ssl" || config.SmtpEncryption == "tls") {
			config.SmtpEnabled = true
		}
	}

	// 基础校验：开启时必填项必须完整
	if config.SmtpEnabled {
		if config.SmtpHost == "" || config.SmtpPort <= 0 || config.SmtpUser == "" || config.SmtpPass == "" ||
			(config.SmtpEncryption != "ssl" && config.SmtpEncryption != "tls") {
			tx.Rollback()
			return fmt.Errorf("邮件设置错误")
		}
	}

	if err := tx.Table("site_configs").Save(&config).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("更新配置失败: %v", err)
	}

	syncmanager.Configure(config)

	if config.StorageEnabled {
		dbType := os.Getenv("DB_TYPE")
		if dbType == "" {
			dbType = "sqlite"
		}
		if dbType == "sqlite" {
			base := strings.TrimSpace(config.StoragePublicBaseURL)
			if base != "" {
				url := strings.TrimRight(base, "/") + "/database.db"
				client := &http.Client{Timeout: 60 * time.Second}
				resp, err := client.Get(url)
				if err == nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
					defer resp.Body.Close()
					tempFile := filepath.Join(os.TempDir(), "cloud_database.db")
					out, err := os.Create(tempFile)
					if err == nil {
						_, _ = io.Copy(out, resp.Body)
						out.Close()
						dbPath := os.Getenv("DB_PATH")
						if dbPath == "" {
							dbPath = "/app/data/noise.db"
						}
						_ = os.MkdirAll(filepath.Dir(dbPath), 0755)
						_ = copyFile(tempFile, dbPath)
						_ = os.Remove(tempFile)
						_ = database.ReconnectDB()
					}
				}
			}
		}
	}

	// 提交事务
	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("提交配置更新失败: %v", err)
	}
	StartInfoFeedAutoRefresh()

	return nil
}

// 获取默认配置
func getDefaultConfig() map[string]interface{} {
	return map[string]interface{}{
		"allowRegistration": true,
		"frontendSettings": map[string]interface{}{
			"siteTitle":           "Noise的说说笔记",
			"subtitleText":        "欢迎访问，点击头像可更换封面背景！",
			"avatarURL":           "https://s2.loli.net/2025/03/24/HnSXKvibAQlosIW.png",
			"username":            "Noise",
			"description":         "执迷不悟",
			"notifyEnabled":       false,
			"backgrounds":         defaultHeaderImages(),
			"cardFooterTitle":     "Noise·说说·笔记~",
			"cardFooterLink":      "note.noisework.cn",
			"pageFooterHTML":      `<div class="text-center text-xs text-gray-400 py-4">来自<a href="https://www.noisework.cn" target="_blank" rel="noopener noreferrer" class="text-orange-400 hover:text-orange-500">Noise</a> 使用<a href="https://github.com/rcy1314/echo-noise" target="_blank" rel="noopener noreferrer" class="text-orange-400 hover:text-orange-500">Ech0-Noise</a>发布</div>`,
			"rssTitle":            "Noise的说说笔记",
			"rssDescription":      "一个说说笔记~",
			"rssAuthorName":       "Noise",
			"rssFaviconURL":       "/favicon-32x32.png",
			"rssEnabled":          false,
			"rssMemberIDs":        []uint{},
			"rssAvailableMembers": []map[string]interface{}{},
			"walineServerURL":     "请前往waline官网https://waline.js.org查看部署配置",
			"enableGithubCard":    false,
			// 页面文案与关于页内容
			"linksTitle":       "友情链接",
			"linksDescription": "推荐站点和朋友们的主页",
			"friendLinks": []map[string]string{
				{"title": "NoiseWork", "link": "https://www.noisework.cn/", "icon": "i-mdi-home", "description": "个人主页与作品集合"},
				{"title": "NoiseBlogs", "link": "https://www.noiseblogs.top/", "icon": "i-mdi-notebook", "description": "技术随笔与学习记录"},
			},
			"commentPageTitle":       "留言",
			"commentPageDescription": "欢迎留下你的看法",
			"aboutPageTitle":         "关于本站",
			"aboutPageDescription":   "这里是站点的介绍与说明",
			"aboutMarkdown":          "# 关于我\n\n这里是一个默认的个人简介示例：\n\n- 喜欢记录与分享\n- 热爱开源与学习\n- 持续打磨产品体验\n\n欢迎通过友链或留言与我交流！",
			"loginExpireDays":        3,
			"loginExpireHours":       0,
			"feedEnabled":            false,
			"feedPageTitle":          "实时聚合内容动态",
			"feedPageDescription":    "聚合综合内容信息源内容，当前结果 {count} 条",
			"feedLimit":              100,
			"feedRefreshSeconds":     7200,
			"feedSources": []map[string]interface{}{
				{"type": "rss", "group": "默认分组", "name": "站点 RSS", "url": "/rss", "enabled": true, "visible": true},
			},
			// 系统欢迎组件默认参数
			"welcomeAvatarURL":           "https://s2.loli.net/2025/03/24/HnSXKvibAQlosIW.png",
			"welcomeName":                "Noise",
			"welcomeDescription":         "执迷不悟",
			"welcomeUseAdmin":            true,
			"githubOAuthEnabled":         false,
			"githubClientId":             "",
			"githubClientSecret":         "",
			"githubCallbackURL":          "",
			"pwaEnabled":                 true,
			"pwaTitle":                   "",
			"pwaDescription":             "",
			"pwaIconURL":                 "",
			"defaultContentTheme":        "light",
			"homeLayoutDefault":          "three",
			"announcementText":           "欢迎访问我的说说笔记！",
			"announcementEnabled":        true,
			"musicEnabled":               false,
			"musicPlaylistId":            "",
			"musicSongId":                "",
			"musicPosition":              "bottom-left",
			"musicTheme":                 "auto",
			"musicLyric":                 true,
			"musicAutoplay":              false,
			"musicDefaultMinimized":      true,
			"musicEmbed":                 false,
			"musicHideOnMobile":          true,
			"musicCssCdnURL":             "",
			"musicJsCdnURL":              "",
			"commentEnabled":             true,
			"commentSystem":              "builtin",
			"commentEmailEnabled":        false,
			"commentEmailAdminNotifyAll": true,
			"commentLoginRequired":       false,
			"hitokotoEnabled":            true,
			"lifeCountdownEnabled":       false,
			"lifeCountdownBirthDate":     "",
			"lifeExpectancyYears":        80,
			// 广告默认参数（多广告位）
			"leftAdEnabled": true,
			"leftAds": []map[string]string{
				{"imageURL": "https://picsum.photos/seed/ad-1/640/640", "linkURL": "https://note.noisework.cn", "description": "写作与记录，开启灵感之旅"},
				{"imageURL": "https://picsum.photos/seed/ad-2/640/640", "linkURL": "https://noisework.cn", "description": "探索新主题与小工具"},
				{"imageURL": "https://picsum.photos/seed/ad-3/640/640", "linkURL": "https://github.com", "description": "开源项目，欢迎 Star"},
			},
			"leftAdsIntervalMs": 4000,
			// 社交链接默认
			"socialLinksEnabled": true,
			"socialLinks": []map[string]string{
				{"name": "GitHub", "url": "https://github.com/rcy1314", "icon": "i-mdi-github"},
				{"name": "X", "url": "https://x.com/liangwenhao3", "icon": "i-mdi-twitter"},
				{"name": "主页", "url": "https://www.noisework.cn/", "icon": "i-mdi-home"},
				{"name": "博客", "url": "https://www.noiseblogs.top/", "icon": "i-mdi-notebook"},
			},
		},
		"voceChatConfig": vocechat.DefaultPublicConfig(),
		"storageEnabled": false,
		"storageConfig": map[string]interface{}{
			"provider":           "",
			"endpoint":           "",
			"region":             "",
			"bucket":             "",
			"accessKey":          "",
			"secretKey":          "",
			"usePathStyle":       true,
			"publicBaseURL":      "",
			"syncRole":           "primary",
			"autoSyncEnabled":    false,
			"syncMode":           "instant",
			"syncIntervalMinute": 15,
		},
		"attachmentStorageEnabled": false,
		"attachmentStorageConfig": map[string]interface{}{
			"provider":          "",
			"endpoint":          "",
			"region":            "",
			"bucket":            "",
			"accessKey":         "",
			"secretKey":         "",
			"usePathStyle":      true,
			"publicBaseURL":     "",
			"enableCompression": false,
		},
	}
}

// 选择第一个非空字符串
func choose(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func pickAny(m map[string]interface{}, keys ...string) interface{} {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return v
		}
	}
	return ""
}

func normalizeFeedSources(raw interface{}) []map[string]interface{} {
	list := []map[string]interface{}{}
	switch arr := raw.(type) {
	case []map[string]interface{}:
		for _, m := range arr {
			itemType := normalizeFeedSourceTypeRaw(pickAny(m, "type", "Type"))
			item := map[string]interface{}{
				"type":    itemType,
				"group":   strings.TrimSpace(fmt.Sprintf("%v", pickAny(m, "group", "Group"))),
				"name":    strings.TrimSpace(fmt.Sprintf("%v", pickAny(m, "name", "Name"))),
				"url":     strings.TrimSpace(fmt.Sprintf("%v", pickAny(m, "url", "URL"))),
				"enabled": parseBoolLike(pickAny(m, "enabled", "Enabled"), true),
				"visible": parseBoolLike(pickAny(m, "visible", "Visible"), true),
			}
			if strings.TrimSpace(fmt.Sprintf("%v", item["url"])) == "" {
				continue
			}
			if strings.TrimSpace(fmt.Sprintf("%v", item["group"])) == "" {
				item["group"] = "默认分组"
			}
			if itemType == "" {
				item["type"] = "rss"
			}
			list = append(list, item)
		}
	case []interface{}:
		for _, it := range arr {
			m, ok := it.(map[string]interface{})
			if !ok {
				continue
			}
			itemType := normalizeFeedSourceTypeRaw(pickAny(m, "type", "Type"))
			item := map[string]interface{}{
				"type":    itemType,
				"group":   strings.TrimSpace(fmt.Sprintf("%v", pickAny(m, "group", "Group"))),
				"name":    strings.TrimSpace(fmt.Sprintf("%v", pickAny(m, "name", "Name"))),
				"url":     strings.TrimSpace(fmt.Sprintf("%v", pickAny(m, "url", "URL"))),
				"enabled": parseBoolLike(pickAny(m, "enabled", "Enabled"), true),
				"visible": parseBoolLike(pickAny(m, "visible", "Visible"), true),
			}
			if strings.TrimSpace(fmt.Sprintf("%v", item["url"])) == "" {
				continue
			}
			if strings.TrimSpace(fmt.Sprintf("%v", item["group"])) == "" {
				item["group"] = "默认分组"
			}
			if itemType == "" {
				item["type"] = "rss"
			}
			list = append(list, item)
		}
	}
	return list
}

func normalizeFeedSourceTypeRaw(raw interface{}) string {
	candidate := raw
	if obj, ok := raw.(map[string]interface{}); ok {
		candidate = pickAny(obj, "value", "type", "label")
	}
	t := strings.ToLower(strings.TrimSpace(fmt.Sprintf("%v", candidate)))
	switch t {
	case "rss":
		return "rss"
	case "note", "custom", "说说笔记", "本项目api", "本项目 api":
		return "note"
	case "ech0":
		return "ech0"
	case "memos":
		return "memos"
	case "mastodon":
		return "mastodon"
	default:
		return "rss"
	}
}

func parseBoolLike(v interface{}, def bool) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		s := strings.ToLower(strings.TrimSpace(x))
		if s == "true" || s == "1" || s == "yes" || s == "on" {
			return true
		}
		if s == "false" || s == "0" || s == "no" || s == "off" {
			return false
		}
	case float64:
		return int(x) == 1
	case int:
		return x == 1
	}
	return def
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
