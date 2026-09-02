package services

import (
	"context"
	"crypto/elliptic"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type WebPushSendResult struct {
	StatusCode int
}

type WebPushSender interface {
	Send(context.Context, models.WebPushSubscription, []byte) (WebPushSendResult, error)
}

type WebPushDispatchResult struct {
	Processed int
	Sent      int
	Retried   int
	Failed    int
	Invalid   int
	Skipped   int
}

type WebPushPreferenceInput struct {
	Enabled                bool
	CommentEnabled         bool
	ReplyEnabled           bool
	GuestbookEnabled       bool
	LikeEnabled            bool
	AnnouncementEnabled    bool
	AccountSecurityEnabled bool
	ShowPreview            bool
}

var ErrWebPushTestRateLimited = errors.New("测试通知发送过于频繁，请稍后再试")

var webPushDispatcherWake = make(chan struct{}, 1)
var webPushDispatcherStart sync.Once

const webPushDeliveryRetention = 30 * 24 * time.Hour

type WebPushRuntimeConfig struct {
	PublicKey  string
	PrivateKey string
	Subject    string
	LoadError  error
}

func (config WebPushRuntimeConfig) Ready() bool {
	return config.LoadError == nil && strings.TrimSpace(config.PublicKey) != "" &&
		strings.TrimSpace(config.PrivateKey) != "" && strings.TrimSpace(config.Subject) != ""
}

func LoadWebPushRuntimeConfig() WebPushRuntimeConfig {
	config := WebPushRuntimeConfig{
		PublicKey:  strings.TrimSpace(os.Getenv("WEB_PUSH_VAPID_PUBLIC_KEY")),
		PrivateKey: strings.TrimSpace(os.Getenv("WEB_PUSH_VAPID_PRIVATE_KEY")),
		Subject:    strings.TrimSpace(os.Getenv("WEB_PUSH_VAPID_SUBJECT")),
	}
	if path := strings.TrimSpace(os.Getenv("WEB_PUSH_VAPID_PRIVATE_KEY_FILE")); path != "" {
		contents, err := os.ReadFile(path)
		if err != nil {
			config.PrivateKey = ""
			config.LoadError = errors.New("无法读取 Web Push 私钥文件")
			return config
		}
		if len(contents) > 8192 {
			config.PrivateKey = ""
			config.LoadError = errors.New("Web Push 私钥文件无效")
			return config
		}
		config.PrivateKey = strings.TrimSpace(string(contents))
	}
	if config.PublicKey == "" && config.PrivateKey == "" && config.Subject == "" {
		return config
	}
	publicKey, publicErr := base64.RawURLEncoding.DecodeString(strings.TrimRight(config.PublicKey, "="))
	privateKey, privateErr := base64.RawURLEncoding.DecodeString(strings.TrimRight(config.PrivateKey, "="))
	subject, subjectErr := url.Parse(config.Subject)
	x, y := elliptic.Unmarshal(elliptic.P256(), publicKey)
	privateScalar := new(big.Int).SetBytes(privateKey)
	validKeyPair := false
	if privateErr == nil && len(privateKey) == 32 && privateScalar.Sign() > 0 && privateScalar.Cmp(elliptic.P256().Params().N) < 0 && x != nil && y != nil {
		derivedX, derivedY := elliptic.P256().ScalarBaseMult(privateKey)
		validKeyPair = derivedX.Cmp(x) == 0 && derivedY.Cmp(y) == 0
	}
	validSubject := subjectErr == nil && ((strings.EqualFold(subject.Scheme, "mailto") && strings.TrimSpace(subject.Opaque) != "") ||
		(strings.EqualFold(subject.Scheme, "https") && strings.TrimSpace(subject.Host) != ""))
	if publicErr != nil || len(publicKey) != 65 || x == nil || y == nil ||
		privateErr != nil || len(privateKey) != 32 || privateScalar.Sign() <= 0 || privateScalar.Cmp(elliptic.P256().Params().N) >= 0 ||
		!validKeyPair || !validSubject {
		config.LoadError = errors.New("Web Push VAPID 配置无效")
	}
	return config
}

type WebPushSubscriptionInput struct {
	Endpoint  string
	P256dh    string
	Auth      string
	Platform  string
	ExpiresAt *time.Time
}

func normalizeWebPushPlatform(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "ios", "ipados", "android", "windows", "macos", "linux":
		return value
	default:
		return "unknown"
	}
}

