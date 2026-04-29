package services

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/syncmanager"
	"github.com/rcy1314/echo-noise/pkg"
)

// GetFrontendConfig 获取前端配置
func GetFrontendConfig() (map[string]interface{}, error) {
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

	effectiveSyncConfirmed := config.StorageSyncConfirmed && syncmanager.IsStorageSyncConfirmedLocal()

	configMap := map[string]interface{}{
		"allowRegistration": allowReg,
		"dbType":            dbType,
		"frontendSettings": map[string]interface{}{
			"siteTitle":        config.SiteTitle,
			"subtitleText":     config.SubtitleText,
			"avatarURL":        config.AvatarURL,
			"username":         config.Username,
			"description":      config.Description,
			"backgrounds":      config.GetBackgroundsList(),
			"cardFooterTitle":  config.CardFooterTitle,
			"cardFooterLink":   config.CardFooterLink,
			"pageFooterHTML":   config.PageFooterHTML,
			"rssTitle":         config.RSSTitle,
			"rssDescription":   config.RSSDescription,
			"rssAuthorName":    config.RSSAuthorName,
			"rssFaviconURL":    config.RSSFaviconURL,
			"walineServerURL":  config.WalineServerURL,
			"enableGithubCard": config.EnableGithubCard,
			"notifyEnabled":    config.NotifyEnabled,
			// 页面文案与关于页内容
			"linksTitle":       choose(config.LinksTitle, getDefaultConfig()["frontendSettings"].(map[string]interface{})["linksTitle"].(string)),
			"linksDescription": choose(config.LinksDescription, getDefaultConfig()["frontendSettings"].(map[string]interface{})["linksDescription"].(string)),
			"linksApplyTitle":  choose(config.LinksApplyTitle, "申请友链须知"),
			"linksApplyText":   choose(config.LinksApplyText, "请提供站点名称、网址、图标（可选）、简介与有效邮箱。提交后需管理员审核，审核通过后展示。"),
			"loginExpireDays": func() int {
				if config.LoginExpireDays > 0 {
					return config.LoginExpireDays
				}
				return 3
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
			"commentEnabled":       config.CommentEnabled,
			"commentSystem":        choose(config.CommentSystem, "builtin"),
			"commentEmailEnabled":  config.CommentEmailEnabled,
			"commentLoginRequired": config.CommentLoginRequired,
			// 扩展组件开关
			"calendarEnabled":        config.CalendarEnabled,
			"timeEnabled":            config.TimeEnabled,
			"hitokotoEnabled":        config.HitokotoEnabled,
			"lifeCountdownEnabled":   config.LifeCountdownEnabled,
			"lifeCountdownBirthDate": choose(config.LifeCountdownBirthDate, ""),
			"lifeExpectancyYears": func() int {
				if config.LifeExpectancyYears > 0 {
					return config.LifeExpectancyYears
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
	if vi, ok := frontendSettings["loginExpireDays"].(float64); ok {
		n := int(vi)
		if n > 0 {
			config.LoginExpireDays = n
		}
	} else if vi2, ok := frontendSettings["loginExpireDays"].(int); ok {
		if vi2 > 0 {
			config.LoginExpireDays = vi2
		}
	} else if vs, ok := frontendSettings["loginExpireDays"].(string); ok {
		if n, err := strconv.Atoi(strings.TrimSpace(vs)); err == nil && n > 0 {
			config.LoginExpireDays = n
		}
	}
	if config.LoginExpireDays <= 0 {
		config.LoginExpireDays = 3
	}
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
			"siteTitle":     "Noise的说说笔记",
			"subtitleText":  "欢迎访问，点击头像可更换封面背景！",
			"avatarURL":     "https://s2.loli.net/2025/03/24/HnSXKvibAQlosIW.png",
			"username":      "Noise",
			"description":   "执迷不悟",
			"notifyEnabled": false,
			"backgrounds": []string{
				"https://s2.loli.net/2025/03/27/KJ1trnU2ksbFEYM.jpg",
				"https://s2.loli.net/2025/03/27/MZqaLczCvwjSmW7.jpg",
				"https://s2.loli.net/2025/03/27/UMijKXwJ9yTqSeE.jpg",
				"https://s2.loli.net/2025/03/27/WJQIlkXvBg2afcR.jpg",
				"https://s2.loli.net/2025/03/27/oHNQtf4spkq2iln.jpg",
				"https://s2.loli.net/2025/03/27/PMRuX5loc6Uaimw.jpg",
				"https://s2.loli.net/2025/03/27/U2WIslbNyTLt4rD.jpg",
				"https://s2.loli.net/2025/03/27/xu1jZL5Og4pqT9d.jpg",
			},
			"cardFooterTitle":  "Noise·说说·笔记~",
			"cardFooterLink":   "note.noisework.cn",
			"pageFooterHTML":   `<div class="text-center text-xs text-gray-400 py-4">来自<a href="https://www.noisework.cn" target="_blank" rel="noopener noreferrer" class="text-orange-400 hover:text-orange-500">Noise</a> 使用<a href="https://github.com/rcy1314/echo-noise" target="_blank" rel="noopener noreferrer" class="text-orange-400 hover:text-orange-500">Ech0-Noise</a>发布</div>`,
			"rssTitle":         "Noise的说说笔记",
			"rssDescription":   "一个说说笔记~",
			"rssAuthorName":    "Noise",
			"rssFaviconURL":    "/favicon-32x32.png",
			"walineServerURL":  "请前往waline官网https://waline.js.org查看部署配置",
			"enableGithubCard": false,
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
			"feedEnabled":            false,
			"feedPageTitle":          "实时聚合内容动态",
			"feedPageDescription":    "聚合综合内容信息源内容，当前结果 {count} 条",
			"feedLimit":              100,
			"feedRefreshSeconds":     7200,
			"feedSources": []map[string]interface{}{
				{"type": "rss", "group": "默认分组", "name": "站点 RSS", "url": "/rss", "enabled": true, "visible": true},
			},
			// 系统欢迎组件默认参数
			"welcomeAvatarURL":       "https://s2.loli.net/2025/03/24/HnSXKvibAQlosIW.png",
			"welcomeName":            "Noise",
			"welcomeDescription":     "执迷不悟",
			"welcomeUseAdmin":        true,
			"githubOAuthEnabled":     false,
			"githubClientId":         "",
			"githubClientSecret":     "",
			"githubCallbackURL":      "",
			"pwaEnabled":             true,
			"pwaTitle":               "",
			"pwaDescription":         "",
			"pwaIconURL":             "",
			"defaultContentTheme":    "light",
			"homeLayoutDefault":      "three",
			"announcementText":       "欢迎访问我的说说笔记！",
			"announcementEnabled":    true,
			"musicEnabled":           false,
			"musicPlaylistId":        "",
			"musicSongId":            "",
			"musicPosition":          "bottom-left",
			"musicTheme":             "auto",
			"musicLyric":             true,
			"musicAutoplay":          false,
			"musicDefaultMinimized":  true,
			"musicEmbed":             false,
			"musicHideOnMobile":      true,
			"musicCssCdnURL":         "",
			"musicJsCdnURL":          "",
			"commentEnabled":         true,
			"commentSystem":          "builtin",
			"commentEmailEnabled":    false,
			"commentLoginRequired":   false,
			"hitokotoEnabled":        true,
			"lifeCountdownEnabled":   false,
			"lifeCountdownBirthDate": "",
			"lifeExpectancyYears":    80,
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
