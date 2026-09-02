package services

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/glebarez/sqlite"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func validWebPushSubscriptionKeys(t *testing.T) (string, string) {
	t.Helper()
	_, publicKey, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		t.Fatalf("generate test subscription key: %v", err)
	}
	return publicKey, base64.RawURLEncoding.EncodeToString(make([]byte, 16))
}

type webPushSenderStub struct {
	statusCode int
	err        error
	seen       int
}

func (sender *webPushSenderStub) Send(_ context.Context, _ models.WebPushSubscription, _ []byte) (WebPushSendResult, error) {
	sender.seen++
	return WebPushSendResult{StatusCode: sender.statusCode}, sender.err
}

func openWebPushTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open web push test database: %v", err)
	}
	if err := db.AutoMigrate(
		&models.User{},
		&models.Message{},
		&models.Comment{},
		&models.SiteConfig{},
		&models.UserNotification{},
		&models.Announcement{},
		&models.WebPushSubscription{},
		&models.WebPushPreference{},
		&models.WebPushDelivery{},
	); err != nil {
		t.Fatalf("migrate web push test database: %v", err)
	}
	return db
}

func TestCreateUserNotificationWithPreviewDoesNotDeadlockSingleSQLiteConnection(t *testing.T) {
	db := openWebPushTestDB(t)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("open sql database: %v", err)
	}
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetMaxOpenConns(1)

	previousDB := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = previousDB })

	recipient := models.User{Username: "preview-recipient", Password: "hashed"}
	actor := models.User{Username: "preview-actor", Password: "hashed"}
	if err := db.Create(&recipient).Error; err != nil {
		t.Fatalf("create preview recipient: %v", err)
	}
	if err := db.Create(&actor).Error; err != nil {
		t.Fatalf("create preview actor: %v", err)
	}
	message := models.Message{UserID: recipient.ID, Username: recipient.Username, Content: "preview message"}
	if err := db.Create(&message).Error; err != nil {
		t.Fatalf("create preview message: %v", err)
	}
	actorID := actor.ID
	comment := models.Comment{MessageID: message.ID, UserID: &actorID, Content: "preview comment body"}
	if err := db.Create(&comment).Error; err != nil {
		t.Fatalf("create preview comment: %v", err)
	}
	if err := db.Create(&models.WebPushPreference{
		UserID: recipient.ID, Enabled: true, CommentEnabled: true, ShowPreview: true,
	}).Error; err != nil {
		t.Fatalf("create preview preference: %v", err)
	}
	if err := db.Create(&models.WebPushSubscription{
		UserID: recipient.ID, SessionID: "preview-session", Endpoint: "https://push.example/preview",
		EndpointHash: "preview-endpoint", P256dh: "p256dh", Auth: "auth",
	}).Error; err != nil {
		t.Fatalf("create preview subscription: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		messageID := message.ID
		commentID := comment.ID
		notification := models.UserNotification{
			RecipientUserID: recipient.ID,
			ActorUserID:     &actorID,
			Type:            models.UserNotificationTypeComment,
			MessageID:       &messageID,
			CommentID:       &commentID,
		}
		done <- db.Transaction(func(tx *gorm.DB) error {
			return createUserNotificationWithWebPush(tx, &notification)
		})
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("create notification with preview: %v", err)
		}
	case <-time.After(750 * time.Millisecond):
		t.Fatal("creating a preview notification deadlocked while the transaction held the only SQLite connection")
	}

	var delivery models.WebPushDelivery
	if err := db.Order("id DESC").First(&delivery).Error; err != nil {
		t.Fatalf("load preview delivery: %v", err)
	}
	if !strings.Contains(delivery.PayloadJSON, actor.Username) || !strings.Contains(delivery.PayloadJSON, comment.Content) {
		t.Fatalf("preview payload did not include the transaction-scoped actor and comment: %s", delivery.PayloadJSON)
	}
}