func webPushEndpointHash(endpoint string) string {
	sum := sha256.Sum256([]byte(endpoint))
	return hex.EncodeToString(sum[:])
}

func validateWebPushSubscriptionInput(input WebPushSubscriptionInput) (WebPushSubscriptionInput, error) {
	input.Endpoint = strings.TrimSpace(input.Endpoint)
	input.P256dh = strings.TrimSpace(input.P256dh)
	input.Auth = strings.TrimSpace(input.Auth)
	input.Platform = normalizeWebPushPlatform(input.Platform)
	parsed, err := url.Parse(input.Endpoint)
	if err != nil || !strings.EqualFold(parsed.Scheme, "https") || strings.TrimSpace(parsed.Host) == "" || parsed.User != nil || (parsed.Port() != "" && parsed.Port() != "443") {
		return input, errors.New("推送订阅地址无效")
	}
	hostname := strings.TrimSpace(parsed.Hostname())
	if strings.EqualFold(hostname, "localhost") {
		return input, errors.New("推送订阅地址无效")
	}
	if ip := net.ParseIP(hostname); ip != nil && !isPublicWebPushIP(ip) {
		return input, errors.New("推送订阅地址无效")
	}
	if input.P256dh == "" || input.Auth == "" || len(input.P256dh) > 1024 || len(input.Auth) > 1024 || len(input.Endpoint) > 4096 {
		return input, errors.New("推送订阅密钥无效")
	}
	p256dh, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(input.P256dh, "="))
	if err != nil || len(p256dh) != 65 {
		return input, errors.New("推送订阅密钥无效")
	}
	x, y := elliptic.Unmarshal(elliptic.P256(), p256dh)
	if x == nil || y == nil {
		return input, errors.New("推送订阅密钥无效")
	}
	auth, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(input.Auth, "="))
	if err != nil || len(auth) != 16 {
		return input, errors.New("推送订阅密钥无效")
	}
	return input, nil
}

func UpsertWebPushSubscription(db *gorm.DB, userID uint, sessionID string, sessionIssuedAt *time.Time, sessionExpiresAt *time.Time, input WebPushSubscriptionInput, userAgent string) (models.WebPushSubscription, error) {
	var subscription models.WebPushSubscription
	if db == nil || userID == 0 || strings.TrimSpace(sessionID) == "" {
		return subscription, errors.New("登录会话无效")
	}
	input, err := validateWebPushSubscriptionInput(input)
	if err != nil {
		return subscription, err
	}
	endpointHash := webPushEndpointHash(input.Endpoint)
	err = db.Transaction(func(tx *gorm.DB) error {
		queryErr := tx.Where("endpoint_hash = ?", endpointHash).First(&subscription).Error
		if queryErr != nil && queryErr != gorm.ErrRecordNotFound {
			return queryErr
		}
		subscription.UserID = userID
		subscription.SessionID = strings.TrimSpace(sessionID)
		subscription.SessionIssuedAt = sessionIssuedAt
		subscription.SessionExpiresAt = sessionExpiresAt
		subscription.Endpoint = input.Endpoint
		subscription.EndpointHash = endpointHash
		subscription.P256dh = input.P256dh
		subscription.Auth = input.Auth
		subscription.Platform = input.Platform
		subscription.UserAgent = compactNotificationText(userAgent, 255)
		subscription.ExpiresAt = input.ExpiresAt
		subscription.DisabledAt = nil
		subscription.FailureCount = 0
		subscription.LastErrorCode = ""
		return tx.Save(&subscription).Error
	})
	return subscription, err
}

