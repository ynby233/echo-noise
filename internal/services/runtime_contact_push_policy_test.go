package services

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/runtimepolicy"
	"github.com/rcy1314/echo-noise/internal/vocechat"
)

func createRuntimeContactPushConfigForTest(t *testing.T, mode string, health string) *models.SiteConfig {
	t.Helper()
	config := &models.SiteConfig{
		RuntimeMode:                     mode,
		RuntimeModeMigrationVersion:     models.RuntimeModeMigrationVersionCurrent,
		VoceChatEnabled:                 mode == models.RuntimeModeVoceChat,
		VoceChatBaseURL:                 "https://vc.example.test",
		VoceChatAdminToken:              "configured-token",
		VoceChatBotAPIKey:               "configured-bot-key",
		VoceChatEmailDomain:             "vc.example",
		VoceChatContactsCacheTTLSeconds: 3600,
		VoceChatContactsEnabled:         true,
		VoceChatNotificationEnabled:     true,
		VoceChatLastHealthStatus:        health,
	}
	if err := database.DB.Create(config).Error; err != nil {
		t.Fatalf("create runtime contact/push config: %v", err)
	}
	return config
}

func seedFreshRuntimeContact(t *testing.T, author, viewer *models.User) {
	t.Helper()
	now := time.Now().UTC()
	rows := []models.VoceChatContactCache{
		{
			UserID: author.ID, ContactUserID: 0, VoceChatUserID: author.VoceChatUserID,
			Source: "vocechat", SyncedAt: now, ExpiresAt: now.Add(time.Hour), LastSyncStatus: models.VoceChatContactSyncStatusOK,
		},
		{
			UserID: author.ID, ContactUserID: viewer.ID,
			VoceChatUserID: author.VoceChatUserID, ContactVoceChatID: viewer.VoceChatUserID,
			Source: "vocechat", SyncedAt: now, ExpiresAt: now.Add(time.Hour), LastSyncStatus: models.VoceChatContactSyncStatusOK,
		},
	}
	if err := database.DB.Create(&rows).Error; err != nil {
		t.Fatalf("seed fresh contact: %v", err)
	}
}

