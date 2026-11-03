package services

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/vocechat"
	"gorm.io/gorm"
)

func createSubscriptionChannelFixture(t *testing.T, db *gorm.DB) (models.UserNotification, models.Message) {
	t.Helper()
	if err := db.AutoMigrate(&models.AuthorSubscription{}); err != nil {
		t.Fatal(err)
	}
	author := models.User{Username: "channel-author", Password: "hashed"}
	recipient := models.User{Username: "channel-recipient", Password: "hashed", VoceChatUserID: "902", VoceChatEmail: "recipient@vc.example", VoceChatNotificationEnabled: true}
	for _, user := range []*models.User{&author, &recipient} {
		if err := db.Create(user).Error; err != nil {
			t.Fatal(err)
		}
	}
	publishedAt := time.Now().UTC()
	subscription := models.AuthorSubscription{SubscriberUserID: recipient.ID, AuthorUserID: author.ID, CreatedAt: publishedAt.Add(-time.Hour)}
	if err := db.Create(&subscription).Error; err != nil {
		t.Fatal(err)
	}
	message := models.Message{UserID: author.ID, Username: author.Username, Content: "原始笔记正文", Visibility: MessageVisibilityPublic}
	if err := db.Create(&message).Error; err != nil {
		t.Fatal(err)
	}
	notification := models.UserNotification{RecipientUserID: recipient.ID, ActorUserID: &author.ID, MessageID: &message.ID, AuthorSubscriptionID: &subscription.ID, Type: models.UserNotificationTypeAuthorSubscription, CreatedAt: publishedAt, UpdatedAt: publishedAt}
	if err := db.Create(&notification).Error; err != nil {
		t.Fatal(err)
	}
	return notification, message
}

func TestSubscriptionWebPushPreferenceDefaultsAndLegacyUpdates(t *testing.T) {
	db := openWebPushTestDB(t)
	preference, err := GetWebPushPreference(db, 42)
	if err != nil || !preference.SubscriptionEnabled {
		t.Fatalf("default subscription preference = %#v, err = %v", preference, err)
	}
	legacy := WebPushPreferenceInput{Enabled: true, CommentEnabled: true}
	preference, err = UpdateWebPushPreference(db, 42, legacy)
	if err != nil || !preference.SubscriptionEnabled {
		t.Fatalf("legacy update cleared default subscription preference: %#v, %v", preference, err)
	}
	off := false
	legacy.SubscriptionEnabled = &off
	preference, err = UpdateWebPushPreference(db, 42, legacy)
	if err != nil || preference.SubscriptionEnabled || webPushPreferenceAllows(preference, models.UserNotificationTypeAuthorSubscription) {
		t.Fatalf("explicit subscription opt-out = %#v, err = %v", preference, err)
	}
	legacy.SubscriptionEnabled = nil
	preference, err = UpdateWebPushPreference(db, 42, legacy)
	if err != nil || preference.SubscriptionEnabled {
		t.Fatalf("legacy update reset explicit opt-out: %#v, %v", preference, err)
	}
}

func TestSubscriptionWebPushPreferenceMigrationPreservesLegacyRowsAndOptOut(t *testing.T) {
	db := openWebPushTestDB(t)
	preference := defaultWebPushPreference(42)
	if err := db.Create(&preference).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&preference).Updates(map[string]interface{}{"enabled": false, "comment_enabled": false, "show_preview": true}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Migrator().DropColumn(&models.WebPushPreference{}, "SubscriptionEnabled"); err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.WebPushPreference{}); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&preference, "user_id = ?", 42).Error; err != nil {
		t.Fatal(err)
	}
	if !preference.SubscriptionEnabled || preference.Enabled || preference.CommentEnabled || !preference.ShowPreview {
		t.Fatalf("legacy preference migration changed existing choices: %#v", preference)
	}
	if err := db.Model(&preference).Update("subscription_enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.WebPushPreference{}); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&preference, "user_id = ?", 42).Error; err != nil || preference.SubscriptionEnabled {
		t.Fatalf("repeat migration lost explicit opt-out: %#v, %v", preference, err)
	}
}

