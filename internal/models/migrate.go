package models

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
)

func MigrateDB(db *gorm.DB) error {
	dbType := db.Dialector.Name()
	var err error
	switch dbType {
	case "postgres":
		err = db.Set("gorm:table_options", "").
			Set("gorm:varchar_size", 255).
			AutoMigrate(&User{}, &Message{}, &Comment{}, &Setting{}, &SiteConfig{}, &NotifyConfig{}, &MessageLike{}, &FriendLink{}, &FriendLinkApply{})
	case "mysql":
		err = db.Set("gorm:table_options", "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci").
			Set("gorm:varchar_size", 191).
			AutoMigrate(&User{}, &Message{}, &Comment{}, &Setting{}, &SiteConfig{}, &NotifyConfig{}, &MessageLike{}, &FriendLink{}, &FriendLinkApply{})
	default: // sqlite
		err = db.Set("gorm:varchar_size", 255).
			AutoMigrate(&User{}, &Message{}, &Comment{}, &Setting{}, &SiteConfig{}, &NotifyConfig{}, &MessageLike{}, &FriendLink{}, &FriendLinkApply{})
	}

	if err != nil {
		return err
	}

	// 使用事务进行初始化操作
	return db.Transaction(func(tx *gorm.DB) error {
		// 为现有用户添加 Token 字段
		var users []User
		if err := tx.Find(&users).Error; err != nil {
			return err
		}
		for _, user := range users {
			if user.Token == "" {
				newToken := GenerateToken(32)
				if err := tx.Model(&User{}).Where("id = ?", user.ID).Update("token", newToken).Error; err != nil {
					return err
				}
			}
			if strings.TrimSpace(user.Password) == "" && user.IsAdmin {
				if err := tx.Model(&User{}).Where("id = ?", user.ID).Update("password", HashPassword("123456")).Error; err != nil {
					return err
				}
			}
		}

		// 注意：默认数据的初始化逻辑已迁移至 services.SeedDefaultData，
		// 以避免重复并确保逻辑统一。migrate.go 仅负责数据库结构迁移和必要的数据修补。

		// 初始化推送配置（恢复默认数据）
		var notifyCount int64
		if err := tx.Model(&NotifyConfig{}).Count(&notifyCount).Error; err == nil && notifyCount == 0 {
			defaultNotifyConfig := NotifyConfig{
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
			if err := tx.Create(&defaultNotifyConfig).Error; err != nil {
				return fmt.Errorf("初始化推送配置失败: %v", err)
			}
		}

		return nil
	})
}
