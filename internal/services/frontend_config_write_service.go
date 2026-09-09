package services

import (
	"encoding/json"
	"fmt"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/syncmanager"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Frontend configuration writes validate and persist the supported site-setting fields.
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
	// 页面文案与关于页内容
	if v, ok := frontendSettings["linksTitle"].(string); ok {
		config.LinksTitle = v
	}
	if v, ok := frontendSettings["linksDescription"].(string); ok {
		config.LinksDescription = v
	}
	if v, ok := frontendSettings["commentPageTitle"].(string); ok {
		config.CommentPageTitle = v
	}
	if v, ok := frontendSettings["commentPageDescription"].(string); ok {
		config.CommentPageDescription = v
	}
	if v, ok := frontendSettings["notificationPageTitle"].(string); ok {
		config.NotificationPageTitle = v
	}
	if v, ok := frontendSettings["notificationPageDescription"].(string); ok {
		config.NotificationPageDescription = v
	}
	if v, ok := frontendSettings["announcementPageTitle"].(string); ok {
		config.AnnouncementPageTitle = v
	}
	if v, ok := frontendSettings["announcementPageDescription"].(string); ok {
		config.AnnouncementPageDescription = v
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
	delegatedAdminLoginExpireDays := config.DelegatedAdminLoginExpireDays
	delegatedAdminLoginExpireHours := config.DelegatedAdminLoginExpireHours
	if n, ok := parsePositiveIntSetting(frontendSettings["delegatedAdminLoginExpireDays"]); ok {
		delegatedAdminLoginExpireDays = n
	}
	if n, ok := parsePositiveIntSetting(frontendSettings["delegatedAdminLoginExpireHours"]); ok {
		delegatedAdminLoginExpireHours = n
	}
	config.DelegatedAdminLoginExpireDays, config.DelegatedAdminLoginExpireHours = normalizeLoginExpireConfig(delegatedAdminLoginExpireDays, delegatedAdminLoginExpireHours)
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
	for _, item := range []struct {
		key string
		set func(bool)
	}{
		{"homeStatsEnabled", func(value bool) { config.HomeStatsEnabled = value }},
		{"popularTagsEnabled", func(value bool) { config.PopularTagsEnabled = value }},
		{"latestGalleryEnabled", func(value bool) { config.LatestGalleryEnabled = value }},
		{"heatmapEnabled", func(value bool) { config.HeatmapEnabled = value }},
	} {
		if raw, exists := frontendSettings[item.key]; exists {
			if value, ok := parseBoolSetting(raw); ok {
				item.set(value)
			}
		}
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
	if rawAds, ok := frontendSettings["leftAds"]; ok {
		bs, _ := json.Marshal(normalizeLeftAds(rawAds))
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
		if v == "three" || v == "two" || v == "single" || v == "masonry" {
			config.HomeLayoutDefault = v
		}
	}

	// 处理背景图片列表
	if rawBackgrounds, ok := frontendSettings["backgrounds"]; ok {
		backgroundsList := normalizeHeaderBackgrounds(rawBackgrounds)
		backgroundsJSON, err := json.Marshal(backgroundsList)
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
	if v, ok := settingMap["recycleBinRetentionDays"].(int); ok {
		config.RecycleBinRetentionDays = v
	} else if v, ok := settingMap["recycleBinRetentionDays"].(float64); ok {
		config.RecycleBinRetentionDays = int(v)
	}
	if v, ok := settingMap["commentRecycleBinRetentionDays"].(int); ok {
		config.CommentRecycleBinRetentionDays = v
	} else if v, ok := settingMap["commentRecycleBinRetentionDays"].(float64); ok {
		config.CommentRecycleBinRetentionDays = int(v)
	}
	if v, ok := settingMap["notifyNoteDeletionByPrimary"].(bool); ok {
		config.NotifyNoteDeletionByPrimary = v
	}
	if v, ok := settingMap["notifyCommentDeletionByPrimary"].(bool); ok {
		config.NotifyCommentDeletionByPrimary = v
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
		applySensitiveStringSetting(sc, "accessKey", "clearAccessKey", &config.StorageAccessKey)
		applySensitiveStringSetting(sc, "secretKey", "clearSecretKey", &config.StorageSecretKey)
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
		applySensitiveStringSetting(sc, "accessKey", "clearAccessKey", &config.AttachmentStorageAccessKey)
		applySensitiveStringSetting(sc, "secretKey", "clearSecretKey", &config.AttachmentStorageSecretKey)
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
	applySensitiveStringSetting(settingMap, "smtpUser", "clearSmtpUser", &config.SmtpUser)
	applySensitiveStringSetting(settingMap, "smtpPass", "clearSmtpPass", &config.SmtpPass)
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