func TestRuntimeContactVisibilityRequiresNormalModeAndLinkedAuthorAcrossReadSurfaces(t *testing.T) {
	db := setupUserServiceTestDB(t)
	config := createRuntimeContactPushConfigForTest(t, models.RuntimeModeVoceChat, "ok")
	storePath := filepath.Join(t.TempDir(), "plain-passwords.db")
	t.Setenv("NOISE_PLAIN_PASSWORD_STORE", storePath)
	mustCreateUser(t, models.User{Username: "primary-contact-runtime", Password: models.HashPassword("primary"), IsAdmin: true})
	author := mustCreateUser(t, models.User{
		Username: "runtime-contact-author", Password: models.HashPassword("author"),
		VoceChatEmail: "author@vc.example", VoceChatUserID: "701", VoceChatSyncStatus: models.VoceChatSyncStatusLinked,
	})
	viewer := mustCreateUser(t, models.User{
		Username: "runtime-contact-viewer", Password: models.HashPassword("viewer"),
		VoceChatEmail: "viewer@vc.example", VoceChatUserID: "702", VoceChatSyncStatus: models.VoceChatSyncStatusLinked,
	})
	if err := vocechat.DefaultPlainPasswordStore().UpsertUserVoceChatPassword(author.ID, author.Username, "author-vc-password", author.VoceChatEmail, author.VoceChatUserID); err != nil {
		t.Fatalf("store author password: %v", err)
	}
	seedFreshRuntimeContact(t, author, viewer)
	message := models.Message{
		Content: "runtime contact surface #runtimecontact", Username: author.Username, UserID: author.ID,
		Visibility: MessageVisibilityContacts,
	}
	if err := ApplyMessageVisibilityForSave(&message); err != nil {
		t.Fatalf("normalize contacts message: %v", err)
	}
	if err := db.Create(&message).Error; err != nil {
		t.Fatalf("create contacts message: %v", err)
	}
	viewerID := viewer.ID
	assertSurfaces := func(wantVisible bool) {
		t.Helper()
		all, err := GetAllMessagesForViewer(&viewerID, false)
		if err != nil {
			t.Fatalf("list messages: %v", err)
		}
		if got := len(all) == 1; got != wantVisible {
			t.Fatalf("list visibility = %v, want %v; rows=%#v", got, wantVisible, all)
		}
		_, detailErr := GetMessageByIDForViewer(message.ID, &viewerID, false)
		if got := detailErr == nil; got != wantVisible {
			t.Fatalf("detail visibility = %v, want %v; err=%v", got, wantVisible, detailErr)
		}
		search, err := SearchMessages("runtime contact surface", 1, 10, &viewerID, false, nil, nil)
		if err != nil {
			t.Fatalf("search messages: %v", err)
		}
		if got := search.Total == 1; got != wantVisible {
			t.Fatalf("search visibility = %v, want %v; total=%d", got, wantVisible, search.Total)
		}
		keyword, tag := "runtime contact", "runtimecontact"
		page, err := GetMessagesByPage(1, 10, &viewerID, false, nil, nil, nil, &keyword, &tag, MessagePinScopeLatest, nil)
		if err != nil {
			t.Fatalf("tag page: %v", err)
		}
		if got := page.Total == 1; got != wantVisible {
			t.Fatalf("tag visibility = %v, want %v; total=%d", got, wantVisible, page.Total)
		}
		calendar, err := GetMessagesGroupByDate(&viewerID, false, nil)
		if err != nil {
			t.Fatalf("calendar messages: %v", err)
		}
		if got := len(calendar) == 1 && calendar[0].Count == 1; got != wantVisible {
			t.Fatalf("calendar visibility = %v, want %v; rows=%#v", got, wantVisible, calendar)
		}
	}

	externalCalls := 0
	stubVoceChatPasswordLogin(t, func(context.Context, vocechat.Config, string, string) (*vocechat.LoginResponse, error) {
		externalCalls++
		return &vocechat.LoginResponse{Token: "author-contact-token", User: vocechat.UserInfo{UID: 701, Email: author.VoceChatEmail}}, nil
	})
	stubVoceChatListContacts(t, func(context.Context, vocechat.Config, string) ([]vocechat.UserContact, error) {
		externalCalls++
		return []vocechat.UserContact{voceChatContactWithStatus(702, vocechat.ContactStatusAdded)}, nil
	})

	assertSurfaces(true)
	if externalCalls != 0 {
		t.Fatalf("fresh normal cache triggered external calls: %d", externalCalls)
	}
	if err := db.Model(&models.SiteConfig{}).Where("id = ?", config.ID).Updates(map[string]interface{}{
		"runtime_mode": models.RuntimeModeLocal, "voce_chat_enabled": false,
	}).Error; err != nil {
		t.Fatalf("switch test config local: %v", err)
	}
	assertSurfaces(false)
	if externalCalls != 0 {
		t.Fatalf("local mode called VoceChat contacts: %d", externalCalls)
	}
	if err := db.Model(&models.SiteConfig{}).Where("id = ?", config.ID).Updates(map[string]interface{}{
		"runtime_mode": models.RuntimeModeVoceChat, "voce_chat_enabled": true, "voce_chat_last_health_status": "failed",
	}).Error; err != nil {
		t.Fatalf("degrade test config: %v", err)
	}
	assertSurfaces(false)
	if externalCalls != 0 {
		t.Fatalf("degraded mode called VoceChat contacts: %d", externalCalls)
	}
	if err := db.Model(&models.SiteConfig{}).Where("id = ?", config.ID).Update("voce_chat_last_health_status", "ok").Error; err != nil {
		t.Fatalf("recover test config: %v", err)
	}
	for _, status := range []string{models.VoceChatSyncStatusPending, models.VoceChatSyncStatusCredentialInvalid} {
		if err := db.Model(&models.User{}).Where("id = ?", author.ID).Update("voce_chat_sync_status", status).Error; err != nil {
			t.Fatalf("set author status %s: %v", status, err)
		}
		assertSurfaces(false)
	}
	if externalCalls != 0 {
		t.Fatalf("ineligible author called VoceChat contacts: %d", externalCalls)
	}
	if err := db.Model(&models.User{}).Where("id = ?", author.ID).Update("voce_chat_sync_status", models.VoceChatSyncStatusLinked).Error; err != nil {
		t.Fatalf("restore linked author: %v", err)
	}
	if err := db.Where("user_id = ?", author.ID).Delete(&models.VoceChatContactCache{}).Error; err != nil {
		t.Fatalf("clear cache before recovery sync: %v", err)
	}
	assertSurfaces(true)
	if externalCalls != 2 {
		t.Fatalf("recovery did not perform one login and one contact sync: calls=%d", externalCalls)
	}
	var stored models.Message
	if err := db.First(&stored, message.ID).Error; err != nil {
		t.Fatalf("reload contacts message: %v", err)
	}
	if stored.Visibility != MessageVisibilityContacts || !stored.Private {
		t.Fatalf("runtime fallback mutated stored visibility: %#v", stored)
	}
}