func createWebPushDeliveryFixture(t *testing.T, db *gorm.DB, sessionExpiresAt *time.Time) (models.WebPushSubscription, models.WebPushDelivery) {
	t.Helper()
	user := models.User{Username: "dispatch-recipient-" + time.Now().Format("150405.000000000"), Password: "hashed"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create dispatch recipient: %v", err)
	}
	subscription := models.WebPushSubscription{
		UserID: user.ID, SessionID: "dispatch-session", SessionExpiresAt: sessionExpiresAt,
		Endpoint: "https://push.example/dispatch", EndpointHash: "dispatch-" + time.Now().Format("150405.000000000"),
		P256dh: "p256dh", Auth: "auth",
	}
	if err := db.Create(&subscription).Error; err != nil {
		t.Fatalf("create dispatch subscription: %v", err)
	}
	delivery := models.WebPushDelivery{
		SourceKind: models.WebPushSourceTest, SourceID: user.ID, SourceVersion: time.Now().UnixNano(),
		SubscriptionID: subscription.ID, RecipientUserID: user.ID, PayloadJSON: `{"title":"test"}`,
		Status: models.WebPushDeliveryPending,
	}
	if err := db.Create(&delivery).Error; err != nil {
		t.Fatalf("create dispatch delivery: %v", err)
	}
	return subscription, delivery
}

func TestQueueWebPushForNotificationTargetsEveryActiveLoginWithoutLeakingPreview(t *testing.T) {
	db := openWebPushTestDB(t)
	user := models.User{Username: "recipient", Password: "hashed"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create recipient: %v", err)
	}
	if err := db.Create(&models.SiteConfig{SiteTitle: "测试站点"}).Error; err != nil {
		t.Fatalf("create site config: %v", err)
	}
	validUntil := time.Now().Add(time.Hour)
	subscriptions := []models.WebPushSubscription{
		{UserID: user.ID, SessionID: "session-a", SessionExpiresAt: &validUntil, Endpoint: "https://push.example/a", EndpointHash: "endpoint-a", P256dh: "p256dh-a", Auth: "auth-a"},
		{UserID: user.ID, SessionID: "session-b", SessionExpiresAt: &validUntil, Endpoint: "https://push.example/b", EndpointHash: "endpoint-b", P256dh: "p256dh-b", Auth: "auth-b"},
	}
	if err := db.Create(&subscriptions).Error; err != nil {
		t.Fatalf("create subscriptions: %v", err)
	}
	notification := models.UserNotification{RecipientUserID: user.ID, Type: models.UserNotificationTypeComment}
	if err := db.Create(&notification).Error; err != nil {
		t.Fatalf("create notification: %v", err)
	}

	if err := QueueWebPushForNotification(db, notification); err != nil {
		t.Fatalf("queue web push notification: %v", err)
	}

	var deliveries []models.WebPushDelivery
	if err := db.Order("subscription_id ASC").Find(&deliveries).Error; err != nil {
		t.Fatalf("load deliveries: %v", err)
	}
	if len(deliveries) != 2 {
		t.Fatalf("delivery count = %d, want one delivery for each active login", len(deliveries))
	}
	for _, delivery := range deliveries {
		if delivery.Status != models.WebPushDeliveryPending {
			t.Fatalf("delivery %d status = %q, want pending", delivery.ID, delivery.Status)
		}
		if !strings.Contains(delivery.PayloadJSON, "你有一条新通知") {
			t.Fatalf("delivery %d does not use the private lock-screen body: %s", delivery.ID, delivery.PayloadJSON)
		}
		if strings.Contains(delivery.PayloadJSON, "评论正文") {
			t.Fatalf("delivery %d leaked a comment preview by default: %s", delivery.ID, delivery.PayloadJSON)
		}
	}
}