type subscriptionPayloadSender struct {
	payloads []webPushPayload
}

func (sender *subscriptionPayloadSender) Send(_ context.Context, _ models.WebPushSubscription, payload []byte) (WebPushSendResult, error) {
	var decoded webPushPayload
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return WebPushSendResult{}, err
	}
	sender.payloads = append(sender.payloads, decoded)
	return WebPushSendResult{StatusCode: http.StatusCreated}, nil
}

func queueSubscriptionChannelDelivery(t *testing.T, db *gorm.DB, notification models.UserNotification) models.WebPushDelivery {
	t.Helper()
	subscription := models.WebPushSubscription{UserID: notification.RecipientUserID, SessionID: "channel-session", Endpoint: "https://push.example/channel", EndpointHash: "channel-endpoint", P256dh: "p256dh", Auth: "auth"}
	if err := db.Create(&subscription).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := GetWebPushPreference(db, notification.RecipientUserID); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&models.WebPushPreference{}).Where("user_id = ?", notification.RecipientUserID).Update("show_preview", true).Error; err != nil {
		t.Fatal(err)
	}
	if err := QueueWebPushForNotification(db, notification); err != nil {
		t.Fatal(err)
	}
	var delivery models.WebPushDelivery
	if err := db.First(&delivery).Error; err != nil {
		t.Fatal(err)
	}
	return delivery
}

func TestQueuedSubscriptionWebPushRechecksEligibility(t *testing.T) {
	for _, change := range []string{"unsubscribe", "resubscribe", "private", "contacts_revoked", "deleted", "tombstone", "source_missing", "author_missing", "recipient_missing", "notification_missing", "notification_recipient", "endpoint_reassigned", "preference_off"} {
		t.Run(change, func(t *testing.T) {
			db := openWebPushTestDB(t)
			originalDB := database.DB
			database.DB = db
			t.Cleanup(func() { database.DB = originalDB })
			notification, message := createSubscriptionChannelFixture(t, db)
			delivery := queueSubscriptionChannelDelivery(t, db, notification)
			var err error
			switch change {
			case "unsubscribe", "resubscribe":
				err = db.Where("subscriber_user_id = ?", notification.RecipientUserID).Delete(&models.AuthorSubscription{}).Error
				if err == nil && change == "resubscribe" {
					err = db.Create(&models.AuthorSubscription{SubscriberUserID: notification.RecipientUserID, AuthorUserID: message.UserID, CreatedAt: notification.CreatedAt.Add(time.Second)}).Error
				}
			case "private":
				err = db.Model(&message).Updates(map[string]interface{}{"visibility": MessageVisibilityPrivate, "private": true}).Error
			case "contacts_revoked":
				err = db.Model(&message).Update("visibility", MessageVisibilityContacts).Error
			case "deleted":
				err = db.Model(&message).Update("deleted_at", time.Now()).Error
			case "tombstone":
				err = db.Model(&message).Update("is_tombstone", true).Error
			case "source_missing":
				err = db.Delete(&message).Error
			case "author_missing":
				err = db.Delete(&models.User{}, message.UserID).Error
			case "recipient_missing":
				err = db.Delete(&models.User{}, notification.RecipientUserID).Error
			case "notification_missing":
				err = db.Delete(&notification).Error
			case "notification_recipient":
				err = db.Model(&notification).Update("recipient_user_id", message.UserID).Error
			case "endpoint_reassigned":
				err = db.Model(&models.WebPushSubscription{}).Where("id = ?", delivery.SubscriptionID).Update("user_id", message.UserID).Error
			case "preference_off":
				err = db.Model(&models.WebPushPreference{}).Where("user_id = ?", notification.RecipientUserID).Update("subscription_enabled", false).Error
			}
			if err != nil {
				t.Fatal(err)
			}
			sender := &subscriptionPayloadSender{}
			result, err := DispatchPendingWebPush(context.Background(), db, sender, 10, time.Now())
			if err != nil || result.Skipped != 1 || len(sender.payloads) != 0 {
				t.Fatalf("revoked queued delivery: result=%#v sends=%d err=%v", result, len(sender.payloads), err)
			}
			if err := db.First(&delivery, delivery.ID).Error; err != nil || delivery.Status != models.WebPushDeliverySkipped {
				t.Fatalf("delivery was not permanently skipped: %#v, %v", delivery, err)
			}
		})
	}
}