func TestRuntimeContactVisibilityFlowsThroughCommentsRepliesAndAttachments(t *testing.T) {
	db := setupUserServiceTestDB(t)
	config := createRuntimeContactPushConfigForTest(t, models.RuntimeModeVoceChat, "ok")
	mustCreateUser(t, models.User{Username: "primary-contact-inheritance", Password: models.HashPassword("primary"), IsAdmin: true})
	author := mustCreateUser(t, models.User{Username: "inherit-author", Password: models.HashPassword("author"), VoceChatEmail: "inherit-author@vc.example", VoceChatUserID: "711", VoceChatSyncStatus: models.VoceChatSyncStatusLinked})
	viewer := mustCreateUser(t, models.User{Username: "inherit-viewer", Password: models.HashPassword("viewer"), VoceChatEmail: "inherit-viewer@vc.example", VoceChatUserID: "712", VoceChatSyncStatus: models.VoceChatSyncStatusLinked})
	seedFreshRuntimeContact(t, author, viewer)
	message := models.Message{Content: "[managed](/api/files/refs/runtime-ref/file.txt)", UserID: author.ID, Username: author.Username, Visibility: MessageVisibilityContacts}
	if err := ApplyMessageVisibilityForSave(&message); err != nil {
		t.Fatalf("normalize contacts message: %v", err)
	}
	if err := db.Create(&message).Error; err != nil {
		t.Fatalf("create contacts message: %v", err)
	}
	parent := models.Comment{MessageID: message.ID, UserID: &author.ID, Content: "contacts parent", Visibility: "contacts"}
	if err := db.Create(&parent).Error; err != nil {
		t.Fatalf("create contacts comment: %v", err)
	}
	reply := models.Comment{MessageID: message.ID, UserID: &author.ID, ParentID: &parent.ID, Content: "public-labelled reply", Visibility: "public"}
	if err := db.Create(&reply).Error; err != nil {
		t.Fatalf("create reply: %v", err)
	}
	blob := models.AttachmentBlob{StorageBackend: "local", StorageKey: "runtime-contact-blob", ContentHash: "runtime-contact-hash"}
	if err := db.Create(&blob).Error; err != nil {
		t.Fatalf("create blob: %v", err)
	}
	reference := models.AttachmentReference{PublicID: "runtime-ref", BlobID: blob.ID, OwnerUserID: author.ID, Kind: "file", OriginalName: "file.txt"}
	if err := db.Create(&reference).Error; err != nil {
		t.Fatalf("create reference: %v", err)
	}
	viewerID := viewer.ID
	assertInherited := func(wantVisible bool) {
		t.Helper()
		scope, err := ResolveContentReadScope(db, &viewerID)
		if err != nil {
			t.Fatalf("resolve scope: %v", err)
		}
		commentMap := CommentMap([]models.Comment{parent, reply})
		if got := scope.CanReadComment(message, parent, commentMap); got != wantVisible {
			t.Fatalf("comment visibility = %v, want %v", got, wantVisible)
		}
		if got := scope.CanReadComment(message, reply, commentMap); got != wantVisible {
			t.Fatalf("reply visibility = %v, want %v", got, wantVisible)
		}
		sources, err := VisibleAttachmentSources(db, &viewerID, reference, "local")
		if err != nil {
			t.Fatalf("visible attachment sources: %v", err)
		}
		if got := len(sources) > 0; got != wantVisible {
			t.Fatalf("attachment visibility = %v, want %v; sources=%#v", got, wantVisible, sources)
		}
	}
	assertInherited(true)
	if err := db.Model(&models.SiteConfig{}).Where("id = ?", config.ID).Updates(map[string]interface{}{"runtime_mode": models.RuntimeModeLocal, "voce_chat_enabled": false}).Error; err != nil {
		t.Fatalf("switch local: %v", err)
	}
	assertInherited(false)
	if err := db.Model(&models.SiteConfig{}).Where("id = ?", config.ID).Updates(map[string]interface{}{"runtime_mode": models.RuntimeModeVoceChat, "voce_chat_enabled": true, "voce_chat_last_health_status": "failed"}).Error; err != nil {
		t.Fatalf("switch degraded: %v", err)
	}
	assertInherited(false)
}

