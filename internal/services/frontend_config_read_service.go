package services

import (
	"encoding/json"
	"fmt"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/syncmanager"
	"github.com/rcy1314/echo-noise/internal/vocechat"
	"github.com/rcy1314/echo-noise/pkg"
	"os"
	"strings"
	"time"
)

// Frontend configuration reads assemble the viewer-scoped public payload.
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
	scrubLegacySiteConfigValues(&config)

	// 新增：读取Setting表的AllowRegistration
	var setting models.Setting
	allowReg := true
	autoApproveReg := false
	if err := db.Table("settings").First(&setting).Error; err == nil {
		allowReg = setting.AllowRegistration
		autoApproveReg = setting.AutoApproveRegistration
	}

	// 读取 DB 类型
	dbType := os.Getenv("DB_TYPE")
	if dbType == "" {
		dbType = "sqlite"
	}

	normalizedAds := normalizeLeftAds(config.LeftAds)
	if len(normalizedAds) == 0 {
		if defFrontend, ok := getDefaultConfig()["frontendSettings"].(map[string]interface{}); ok {
			normalizedAds = normalizeLeftAds(defFrontend["leftAds"])
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
	widgetVisibility := resolveWidgetVisibilitySettings(db, viewerUserID, config)
	rssConfig, err := buildRSSConfig(db, config)
	if err != nil {
		rssConfig = defaultRSSConfigValues()
	}
	rssMemberIDsForViewer := []uint{}
	rssAvailableMembersForViewer := []map[string]interface{}{}
	viewerIsAdmin := false
	viewerIsPrimaryAdmin := false
	if viewerUserID > 0 {
		var viewer models.User
		if err := db.Select("id, is_admin").First(&viewer, viewerUserID).Error; err == nil && viewer.IsAdmin {
			viewerIsAdmin = true
			viewerIsPrimaryAdmin = viewer.ID == models.PrimaryAdminUserID
			if viewerIsPrimaryAdmin {
				rssMemberIDsForViewer = rssConfig.MemberIDs
				rssAvailableMembersForViewer = rssConfig.AvailableMembers
			}
		}
	}
	effectiveSyncConfirmed := config.StorageSyncConfirmed && syncmanager.IsStorageSyncConfirmedLocal()

	configMap := map[string]interface{}{
		"allowRegistration":       allowReg,
		"autoApproveRegistration": autoApproveReg,
		"dbType":                  dbType,
		"frontendSettings": map[string]interface{}{
			"siteTitle":           config.SiteTitle,
			"subtitleText":        config.SubtitleText,
			"avatarURL":           config.AvatarURL,
			"username":            config.Username,
			"description":         config.Description,
			"backgrounds":         config.GetBackgroundsConfig(),
			"pageFooterHTML":      config.PageFooterHTML,
			"rssTitle":            rssConfig.Title,
			"rssDescription":      rssConfig.Description,
			"rssAuthorName":       rssConfig.AuthorName,
			"rssFaviconURL":       rssConfig.FaviconURL,
			"rssEnabled":          rssConfig.Enabled,
			"rssMemberIDs":        rssMemberIDsForViewer,
			"rssAvailableMembers": rssAvailableMembersForViewer,
			"enableGithubCard":    config.EnableGithubCard,
			"notifyEnabled":       config.NotifyEnabled,
			// 页面文案与关于页内容
			"loginExpireDays": func() int {
				days, _ := normalizeLoginExpireConfig(config.LoginExpireDays, config.LoginExpireHours)
				return days
			}(),
			"loginExpireHours": func() int {
				_, hours := normalizeLoginExpireConfig(config.LoginExpireDays, config.LoginExpireHours)
				return hours
			}(),
			"delegatedAdminLoginExpireDays": func() int {
				days, _ := normalizeLoginExpireConfig(config.DelegatedAdminLoginExpireDays, config.DelegatedAdminLoginExpireHours)
				return days
			}(),
			"delegatedAdminLoginExpireHours": func() int {
				_, hours := normalizeLoginExpireConfig(config.DelegatedAdminLoginExpireDays, config.DelegatedAdminLoginExpireHours)
				return hours
			}(),
			"commentPageTitle":            choose(config.CommentPageTitle, getDefaultConfig()["frontendSettings"].(map[string]interface{})["commentPageTitle"].(string)),
			"commentPageDescription":      choose(config.CommentPageDescription, getDefaultConfig()["frontendSettings"].(map[string]interface{})["commentPageDescription"].(string)),
			"notificationPageTitle":       choose(config.NotificationPageTitle, getDefaultConfig()["frontendSettings"].(map[string]interface{})["notificationPageTitle"].(string)),
			"notificationPageDescription": choose(config.NotificationPageDescription, getDefaultConfig()["frontendSettings"].(map[string]interface{})["notificationPageDescription"].(string)),
			"announcementPageTitle":       choose(config.AnnouncementPageTitle, getDefaultConfig()["frontendSettings"].(map[string]interface{})["announcementPageTitle"].(string)),
			"announcementPageDescription": choose(config.AnnouncementPageDescription, getDefaultConfig()["frontendSettings"].(map[string]interface{})["announcementPageDescription"].(string)),
			"aboutPageTitle":              choose(config.AboutPageTitle, getDefaultConfig()["frontendSettings"].(map[string]interface{})["aboutPageTitle"].(string)),
			"aboutPageDescription":        choose(config.AboutPageDescription, getDefaultConfig()["frontendSettings"].(map[string]interface{})["aboutPageDescription"].(string)),
			"aboutMarkdown":               choose(config.AboutMarkdown, getDefaultConfig()["frontendSettings"].(map[string]interface{})["aboutMarkdown"].(string)),
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
			// PWA 设置
			"pwaEnabled":     config.PwaEnabled,
			"pwaTitle":       choose(config.PwaTitle, config.SiteTitle),
			"pwaDescription": choose(config.PwaDescription, config.Description),
			"pwaIconURL":     choose(config.PwaIconURL, config.RSSFaviconURL),
			// 默认内容主题
			"defaultContentTheme": choose(config.ContentThemeDefault, "dark"),
			"homeLayoutDefault":   choose(config.HomeLayoutDefault, "three"),
			// 公告栏
			"announcementText":    choose(config.AnnouncementText, neutralAnnouncement),
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
			"commentEnabled": config.CommentEnabled,
			// 扩展组件开关
			"calendarEnabled":        widgetVisibility.CalendarEnabled,
			"timeEnabled":            config.TimeEnabled,
			"hitokotoEnabled":        widgetVisibility.HitokotoEnabled,
			"lifeCountdownEnabled":   lifeCountdown.Enabled,
			"homeStatsEnabled":       widgetVisibility.HomeStatsEnabled,
			"popularTagsEnabled":     widgetVisibility.PopularTagsEnabled,
			"latestGalleryEnabled":   widgetVisibility.LatestGalleryEnabled,
			"heatmapEnabled":         widgetVisibility.HeatmapEnabled,
			"lifeCountdownBirthDate": choose(lifeCountdown.BirthDate, ""),
			"lifeExpectancyYears": func() int {
				if lifeCountdown.LifeExpectancyYears > 0 {
					return lifeCountdown.LifeExpectancyYears
				}
				return 0
			}(),

			"leftAdEnabled":     config.LeftAdEnabled,
			"leftAds":           normalizedAds,
			"leftAdsIntervalMs": leftAdsInterval,
			// 社交链接
			"socialLinksEnabled": config.SocialLinksEnabled,
			"socialLinks":        normalizedSocialLinks,
		},
		"voceChatConfig": vocechat.PublicConfigFromSiteConfig(config, viewerUserID == models.PrimaryAdminUserID),
		"storageEnabled": config.StorageEnabled,
		"storageConfig": map[string]interface{}{
			"provider":            choose(config.StorageProvider, ""),
			"endpoint":            choose(config.StorageEndpoint, ""),
			"region":              choose(config.StorageRegion, ""),
			"bucket":              choose(config.StorageBucket, ""),
			"accessKey":           "",
			"secretKey":           "",
			"accessKeyConfigured": viewerIsAdmin && strings.TrimSpace(config.StorageAccessKey) != "",
			"secretKeyConfigured": viewerIsAdmin && strings.TrimSpace(config.StorageSecretKey) != "",
			"usePathStyle":        config.StorageUsePathStyle,
			"publicBaseURL":       choose(config.StoragePublicBaseURL, ""),
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
		"recycleBinRetentionDays": func() int {
			if viewerUserID == models.PrimaryAdminUserID {
				return config.RecycleBinRetentionDays
			}
			return 0
		}(),
		"commentRecycleBinRetentionDays": func() int {
			if viewerUserID == models.PrimaryAdminUserID {
				return config.CommentRecycleBinRetentionDays
			}
			return 0
		}(),
		"notifyNoteDeletionByPrimary": func() bool {
			return viewerUserID == models.PrimaryAdminUserID && config.NotifyNoteDeletionByPrimary
		}(),
		"notifyCommentDeletionByPrimary": func() bool {
			return viewerUserID == models.PrimaryAdminUserID && config.NotifyCommentDeletionByPrimary
		}(),
		"attachmentStorageConfig": map[string]interface{}{
			"provider":            choose(config.AttachmentStorageProvider, ""),
			"endpoint":            choose(config.AttachmentStorageEndpoint, ""),
			"region":              choose(config.AttachmentStorageRegion, ""),
			"bucket":              choose(config.AttachmentStorageBucket, ""),
			"accessKey":           "",
			"secretKey":           "",
			"accessKeyConfigured": viewerIsAdmin && strings.TrimSpace(config.AttachmentStorageAccessKey) != "",
			"secretKeyConfigured": viewerIsAdmin && strings.TrimSpace(config.AttachmentStorageSecretKey) != "",
			"usePathStyle":        config.AttachmentStorageUsePathStyle,
			"publicBaseURL":       choose(config.AttachmentStoragePublicBaseURL, ""),
			"enableCompression":   config.EnableCompression,
			"ffmpegInstalled":     pkg.CheckFFmpegInstalled(),
		},
		"smtpEnabled":        config.SmtpEnabled,
		"smtpDriver":         config.SmtpDriver,
		"smtpHost":           config.SmtpHost,
		"smtpPort":           config.SmtpPort,
		"smtpUser":           "",
		"smtpPass":           "",
		"smtpUserConfigured": viewerIsAdmin && strings.TrimSpace(config.SmtpUser) != "",
		"smtpPassConfigured": viewerIsAdmin && strings.TrimSpace(config.SmtpPass) != "",
		"smtpFrom":           config.SmtpFrom,
		"smtpEncryption":     config.SmtpEncryption,
		"smtpTLS":            config.SmtpTLS,
	}
	if !viewerIsPrimaryAdmin {
		if frontendSettings, ok := configMap["frontendSettings"].(map[string]interface{}); ok {
			delete(frontendSettings, "loginExpireDays")
			delete(frontendSettings, "loginExpireHours")
			delete(frontendSettings, "delegatedAdminLoginExpireDays")
			delete(frontendSettings, "delegatedAdminLoginExpireHours")
		}
	}
	return configMap, nil
}

// UpdateSetting 更新站点配置