func TestQueuedSubscriptionWebPushRefreshesCurrentPreviewAndPrivacy(t *testing.T) {
	for _, showPreview := range []bool{true, false} {
		t.Run(map[bool]string{true: "current_content", false: "preview_disabled"}[showPreview], func(t *testing.T) {
			db := openWebPushTestDB(t)
			notification, message := createSubscriptionChannelFixture(t, db)
			queueSubscriptionChannelDelivery(t, db, notification)
			if err := db.Model(&message).Update("content", "新的  笔记正文\n第二行").Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Model(&models.WebPushPreference{}).Where("user_id = ?", notification.RecipientUserID).Update("show_preview", showPreview).Error; err != nil {
				t.Fatal(err)
			}
			sender := &subscriptionPayloadSender{}
			result, err := DispatchPendingWebPush(context.Background(), db, sender, 10, time.Now())
			if err != nil || result.Sent != 1 || len(sender.payloads) != 1 {
				t.Fatalf("dispatch current preview: result=%#v sends=%d err=%v", result, len(sender.payloads), err)
			}
			payload := sender.payloads[0]
			if showPreview {
				if payload.Title != "channel-author 发布了新笔记" || payload.Body != "新的 笔记正文 第二行" {
					t.Fatalf("current subscription preview = %#v", payload)
				}
			} else if payload.Body != "你有一条新通知" || strings.Contains(payload.Title, "channel-author") {
				t.Fatalf("disabled preview leaked queued content: %#v", payload)
			}
		})
	}
}

func TestSubscriptionVoceChatTitleAndEligibility(t *testing.T) {
	for _, change := range []string{"allowed", "unsubscribe", "private", "deleted"} {
		t.Run(change, func(t *testing.T) {
			db := setupUserServiceTestDB(t)
			notification, message := createSubscriptionChannelFixture(t, db)
			var markdown string
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
				if request.URL.Path != "/api/bot/send_to_user/902" {
					t.Errorf("unexpected VoceChat target: %s", request.URL.Path)
				}
				body, _ := io.ReadAll(request.Body)
				markdown = string(body)
				response.WriteHeader(http.StatusOK)
			}))
			defer server.Close()
			config := createRuntimeContactPushConfigForTest(t, models.RuntimeModeVoceChat, "ok")
			if err := db.Model(config).Update("voce_chat_base_url", server.URL).Error; err != nil {
				t.Fatal(err)
			}
			var err error
			switch change {
			case "unsubscribe":
				err = db.Where("subscriber_user_id = ?", notification.RecipientUserID).Delete(&models.AuthorSubscription{}).Error
			case "private":
				err = db.Model(&message).Updates(map[string]interface{}{"visibility": MessageVisibilityPrivate, "private": true}).Error
			case "deleted":
				err = db.Model(&message).Update("deleted_at", time.Now()).Error
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := sendUserNotificationToVoceChat(context.Background(), notification.ID); err != nil {
				t.Fatal(err)
			}
			if change == "allowed" {
				if !strings.Contains(markdown, "channel-author 发布了新笔记") || !strings.Contains(markdown, "> 原始笔记正文") {
					t.Fatalf("subscription markdown = %q", markdown)
				}
			} else if markdown != "" {
				t.Fatalf("ineligible subscription sent markdown: %q", markdown)
			}
		})
	}
}