type webPushPayload struct {
	Title          string `json:"title"`
	Body           string `json:"body"`
	Icon           string `json:"icon"`
	Badge          string `json:"badge"`
	Tag            string `json:"tag"`
	URL            string `json:"url"`
	NotificationID uint   `json:"notificationId,omitempty"`
	UnreadCount    int64  `json:"unreadCount,omitempty"`
}

func defaultWebPushPreference(userID uint) models.WebPushPreference {
	return models.WebPushPreference{
		UserID: userID, Enabled: true, CommentEnabled: true, ReplyEnabled: true,
		GuestbookEnabled: true, LikeEnabled: false, AnnouncementEnabled: true,
		AccountSecurityEnabled: true, ShowPreview: false,
	}
}

func loadWebPushPreference(db *gorm.DB, userID uint) (models.WebPushPreference, error) {
	preference := defaultWebPushPreference(userID)
	var stored models.WebPushPreference
	err := db.First(&stored, "user_id = ?", userID).Error
	if err == nil {
		return stored, nil
	}
	if err != gorm.ErrRecordNotFound {
		return preference, err
	}
	if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&preference).Error; err != nil {
		return preference, err
	}
	return preference, nil
}

func GetWebPushPreference(db *gorm.DB, userID uint) (models.WebPushPreference, error) {
	if db == nil || userID == 0 {
		return models.WebPushPreference{}, errors.New("用户无效")
	}
	return loadWebPushPreference(db, userID)
}

func UpdateWebPushPreference(db *gorm.DB, userID uint, input WebPushPreferenceInput) (models.WebPushPreference, error) {
	preference, err := loadWebPushPreference(db, userID)
	if err != nil {
		return preference, err
	}
	preference.Enabled = input.Enabled
	preference.CommentEnabled = input.CommentEnabled
	preference.ReplyEnabled = input.ReplyEnabled
	preference.GuestbookEnabled = input.GuestbookEnabled
	preference.LikeEnabled = input.LikeEnabled
	preference.AnnouncementEnabled = input.AnnouncementEnabled
	preference.AccountSecurityEnabled = input.AccountSecurityEnabled
	preference.ShowPreview = input.ShowPreview
	err = db.Save(&preference).Error
	return preference, err
}

func webPushPreferenceAllows(preference models.WebPushPreference, notificationType string) bool {
	if !preference.Enabled {
		return false
	}
	switch notificationType {
	case models.UserNotificationTypeComment:
		return preference.CommentEnabled
	case models.UserNotificationTypeReply:
		return preference.ReplyEnabled
	case models.UserNotificationTypeGuestbook:
		return preference.GuestbookEnabled
	case models.UserNotificationTypeLike:
		return preference.LikeEnabled
	case models.UserNotificationTypeVoceChatCredentials,
		models.UserNotificationTypeVoceChatPasswordChanged,
		models.UserNotificationTypePasswordUpdateIncomplete:
		return preference.AccountSecurityEnabled
	default:
		return false
	}
}

func notificationSourceVersion(notification models.UserNotification) int64 {
	if !notification.UpdatedAt.IsZero() {
		return notification.UpdatedAt.UnixNano()
	}
	if !notification.CreatedAt.IsZero() {
		return notification.CreatedAt.UnixNano()
	}
	return time.Now().UnixNano()
}

