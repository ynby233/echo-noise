package services

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// SeedDefaultData 初始化默认数据
func SeedDefaultData() error {
	db, err := database.GetDB()
	if err != nil {
		return err
	}

	// 1. 初始化站点配置
	var count int64
	if err := db.Model(&models.SiteConfig{}).Count(&count).Error; err == nil && count == 0 {
		defaultBg := defaultHeaderImagesJSON()

		defaultConfig := models.SiteConfig{
			SiteTitle:                   neutralSiteTitle,
			SubtitleText:                "欢迎访问！",
			AvatarURL:                   neutralAvatarURL,
			Username:                    neutralOwnerName,
			Description:                 neutralDescription,
			Backgrounds:                 defaultBg,
			PageFooterHTML:              "",
			RSSTitle:                    neutralRSSTitle,
			RSSDescription:              neutralRSSDescription,
			RSSAuthorName:               neutralOwnerName,
			RSSFaviconURL:               "/favicon.svg",
			AnnouncementText:            neutralAnnouncement,
			AnnouncementEnabled:         true,
			CommentPageTitle:            "留言",
			CommentPageDescription:      "欢迎留下你的看法",
			NotificationPageTitle:       "通知",
			NotificationPageDescription: "欢迎彼此间互相交流",
			CommentEnabled:              true,
			CommentLoginRequired:        true,
			CalendarEnabled:             true,
			TimeEnabled:                 true,
			HitokotoEnabled:             true,
			LifeCountdownEnabled:        false,
			LifeCountdownBirthDate:      "",
			LifeExpectancyYears:         80,
			// 广告默认参数
			LeftAdEnabled:     true,
			LeftAds:           `[{"imageURL":"https://picsum.photos/seed/ad-1/640/640","linkURL":"","description":"写作与记录","textColor":"#ffffff","textDisplayMode":"hover"},{"imageURL":"https://picsum.photos/seed/ad-2/640/640","linkURL":"","description":"探索新主题与小工具","textColor":"#ffffff","textDisplayMode":"hover"},{"imageURL":"https://picsum.photos/seed/ad-3/640/640","linkURL":"","description":"记录日常内容","textColor":"#ffffff","textDisplayMode":"hover"}]`,
			LeftAdsIntervalMs: 4000,
			LoginExpireDays:   3,
			// 社交链接默认
			SocialLinksEnabled:  true,
			SocialLinks:         `[]`,
			FeedPageTitle:       "实时聚合内容动态",
			FeedPageDescription: "聚合综合内容信息源内容，当前结果 {count} 条",
			// PWA defaults
			PwaEnabled:                  true,
			PwaTitle:                    neutralSiteTitle,
			PwaDescription:              neutralPwaDescription,
			HomeLayoutDefault:           "three",
			RuntimeMode:                 models.RuntimeModeLocal,
			RuntimeModeMigrationVersion: models.RuntimeModeMigrationVersionCurrent,
			// Cloud Storage Defaults
			StorageEnabled:           false,
			AttachmentStorageEnabled: false,
		}
		if err := db.Create(&defaultConfig).Error; err != nil {
			return fmt.Errorf("初始化站点配置失败: %v", err)
		}
	}

	// 1.5 初始化系统设置 (AllowRegistration)
	if err := db.Model(&models.Setting{}).Count(&count).Error; err == nil && count == 0 {
		defaultSetting := models.Setting{
			AllowRegistration:       true,
			AutoApproveRegistration: false,
		}
		if err := db.Create(&defaultSetting).Error; err != nil {
			return fmt.Errorf("初始化系统设置失败: %v", err)
		}
	}

	// 1.6 初始化推送配置（恢复默认数据）
	if err := db.Model(&models.NotifyConfig{}).Count(&count).Error; err == nil && count == 0 {
		defaultNotifyConfig := models.NotifyConfig{
			WebhookEnabled:           false,
			WebhookURL:               "WebhookURL",
			TelegramEnabled:          false,
			TelegramToken:            "bot_token",
			TelegramChatID:           "chat_id",
			WeworkEnabled:            false,
			WeworkKey:                "WebhookURL",
			FeishuEnabled:            false,
			FeishuWebhook:            "FeishuWebhook",
			FeishuSecret:             "secret",
			TwitterEnabled:           false,
			TwitterApiKey:            "twitter_api_key",
			TwitterApiSecret:         "twitter_api_secret",
			TwitterAccessToken:       "twitter_access_token",
			TwitterAccessTokenSecret: "twitter_access_token_secret",
			CustomHttpEnabled:        false,
			CustomHttpUrl:            "https://example.com/notify",
			CustomHttpMethod:         "POST",
			CustomHttpHeaders:        `{"Authorization":"Bearer token"}`,
			CustomHttpBody:           `{"content":"{{content}}"}`,
		}
		if err := db.Create(&defaultNotifyConfig).Error; err != nil {
			return fmt.Errorf("初始化推送配置失败: %v", err)
		}
	}

	// 2. 初始化默认系统用户 (如果不存在)
	// 移动端嵌入式后端：不创建默认 admin 用户，确保“首次注册用户”成为管理员（不影响 Docker/桌面端默认账号逻辑）
	if strings.TrimSpace(os.Getenv("NOISE_MOBILE")) != "1" {
		if err := db.Model(&models.User{}).Count(&count).Error; err == nil && count == 0 {
			hashed, _ := bcrypt.GenerateFromPassword([]byte("admin"), bcrypt.DefaultCost)
			sysUser := models.User{
				Username:      "admin",
				Password:      string(hashed),
				IsAdmin:       true,
				Token:         models.GenerateToken(32),
				Description:   "欢迎访问",
				AvatarURL:     neutralAvatarURL,
				Email:         "",
				EmailVerified: true,
			}
			if err := db.Create(&sysUser).Error; err != nil {
				return fmt.Errorf("初始化系统默认用户失败: %v", err)
			}
		}
	}

	// 3. 初始化并修复规范留言板：逻辑所有者始终是 1 号管理员。
	if _, err := EnsureGuestbook(db); err != nil {
		return fmt.Errorf("初始化留言板失败: %v", err)
	}

	// 4. 初始化默认演示消息
	var msgCount int64
	db.Model(&models.Message{}).Count(&msgCount)
	// 如果只有留言板一条消息（或者没有消息），则添加演示消息
	if msgCount <= 1 {
		var admin models.User
		db.Where("is_admin = ?", true).First(&admin)
		uid := admin.ID
		if uid == 0 {
			uid = 1 // Fallback
		}

		messages := []models.Message{
			{
				Content:   neutralWelcomeMessage,
				UserID:    uid,
				Username:  admin.Username,
				CreatedAt: time.Now(),
			},
			{
				Content:   "这里有一些关于自己的美好记录。 #日记 #示例",
				UserID:    uid,
				Username:  admin.Username,
				CreatedAt: time.Now().Add(-1 * time.Hour),
			},
			{
				Content:   "探索未知的世界。 #Travel",
				UserID:    uid,
				Username:  admin.Username,
				CreatedAt: time.Now().Add(-2 * time.Hour),
			},
			{
				Content:   "记录生活中的点滴。 #Life #Daily",
				UserID:    uid,
				Username:  admin.Username,
				CreatedAt: time.Now().Add(-3 * time.Hour),
			},
		}

		for _, m := range messages {
			var c int64
			db.Model(&models.Message{}).Where("content = ? AND user_id = ?", m.Content, uid).Count(&c)
			if c == 0 {
				db.Create(&m)
			}
		}
	}

	if err := collapseLegacyDefaultBackgrounds(db); err != nil {
		return err
	}
	if err := scrubPersistedLegacyPublicBranding(db); err != nil {
		return err
	}

	return nil
}

func collapseLegacyDefaultBackgrounds(db *gorm.DB) error {
	var configs []models.SiteConfig
	if err := db.Table("site_configs").Find(&configs).Error; err != nil {
		return fmt.Errorf("查询站点头图配置失败: %v", err)
	}

	defaultBg := defaultHeaderImagesJSON()
	for _, config := range configs {
		if !shouldCollapseLegacyBackgrounds(config.GetBackgroundsList()) {
			continue
		}
		if strings.TrimSpace(config.Backgrounds) == defaultBg {
			continue
		}
		if err := db.Table("site_configs").Where("id = ?", config.ID).Update("backgrounds", defaultBg).Error; err != nil {
			return fmt.Errorf("收敛默认头图配置失败: %v", err)
		}
	}

	return nil
}
