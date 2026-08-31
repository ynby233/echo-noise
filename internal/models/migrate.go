package models

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

func MigrateDB(db *gorm.DB) error {
	dbType := db.Dialector.Name()
	var err error
	if dbType == "sqlite" {
		if err := prepareSQLiteRegistrationApplicationCandidateMigration(db); err != nil {
			return err
		}
	}
	switch dbType {
	case "postgres":
		err = db.Set("gorm:table_options", "").
			Set("gorm:varchar_size", 255).
			AutoMigrate(&User{}, &Message{}, &CloudAttachmentObject{}, &AttachmentBlob{}, &AttachmentReference{}, &LocalAttachmentGrant{}, &Comment{}, &UserNotification{}, &PasswordAlertCleanupTask{}, &WebPushSubscription{}, &WebPushPreference{}, &WebPushDelivery{}, &Announcement{}, &AnnouncementRead{}, &AnnouncementPushDelivery{}, &Setting{}, &SiteConfig{}, &NotifyConfig{}, &MessageLike{}, &UserLifeCountdownConfig{}, &UserFrontendPreference{}, &RegistrationApplication{}, &RegistrationApplicationSequence{}, &VoceChatProvisioningRun{}, &VoceChatProvisioningTask{}, &VoceChatContactCache{}, &FriendLink{}, &FriendLinkApply{}, &SecurityAttackLog{}, &SecurityIPBan{}, &SecurityConfig{}, &SecurityLoginAudit{}, &LoginAuditConfig{}, &SecurityAccessLog{}, &SecuritySiteVisitLog{}, &AdminCapabilityGrant{}, &AdminAuditLog{}, &AdminAuditConfig{})
	case "mysql":
		err = db.Set("gorm:table_options", "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci").
			Set("gorm:varchar_size", 191).
			AutoMigrate(&User{}, &Message{}, &CloudAttachmentObject{}, &AttachmentBlob{}, &AttachmentReference{}, &LocalAttachmentGrant{}, &Comment{}, &UserNotification{}, &PasswordAlertCleanupTask{}, &WebPushSubscription{}, &WebPushPreference{}, &WebPushDelivery{}, &Announcement{}, &AnnouncementRead{}, &AnnouncementPushDelivery{}, &Setting{}, &SiteConfig{}, &NotifyConfig{}, &MessageLike{}, &UserLifeCountdownConfig{}, &UserFrontendPreference{}, &RegistrationApplication{}, &RegistrationApplicationSequence{}, &VoceChatProvisioningRun{}, &VoceChatProvisioningTask{}, &VoceChatContactCache{}, &FriendLink{}, &FriendLinkApply{}, &SecurityAttackLog{}, &SecurityIPBan{}, &SecurityConfig{}, &SecurityLoginAudit{}, &LoginAuditConfig{}, &SecurityAccessLog{}, &SecuritySiteVisitLog{}, &AdminCapabilityGrant{}, &AdminAuditLog{}, &AdminAuditConfig{})
	default: // sqlite
		err = db.Set("gorm:varchar_size", 255).
			AutoMigrate(&User{}, &Message{}, &CloudAttachmentObject{}, &AttachmentBlob{}, &AttachmentReference{}, &LocalAttachmentGrant{}, &Comment{}, &UserNotification{}, &PasswordAlertCleanupTask{}, &WebPushSubscription{}, &WebPushPreference{}, &WebPushDelivery{}, &Announcement{}, &AnnouncementRead{}, &AnnouncementPushDelivery{}, &Setting{}, &SiteConfig{}, &NotifyConfig{}, &MessageLike{}, &UserLifeCountdownConfig{}, &UserFrontendPreference{}, &RegistrationApplication{}, &RegistrationApplicationSequence{}, &VoceChatProvisioningRun{}, &VoceChatProvisioningTask{}, &VoceChatContactCache{}, &FriendLink{}, &FriendLinkApply{}, &SecurityAttackLog{}, &SecurityIPBan{}, &SecurityConfig{}, &SecurityLoginAudit{}, &LoginAuditConfig{}, &SecurityAccessLog{}, &SecuritySiteVisitLog{}, &AdminCapabilityGrant{}, &AdminAuditLog{}, &AdminAuditConfig{})
	}

	if err != nil {
		return err
	}
	if dbType == "sqlite" {
		if err := enforceSQLiteRegistrationCandidateNotNull(db); err != nil {
			return err
		}
	}
	if err := dropRetiredAuthenticationAndCommentColumns(db); err != nil {
		return err
	}
	if err := db.Where("capability IN ?", []string{"rss.view", "rss.manage", "content.view_hidden"}).Delete(&AdminCapabilityGrant{}).Error; err != nil {
		return fmt.Errorf("remove retired capability grants: %w", err)
	}
	if err := db.Where("capability = ? AND user_id IN (?)", "comments.delete", db.Model(&AdminCapabilityGrant{}).Select("user_id").Where("capability = ?", "comments.trash")).
		Delete(&AdminCapabilityGrant{}).Error; err != nil {
		return fmt.Errorf("remove duplicate legacy comment delete grants: %w", err)
	}
	if err := db.Model(&AdminCapabilityGrant{}).
		Where("capability = ?", "comments.delete").
		Update("capability", "comments.trash").Error; err != nil {
		return fmt.Errorf("migrate comment delete grants: %w", err)
	}

	// 使用事务进行初始化操作
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := migrateRuntimePolicyData(tx); err != nil {
			return err
		}
		if err := migrateRegistrationApplicationAllocationData(tx); err != nil {
			return err
		}

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
			if user.LoginIssuedAt == nil {
				issuedAt := time.Now()
				if err := tx.Model(&User{}).Where("id = ?", user.ID).Update("login_issued_at", issuedAt).Error; err != nil {
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
	}); err != nil {
		return err
	}
	return ensureRegistrationApplicationCandidateUniqueIndex(db)
}

