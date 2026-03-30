package services

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
	"golang.org/x/crypto/bcrypt"
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
		defaultBg := `["https://s2.loli.net/2025/03/27/KJ1trnU2ksbFEYM.jpg","https://s2.loli.net/2025/03/27/MZqaLczCvwjSmW7.jpg","https://s2.loli.net/2025/03/27/UMijKXwJ9yTqSeE.jpg","https://s2.loli.net/2025/03/27/WJQIlkXvBg2afcR.jpg","https://s2.loli.net/2025/03/27/oHNQtf4spkq2iln.jpg","https://s2.loli.net/2025/03/27/PMRuX5loc6Uaimw.jpg","https://s2.loli.net/2025/03/27/U2WIslbNyTLt4rD.jpg","https://s2.loli.net/2025/03/27/xu1jZL5Og4pqT9d.jpg","https://s2.loli.net/2025/03/27/OXqwzZ6v3PVIns9.jpg","https://s2.loli.net/2025/03/27/HGuqlE6apgNywbh.jpg","https://s2.loli.net/2025/03/26/d7iyuPYA8cRqD1K.jpg","https://s2.loli.net/2025/03/27/7Zck3y6XTzhYPs5.jpg","https://s2.loli.net/2025/03/27/y67m2k5xcSdTsHN.jpg"]`

		defaultConfig := models.SiteConfig{
			SiteTitle:              "说说笔记",
			SubtitleText:           "欢迎访问！",
			AvatarURL:              "https://s2.loli.net/2025/03/24/HnSXKvibAQlosIW.png",
			Username:               "Noise",
			Description:            "执迷不悟",
			Backgrounds:            defaultBg,
			CardFooterTitle:        "说说·笔记~",
			CardFooterLink:         "note.noisework.cn",
			PageFooterHTML:         `<div class="text-center text-xs text-gray-400 py-4">来自<a href="https://www.noisework.cn" target="_blank" rel="noopener noreferrer" class="text-orange-400 hover:text-orange-500">Noise</a> 使用<a href="https://github.com/rcy1314/echo-noise" target="_blank" rel="noopener noreferrer" class="text-orange-400 hover:text-orange-500">Ech0-Noise</a>发布</div>`,
			RSSTitle:               "说说笔记",
			RSSDescription:         "一个说说笔记~",
			RSSAuthorName:          "Noise",
			RSSFaviconURL:          "/favicon.svg",
			WalineServerURL:        "请前往waline官网https://waline.js.org查看部署配置",
			AnnouncementText:       "欢迎访问我的说说笔记！",
			AnnouncementEnabled:    true,
			CommentEnabled:         true,
			CommentSystem:          "builtin",
			CommentLoginRequired:   true,
			CalendarEnabled:        true,
			TimeEnabled:            true,
			HitokotoEnabled:        true,
			LifeCountdownEnabled:   false,
			LifeCountdownBirthDate: "",
			LifeExpectancyYears:    80,
			// 广告默认参数
			LeftAdEnabled:     true,
			LeftAds:           `[{"imageURL":"https://picsum.photos/seed/ad-1/640/640","linkURL":"https://note.noisework.cn","description":"写作与记录，开启灵感之旅"},{"imageURL":"https://picsum.photos/seed/ad-2/640/640","linkURL":"https://noisework.cn","description":"探索新主题与小工具"},{"imageURL":"https://picsum.photos/seed/ad-3/640/640","linkURL":"https://github.com","description":"开源项目，欢迎 Star"}]`,
			LeftAdsIntervalMs: 4000,
			// 社交链接默认
			SocialLinksEnabled: true,
			SocialLinks:        `[{"name":"TG","url":"https://tg.noisework.cn","icon":"i-mdi-near-me"},{"name":"X","url":"https://x.com/liangwenhao3","icon":"i-mdi-twitter"},{"name":"主页","url":"https://www.noisework.cn/","icon":"i-mdi-home"},{"name":"博客","url":"https://www.noiseblogs.top/","icon":"i-mdi-notebook"}]`,
			// PWA defaults
			PwaEnabled:        true,
			PwaTitle:          "说说笔记",
			PwaDescription:    "一个丰富的个人说说笔记",
			HomeLayoutDefault: "three",
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
			AllowRegistration: true,
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
				AvatarURL:     "https://s2.loli.net/2025/03/24/HnSXKvibAQlosIW.png",
				Email:         "",
				EmailVerified: true,
			}
			if err := db.Create(&sysUser).Error; err != nil {
				return fmt.Errorf("初始化系统默认用户失败: %v", err)
			}
		}
	}

	// 3. 初始化留言板 (如果不存在)
	var msg models.Message
	if err := db.Where("content LIKE ?", "%#guestbook%").First(&msg).Error; err != nil {
		// 查找管理员ID
		var admin models.User
		db.Where("is_admin = ?", true).First(&admin)
		uid := admin.ID
		if uid == 0 {
			uid = 1 // Fallback
		}

		guestbook := models.Message{
			Content:   "留言板\n\n此条用于承载全站留言，不会参与普通内容展示。\n\n#留言 #guestbook",
			UserID:    uid,
			Username:  admin.Username,
			Private:   false,
			Pinned:    false,
			CreatedAt: time.Now(),
		}
		if err := db.Create(&guestbook).Error; err != nil {
			return fmt.Errorf("初始化留言板失败: %v", err)
		}
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
				Content:   "欢迎来到说说笔记！默认用户名及密码均为admin，记得到后台页修改你的用户名或密码,密码带有加强设置，如需简单密码可在用户管理面板中展开后重置密码",
				UserID:    uid,
				Username:  admin.Username,
				CreatedAt: time.Now(),
			},
			{
				Content:   "这里有一些关于自己的美好记录。 #日记 #示例",
				UserID:    uid,
				Username:  admin.Username,
				ImageURL:  "https://s2.loli.net/2025/12/16/nsROlxQD5EPZq6h.jpg",
				CreatedAt: time.Now().Add(-1 * time.Hour),
			},
			{
				Content:   "探索未知的世界。 #Travel",
				UserID:    uid,
				Username:  admin.Username,
				ImageURL:  "https://s2.loli.net/2025/04/05/EnakPbZJjpGxRTw.jpg",
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

	return nil
}