func QueueWebPushForNotification(db *gorm.DB, notification models.UserNotification) error {
	if db == nil || notification.ID == 0 || notification.RecipientUserID == 0 {
		return nil
	}
	if !db.Migrator().HasTable(&models.WebPushSubscription{}) || !db.Migrator().HasTable(&models.WebPushDelivery{}) {
		return nil
	}
	preference, err := loadWebPushPreference(db, notification.RecipientUserID)
	if err != nil || !webPushPreferenceAllows(preference, notification.Type) {
		return err
	}

	var siteConfig models.SiteConfig
	_ = db.Order("id ASC").First(&siteConfig).Error
	siteTitle := strings.TrimSpace(siteConfig.SiteTitle)
	if siteTitle == "" {
		siteTitle = neutralSiteTitle
	}
	var unreadCount int64
	_ = db.Model(&models.UserNotification{}).
		Where("recipient_user_id = ? AND read_at IS NULL", notification.RecipientUserID).
		Count(&unreadCount).Error
	title := siteTitle
	body := "你有一条新通知"
	if preference.ShowPreview && notification.Type != models.UserNotificationTypeGuestbook &&
		notification.Type != models.UserNotificationTypeVoceChatCredentials &&
		notification.Type != models.UserNotificationTypeVoceChatPasswordChanged &&
		notification.Type != models.UserNotificationTypePasswordUpdateIncomplete {
		if previewTitle := userNotificationPushTitle(notification.Type, notificationActorName(db, notification.ActorUserID)); previewTitle != "" {
			title = previewTitle
		}
		if previewBody := userNotificationPushSnippet(db, notification); previewBody != "" {
			body = previewBody
		}
	}
	payload, err := json.Marshal(webPushPayload{
		Title: title, Body: body, Icon: "/android-chrome-192x192.png",
		Badge: "/android-chrome-192x192.png", Tag: fmt.Sprintf("notification-%d", notification.ID),
		URL:            fmt.Sprintf("/?tab=notifications&notification_id=%d", notification.ID),
		NotificationID: notification.ID, UnreadCount: unreadCount,
	})
	if err != nil {
		return err
	}

	now := time.Now()
	var subscriptions []models.WebPushSubscription
	if err := db.Where(
		"user_id = ? AND disabled_at IS NULL AND (expires_at IS NULL OR expires_at > ?) AND (session_expires_at IS NULL OR session_expires_at > ?)",
		notification.RecipientUserID, now, now,
	).Find(&subscriptions).Error; err != nil {
		return err
	}
	deliveries := make([]models.WebPushDelivery, 0, len(subscriptions))
	for _, subscription := range subscriptions {
		deliveries = append(deliveries, models.WebPushDelivery{
			SourceKind: models.WebPushSourceNotification, SourceID: notification.ID,
			SourceVersion: notificationSourceVersion(notification), SubscriptionID: subscription.ID,
			RecipientUserID: notification.RecipientUserID, PayloadJSON: string(payload),
			Status: models.WebPushDeliveryPending,
		})
	}
	if len(deliveries) == 0 {
		return nil
	}
	if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&deliveries).Error; err != nil {
		return err
	}
	WakeWebPushDispatcher()
	return nil
}

func createUserNotificationWithWebPush(db *gorm.DB, notification *models.UserNotification) error {
	if db == nil || notification == nil {
		return errors.New("通知存储不可用")
	}
	if err := db.Create(notification).Error; err != nil {
		return err
	}
	return QueueWebPushForNotification(db, *notification)
}

func UnregisterWebPushSubscription(db *gorm.DB, userID uint, sessionID string, endpoint string) error {
	if db == nil || userID == 0 || strings.TrimSpace(sessionID) == "" || strings.TrimSpace(endpoint) == "" {
		return nil
	}
	return db.Where("user_id = ? AND session_id = ? AND endpoint_hash = ?", userID, strings.TrimSpace(sessionID), webPushEndpointHash(strings.TrimSpace(endpoint))).
		Delete(&models.WebPushSubscription{}).Error
}

func DisableWebPushSubscriptionsForSession(db *gorm.DB, userID uint, sessionID string) error {
	if db == nil || userID == 0 || strings.TrimSpace(sessionID) == "" {
		return nil
	}
	now := time.Now()
	return db.Model(&models.WebPushSubscription{}).
		Where("user_id = ? AND session_id = ? AND disabled_at IS NULL", userID, strings.TrimSpace(sessionID)).
		Updates(map[string]interface{}{"disabled_at": now, "last_error_code": "logged_out"}).Error
}

func HasActiveWebPushSubscriptionForSession(db *gorm.DB, userID uint, sessionID string, now time.Time) (bool, error) {
	if db == nil || userID == 0 || strings.TrimSpace(sessionID) == "" {
		return false, nil
	}
	var count int64
	err := db.Model(&models.WebPushSubscription{}).
		Where("user_id = ? AND session_id = ? AND disabled_at IS NULL AND (expires_at IS NULL OR expires_at > ?) AND (session_expires_at IS NULL OR session_expires_at > ?)", userID, strings.TrimSpace(sessionID), now, now).
		Count(&count).Error
	return count > 0, err
}