func TestLikeWebPushIsAvailableButDisabledUntilTheUserEnablesIt(t *testing.T) {
	db := openWebPushTestDB(t)
	user := models.User{Username: "like-recipient", Password: "hashed"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create recipient: %v", err)
	}
	subscription := models.WebPushSubscription{
		UserID: user.ID, SessionID: "session-like", Endpoint: "https://push.example/like",
		EndpointHash: "endpoint-like", P256dh: "p256dh-like", Auth: "auth-like",
	}
	if err := db.Create(&subscription).Error; err != nil {
		t.Fatalf("create subscription: %v", err)
	}
	notification := models.UserNotification{RecipientUserID: user.ID, Type: models.UserNotificationTypeLike}
	if err := db.Create(&notification).Error; err != nil {
		t.Fatalf("create like notification: %v", err)
	}

	if err := QueueWebPushForNotification(db, notification); err != nil {
		t.Fatalf("queue with default preference: %v", err)
	}
	var count int64
	if err := db.Model(&models.WebPushDelivery{}).Count(&count).Error; err != nil {
		t.Fatalf("count default deliveries: %v", err)
	}
	if count != 0 {
		t.Fatalf("default like delivery count = %d, want 0", count)
	}

	if err := db.Model(&models.WebPushPreference{}).Where("user_id = ?", user.ID).Update("like_enabled", true).Error; err != nil {
		t.Fatalf("enable like push: %v", err)
	}
	if err := QueueWebPushForNotification(db, notification); err != nil {
		t.Fatalf("queue after enabling like push: %v", err)
	}
	if err := db.Model(&models.WebPushDelivery{}).Count(&count).Error; err != nil {
		t.Fatalf("count enabled deliveries: %v", err)
	}
	if count != 1 {
		t.Fatalf("enabled like delivery count = %d, want 1", count)
	}
}

func TestUpsertWebPushSubscriptionMovesTheBrowserEndpointToTheCurrentLogin(t *testing.T) {
	db := openWebPushTestDB(t)
	users := []models.User{
		{Username: "old-owner", Password: "hashed"},
		{Username: "current-owner", Password: "hashed"},
	}
	if err := db.Create(&users).Error; err != nil {
		t.Fatalf("create users: %v", err)
	}
	p256dh, auth := validWebPushSubscriptionKeys(t)
	input := WebPushSubscriptionInput{
		Endpoint: "https://push.example/shared-browser", P256dh: p256dh, Auth: auth, Platform: "ios",
	}
	if _, err := UpsertWebPushSubscription(db, users[0].ID, "old-session", nil, nil, input, "old-agent"); err != nil {
		t.Fatalf("register old login: %v", err)
	}
	currentExpiry := time.Now().Add(2 * time.Hour)
	if _, err := UpsertWebPushSubscription(db, users[1].ID, "current-session", nil, &currentExpiry, input, "current-agent"); err != nil {
		t.Fatalf("register current login: %v", err)
	}

	var subscriptions []models.WebPushSubscription
	if err := db.Find(&subscriptions).Error; err != nil {
		t.Fatalf("load subscriptions: %v", err)
	}
	if len(subscriptions) != 1 {
		t.Fatalf("subscription count = %d, want one endpoint owner", len(subscriptions))
	}
	got := subscriptions[0]
	if got.UserID != users[1].ID || got.SessionID != "current-session" || got.DisabledAt != nil {
		t.Fatalf("subscription owner = user %d session %q disabled=%v", got.UserID, got.SessionID, got.DisabledAt)
	}
	if got.SessionExpiresAt == nil || !got.SessionExpiresAt.Equal(currentExpiry) {
		t.Fatalf("session expiry = %v, want %v", got.SessionExpiresAt, currentExpiry)
	}
}