func prepareSQLiteRegistrationApplicationCandidateMigration(db *gorm.DB) error {
	if !db.Migrator().HasTable(&RegistrationApplication{}) {
		return nil
	}
	if !db.Migrator().HasColumn(&RegistrationApplication{}, "VoceChatCandidateEmail") {
		if err := db.Exec("ALTER TABLE registration_applications ADD COLUMN voce_chat_candidate_email VARCHAR(191)").Error; err != nil {
			return fmt.Errorf("add compatible registration candidate email column: %w", err)
		}
	}
	if err := db.AutoMigrate(&RegistrationApplicationSequence{}); err != nil {
		return fmt.Errorf("create registration sequence before candidate backfill: %w", err)
	}
	if err := migrateRegistrationApplicationAllocationData(db); err != nil {
		return fmt.Errorf("backfill registration candidates before SQLite constraint migration: %w", err)
	}
	return nil
}

func enforceSQLiteRegistrationCandidateNotNull(db *gorm.DB) error {
	if !db.Migrator().HasTable(&RegistrationApplication{}) || !db.Migrator().HasColumn(&RegistrationApplication{}, "VoceChatCandidateEmail") {
		return nil
	}
	var column struct {
		NotNull int `gorm:"column:notnull"`
	}
	if err := db.Raw("SELECT `notnull` FROM pragma_table_info('registration_applications') WHERE name = ?", "voce_chat_candidate_email").Scan(&column).Error; err != nil {
		return fmt.Errorf("inspect registration candidate email constraint: %w", err)
	}
	if column.NotNull == 1 {
		return nil
	}
	var nullCount int64
	if err := db.Table("registration_applications").Where("voce_chat_candidate_email IS NULL OR TRIM(voce_chat_candidate_email) = ''").Count(&nullCount).Error; err != nil {
		return fmt.Errorf("validate registration candidate email backfill: %w", err)
	}
	if nullCount != 0 {
		return fmt.Errorf("registration candidate email backfill left %d empty rows", nullCount)
	}
	if err := db.Migrator().AlterColumn(&RegistrationApplication{}, "VoceChatCandidateEmail"); err != nil {
		return fmt.Errorf("enforce registration candidate email constraint: %w", err)
	}
	column.NotNull = 0
	if err := db.Raw("SELECT `notnull` FROM pragma_table_info('registration_applications') WHERE name = ?", "voce_chat_candidate_email").Scan(&column).Error; err != nil {
		return fmt.Errorf("verify registration candidate email constraint: %w", err)
	}
	if column.NotNull != 1 {
		return errors.New("registration candidate email column remains nullable after migration")
	}
	return nil
}

func migrateRegistrationApplicationAllocationData(db *gorm.DB) error {
	var config SiteConfig
	_ = db.Order("id ASC").First(&config).Error
	domain := strings.TrimPrefix(strings.TrimSpace(config.VoceChatEmailDomain), "@")
	if domain == "" {
		domain = "vc.com"
	}

	var applications []RegistrationApplication
	if err := db.Order("id ASC").Find(&applications).Error; err != nil {
		return fmt.Errorf("load registration applications for allocation migration: %w", err)
	}
	var highest uint64
	for index := range applications {
		numericID, err := strconv.ParseUint(strings.TrimSpace(applications[index].ApplicationID), 10, 64)
		if err == nil && numericID > highest {
			highest = numericID
		}
		if strings.TrimSpace(applications[index].VoceChatCandidateEmail) != "" {
			continue
		}
		candidate := strings.TrimSpace(applications[index].ApplicationID) + "@" + domain
		if err := db.Model(&RegistrationApplication{}).Where("id = ?", applications[index].ID).Update("voce_chat_candidate_email", candidate).Error; err != nil {
			return fmt.Errorf("backfill registration candidate email for application %d: %w", applications[index].ID, err)
		}
	}

	var sequence RegistrationApplicationSequence
	if err := db.First(&sequence, 1).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("load registration application sequence: %w", err)
		}
		sequence = RegistrationApplicationSequence{ID: 1, LastValue: highest}
		if err := db.Create(&sequence).Error; err != nil {
			return fmt.Errorf("create registration application sequence: %w", err)
		}
		return nil
	}
	if sequence.LastValue < highest {
		if err := db.Model(&RegistrationApplicationSequence{}).Where("id = ?", 1).Update("last_value", highest).Error; err != nil {
			return fmt.Errorf("advance registration application sequence: %w", err)
		}
	}
	return nil
}