func queueWebPushDeliveries(db *gorm.DB, deliveries []models.WebPushDelivery) error {
	if len(deliveries) == 0 {
		return nil
	}
	if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&deliveries).Error; err != nil {
		return err
	}
	WakeWebPushDispatcher()
	return nil
}

func QueueWebPushForAnnouncement(db *gorm.DB, announcement models.Announcement) error {
	if db == nil || announcement.ID == 0 || !announcement.PushEnabled || announcement.Status != models.AnnouncementStatusPublished {
		return nil
	}
	if !db.Migrator().HasTable(&models.WebPushSubscription{}) || !db.Migrator().HasTable(&models.WebPushDelivery{}) {
		return nil
	}
	var siteConfig models.SiteConfig
	_ = db.Order("id ASC").First(&siteConfig).Error
	siteTitle := strings.TrimSpace(siteConfig.SiteTitle)
	if siteTitle == "" {
		siteTitle = neutralSiteTitle
	}
	now := time.Now()
	var subscriptions []models.WebPushSubscription
	if err := db.Where("disabled_at IS NULL AND (expires_at IS NULL OR expires_at > ?) AND (session_expires_at IS NULL OR session_expires_at > ?)", now, now).
		Find(&subscriptions).Error; err != nil {
		return err
	}
	deliveries := make([]models.WebPushDelivery, 0, len(subscriptions))
	for _, subscription := range subscriptions {
		preference, err := loadWebPushPreference(db, subscription.UserID)
		if err != nil {
			return err
		}
		if !preference.Enabled || !preference.AnnouncementEnabled {
			continue
		}
		title := siteTitle
		body := "你有一条新通知"
		if preference.ShowPreview {
			title = strings.TrimSpace(announcement.Title)
			if title == "" {
				title = siteTitle
			}
			if preview := compactNotificationText(announcement.Content, 120); preview != "" {
				body = preview
			}
		}
		payload, err := json.Marshal(webPushPayload{
			Title: title, Body: body, Icon: "/android-chrome-192x192.png", Badge: "/android-chrome-192x192.png",
			Tag: fmt.Sprintf("announcement-%d", announcement.ID), URL: fmt.Sprintf("/?tab=announcements&announcement_id=%d", announcement.ID),
		})
		if err != nil {
			return err
		}
		sourceVersion := announcement.UpdatedAt.UnixNano()
		if sourceVersion == 0 {
			sourceVersion = int64(announcement.Revision)
		}
		deliveries = append(deliveries, models.WebPushDelivery{
			SourceKind: models.WebPushSourceAnnouncement, SourceID: announcement.ID, SourceVersion: sourceVersion,
			SubscriptionID: subscription.ID, RecipientUserID: subscription.UserID, PayloadJSON: string(payload),
			Status: models.WebPushDeliveryPending,
		})
	}
	return queueWebPushDeliveries(db, deliveries)
}