func TestUpsertWebPushSubscriptionRejectsPrivateTargetsAndMalformedKeys(t *testing.T) {
	db := openWebPushTestDB(t)
	user := models.User{Username: "invalid-subscription-owner", Password: "hashed"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create invalid subscription owner: %v", err)
	}
	p256dh, auth := validWebPushSubscriptionKeys(t)
	tests := []WebPushSubscriptionInput{
		{Endpoint: "https://127.0.0.1/push", P256dh: p256dh, Auth: auth},
		{Endpoint: "https://localhost/push", P256dh: p256dh, Auth: auth},
		{Endpoint: "https://push.example/invalid", P256dh: "not-a-key", Auth: auth},
		{Endpoint: "https://push.example/invalid", P256dh: p256dh, Auth: "not-an-auth-secret"},
	}
	for _, input := range tests {
		if _, err := UpsertWebPushSubscription(db, user.ID, "invalid-session", nil, nil, input, "test"); err == nil {
			t.Fatalf("invalid subscription was accepted: %#v", input)
		}
	}
}

func TestDispatchPendingWebPushDisablesGoneSubscription(t *testing.T) {
	db := openWebPushTestDB(t)
	user := models.User{Username: "gone-recipient", Password: "hashed"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create recipient: %v", err)
	}
	subscription := models.WebPushSubscription{
		UserID: user.ID, SessionID: "gone-session", Endpoint: "https://push.example/gone",
		EndpointHash: "endpoint-gone", P256dh: "p256dh", Auth: "auth",
	}
	if err := db.Create(&subscription).Error; err != nil {
		t.Fatalf("create subscription: %v", err)
	}
	delivery := models.WebPushDelivery{
		SourceKind: models.WebPushSourceTest, SourceID: 1, SourceVersion: 1,
		SubscriptionID: subscription.ID, RecipientUserID: user.ID,
		PayloadJSON: `{"title":"test"}`, Status: models.WebPushDeliveryPending,
	}
	if err := db.Create(&delivery).Error; err != nil {
		t.Fatalf("create delivery: %v", err)
	}
	sender := &webPushSenderStub{statusCode: 410}

	result, err := DispatchPendingWebPush(context.Background(), db, sender, 10, time.Now())
	if err != nil {
		t.Fatalf("dispatch pending web push: %v", err)
	}
	if result.Invalid != 1 || sender.seen != 1 {
		t.Fatalf("dispatch result = %#v, sender calls = %d", result, sender.seen)
	}
	if err := db.First(&subscription, subscription.ID).Error; err != nil {
		t.Fatalf("reload subscription: %v", err)
	}
	if subscription.DisabledAt == nil || subscription.LastErrorCode != "gone" {
		t.Fatalf("gone subscription remained active: disabled=%v error=%q", subscription.DisabledAt, subscription.LastErrorCode)
	}
	if err := db.First(&delivery, delivery.ID).Error; err != nil {
		t.Fatalf("reload delivery: %v", err)
	}
	if delivery.Status != models.WebPushDeliveryInvalid || delivery.LastStatusCode != 410 {
		t.Fatalf("gone delivery status=%q http=%d", delivery.Status, delivery.LastStatusCode)
	}
}

func TestDispatchPendingWebPushMarksSuccessfulDeliveryAndResetsFailures(t *testing.T) {
	db := openWebPushTestDB(t)
	subscription, delivery := createWebPushDeliveryFixture(t, db, nil)
	if err := db.Model(&subscription).Updates(map[string]any{"failure_count": 3, "last_error_code": "temporary_failure"}).Error; err != nil {
		t.Fatalf("seed subscription failure state: %v", err)
	}
	now := time.Now().Truncate(time.Millisecond)
	sender := &webPushSenderStub{statusCode: 201}

	result, err := DispatchPendingWebPush(context.Background(), db, sender, 10, now)
	if err != nil {
		t.Fatalf("dispatch successful web push: %v", err)
	}
	if result.Sent != 1 || result.Processed != 1 || sender.seen != 1 {
		t.Fatalf("dispatch result = %#v, sender calls = %d", result, sender.seen)
	}
	if err := db.First(&delivery, delivery.ID).Error; err != nil {
		t.Fatalf("reload successful delivery: %v", err)
	}
	if delivery.Status != models.WebPushDeliverySent || delivery.SentAt == nil || delivery.AttemptCount != 1 {
		t.Fatalf("successful delivery = %#v", delivery)
	}
	if err := db.First(&subscription, subscription.ID).Error; err != nil {
		t.Fatalf("reload successful subscription: %v", err)
	}
	if subscription.FailureCount != 0 || subscription.LastSuccessAt == nil || subscription.LastErrorCode != "" {
		t.Fatalf("successful subscription state = %#v", subscription)
	}
}