func TestSubscriptionVoceChatRechecksAfterRemoteRecipientResolution(t *testing.T) {
	for _, change := range []string{"allowed", "unsubscribe", "private", "deleted", "notification_recipient", "recipient_binding", "recipient_disabled", "edited"} {
		t.Run(change, func(t *testing.T) {
			db := setupUserServiceTestDB(t)
			// The primary administrator resolves the personal VoceChat binding remotely.
			recipient := models.User{Username: "personal-recipient", Password: "hashed", VoceChatUserID: "901", VoceChatEmail: "personal@vc.example", VoceChatNotificationEnabled: true}
			if err := db.Create(&recipient).Error; err != nil {
				t.Fatal(err)
			}
			notification, message := createSubscriptionChannelFixture(t, db)
			if err := db.Model(&notification).Update("recipient_user_id", recipient.ID).Error; err != nil {
				t.Fatal(err)
			}
			subscription := models.AuthorSubscription{SubscriberUserID: recipient.ID, AuthorUserID: message.UserID, CreatedAt: notification.CreatedAt.Add(-time.Hour)}
			if err := db.Create(&subscription).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Model(&notification).Update("author_subscription_id", subscription.ID).Error; err != nil {
				t.Fatal(err)
			}
			storePath := filepath.Join(t.TempDir(), "channel-passwords.db")
			t.Setenv("NOISE_PLAIN_PASSWORD_STORE", storePath)
			if err := vocechat.NewPlainPasswordStore(storePath).UpsertUserVoceChatPassword(recipient.ID, recipient.Username, "test-secret", recipient.VoceChatEmail, recipient.VoceChatUserID); err != nil {
				t.Fatal(err)
			}
			var markdown string
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
				if request.URL.Path != "/api/bot/send_to_user/901" {
					t.Errorf("unexpected resolved recipient: %s", request.URL.Path)
				}
				body, _ := io.ReadAll(request.Body)
				markdown = string(body)
				response.WriteHeader(http.StatusOK)
			}))
			defer server.Close()
			config := createRuntimeContactPushConfigForTest(t, models.RuntimeModeVoceChat, "ok")
			if err := db.Model(config).Update("voce_chat_base_url", server.URL).Error; err != nil {
				t.Fatal(err)
			}
			resolved := false
			stubVoceChatPasswordLogin(t, func(_ context.Context, _ vocechat.Config, email, _ string) (*vocechat.LoginResponse, error) {
				resolved = true
				if email != recipient.VoceChatEmail {
					t.Fatalf("recipient select omitted VoceChat email: %q", email)
				}
				var err error
				switch change {
				case "unsubscribe":
					err = db.Where("subscriber_user_id = ?", recipient.ID).Delete(&models.AuthorSubscription{}).Error
				case "private":
					err = db.Model(&message).Updates(map[string]interface{}{"visibility": MessageVisibilityPrivate, "private": true}).Error
				case "deleted":
					err = db.Model(&message).Update("deleted_at", time.Now()).Error
				case "notification_recipient":
					err = db.Model(&notification).Update("recipient_user_id", message.UserID).Error
				case "recipient_binding":
					err = db.Model(&recipient).Update("voce_chat_user_id", "999").Error
				case "recipient_disabled":
					err = db.Model(&recipient).Update("voce_chat_notification_enabled", false).Error
				case "edited":
					err = db.Model(&message).Update("content", "远程解析期间更新的正文").Error
				}
				if err != nil {
					t.Fatal(err)
				}
				return &vocechat.LoginResponse{User: vocechat.UserInfo{UID: 901, Email: email}}, nil
			})
			if err := sendUserNotificationToVoceChat(context.Background(), notification.ID); err != nil {
				t.Fatal(err)
			}
			if !resolved {
				t.Fatal("remote recipient resolution was not reached")
			}
			if change == "allowed" || change == "edited" {
				wantBody := "原始笔记正文"
				if change == "edited" {
					wantBody = "远程解析期间更新的正文"
				}
				if !strings.Contains(markdown, "channel-author 发布了新笔记") || !strings.Contains(markdown, wantBody) {
					t.Fatalf("resolved subscription markdown = %q", markdown)
				}
			} else if markdown != "" {
				t.Fatalf("revoked during remote resolution sent markdown: %q", markdown)
			}
		})
	}
}