func QueueWebPushTest(db *gorm.DB, userID uint, now time.Time) error {
	if db == nil || userID == 0 {
		return errors.New("用户无效")
	}
	if now.IsZero() {
		now = time.Now()
	}
	err := db.Transaction(func(tx *gorm.DB) error {
		preference, err := loadWebPushPreference(tx, userID)
		if err != nil {
			return err
		}
		if preference.LastTestSentAt != nil && now.Sub(*preference.LastTestSentAt) < time.Minute {
			return ErrWebPushTestRateLimited
		}
		var siteConfig models.SiteConfig
		_ = tx.Order("id ASC").First(&siteConfig).Error
		title := strings.TrimSpace(siteConfig.SiteTitle)
		if title == "" {
			title = neutralSiteTitle
		}
		payload, err := json.Marshal(webPushPayload{
			Title: title, Body: "测试通知已送达", Icon: "/android-chrome-192x192.png", Badge: "/android-chrome-192x192.png",
			Tag: fmt.Sprintf("web-push-test-%d", now.Unix()), URL: "/?tab=notifications",
		})
		if err != nil {
			return err
		}
		var subscriptions []models.WebPushSubscription
		if err := tx.Where("user_id = ? AND disabled_at IS NULL AND (expires_at IS NULL OR expires_at > ?) AND (session_expires_at IS NULL OR session_expires_at > ?)", userID, now, now).
			Find(&subscriptions).Error; err != nil {
			return err
		}
		if len(subscriptions) == 0 {
			return errors.New("当前账号没有可用的推送订阅")
		}
		deliveries := make([]models.WebPushDelivery, 0, len(subscriptions))
		for _, subscription := range subscriptions {
			deliveries = append(deliveries, models.WebPushDelivery{
				SourceKind: models.WebPushSourceTest, SourceID: userID, SourceVersion: now.UnixNano(),
				SubscriptionID: subscription.ID, RecipientUserID: userID, PayloadJSON: string(payload), Status: models.WebPushDeliveryPending,
			})
		}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&deliveries).Error; err != nil {
			return err
		}
		return tx.Model(&models.WebPushPreference{}).Where("user_id = ?", userID).Update("last_test_sent_at", now).Error
	})
	if err == nil {
		WakeWebPushDispatcher()
	}
	return err
}

func WakeWebPushDispatcher() {
	select {
	case webPushDispatcherWake <- struct{}{}:
	default:
	}
}

func StartWebPushDispatcher(ctx context.Context, db *gorm.DB) {
	if ctx == nil || db == nil {
		return
	}
	webPushDispatcherStart.Do(func() {
		go func() {
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()
			run := func() {
				if _, err := PruneWebPushDeliveries(db, time.Now()); err != nil {
					log.Printf("Web Push 投递历史清理失败: error_type=%T", err)
				}
				config := LoadWebPushRuntimeConfig()
				if !config.Ready() {
					return
				}
				dispatchCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
				defer cancel()
				result, err := DispatchPendingWebPush(dispatchCtx, db, VAPIDWebPushSender{Config: config}, 50, time.Now())
				if err != nil && !errors.Is(err, context.Canceled) {
					log.Printf("Web Push 投递批次失败: error_type=%T", err)
					return
				}
				if result.Processed > 0 {
					log.Printf("Web Push 投递完成: 处理=%d 成功=%d 重试=%d 失效=%d 失败=%d 跳过=%d", result.Processed, result.Sent, result.Retried, result.Invalid, result.Failed, result.Skipped)
				}
			}
			run()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					run()
				case <-webPushDispatcherWake:
					run()
				}
			}
		}()
	})
}

func PruneWebPushDeliveries(db *gorm.DB, now time.Time) (int64, error) {
	if db == nil {
		return 0, errors.New("Web Push 投递存储不可用")
	}
	if now.IsZero() {
		now = time.Now()
	}
	terminal := []string{
		models.WebPushDeliverySent,
		models.WebPushDeliveryFailed,
		models.WebPushDeliveryInvalid,
		models.WebPushDeliverySkipped,
	}
	result := db.Where("status IN ? AND updated_at < ?", terminal, now.Add(-webPushDeliveryRetention)).
		Delete(&models.WebPushDelivery{})
	return result.RowsAffected, result.Error
}

func webPushRetryDelay(attempt uint) time.Duration {
	if attempt == 0 {
		attempt = 1
	}
	delay := time.Minute
	for index := uint(1); index < attempt && delay < 6*time.Hour; index++ {
		delay *= 2
	}
	if delay > 6*time.Hour {
		return 6 * time.Hour
	}
	return delay
}

func claimWebPushDelivery(db *gorm.DB, deliveryID uint, now time.Time) (bool, error) {
	leaseUntil := now.Add(2 * time.Minute)
	result := db.Model(&models.WebPushDelivery{}).
		Where("id = ? AND (status = ? OR (status = ? AND (next_attempt_at IS NULL OR next_attempt_at <= ?)) OR (status = ? AND lease_until < ?))",
			deliveryID, models.WebPushDeliveryPending, models.WebPushDeliveryRetry, now, models.WebPushDeliverySending, now).
		Updates(map[string]interface{}{
			"status":          models.WebPushDeliverySending,
			"attempt_count":   gorm.Expr("attempt_count + 1"),
			"last_attempt_at": now,
			"lease_until":     leaseUntil,
		})
	return result.RowsAffected == 1, result.Error
}