func TestDispatchPendingWebPushRetriesTemporaryFailureWithBackoff(t *testing.T) {
	db := openWebPushTestDB(t)
	subscription, delivery := createWebPushDeliveryFixture(t, db, nil)
	now := time.Now().Truncate(time.Millisecond)
	sender := &webPushSenderStub{statusCode: 503, err: errors.New("temporary provider failure")}

	result, err := DispatchPendingWebPush(context.Background(), db, sender, 10, now)
	if err != nil {
		t.Fatalf("dispatch temporary failure: %v", err)
	}
	if result.Retried != 1 || sender.seen != 1 {
		t.Fatalf("dispatch result = %#v, sender calls = %d", result, sender.seen)
	}
	if err := db.First(&delivery, delivery.ID).Error; err != nil {
		t.Fatalf("reload retry delivery: %v", err)
	}
	if delivery.Status != models.WebPushDeliveryRetry || delivery.NextAttemptAt == nil || delivery.AttemptCount != 1 {
		t.Fatalf("retry delivery = %#v", delivery)
	}
	wantRetryAt := now.Add(time.Minute)
	if !delivery.NextAttemptAt.Equal(wantRetryAt) {
		t.Fatalf("next attempt = %v, want %v", delivery.NextAttemptAt, wantRetryAt)
	}
	if err := db.First(&subscription, subscription.ID).Error; err != nil {
		t.Fatalf("reload retry subscription: %v", err)
	}
	if subscription.FailureCount != 1 || subscription.LastErrorCode != "temporary_failure" {
		t.Fatalf("retry subscription state = %#v", subscription)
	}
}

func TestDispatchPendingWebPushSkipsExpiredLoginSession(t *testing.T) {
	db := openWebPushTestDB(t)
	expiredAt := time.Now().Add(-time.Minute)
	_, delivery := createWebPushDeliveryFixture(t, db, &expiredAt)
	sender := &webPushSenderStub{statusCode: 200}

	result, err := DispatchPendingWebPush(context.Background(), db, sender, 10, time.Now())
	if err != nil {
		t.Fatalf("dispatch expired session: %v", err)
	}
	if result.Skipped != 1 || sender.seen != 0 {
		t.Fatalf("expired dispatch result = %#v, sender calls = %d", result, sender.seen)
	}
	if err := db.First(&delivery, delivery.ID).Error; err != nil {
		t.Fatalf("reload skipped delivery: %v", err)
	}
	if delivery.Status != models.WebPushDeliverySkipped || delivery.LastError != "subscription_inactive" {
		t.Fatalf("expired delivery = %#v", delivery)
	}
}

func TestDispatchPendingWebPushAppliesCurrentSessionExpiryPolicy(t *testing.T) {
	db := openWebPushTestDB(t)
	database.DB = db
	t.Cleanup(func() { database.DB = nil })
	if err := db.Create(&models.User{ID: models.PrimaryAdminUserID, Username: "policy-primary", Password: "hashed", IsAdmin: true}).Error; err != nil {
		t.Fatalf("create policy primary: %v", err)
	}
	config := models.SiteConfig{}
	if err := db.Create(&config).Error; err != nil {
		t.Fatalf("create login expiry policy: %v", err)
	}
	if err := db.Model(&config).Updates(map[string]any{"login_expire_days": 0, "login_expire_hours": 1}).Error; err != nil {
		t.Fatalf("update login expiry policy: %v", err)
	}
	issuedAt := time.Now().Add(-2 * time.Hour)
	storedExpiry := time.Now().Add(24 * time.Hour)
	subscription, delivery := createWebPushDeliveryFixture(t, db, &storedExpiry)
	if err := db.Model(&subscription).Update("session_issued_at", issuedAt).Error; err != nil {
		t.Fatalf("set subscription issued at: %v", err)
	}
	sender := &webPushSenderStub{statusCode: 200}

	result, err := DispatchPendingWebPush(context.Background(), db, sender, 10, time.Now())
	if err != nil {
		t.Fatalf("dispatch policy-expired session: %v", err)
	}
	if result.Skipped != 1 || sender.seen != 0 {
		t.Fatalf("policy-expired result = %#v, sender calls = %d", result, sender.seen)
	}
	if err := db.First(&delivery, delivery.ID).Error; err != nil {
		t.Fatalf("reload policy-expired delivery: %v", err)
	}
	if delivery.Status != models.WebPushDeliverySkipped || delivery.LastError != "subscription_inactive" {
		t.Fatalf("policy-expired delivery = %#v", delivery)
	}
}

