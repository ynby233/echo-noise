package services

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type announcementPushSenderStub struct {
	results map[uint]AnnouncementPushSendResult
	errors  map[uint]error
	seenUID map[uint]string
}

func (s announcementPushSenderStub) Send(_ context.Context, _ models.Announcement, recipient models.User) (AnnouncementPushSendResult, error) {
	if s.seenUID != nil {
		s.seenUID[recipient.ID] = recipient.VoceChatUserID
	}
	if err := s.errors[recipient.ID]; err != nil {
		return AnnouncementPushSendResult{}, err
	}
	return s.results[recipient.ID], nil
}

func TestDispatchPendingAnnouncementPushesUsesPersistedRecipientUID(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.Announcement{}, &models.AnnouncementPushDelivery{}); err != nil {
		t.Fatalf("migrate test db: %v", err)
	}
	publishedAt := time.Now()
	announcement := models.Announcement{Title: "目标快照", Content: "正文", Status: models.AnnouncementStatusPublished, PublishedAt: &publishedAt, Revision: 1, PushEnabled: true}
	user := models.User{Username: "retargeted-user", Password: "hashed", VoceChatNotificationEnabled: false, VoceChatUserID: "202"}
	if err := db.Create(&announcement).Error; err != nil {
		t.Fatalf("seed announcement: %v", err)
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	delivery := models.AnnouncementPushDelivery{
		AnnouncementID: announcement.ID, RecipientUserID: user.ID, RecipientVoceChatUserID: "101", Status: models.AnnouncementPushPending,
	}
	if err := db.Create(&delivery).Error; err != nil {
		t.Fatalf("seed delivery: %v", err)
	}
	seenUID := map[uint]string{}
	sender := announcementPushSenderStub{
		results: map[uint]AnnouncementPushSendResult{user.ID: {RecipientVoceChatUserID: "101"}},
		errors:  map[uint]error{},
		seenUID: seenUID,
	}
	if _, err := DispatchPendingAnnouncementPushes(context.Background(), db, sender, 10); err != nil {
		t.Fatalf("dispatch pending delivery: %v", err)
	}
	if seenUID[user.ID] != "101" {
		t.Fatalf("sender recipient UID = %q, want persisted UID 101", seenUID[user.ID])
	}
}

func TestDispatchPendingAnnouncementPushesPersistsSentFailedAndSkippedOutcomes(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.Announcement{}, &models.AnnouncementPushDelivery{}); err != nil {
		t.Fatalf("migrate test db: %v", err)
	}
	publishedAt := time.Now()
	announcement := models.Announcement{Title: "系统公告", Content: "正文", Status: models.AnnouncementStatusPublished, PublishedAt: &publishedAt, Revision: 1, PushEnabled: true}
	if err := db.Create(&announcement).Error; err != nil {
		t.Fatalf("seed announcement: %v", err)
	}
	users := []models.User{
		{Username: "sent-user", Password: "hashed", VoceChatNotificationEnabled: true, VoceChatUserID: "101"},
		{Username: "failed-user", Password: "hashed", VoceChatNotificationEnabled: true, VoceChatUserID: "102"},
		{Username: "skipped-user", Password: "hashed", VoceChatNotificationEnabled: false, VoceChatUserID: "103"},
	}
	if err := db.Create(&users).Error; err != nil {
		t.Fatalf("seed users: %v", err)
	}
	deliveries := make([]models.AnnouncementPushDelivery, 0, len(users))
	for _, user := range users {
		deliveries = append(deliveries, models.AnnouncementPushDelivery{
			AnnouncementID: announcement.ID, RecipientUserID: user.ID, RecipientVoceChatUserID: user.VoceChatUserID, Status: models.AnnouncementPushPending,
		})
	}
	if err := db.Create(&deliveries).Error; err != nil {
		t.Fatalf("seed deliveries: %v", err)
	}
	sender := announcementPushSenderStub{
		results: map[uint]AnnouncementPushSendResult{
			users[0].ID: {RecipientVoceChatUserID: "101"},
			users[2].ID: {Skipped: true, Detail: "用户已关闭接收 VoceChat 推送"},
		},
		errors: map[uint]error{users[1].ID: errors.New("vocechat unavailable")},
	}

	result, err := DispatchPendingAnnouncementPushes(context.Background(), db, sender, 10)
	if err != nil {
		t.Fatalf("dispatch pending deliveries: %v", err)
	}
	if result.Processed != 3 || result.Sent != 1 || result.Failed != 1 || result.Skipped != 1 {
		t.Fatalf("dispatch result = %#v", result)
	}
	summary, err := GetAnnouncementPushSummary(db, announcement.ID)
	if err != nil {
		t.Fatalf("get push summary: %v", err)
	}
	if summary.Total != 3 || summary.Pending != 0 || summary.Processing != 0 || summary.Sent != 1 || summary.Failed != 1 || summary.Skipped != 1 {
		t.Fatalf("summary = %#v", summary)
	}
}