func ensureRegistrationApplicationCandidateUniqueIndex(db *gorm.DB) error {
	const indexName = "idx_registration_applications_candidate_email_unique"
	if db.Migrator().HasIndex(&RegistrationApplication{}, indexName) {
		return nil
	}
	if err := db.Exec("CREATE UNIQUE INDEX " + indexName + " ON registration_applications (voce_chat_candidate_email)").Error; err != nil {
		return fmt.Errorf("create unique registration candidate email index: %w", err)
	}
	return nil
}

func migrateRuntimePolicyData(db *gorm.DB) error {
	var configs []SiteConfig
	if err := db.Order("id ASC").Find(&configs).Error; err != nil {
		return fmt.Errorf("load runtime mode configuration: %w", err)
	}
	for index := range configs {
		if configs[index].RuntimeModeMigrationVersion >= RuntimeModeMigrationVersionCurrent {
			continue
		}
		mode := RuntimeModeLocal
		if configs[index].VoceChatEnabled {
			mode = RuntimeModeVoceChat
		}
		if err := db.Model(&SiteConfig{}).Where("id = ?", configs[index].ID).Updates(map[string]interface{}{
			"runtime_mode":                   mode,
			"runtime_mode_migration_version": RuntimeModeMigrationVersionCurrent,
		}).Error; err != nil {
			return fmt.Errorf("migrate runtime mode for site config %d: %w", configs[index].ID, err)
		}
		configs[index].RuntimeMode = mode
		configs[index].RuntimeModeMigrationVersion = RuntimeModeMigrationVersionCurrent
	}

	configuredMode := RuntimeModeLocal
	if len(configs) > 0 && strings.EqualFold(strings.TrimSpace(configs[0].RuntimeMode), RuntimeModeVoceChat) {
		configuredMode = RuntimeModeVoceChat
	}
	var users []User
	if err := db.Where("id <> ?", PrimaryAdminUserID).Find(&users).Error; err != nil {
		return fmt.Errorf("load users for runtime state migration: %w", err)
	}
	for index := range users {
		status := strings.TrimSpace(users[index].VoceChatSyncStatus)
		if status != "" && status != VoceChatSyncStatusNone {
			continue
		}
		nextStatus := VoceChatSyncStatusUnbound
		if strings.TrimSpace(users[index].VoceChatEmail) != "" && strings.TrimSpace(users[index].VoceChatUserID) != "" {
			nextStatus = VoceChatSyncStatusLinked
		} else if configuredMode == RuntimeModeVoceChat {
			nextStatus = VoceChatSyncStatusPending
		}
		if err := db.Model(&User{}).Where("id = ?", users[index].ID).Update("voce_chat_sync_status", nextStatus).Error; err != nil {
			return fmt.Errorf("migrate VoceChat account state for user %d: %w", users[index].ID, err)
		}
	}
	return nil
}

func dropRetiredAuthenticationAndCommentColumns(db *gorm.DB) error {
	if db.Migrator().HasColumn("site_configs", "comment_email_site_url") && db.Migrator().HasColumn(&SiteConfig{}, "SitePublicURL") {
		if err := db.Exec(`UPDATE site_configs
			SET site_public_url = comment_email_site_url
			WHERE (site_public_url IS NULL OR TRIM(site_public_url) = '')
			  AND comment_email_site_url IS NOT NULL
			  AND TRIM(comment_email_site_url) <> ''`).Error; err != nil {
			return fmt.Errorf("migrate legacy comment email site URL to site public URL: %w", err)
		}
	}
	for _, column := range []string{
		"github_o_auth_enabled",
		"github_client_id",
		"github_client_secret",
		"github_callback_url",
		"comment_system",
		"comment_email_enabled",
		"comment_email_admin_notify_all",
		"comment_login_required",
		"comment_email_reply_name",
		"comment_email_admin_prefix",
		"comment_email_reply_prefix",
		"comment_email_reply_template",
		"comment_email_admin_template",
		"comment_email_site_url",
		"comment_email_reply_template_html",
		"comment_email_admin_template_html",
	} {
		if db.Migrator().HasColumn("site_configs", column) {
			if err := db.Exec("ALTER TABLE site_configs DROP COLUMN " + column).Error; err != nil {
				return fmt.Errorf("drop retired site_configs column %s: %w", column, err)
			}
		}
	}
	return nil
}