func TestDispatchPendingWebPushNeverSendsAfterRecipientDeletion(t *testing.T) {
	db := openWebPushTestDB(t)
	subscription, delivery := createWebPushDeliveryFixture(t, db, nil)
	if err := db.Delete(&models.User{}, subscription.UserID).Error; err != nil {
		t.Fatalf("delete push recipient: %v", err)
	}
	sender := &webPushSenderStub{statusCode: 200}

	result, err := DispatchPendingWebPush(context.Background(), db, sender, 10, time.Now())
	if err != nil {
		t.Fatalf("dispatch deleted recipient: %v", err)
	}
	if result.Skipped != 1 || sender.seen != 0 {
		t.Fatalf("deleted recipient result = %#v, sender calls = %d", result, sender.seen)
	}
	if err := db.First(&delivery, delivery.ID).Error; err != nil {
		t.Fatalf("reload deleted-recipient delivery: %v", err)
	}
	if delivery.Status != models.WebPushDeliverySkipped || delivery.LastError != "recipient_missing" {
		t.Fatalf("deleted-recipient delivery = %#v", delivery)
	}
}

func TestDispatchPendingWebPushHonorsPreferenceChangedAfterQueueing(t *testing.T) {
	db := openWebPushTestDB(t)
	user := models.User{Username: "preference-change-recipient", Password: "hashed"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create preference-change recipient: %v", err)
	}
	subscription := models.WebPushSubscription{
		UserID: user.ID, SessionID: "preference-session", Endpoint: "https://push.example/preference",
		EndpointHash: "preference-endpoint", P256dh: "p256dh", Auth: "auth",
	}
	if err := db.Create(&subscription).Error; err != nil {
		t.Fatalf("create preference-change subscription: %v", err)
	}
	notification := models.UserNotification{RecipientUserID: user.ID, Type: models.UserNotificationTypeComment}
	if err := db.Create(&notification).Error; err != nil {
		t.Fatalf("create preference-change notification: %v", err)
	}
	if err := QueueWebPushForNotification(db, notification); err != nil {
		t.Fatalf("queue preference-change notification: %v", err)
	}
	preference, err := loadWebPushPreference(db, user.ID)
	if err != nil {
		t.Fatalf("load preference: %v", err)
	}
	if err := db.Model(&preference).Update("comment_enabled", false).Error; err != nil {
		t.Fatalf("disable comment push after queueing: %v", err)
	}
	sender := &webPushSenderStub{statusCode: 200}

	result, err := DispatchPendingWebPush(context.Background(), db, sender, 10, time.Now())
	if err != nil {
		t.Fatalf("dispatch preference-changed delivery: %v", err)
	}
	if result.Skipped != 1 || sender.seen != 0 {
		t.Fatalf("preference-changed result = %#v, sender calls = %d", result, sender.seen)
	}
}