func finishWebPushDelivery(db *gorm.DB, deliveryID uint, updates map[string]interface{}) error {
	updates["lease_until"] = nil
	return db.Model(&models.WebPushDelivery{}).Where("id = ?", deliveryID).Updates(updates).Error
}

func webPushDeliveryAllowedNow(db *gorm.DB, delivery models.WebPushDelivery) (bool, error) {
	switch delivery.SourceKind {
	case models.WebPushSourceTest:
		return true, nil
	case models.WebPushSourceNotification:
		var notification models.UserNotification
		if err := db.Select("id", "type").First(&notification, delivery.SourceID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return false, nil
			}
			return false, err
		}
		preference, err := loadWebPushPreference(db, delivery.RecipientUserID)
		return err == nil && webPushPreferenceAllows(preference, notification.Type), err
	case models.WebPushSourceAnnouncement:
		var announcement models.Announcement
		if err := db.Select("id", "status", "push_enabled").First(&announcement, delivery.SourceID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return false, nil
			}
			return false, err
		}
		if announcement.Status != models.AnnouncementStatusPublished || !announcement.PushEnabled {
			return false, nil
		}
		preference, err := loadWebPushPreference(db, delivery.RecipientUserID)
		return err == nil && preference.Enabled && preference.AnnouncementEnabled, err
	default:
		return false, nil
	}
}