func TestRuntimePushPolicySkipsHistoricalBacklogButKeepsSiteNotification(t *testing.T) {
	db := setupUserServiceTestDB(t)
	config := createRuntimeContactPushConfigForTest(t, models.RuntimeModeLocal, "ok")
	primary := mustCreateUser(t, models.User{Username: "primary-push-runtime", Password: models.HashPassword("primary"), IsAdmin: true})
	recipient := mustCreateUser(t, models.User{Username: "push-runtime-recipient", Password: models.HashPassword("recipient"), VoceChatUserID: "801", VoceChatNotificationEnabled: true})
	notification := models.UserNotification{RecipientUserID: recipient.ID, ActorUserID: &primary.ID, Type: models.UserNotificationTypeLike}
	if err := db.Create(&notification).Error; err != nil {
		t.Fatalf("create site notification: %v", err)
	}
	if err := sendUserNotificationToVoceChat(context.Background(), notification.ID); err != nil {
		t.Fatalf("local mode notification push should be skipped: %v", err)
	}
	var notificationCount int64
	if err := db.Model(&models.UserNotification{}).Where("id = ?", notification.ID).Count(&notificationCount).Error; err != nil || notificationCount != 1 {
		t.Fatalf("site notification was not retained: count=%d err=%v", notificationCount, err)
	}
	publishedAt := time.Now().UTC()
	announcement := models.Announcement{Title: "runtime push", Content: "body", Status: models.AnnouncementStatusPublished, PushEnabled: true, PublishedAt: &publishedAt, AuthorUserID: primary.ID}
	if err := db.Create(&announcement).Error; err != nil {
		t.Fatalf("create announcement: %v", err)
	}
	delivery := models.AnnouncementPushDelivery{AnnouncementID: announcement.ID, RecipientUserID: recipient.ID, RecipientVoceChatUserID: recipient.VoceChatUserID, Status: models.AnnouncementPushPending}
	if err := db.Create(&delivery).Error; err != nil {
		t.Fatalf("create pending delivery: %v", err)
	}
	result, err := DispatchPendingAnnouncementPushes(context.Background(), db, NewVoceChatAnnouncementPushSender(db), 10)
	if err != nil {
		t.Fatalf("dispatch local-mode delivery: %v", err)
	}
	if result.Skipped != 1 || result.Failed != 0 || result.Sent != 0 {
		t.Fatalf("local-mode dispatch = %#v", result)
	}
	if err := db.First(&delivery, delivery.ID).Error; err != nil {
		t.Fatalf("reload skipped delivery: %v", err)
	}
	if delivery.Status != models.AnnouncementPushSkipped || !strings.Contains(delivery.LastError, "当前运行状态") {
		t.Fatalf("local-mode delivery was not permanently skipped: %#v", delivery)
	}
	if err := db.Model(&models.SiteConfig{}).Where("id = ?", config.ID).Updates(map[string]interface{}{
		"runtime_mode": models.RuntimeModeVoceChat, "voce_chat_enabled": true, "voce_chat_last_health_status": "ok",
	}).Error; err != nil {
		t.Fatalf("recover VoceChat mode: %v", err)
	}
	result, err = DispatchPendingAnnouncementPushes(context.Background(), db, announcementPushSenderStub{results: map[uint]AnnouncementPushSendResult{recipient.ID: {RecipientVoceChatUserID: recipient.VoceChatUserID}}}, 10)
	if err != nil {
		t.Fatalf("dispatch after recovery: %v", err)
	}
	if result.Processed != 0 {
		t.Fatalf("recovery replayed historical push: %#v", result)
	}
}