func TestQueueWebPushTestTargetsAllLoginsAndRateLimitsTheAccount(t *testing.T) {
	db := openWebPushTestDB(t)
	user := models.User{Username: "test-push-recipient", Password: "hashed"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create test recipient: %v", err)
	}
	for index := 0; index < 2; index++ {
		subscription := models.WebPushSubscription{
			UserID: user.ID, SessionID: string(rune('a' + index)), Endpoint: "https://push.example/test-" + string(rune('a'+index)),
			EndpointHash: "test-endpoint-" + string(rune('a'+index)), P256dh: "p256dh", Auth: "auth",
		}
		if err := db.Create(&subscription).Error; err != nil {
			t.Fatalf("create test subscription: %v", err)
		}
	}
	now := time.Now().Truncate(time.Millisecond)
	if err := QueueWebPushTest(db, user.ID, now); err != nil {
		t.Fatalf("queue first test notification: %v", err)
	}
	var count int64
	if err := db.Model(&models.WebPushDelivery{}).Where("source_kind = ?", models.WebPushSourceTest).Count(&count).Error; err != nil {
		t.Fatalf("count test deliveries: %v", err)
	}
	if count != 2 {
		t.Fatalf("test delivery count = %d, want 2", count)
	}
	if err := QueueWebPushTest(db, user.ID, now.Add(30*time.Second)); !errors.Is(err, ErrWebPushTestRateLimited) {
		t.Fatalf("second test error = %v, want rate limit", err)
	}
}

func TestQueueWebPushForAnnouncementHonorsAccountPreference(t *testing.T) {
	db := openWebPushTestDB(t)
	users := []models.User{{Username: "announcement-on", Password: "hashed"}, {Username: "announcement-off", Password: "hashed"}}
	if err := db.Create(&users).Error; err != nil {
		t.Fatalf("create announcement recipients: %v", err)
	}
	for index, user := range users {
		subscription := models.WebPushSubscription{
			UserID: user.ID, SessionID: "announcement-session", Endpoint: "https://push.example/announcement-" + string(rune('a'+index)),
			EndpointHash: "announcement-endpoint-" + string(rune('a'+index)), P256dh: "p256dh", Auth: "auth",
		}
		if err := db.Create(&subscription).Error; err != nil {
			t.Fatalf("create announcement subscription: %v", err)
		}
	}
	preference := defaultWebPushPreference(users[1].ID)
	if err := db.Create(&preference).Error; err != nil {
		t.Fatalf("create disabled announcement preference: %v", err)
	}
	if err := db.Model(&preference).Update("announcement_enabled", false).Error; err != nil {
		t.Fatalf("disable announcement preference: %v", err)
	}
	announcement := models.Announcement{Title: "维护通知", Content: "今晚维护", Status: models.AnnouncementStatusPublished, PushEnabled: true}
	if err := db.Create(&announcement).Error; err != nil {
		t.Fatalf("create announcement: %v", err)
	}

	if err := QueueWebPushForAnnouncement(db, announcement); err != nil {
		t.Fatalf("queue announcement push: %v", err)
	}
	var deliveries []models.WebPushDelivery
	if err := db.Where("source_kind = ?", models.WebPushSourceAnnouncement).Find(&deliveries).Error; err != nil {
		t.Fatalf("load announcement deliveries: %v", err)
	}
	if len(deliveries) != 1 || deliveries[0].RecipientUserID != users[0].ID {
		t.Fatalf("announcement deliveries = %#v", deliveries)
	}
}