func DispatchPendingWebPush(ctx context.Context, db *gorm.DB, sender WebPushSender, limit int, now time.Time) (WebPushDispatchResult, error) {
	result := WebPushDispatchResult{}
	if db == nil || sender == nil {
		return result, errors.New("Web Push 投递器不可用")
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if now.IsZero() {
		now = time.Now()
	}
	var candidates []models.WebPushDelivery
	if err := db.Where(
		"status = ? OR (status = ? AND (next_attempt_at IS NULL OR next_attempt_at <= ?)) OR (status = ? AND lease_until < ?)",
		models.WebPushDeliveryPending, models.WebPushDeliveryRetry, now, models.WebPushDeliverySending, now,
	).Order("created_at ASC, id ASC").Limit(limit).Find(&candidates).Error; err != nil {
		return result, err
	}
	for _, candidate := range candidates {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		claimed, err := claimWebPushDelivery(db, candidate.ID, now)
		if err != nil {
			return result, err
		}
		if !claimed {
			continue
		}
		result.Processed++
		var delivery models.WebPushDelivery
		if err := db.First(&delivery, candidate.ID).Error; err != nil {
			return result, err
		}
		var subscription models.WebPushSubscription
		if err := db.First(&subscription, delivery.SubscriptionID).Error; err != nil {
			if err := finishWebPushDelivery(db, delivery.ID, map[string]interface{}{
				"status": models.WebPushDeliverySkipped, "last_error": "subscription_missing",
			}); err != nil {
				return result, err
			}
			result.Skipped++
			continue
		}
		var recipient models.User
		recipientErr := db.Select("id", "is_admin").First(&recipient, delivery.RecipientUserID).Error
		if errors.Is(recipientErr, gorm.ErrRecordNotFound) {
			disabledAt := now
			if err := db.Model(&models.WebPushSubscription{}).Where("id = ?", subscription.ID).Updates(map[string]interface{}{
				"disabled_at": disabledAt, "last_error_code": "user_missing",
			}).Error; err != nil {
				return result, err
			}
			if err := finishWebPushDelivery(db, delivery.ID, map[string]interface{}{
				"status": models.WebPushDeliverySkipped, "last_error": "recipient_missing",
			}); err != nil {
				return result, err
			}
			result.Skipped++
			continue
		}
		if recipientErr != nil {
			return result, recipientErr
		}
		sessionExpiredByPolicy := subscription.SessionIssuedAt != nil &&
			IsUserLoginExpired(&recipient, subscription.SessionIssuedAt.Unix(), now)
		if subscription.DisabledAt != nil || (subscription.ExpiresAt != nil && !subscription.ExpiresAt.After(now)) ||
			(subscription.SessionExpiresAt != nil && !subscription.SessionExpiresAt.After(now)) || sessionExpiredByPolicy {
			if err := finishWebPushDelivery(db, delivery.ID, map[string]interface{}{
				"status": models.WebPushDeliverySkipped, "last_error": "subscription_inactive",
			}); err != nil {
				return result, err
			}
			result.Skipped++
			continue
		}
		allowed, err := webPushDeliveryAllowedNow(db, delivery)
		if err != nil {
			return result, err
		}
		if !allowed {
			if err := finishWebPushDelivery(db, delivery.ID, map[string]interface{}{
				"status": models.WebPushDeliverySkipped, "last_error": "delivery_no_longer_allowed",
			}); err != nil {
				return result, err
			}
			result.Skipped++
			continue
		}
		sendResult, sendErr := sender.Send(ctx, subscription, []byte(delivery.PayloadJSON))
		statusCode := sendResult.StatusCode
		switch {
		case sendErr == nil && statusCode >= 200 && statusCode < 300:
			sentAt := now
			if err := finishWebPushDelivery(db, delivery.ID, map[string]interface{}{
				"status": models.WebPushDeliverySent, "sent_at": sentAt, "last_status_code": statusCode,
				"last_error": "", "next_attempt_at": nil,
			}); err != nil {
				return result, err
			}
			if err := db.Model(&models.WebPushSubscription{}).Where("id = ?", subscription.ID).Updates(map[string]interface{}{
				"failure_count": 0, "last_success_at": sentAt, "last_error_code": "",
			}).Error; err != nil {
				return result, err
			}
			result.Sent++
		case statusCode == 404 || statusCode == 410:
			disabledAt := now
			if err := db.Model(&models.WebPushSubscription{}).Where("id = ?", subscription.ID).Updates(map[string]interface{}{
				"disabled_at": disabledAt, "last_failure_at": now, "last_error_code": "gone",
				"failure_count": gorm.Expr("failure_count + 1"),
			}).Error; err != nil {
				return result, err
			}
			if err := finishWebPushDelivery(db, delivery.ID, map[string]interface{}{
				"status": models.WebPushDeliveryInvalid, "last_status_code": statusCode,
				"last_error": "subscription_gone", "next_attempt_at": nil,
			}); err != nil {
				return result, err
			}
			result.Invalid++
		case sendErr != nil || statusCode == 429 || statusCode >= 500:
			if delivery.AttemptCount >= 8 {
				if err := finishWebPushDelivery(db, delivery.ID, map[string]interface{}{
					"status": models.WebPushDeliveryFailed, "last_status_code": statusCode,
					"last_error": "retry_exhausted", "next_attempt_at": nil,
				}); err != nil {
					return result, err
				}
				result.Failed++
				continue
			}
			nextAttempt := now.Add(webPushRetryDelay(delivery.AttemptCount))
			if err := finishWebPushDelivery(db, delivery.ID, map[string]interface{}{
				"status": models.WebPushDeliveryRetry, "last_status_code": statusCode,
				"last_error": "temporary_failure", "next_attempt_at": nextAttempt,
			}); err != nil {
				return result, err
			}
			if err := db.Model(&models.WebPushSubscription{}).Where("id = ?", subscription.ID).Updates(map[string]interface{}{
				"last_failure_at": now, "last_error_code": "temporary_failure",
				"failure_count": gorm.Expr("failure_count + 1"),
			}).Error; err != nil {
				return result, err
			}
			result.Retried++
		default:
			if err := finishWebPushDelivery(db, delivery.ID, map[string]interface{}{
				"status": models.WebPushDeliveryFailed, "last_status_code": statusCode,
				"last_error": "permanent_failure", "next_attempt_at": nil,
			}); err != nil {
				return result, err
			}
			result.Failed++
		}
	}
	return result, nil
}