func TestVoceChatPushPolicyMatchesRuntimeStates(t *testing.T) {
	for _, test := range []struct {
		name   string
		mode   string
		health string
		want   bool
	}{
		{name: "local", mode: models.RuntimeModeLocal, health: "ok", want: false},
		{name: "normal", mode: models.RuntimeModeVoceChat, health: "ok", want: true},
		{name: "degraded", mode: models.RuntimeModeVoceChat, health: "failed", want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			setupUserServiceTestDB(t)
			createRuntimeContactPushConfigForTest(t, test.mode, test.health)
			policy, err := ResolveRuntimePolicy()
			if err != nil {
				t.Fatalf("resolve policy: %v", err)
			}
			if policy.SendVoceChatPush != test.want || (policy.RuntimeState == runtimepolicy.StateVoceChatNormal) != test.want {
				t.Fatalf("push policy = %#v, want send=%v", policy, test.want)
			}
		})
	}
}

func TestRuntimePushFailureDegradesSiteWithoutCredentialAlertsOrBacklog(t *testing.T) {
	db := setupUserServiceTestDB(t)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusServiceUnavailable)
		_, _ = response.Write([]byte("temporary outage"))
	}))
	defer server.Close()
	config := createRuntimeContactPushConfigForTest(t, models.RuntimeModeVoceChat, "ok")
	if err := db.Model(&models.SiteConfig{}).Where("id = ?", config.ID).Update("voce_chat_base_url", server.URL).Error; err != nil {
		t.Fatalf("set push test server: %v", err)
	}
	primary := mustCreateUser(t, models.User{Username: "primary-push-failure", Password: models.HashPassword("primary"), IsAdmin: true})
	recipient := mustCreateUser(t, models.User{Username: "recipient-push-failure", Password: models.HashPassword("recipient"), VoceChatUserID: "811", VoceChatNotificationEnabled: true})
	notification := models.UserNotification{RecipientUserID: recipient.ID, ActorUserID: &primary.ID, Type: models.UserNotificationTypeLike}
	if err := db.Create(&notification).Error; err != nil {
		t.Fatalf("create site notification: %v", err)
	}
	if err := sendUserNotificationToVoceChat(context.Background(), notification.ID); err == nil {
		t.Fatal("temporary user push failure unexpectedly succeeded")
	}
	var storedConfig models.SiteConfig
	if err := db.First(&storedConfig, config.ID).Error; err != nil {
		t.Fatalf("reload degraded config: %v", err)
	}
	if storedConfig.VoceChatLastHealthStatus != "failed" || storedConfig.VoceChatLastHealthError != "VoceChat 推送服务暂不可用" {
		t.Fatalf("push failure health = status %q error %q", storedConfig.VoceChatLastHealthStatus, storedConfig.VoceChatLastHealthError)
	}
	var credentialAlerts int64
	if err := db.Model(&models.UserNotification{}).Where("type IN ?", []string{models.UserNotificationTypeVoceChatCredentials, models.UserNotificationTypeVoceChatPasswordChanged}).Count(&credentialAlerts).Error; err != nil || credentialAlerts != 0 {
		t.Fatalf("site outage created credential alerts: count=%d err=%v", credentialAlerts, err)
	}
	if err := db.Model(&models.SiteConfig{}).Where("id = ?", config.ID).Updates(map[string]interface{}{"voce_chat_last_health_status": "ok", "voce_chat_last_health_error": ""}).Error; err != nil {
		t.Fatalf("reset health before announcement: %v", err)
	}
	publishedAt := time.Now().UTC()
	announcement := models.Announcement{Title: "push outage", Content: "body", Status: models.AnnouncementStatusPublished, PushEnabled: true, PublishedAt: &publishedAt, AuthorUserID: primary.ID}
	if err := db.Create(&announcement).Error; err != nil {
		t.Fatalf("create announcement: %v", err)
	}
	delivery := models.AnnouncementPushDelivery{AnnouncementID: announcement.ID, RecipientUserID: recipient.ID, RecipientVoceChatUserID: recipient.VoceChatUserID, Status: models.AnnouncementPushPending}
	if err := db.Create(&delivery).Error; err != nil {
		t.Fatalf("create delivery: %v", err)
	}
	result, err := DispatchPendingAnnouncementPushes(context.Background(), db, NewVoceChatAnnouncementPushSender(db), 10)
	if err != nil {
		t.Fatalf("dispatch during outage: %v", err)
	}
	if result.Skipped != 1 || result.Failed != 0 {
		t.Fatalf("outage delivery result = %#v", result)
	}
	if err := db.First(&delivery, delivery.ID).Error; err != nil || delivery.Status != models.AnnouncementPushSkipped {
		t.Fatalf("outage delivery retained backlog: delivery=%#v err=%v", delivery, err)
	}
}