func TestPruneWebPushDeliveriesKeepsPendingAndRecentRows(t *testing.T) {
	db := openWebPushTestDB(t)
	now := time.Now().Truncate(time.Millisecond)
	rows := []models.WebPushDelivery{
		{SourceKind: models.WebPushSourceTest, SourceID: 1, SourceVersion: 1, SubscriptionID: 1, RecipientUserID: 1, PayloadJSON: `{}`, Status: models.WebPushDeliverySent, UpdatedAt: now.Add(-31 * 24 * time.Hour)},
		{SourceKind: models.WebPushSourceTest, SourceID: 2, SourceVersion: 2, SubscriptionID: 2, RecipientUserID: 1, PayloadJSON: `{}`, Status: models.WebPushDeliveryPending, UpdatedAt: now.Add(-31 * 24 * time.Hour)},
		{SourceKind: models.WebPushSourceTest, SourceID: 3, SourceVersion: 3, SubscriptionID: 3, RecipientUserID: 1, PayloadJSON: `{}`, Status: models.WebPushDeliveryFailed, UpdatedAt: now.Add(-time.Hour)},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatalf("create pruning fixtures: %v", err)
	}
	pruned, err := PruneWebPushDeliveries(db, now)
	if err != nil {
		t.Fatalf("prune web push deliveries: %v", err)
	}
	if pruned != 1 {
		t.Fatalf("pruned rows = %d, want 1", pruned)
	}
	var remaining int64
	if err := db.Model(&models.WebPushDelivery{}).Count(&remaining).Error; err != nil {
		t.Fatalf("count remaining deliveries: %v", err)
	}
	if remaining != 2 {
		t.Fatalf("remaining delivery rows = %d, want 2", remaining)
	}
}

func TestLoadWebPushRuntimeConfigPrefersMountedPrivateKeyFile(t *testing.T) {
	privateKey, publicKey, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		t.Fatalf("generate VAPID config fixture: %v", err)
	}
	privateKeyPath := filepath.Join(t.TempDir(), "vapid-private-key")
	if err := os.WriteFile(privateKeyPath, []byte("  "+privateKey+"\n"), 0o600); err != nil {
		t.Fatalf("write private key file: %v", err)
	}
	t.Setenv("WEB_PUSH_VAPID_PUBLIC_KEY", publicKey)
	t.Setenv("WEB_PUSH_VAPID_PRIVATE_KEY", "private-from-env")
	t.Setenv("WEB_PUSH_VAPID_PRIVATE_KEY_FILE", privateKeyPath)
	t.Setenv("WEB_PUSH_VAPID_SUBJECT", "mailto:owner@example.com")

	config := LoadWebPushRuntimeConfig()
	if !config.Ready() {
		t.Fatal("web push config should be ready")
	}
	if config.PrivateKey != privateKey {
		t.Fatalf("private key source = %q, want mounted file", config.PrivateKey)
	}
	if config.PublicKey != publicKey || config.Subject != "mailto:owner@example.com" {
		t.Fatalf("runtime config = %#v", config)
	}
}

func TestLoadWebPushRuntimeConfigRejectsMalformedKeysAndSubject(t *testing.T) {
	t.Setenv("WEB_PUSH_VAPID_PUBLIC_KEY", "not-a-public-key")
	t.Setenv("WEB_PUSH_VAPID_PRIVATE_KEY", "not-a-private-key")
	t.Setenv("WEB_PUSH_VAPID_PRIVATE_KEY_FILE", "")
	t.Setenv("WEB_PUSH_VAPID_SUBJECT", "javascript:invalid")

	config := LoadWebPushRuntimeConfig()
	if config.Ready() || config.LoadError == nil {
		t.Fatalf("malformed VAPID config was accepted: %#v", config)
	}
}

func TestLoadWebPushRuntimeConfigRejectsMismatchedKeyPair(t *testing.T) {
	privateKey, _, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		t.Fatalf("generate first VAPID key pair: %v", err)
	}
	_, unrelatedPublicKey, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		t.Fatalf("generate second VAPID key pair: %v", err)
	}
	t.Setenv("WEB_PUSH_VAPID_PUBLIC_KEY", unrelatedPublicKey)
	t.Setenv("WEB_PUSH_VAPID_PRIVATE_KEY", privateKey)
	t.Setenv("WEB_PUSH_VAPID_PRIVATE_KEY_FILE", "")
	t.Setenv("WEB_PUSH_VAPID_SUBJECT", "https://example.com/")

	config := LoadWebPushRuntimeConfig()
	if config.Ready() || config.LoadError == nil {
		t.Fatalf("mismatched VAPID key pair was accepted: %#v", config)
	}
}
