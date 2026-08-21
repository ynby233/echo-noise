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
			AutoMigrate(&User{}, &Message{}, &CloudAttachmentObject{}, &AttachmentBlob{}, &AttachmentReference{}, &LocalAttachmentGrant{}, &Comment{}, &UserNotification{}, &Announcement{}, &AnnouncementRead{}, &AnnouncementPushDelivery{}, &Setting{}, &SiteConfig{}, &NotifyConfig{}, &MessageLike{}, &UserLifeCountdownConfig{}, &UserFrontendPreference{}, &RegistrationApplication{}, &VoceChatContactCache{}, &FriendLink{}, &FriendLinkApply{}, &SecurityAttackLog{}, &SecurityIPBan{}, &SecurityConfig{}, &SecurityLoginAudit{}, &SecurityAccessLog{}, &SecuritySiteVisitLog{}, &AdminCapabilityGrant{}, &AdminAuditLog{}, &AdminAuditConfig{})
	case "mysql":
		err = db.Set("gorm:table_options", "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci").
			Set("gorm:varchar_size", 191).
			AutoMigrate(&User{}, &Message{}, &CloudAttachmentObject{}, &AttachmentBlob{}, &AttachmentReference{}, &LocalAttachmentGrant{}, &Comment{}, &UserNotification{}, &Announcement{}, &AnnouncementRead{}, &AnnouncementPushDelivery{}, &Setting{}, &SiteConfig{}, &NotifyConfig{}, &MessageLike{}, &UserLifeCountdownConfig{}, &UserFrontendPreference{}, &RegistrationApplication{}, &VoceChatContactCache{}, &FriendLink{}, &FriendLinkApply{}, &SecurityAttackLog{}, &SecurityIPBan{}, &SecurityConfig{}, &SecurityLoginAudit{}, &SecurityAccessLog{}, &SecuritySiteVisitLog{}, &AdminCapabilityGrant{}, &AdminAuditLog{}, &AdminAuditConfig{})
	default: // sqlite
		err = db.Set("gorm:varchar_size", 255).
			AutoMigrate(&User{}, &Message{}, &CloudAttachmentObject{}, &AttachmentBlob{}, &AttachmentReference{}, &LocalAttachmentGrant{}, &Comment{}, &UserNotification{}, &Announcement{}, &AnnouncementRead{}, &AnnouncementPushDelivery{}, &Setting{}, &SiteConfig{}, &NotifyConfig{}, &MessageLike{}, &UserLifeCountdownConfig{}, &UserFrontendPreference{}, &RegistrationApplication{}, &VoceChatContactCache{}, &FriendLink{}, &FriendLinkApply{}, &SecurityAttackLog{}, &SecurityIPBan{}, &SecurityConfig{}, &SecurityLoginAudit{}, &SecurityAccessLog{}, &SecuritySiteVisitLog{}, &AdminCapabilityGrant{}, &AdminAuditLog{}, &AdminAuditConfig{})
	}

	if err != nil {
		return err
	}
	if err := dropRetiredAuthenticationAndCommentColumns(db); err != nil {
		return err
	}
	if err := db.Where("capability IN ?", []string{"rss.view", "rss.manage"}).Delete(&AdminCapabilityGrant{}).Error; err != nil {
		return fmt.Errorf("remove retired RSS capability grants: %w", err)
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

		if err := tx.Model(&Message{}).
			Where("private = ? AND (visibility IS NULL OR visibility = ? OR visibility = ?)", true, "", "public").
			Update("visibility", "private").Error; err != nil {
			return err
		}
		if err := tx.Model(&Message{}).
			Where("private = ? AND (visibility IS NULL OR visibility = ?)", false, "").
			Update("visibility", "public").Error; err != nil {
			return err
		}

		// Historical pin records did not retain the time of the pin operation.
		// Use created_at as a deterministic fallback; new mutations write their
		// own timestamps.
		if err := tx.Model(&Message{}).
			Where("pinned = ? AND pinned_at IS NULL", true).
			Update("pinned_at", gorm.Expr("created_at")).Error; err != nil {
			return err
		}
		if err := tx.Model(&Message{}).
			Where("personal_pinned = ? AND personal_pinned_at IS NULL", true).
			Update("personal_pinned_at", gorm.Expr("created_at")).Error; err != nil {
			return err
		}

		var messages []Message
		if err := tx.Find(&messages).Error; err != nil {
			return err
		}

		// Freeze the canonical guestbook marker during migration. Existing
		// deployments only had a content-string convention, so recognize the
		// narrow historical marker once, keep the lowest-ID candidate, and
		// repair only that system row's logical owner to user 1.
		var canonicalGuestbook *Message
		for index := range messages {
			if !messages[index].IsGuestbook && !IsCanonicalGuestbookContent(messages[index].Content) {
				continue
			}
			if canonicalGuestbook == nil || messages[index].ID < canonicalGuestbook.ID {
				candidate := messages[index]
				canonicalGuestbook = &candidate
			}
		}
		if canonicalGuestbook != nil {
			if err := tx.Model(&Message{}).Where("is_guestbook = ?", true).Update("is_guestbook", false).Error; err != nil {
				return err
			}
			username := ""
			var primary User
			if err := tx.Select("id", "username").First(&primary, PrimaryAdminUserID).Error; err == nil {
				username = primary.Username
			}
			if err := tx.Model(&Message{}).Where("id = ?", canonicalGuestbook.ID).Updates(map[string]interface{}{
				"is_guestbook": true,
				"user_id":      PrimaryAdminUserID,
				"username":     username,
			}).Error; err != nil {
				return err
			}
		}
		for index := range messages {
			if err := SyncLocalAttachmentGrants(tx, &messages[index]); err != nil {
				return fmt.Errorf("backfill local attachment visibility for message %d: %w", messages[index].ID, err)
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

func dropRetiredAuthenticationAndCommentColumns(db *gorm.DB) error {
	for _, column := range []string{
		"github_o_auth_enabled",
		"github_client_id",
		"github_client_secret",
		"github_callback_url",
		"comment_system",
	} {
		if db.Migrator().HasColumn("site_configs", column) {
			if err := db.Exec("ALTER TABLE site_configs DROP COLUMN " + column).Error; err != nil {
				return fmt.Errorf("drop retired site_configs column %s: %w", column, err)
			}
		}
	}
	return nil
}