func TestRecoverStaleAnnouncementPushDeliveriesRequeuesOnlyExpiredWork(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	if err := db.AutoMigrate(&models.AnnouncementPushDelivery{}); err != nil {
		t.Fatalf("migrate test db: %v", err)
	}
	oldAttempt := time.Now().Add(-10 * time.Minute)
	freshAttempt := time.Now()
	deliveries := []models.AnnouncementPushDelivery{
		{AnnouncementID: 1, RecipientUserID: 1, Status: models.AnnouncementPushSending, LastAttemptAt: &oldAttempt},
		{AnnouncementID: 1, RecipientUserID: 2, Status: models.AnnouncementPushSending, LastAttemptAt: &freshAttempt},
	}
	if err := db.Create(&deliveries).Error; err != nil {
		t.Fatalf("seed deliveries: %v", err)
	}
	recovered, err := RecoverStaleAnnouncementPushDeliveries(db, time.Now().Add(-5*time.Minute))
	if err != nil {
		t.Fatalf("recover stale deliveries: %v", err)
	}
	if recovered != 1 {
		t.Fatalf("recovered = %d, want 1", recovered)
	}
	summary, err := GetAnnouncementPushSummary(db, 1)
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if summary.Pending != 1 || summary.Processing != 1 {
		t.Fatalf("summary = %#v", summary)
	}
}

func TestVoceChatAnnouncementPushSenderUsesPersistedPublishTimeRecipient(t *testing.T) {
	var receivedPath string
	var receivedBody string
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		receivedPath = request.URL.Path
		body, _ := io.ReadAll(request.Body)
		receivedBody = string(body)
		response.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	if err := db.AutoMigrate(&models.SiteConfig{}); err != nil {
		t.Fatalf("migrate site config: %v", err)
	}
	config := models.SiteConfig{
		SiteTitle: "测试站点", VoceChatEnabled: true, VoceChatNotificationEnabled: true,
		VoceChatBaseURL: server.URL, VoceChatBotAPIKey: "bot-key",
	}
	if err := db.Create(&config).Error; err != nil {
		t.Fatalf("seed site config: %v", err)
	}
	sender := NewVoceChatAnnouncementPushSender(db)
	result, err := sender.Send(context.Background(), models.Announcement{Title: "维护通知", Content: "今晚 **22:00** 维护"}, models.User{
		// The user has opted out since publication, but this recipient was already
		// captured in a persistent delivery row at publish time.
		ID: 2, Username: "alice", VoceChatNotificationEnabled: false, VoceChatUserID: "101",
	})
	if err != nil {
		t.Fatalf("send announcement: %v", err)
	}
	if result.Skipped || result.RecipientVoceChatUserID != "101" {
		t.Fatalf("send result = %#v", result)
	}
	if receivedPath != "/api/bot/send_to_user/101" {
		t.Fatalf("path = %q", receivedPath)
	}
	if !strings.Contains(receivedBody, "测试站点 公告") || !strings.Contains(receivedBody, "维护通知") || !strings.Contains(receivedBody, "今晚 **22:00** 维护") {
		t.Fatalf("markdown body = %q", receivedBody)
	}
}
