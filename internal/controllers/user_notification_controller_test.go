package controllers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/services"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupUserNotificationTest(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.Message{}, &models.MessageLike{}, &models.Comment{}, &models.UserNotification{}, &models.PasswordAlertCleanupTask{}, &models.SiteConfig{}, &models.AdminCapabilityGrant{}); err != nil {
		t.Fatalf("migrate test database: %v", err)
	}
	database.DB = db
	models.SetDB(db)
	t.Cleanup(func() {
		database.DB = nil
		models.SetDB(nil)
	})
	return db
}

type notificationQueryMetrics struct {
	mu      sync.Mutex
	enabled bool
	queries int
	rows    map[string]int64
	tables  map[string]int
}

func captureNotificationQueries(t *testing.T, db *gorm.DB) *notificationQueryMetrics {
	t.Helper()
	metrics := &notificationQueryMetrics{rows: map[string]int64{}, tables: map[string]int{}}
	if err := db.Callback().Query().After("gorm:query").Register("test:capture_notification_queries", func(tx *gorm.DB) {
		metrics.mu.Lock()
		defer metrics.mu.Unlock()
		if !metrics.enabled {
			return
		}
		metrics.queries++
		metrics.rows[tx.Statement.Table] += tx.RowsAffected
		metrics.tables[tx.Statement.Table]++
	}); err != nil {
		t.Fatalf("register query metrics: %v", err)
	}
	return metrics
}

func (metrics *notificationQueryMetrics) start() {
	metrics.mu.Lock()
	defer metrics.mu.Unlock()
	metrics.enabled = true
	metrics.queries = 0
	metrics.rows = map[string]int64{}
	metrics.tables = map[string]int{}
}

func requestUserNotificationEndpoint(t *testing.T, userID uint, path string, handler gin.HandlerFunc) []byte {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", userID)
		c.Next()
	})
	routePath := path
	if queryStart := strings.IndexByte(routePath, '?'); queryStart >= 0 {
		routePath = routePath[:queryStart]
	}
	router.GET(routePath, handler)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, path, nil)
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET %s status = %d, body = %s", path, recorder.Code, recorder.Body.String())
	}
	return recorder.Body.Bytes()
}

func TestUnreadNotificationCountAvoidsPageHydrationAndPerItemScopeQueries(t *testing.T) {
	db := setupUserNotificationTest(t)
	viewer := models.User{Username: "viewer"}
	actor := models.User{Username: "actor"}
	if err := db.Create(&viewer).Error; err != nil {
		t.Fatalf("create viewer: %v", err)
	}
	if err := db.Create(&actor).Error; err != nil {
		t.Fatalf("create actor: %v", err)
	}
	message := models.Message{Content: "visible message", UserID: viewer.ID, Visibility: "public"}
	if err := db.Create(&message).Error; err != nil {
		t.Fatalf("create message: %v", err)
	}
	if err := db.Create(&models.MessageLike{MessageID: message.ID, UserID: &actor.ID}).Error; err != nil {
		t.Fatalf("create active like: %v", err)
	}
	notifications := make([]models.UserNotification, 400)
	createdAt := time.Now().UTC()
	for i := range notifications {
		notifications[i] = models.UserNotification{
			RecipientUserID: viewer.ID,
			ActorUserID:     &actor.ID,
			Type:            models.UserNotificationTypeLike,
			MessageID:       &message.ID,
			CreatedAt:       createdAt,
		}
	}
	if err := db.CreateInBatches(&notifications, 100).Error; err != nil {
		t.Fatalf("create notifications: %v", err)
	}

	metrics := captureNotificationQueries(t, db)
	metrics.start()
	body := requestUserNotificationEndpoint(t, viewer.ID, "/api/notifications/unread-count", GetUserNotificationUnreadCount)
	var response struct {
		Code int `json:"code"`
		Data struct {
			UnreadCount int `json:"unread_count"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Code != 1 || response.Data.UnreadCount != len(notifications) {
		t.Fatalf("unexpected unread response: %s", body)
	}
	metrics.mu.Lock()
	defer metrics.mu.Unlock()
	if metrics.rows["comments"] != 0 {
		t.Fatalf("unread count hydrated %d comment rows", metrics.rows["comments"])
	}
	if metrics.queries > 25 {
		t.Fatalf("unread count executed %d queries for %d notifications; viewer scope is being resolved per item", metrics.queries, len(notifications))
	}
}

func TestNotificationPageHydratesOnlySelectedItems(t *testing.T) {
	db := setupUserNotificationTest(t)
	viewer := models.User{Username: "viewer"}
	actor := models.User{Username: "actor"}
	if err := db.Create(&viewer).Error; err != nil {
		t.Fatalf("create viewer: %v", err)
	}
	if err := db.Create(&actor).Error; err != nil {
		t.Fatalf("create actor: %v", err)
	}
	createdAt := time.Now().UTC()
	for i := 0; i < 100; i++ {
		message := models.Message{Content: "message", UserID: viewer.ID, Visibility: "public"}
		if err := db.Create(&message).Error; err != nil {
			t.Fatalf("create message %d: %v", i, err)
		}
		comment := models.Comment{MessageID: message.ID, UserID: &actor.ID, Content: "comment", Visibility: "public"}
		if err := db.Create(&comment).Error; err != nil {
			t.Fatalf("create comment %d: %v", i, err)
		}
		notification := models.UserNotification{
			RecipientUserID: viewer.ID,
			ActorUserID:     &actor.ID,
			Type:            models.UserNotificationTypeComment,
			MessageID:       &message.ID,
			CommentID:       &comment.ID,
			CreatedAt:       createdAt,
		}
		if err := db.Create(&notification).Error; err != nil {
			t.Fatalf("create notification %d: %v", i, err)
		}
	}

	metrics := captureNotificationQueries(t, db)
	metrics.start()
	body := requestUserNotificationEndpoint(t, viewer.ID, "/api/notifications?page=1&pageSize=10", ListUserNotifications)
	var response struct {
		Code int `json:"code"`
		Data struct {
			Items []struct {
				ID           uint   `json:"id"`
				TargetStatus string `json:"target_status"`
			} `json:"items"`
			Total       int `json:"total"`
			UnreadCount int `json:"unread_count"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Code != 1 || len(response.Data.Items) != 10 || response.Data.Total != 100 || response.Data.UnreadCount != 100 {
		t.Fatalf("unexpected notification page response: %s", body)
	}
	for index, item := range response.Data.Items {
		wantID := uint(100 - index)
		if item.ID != wantID || item.TargetStatus != userNotificationTargetStatusAvailable {
			t.Fatalf("item %d = %#v, want id %d available", index, item, wantID)
		}
	}
	metrics.mu.Lock()
	defer metrics.mu.Unlock()
	if metrics.rows["comments"] > 10 {
		t.Fatalf("page of 10 hydrated %d comment rows", metrics.rows["comments"])
	}
	if metrics.queries > 30 {
		t.Fatalf("page of 10 executed %d queries", metrics.queries)
	}
}

func TestNotificationEndpointsPreserveFilteringPlaceholdersCountsAndOrdering(t *testing.T) {
	db := setupUserNotificationTest(t)
	viewer := models.User{Username: "viewer"}
	actor := models.User{Username: "actor"}
	other := models.User{Username: "other"}
	secondViewer := models.User{Username: "second-viewer"}
	for _, user := range []*models.User{&viewer, &actor, &other, &secondViewer} {
		if err := db.Create(user).Error; err != nil {
			t.Fatalf("create user %s: %v", user.Username, err)
		}
	}
	publicMessage := models.Message{Content: "public message", UserID: viewer.ID, Visibility: "public"}
	privateMessage := models.Message{Content: "private message", UserID: actor.ID, Visibility: "private", Private: true}
	for _, message := range []*models.Message{&publicMessage, &privateMessage} {
		if err := db.Create(message).Error; err != nil {
			t.Fatalf("create message: %v", err)
		}
	}
	publicComment := models.Comment{MessageID: publicMessage.ID, UserID: &actor.ID, Content: "public comment", Visibility: "public"}
	privateComment := models.Comment{MessageID: privateMessage.ID, UserID: &actor.ID, Content: "private comment", Visibility: "private"}
	for _, comment := range []*models.Comment{&publicComment, &privateComment} {
		if err := db.Create(comment).Error; err != nil {
			t.Fatalf("create comment: %v", err)
		}
	}
	if err := db.Create(&models.MessageLike{MessageID: publicMessage.ID, UserID: &actor.ID}).Error; err != nil {
		t.Fatalf("create active like: %v", err)
	}
	missingMessageID := uint(9999)
	readAt := time.Now().UTC().Add(-time.Minute)
	createdAt := time.Now().UTC()
	notifications := []models.UserNotification{
		{ID: 700, RecipientUserID: viewer.ID, Type: models.UserNotificationTypeVoceChatPasswordChanged, ReadAt: &readAt, CreatedAt: createdAt},
		{ID: 699, RecipientUserID: viewer.ID, Type: models.UserNotificationTypeContentDeletion, DeletionSnapshotJSON: "[]", CreatedAt: createdAt},
		{ID: 698, RecipientUserID: viewer.ID, ActorUserID: &actor.ID, Type: models.UserNotificationTypeLike, MessageID: &missingMessageID, CreatedAt: createdAt},
		{ID: 697, RecipientUserID: viewer.ID, ActorUserID: &actor.ID, Type: models.UserNotificationTypeComment, MessageID: &privateMessage.ID, CommentID: &privateComment.ID, CreatedAt: createdAt},
		{ID: 696, RecipientUserID: viewer.ID, ActorUserID: &actor.ID, Type: models.UserNotificationTypeLike, MessageID: &publicMessage.ID, CreatedAt: createdAt},
		{ID: 695, RecipientUserID: viewer.ID, ActorUserID: &other.ID, Type: models.UserNotificationTypeLike, MessageID: &publicMessage.ID, CreatedAt: createdAt},
		{ID: 694, RecipientUserID: viewer.ID, ActorUserID: &actor.ID, Type: models.UserNotificationTypeComment, MessageID: &publicMessage.ID, CommentID: &publicComment.ID, CreatedAt: createdAt},
		{ID: 693, RecipientUserID: secondViewer.ID, ActorUserID: &actor.ID, Type: models.UserNotificationTypeComment, MessageID: &privateMessage.ID, CommentID: &privateComment.ID, CreatedAt: createdAt},
	}
	if err := db.Create(&notifications).Error; err != nil {
		t.Fatalf("create notifications: %v", err)
	}

	decodePage := func(path string) struct {
		Code int `json:"code"`
		Data struct {
			Items []struct {
				ID           uint   `json:"id"`
				TargetStatus string `json:"target_status"`
			} `json:"items"`
			Total       int `json:"total"`
			UnreadCount int `json:"unread_count"`
		} `json:"data"`
	} {
		body := requestUserNotificationEndpoint(t, viewer.ID, path, ListUserNotifications)
		var response struct {
			Code int `json:"code"`
			Data struct {
				Items []struct {
					ID           uint   `json:"id"`
					TargetStatus string `json:"target_status"`
				} `json:"items"`
				Total       int `json:"total"`
				UnreadCount int `json:"unread_count"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &response); err != nil {
			t.Fatalf("decode page: %v", err)
		}
		return response
	}
	firstPage := decodePage("/api/notifications?page=1&pageSize=3")
	if firstPage.Code != 1 || firstPage.Data.Total != 6 || firstPage.Data.UnreadCount != 5 {
		t.Fatalf("unexpected first-page totals: %#v", firstPage)
	}
	wantFirst := []struct {
		id     uint
		status string
	}{{700, userNotificationTargetStatusUnavailable}, {699, userNotificationTargetStatusSnapshot}, {698, userNotificationTargetStatusUnavailable}}
	for index, want := range wantFirst {
		if firstPage.Data.Items[index].ID != want.id || firstPage.Data.Items[index].TargetStatus != want.status {
			t.Fatalf("first page item %d = %#v, want id=%d status=%s", index, firstPage.Data.Items[index], want.id, want.status)
		}
	}
	secondPage := decodePage("/api/notifications?page=2&pageSize=3")
	wantSecond := []struct {
		id     uint
		status string
	}{{697, userNotificationTargetStatusUnavailable}, {696, userNotificationTargetStatusAvailable}, {694, userNotificationTargetStatusAvailable}}
	for index, want := range wantSecond {
		if secondPage.Data.Items[index].ID != want.id || secondPage.Data.Items[index].TargetStatus != want.status {
			t.Fatalf("second page item %d = %#v, want id=%d status=%s", index, secondPage.Data.Items[index], want.id, want.status)
		}
	}

	unreadBody := requestUserNotificationEndpoint(t, viewer.ID, "/api/notifications/unread-count", GetUserNotificationUnreadCount)
	var unreadResponse struct {
		Code int `json:"code"`
		Data struct {
			UnreadCount int `json:"unread_count"`
		} `json:"data"`
	}
	if err := json.Unmarshal(unreadBody, &unreadResponse); err != nil {
		t.Fatalf("decode unread response: %v", err)
	}
	if unreadResponse.Code != 1 || unreadResponse.Data.UnreadCount != 5 {
		t.Fatalf("unexpected unread response: %s", unreadBody)
	}

	secondBody := requestUserNotificationEndpoint(t, secondViewer.ID, "/api/notifications?page=1&pageSize=20", ListUserNotifications)
	var secondResponse struct {
		Code int `json:"code"`
		Data struct {
			Items []struct {
				TargetStatus string `json:"target_status"`
			} `json:"items"`
			Total       int `json:"total"`
			UnreadCount int `json:"unread_count"`
		} `json:"data"`
	}
	if err := json.Unmarshal(secondBody, &secondResponse); err != nil {
		t.Fatalf("decode second viewer response: %v", err)
	}
	if secondResponse.Data.Total != 1 || secondResponse.Data.UnreadCount != 1 || len(secondResponse.Data.Items) != 1 || secondResponse.Data.Items[0].TargetStatus != userNotificationTargetStatusUnavailable {
		t.Fatalf("private target semantics changed for second viewer: %s", secondBody)
	}
}

func TestNotificationListPreservesEveryNotificationType(t *testing.T) {
	db := setupUserNotificationTest(t)
	viewer := models.User{Username: "viewer"}
	actor := models.User{Username: "actor"}
	for _, user := range []*models.User{&viewer, &actor} {
		if err := db.Create(user).Error; err != nil {
			t.Fatalf("create user: %v", err)
		}
	}
	message := models.Message{Content: "message", UserID: viewer.ID, Visibility: "public"}
	guestbook := models.Message{Content: "guestbook", UserID: viewer.ID, Visibility: "public", IsGuestbook: true}
	for _, target := range []*models.Message{&message, &guestbook} {
		if err := db.Create(target).Error; err != nil {
			t.Fatalf("create message: %v", err)
		}
	}
	comment := models.Comment{MessageID: message.ID, UserID: &actor.ID, Content: "comment", Visibility: "public"}
	parent := models.Comment{MessageID: message.ID, UserID: &viewer.ID, Content: "parent", Visibility: "public"}
	if err := db.Create(&comment).Error; err != nil {
		t.Fatalf("create comment: %v", err)
	}
	if err := db.Create(&parent).Error; err != nil {
		t.Fatalf("create parent: %v", err)
	}
	reply := models.Comment{MessageID: message.ID, UserID: &actor.ID, Content: "reply", Visibility: "public", ParentID: &parent.ID}
	guestbookComment := models.Comment{MessageID: guestbook.ID, UserID: &actor.ID, Content: "guestbook comment", Visibility: "public"}
	for _, target := range []*models.Comment{&reply, &guestbookComment} {
		if err := db.Create(target).Error; err != nil {
			t.Fatalf("create comment: %v", err)
		}
	}
	if err := db.Create(&models.MessageLike{MessageID: message.ID, UserID: &actor.ID}).Error; err != nil {
		t.Fatalf("create like: %v", err)
	}
	createdAt := time.Now().UTC()
	notifications := []models.UserNotification{
		{ID: 808, RecipientUserID: viewer.ID, ActorUserID: &actor.ID, Type: models.UserNotificationTypeLike, MessageID: &message.ID, CreatedAt: createdAt},
		{ID: 807, RecipientUserID: viewer.ID, ActorUserID: &actor.ID, Type: models.UserNotificationTypeComment, MessageID: &message.ID, CommentID: &comment.ID, CreatedAt: createdAt},
		{ID: 806, RecipientUserID: viewer.ID, ActorUserID: &actor.ID, Type: models.UserNotificationTypeReply, MessageID: &message.ID, CommentID: &reply.ID, ParentCommentID: &parent.ID, CreatedAt: createdAt},
		{ID: 805, RecipientUserID: viewer.ID, ActorUserID: &actor.ID, Type: models.UserNotificationTypeGuestbook, MessageID: &guestbook.ID, CommentID: &guestbookComment.ID, CreatedAt: createdAt},
		{ID: 804, RecipientUserID: viewer.ID, Type: models.UserNotificationTypeVoceChatCredentials, CreatedAt: createdAt},
		{ID: 803, RecipientUserID: viewer.ID, Type: models.UserNotificationTypeVoceChatPasswordChanged, CreatedAt: createdAt},
		{ID: 802, RecipientUserID: viewer.ID, Type: models.UserNotificationTypePasswordUpdateIncomplete, CreatedAt: createdAt},
		{ID: 801, RecipientUserID: viewer.ID, Type: models.UserNotificationTypeContentDeletion, DeletionSnapshotJSON: "[]", CreatedAt: createdAt},
	}
	if err := db.Create(&notifications).Error; err != nil {
		t.Fatalf("create notifications: %v", err)
	}

	body := requestUserNotificationEndpoint(t, viewer.ID, "/api/notifications?page=1&pageSize=20", ListUserNotifications)
	var response struct {
		Code int `json:"code"`
		Data struct {
			Items []struct {
				ID            uint   `json:"id"`
				Type          string `json:"type"`
				TargetStatus  string `json:"target_status"`
				TargetTab     string `json:"target_tab"`
				Comment       any    `json:"comment"`
				ParentComment any    `json:"parent_comment"`
			} `json:"items"`
			Total       int `json:"total"`
			UnreadCount int `json:"unread_count"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Code != 1 || response.Data.Total != len(notifications) || response.Data.UnreadCount != len(notifications) || len(response.Data.Items) != len(notifications) {
		t.Fatalf("unexpected page totals: %s", body)
	}
	wantStatus := map[string]string{
		models.UserNotificationTypeLike:                     userNotificationTargetStatusAvailable,
		models.UserNotificationTypeComment:                  userNotificationTargetStatusAvailable,
		models.UserNotificationTypeReply:                    userNotificationTargetStatusAvailable,
		models.UserNotificationTypeGuestbook:                userNotificationTargetStatusAvailable,
		models.UserNotificationTypeVoceChatCredentials:      userNotificationTargetStatusUnavailable,
		models.UserNotificationTypeVoceChatPasswordChanged:  userNotificationTargetStatusUnavailable,
		models.UserNotificationTypePasswordUpdateIncomplete: userNotificationTargetStatusUnavailable,
		models.UserNotificationTypeContentDeletion:          userNotificationTargetStatusSnapshot,
	}
	for _, item := range response.Data.Items {
		if item.TargetStatus != wantStatus[item.Type] {
			t.Fatalf("type %s status = %s, want %s", item.Type, item.TargetStatus, wantStatus[item.Type])
		}
		if item.Type == models.UserNotificationTypeReply && item.ParentComment == nil {
			t.Fatal("reply notification lost its parent comment")
		}
		if item.Type == models.UserNotificationTypeGuestbook && item.TargetTab != "comment" {
			t.Fatalf("guestbook target tab = %q", item.TargetTab)
		}
	}
}

func TestDeletionNotificationPageLoadsRetentionPolicyOnce(t *testing.T) {
	db := setupUserNotificationTest(t)
	viewer := models.User{Username: "viewer"}
	if err := db.Create(&viewer).Error; err != nil {
		t.Fatalf("create viewer: %v", err)
	}
	if err := db.Create(&models.SiteConfig{RecycleBinRetentionDays: 30, CommentRecycleBinRetentionDays: 7}).Error; err != nil {
		t.Fatalf("create site config: %v", err)
	}
	createdAt := time.Now().UTC()
	notifications := make([]models.UserNotification, 50)
	for index := range notifications {
		notifications[index] = models.UserNotification{
			RecipientUserID:      viewer.ID,
			Type:                 models.UserNotificationTypeContentDeletion,
			DeletionSnapshotJSON: "[]",
			CreatedAt:            createdAt,
		}
	}
	if err := db.CreateInBatches(&notifications, 25).Error; err != nil {
		t.Fatalf("create deletion notifications: %v", err)
	}
	metrics := captureNotificationQueries(t, db)
	metrics.start()
	body := requestUserNotificationEndpoint(t, viewer.ID, "/api/notifications?page=1&pageSize=50", ListUserNotifications)
	var response struct {
		Code int `json:"code"`
		Data struct {
			Items []json.RawMessage `json:"items"`
			Total int               `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Code != 1 || response.Data.Total != 50 || len(response.Data.Items) != 50 {
		t.Fatalf("unexpected deletion notification page: %s", body)
	}
	metrics.mu.Lock()
	defer metrics.mu.Unlock()
	if metrics.tables["site_configs"] > 1 {
		t.Fatalf("deletion page loaded retention policy %d times", metrics.tables["site_configs"])
	}
}

func TestNotificationPageKeepsEligibilityLookupFailureAsLoadError(t *testing.T) {
	db := setupUserNotificationTest(t)
	viewer := models.User{Username: "viewer"}
	actor := models.User{Username: "actor"}
	if err := db.Create(&viewer).Error; err != nil {
		t.Fatalf("create viewer: %v", err)
	}
	if err := db.Create(&actor).Error; err != nil {
		t.Fatalf("create actor: %v", err)
	}
	message := models.Message{Content: "message", UserID: viewer.ID, Visibility: "public"}
	if err := db.Create(&message).Error; err != nil {
		t.Fatalf("create message: %v", err)
	}
	if err := db.Create(&models.MessageLike{MessageID: message.ID, UserID: &actor.ID}).Error; err != nil {
		t.Fatalf("create like: %v", err)
	}
	notification := models.UserNotification{RecipientUserID: viewer.ID, ActorUserID: &actor.ID, Type: models.UserNotificationTypeLike, MessageID: &message.ID}
	if err := db.Create(&notification).Error; err != nil {
		t.Fatalf("create notification: %v", err)
	}
	failNextMessageLookup := true
	if err := db.Callback().Query().Before("gorm:query").Register("test:fail_first_eligibility_message_lookup", func(tx *gorm.DB) {
		if failNextMessageLookup && tx.Statement != nil && tx.Statement.Schema != nil && tx.Statement.Schema.Table == "messages" {
			failNextMessageLookup = false
			tx.AddError(errors.New("forced transient message lookup failure"))
		}
	}); err != nil {
		t.Fatalf("register query failure: %v", err)
	}
	body := requestUserNotificationEndpoint(t, viewer.ID, "/api/notifications?page=1&pageSize=20", ListUserNotifications)
	var response struct {
		Code int `json:"code"`
		Data struct {
			Items []struct {
				TargetStatus string `json:"target_status"`
			} `json:"items"`
			Total int `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Code != 1 || response.Data.Total != 1 || len(response.Data.Items) != 1 || response.Data.Items[0].TargetStatus != userNotificationTargetStatusLoadError {
		t.Fatalf("eligibility lookup failure was not preserved as a load-error placeholder: %s", body)
	}
}

func TestDeletionNotificationRecomputesCountdownFromCurrentRetentionPolicy(t *testing.T) {
	db := setupUserNotificationTest(t)
	if err := db.Create(&models.SiteConfig{CommentRecycleBinRetentionDays: 7}).Error; err != nil {
		t.Fatalf("create site config: %v", err)
	}
	deletedAt := time.Now().UTC().Add(-25 * time.Hour)
	staleDeadline := deletedAt.Add(30 * 24 * time.Hour)
	payload, err := json.Marshal([]services.DeletionSnapshotItem{{
		TargetType: "comment", TargetID: 9, ContentText: "snapshot", DeletedAt: &deletedAt, ScheduledDeletionAt: &staleDeadline,
	}})
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	notification := models.UserNotification{
		ID: 10, RecipientUserID: 11, Type: models.UserNotificationTypeContentDeletion,
		DeletionEvent: services.DeletionEventTrashed, DeletionActorLabel: "内容管理员", DeletionSnapshotJSON: string(payload),
	}
	items := buildVisibleUserNotifications([]models.UserNotification{notification}, 11)
	if len(items) != 1 || items[0].TargetStatus != userNotificationTargetStatusSnapshot || len(items[0].DeletionSnapshots) != 1 {
		t.Fatalf("unexpected deletion notification response: %#v", items)
	}
	got := items[0].DeletionSnapshots[0].ScheduledDeletionAt
	want := deletedAt.Add(7 * 24 * time.Hour)
	if got == nil || got.Sub(want) > time.Second || want.Sub(*got) > time.Second {
		t.Fatalf("current-policy deadline = %v, want %v", got, want)
	}
	if items[0].Actor != nil || items[0].ActorUserID != nil {
		t.Fatalf("deletion notification must not expose administrator identity: %#v", items[0])
	}
}

func TestBuildUserNotificationsReportsAssociationQueryFailureAsLoadError(t *testing.T) {
	db := setupUserNotificationTest(t)
	if err := db.Callback().Query().Before("gorm:query").Register("test:fail_message_lookup", func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Schema != nil && tx.Statement.Schema.Table == "messages" {
			tx.AddError(errors.New("forced message lookup failure"))
		}
	}); err != nil {
		t.Fatalf("register query failure: %v", err)
	}
	viewerID := uint(90)
	actorID := uint(91)
	messageID := uint(92)
	notification := models.UserNotification{
		ID:              93,
		Type:            models.UserNotificationTypeComment,
		RecipientUserID: viewerID,
		ActorUserID:     &actorID,
		MessageID:       &messageID,
	}

	items := buildVisibleUserNotifications([]models.UserNotification{notification}, viewerID)

	if len(items) != 1 {
		t.Fatalf("expected a load-error placeholder, got %d items", len(items))
	}
	if items[0].TargetStatus != userNotificationTargetStatusLoadError {
		t.Fatalf("expected load-error placeholder, got %q", items[0].TargetStatus)
	}
	if items[0].Message != nil || items[0].Comment != nil || items[0].ParentComment != nil {
		t.Fatal("load-error placeholder must not expose associated content")
	}
}

func TestBuildUserNotificationsRendersIncompletePasswordUpdateAsUnavailableSystemNotice(t *testing.T) {
	setupUserNotificationTest(t)
	viewerID := uint(94)
	notification := models.UserNotification{
		ID:              95,
		RecipientUserID: viewerID,
		Type:            models.UserNotificationTypePasswordUpdateIncomplete,
	}

	items := buildVisibleUserNotifications([]models.UserNotification{notification}, viewerID)
	if len(items) != 1 || items[0].TargetStatus != userNotificationTargetStatusUnavailable {
		t.Fatalf("expected incomplete password update system notice, got %d items with status %q", len(items), func() string {
			if len(items) == 0 {
				return ""
			}
			return items[0].TargetStatus
		}())
	}
	if items[0].Message != nil || items[0].Comment != nil || items[0].ParentComment != nil {
		t.Fatal("incomplete password update system notice must not expose unrelated content")
	}
}

func TestBuildUserNotificationsReportsCommentQueryFailureAsLoadError(t *testing.T) {
	db := setupUserNotificationTest(t)
	viewer := models.User{Username: "viewer"}
	author := models.User{Username: "author"}
	if err := db.Create(&viewer).Error; err != nil {
		t.Fatalf("create viewer: %v", err)
	}
	if err := db.Create(&author).Error; err != nil {
		t.Fatalf("create author: %v", err)
	}
	message := models.Message{Content: "visible message", UserID: author.ID, Visibility: "public"}
	if err := db.Create(&message).Error; err != nil {
		t.Fatalf("create message: %v", err)
	}
	if err := db.Callback().Query().Before("gorm:query").Register("test:fail_comment_lookup", func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Schema != nil && tx.Statement.Schema.Table == "comments" {
			tx.AddError(errors.New("forced comment lookup failure"))
		}
	}); err != nil {
		t.Fatalf("register query failure: %v", err)
	}
	commentID := uint(101)
	notification := models.UserNotification{
		ID:              102,
		Type:            models.UserNotificationTypeComment,
		RecipientUserID: viewer.ID,
		ActorUserID:     &author.ID,
		MessageID:       &message.ID,
		CommentID:       &commentID,
	}

	items := buildVisibleUserNotifications([]models.UserNotification{notification}, viewer.ID)

	if len(items) != 1 || items[0].TargetStatus != userNotificationTargetStatusLoadError {
		t.Fatalf("expected comment load-error placeholder, got %#v", items)
	}
}

func TestBuildUserNotificationsReportsLikeQueryFailureAsLoadError(t *testing.T) {
	db := setupUserNotificationTest(t)
	viewer := models.User{Username: "viewer"}
	author := models.User{Username: "author"}
	if err := db.Create(&viewer).Error; err != nil {
		t.Fatalf("create viewer: %v", err)
	}
	if err := db.Create(&author).Error; err != nil {
		t.Fatalf("create author: %v", err)
	}
	message := models.Message{Content: "visible message", UserID: viewer.ID, Visibility: "public"}
	if err := db.Create(&message).Error; err != nil {
		t.Fatalf("create message: %v", err)
	}
	if err := db.Callback().Query().Before("gorm:query").Register("test:fail_like_lookup", func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Schema != nil && tx.Statement.Schema.Table == "message_likes" {
			tx.AddError(errors.New("forced like lookup failure"))
		}
	}); err != nil {
		t.Fatalf("register query failure: %v", err)
	}
	notification := models.UserNotification{
		ID:              103,
		Type:            models.UserNotificationTypeLike,
		RecipientUserID: viewer.ID,
		ActorUserID:     &author.ID,
		MessageID:       &message.ID,
	}

	items := buildVisibleUserNotifications([]models.UserNotification{notification}, viewer.ID)

	if len(items) != 1 || items[0].TargetStatus != userNotificationTargetStatusLoadError {
		t.Fatalf("expected like load-error placeholder, got %#v", items)
	}
}

func TestBuildUserNotificationsKeepsMissingMessageLikeAsDeletedPlaceholder(t *testing.T) {
	setupUserNotificationTest(t)
	viewerID := uint(12)
	actorID := uint(34)
	missingMessageID := uint(56)
	notification := models.UserNotification{
		ID:              78,
		Type:            models.UserNotificationTypeLike,
		RecipientUserID: viewerID,
		ActorUserID:     &actorID,
		MessageID:       &missingMessageID,
	}

	items := buildVisibleUserNotifications([]models.UserNotification{notification}, viewerID)

	if len(items) != 1 {
		t.Fatalf("expected the notification to remain visible, got %d items", len(items))
	}
	if items[0].TargetStatus != userNotificationTargetStatusUnavailable {
		t.Fatalf("expected deleted-message placeholder, got %q", items[0].TargetStatus)
	}
	if items[0].Message != nil || items[0].Comment != nil || items[0].ParentComment != nil {
		t.Fatal("deleted-message placeholder must not expose associated content")
	}
	if items[0].TargetURL != "" || items[0].TargetTab != "" {
		t.Fatal("deleted-message placeholder must not expose a jump target")
	}
}

func TestBuildUserNotificationsHidesMissingCommentContentBehindUnavailablePlaceholder(t *testing.T) {
	db := setupUserNotificationTest(t)
	viewer := models.User{Username: "viewer"}
	author := models.User{Username: "author"}
	if err := db.Create(&viewer).Error; err != nil {
		t.Fatalf("create viewer: %v", err)
	}
	if err := db.Create(&author).Error; err != nil {
		t.Fatalf("create author: %v", err)
	}
	message := models.Message{Content: "visible message", UserID: author.ID, Visibility: "public"}
	if err := db.Create(&message).Error; err != nil {
		t.Fatalf("create message: %v", err)
	}
	missingCommentID := uint(999)
	notification := models.UserNotification{
		ID:              80,
		Type:            models.UserNotificationTypeComment,
		RecipientUserID: viewer.ID,
		ActorUserID:     &author.ID,
		MessageID:       &message.ID,
		CommentID:       &missingCommentID,
	}

	items := buildVisibleUserNotifications([]models.UserNotification{notification}, viewer.ID)

	if len(items) != 1 {
		t.Fatalf("expected the notification to remain visible, got %d items", len(items))
	}
	if items[0].TargetStatus != userNotificationTargetStatusUnavailable {
		t.Fatalf("expected unavailable placeholder, got %q", items[0].TargetStatus)
	}
	if items[0].Message != nil || items[0].Comment != nil || items[0].ParentComment != nil {
		t.Fatal("unavailable placeholder must not expose associated content")
	}
}

func TestBuildUserNotificationsHidesRestrictedMessageBehindUnavailablePlaceholder(t *testing.T) {
	db := setupUserNotificationTest(t)
	viewer := models.User{Username: "viewer"}
	author := models.User{Username: "author"}
	if err := db.Create(&viewer).Error; err != nil {
		t.Fatalf("create viewer: %v", err)
	}
	if err := db.Create(&author).Error; err != nil {
		t.Fatalf("create author: %v", err)
	}
	message := models.Message{Content: "restricted content", UserID: author.ID, Visibility: "private", Private: true}
	if err := db.Create(&message).Error; err != nil {
		t.Fatalf("create message: %v", err)
	}
	notification := models.UserNotification{
		ID:              81,
		Type:            models.UserNotificationTypeComment,
		RecipientUserID: viewer.ID,
		ActorUserID:     &author.ID,
		MessageID:       &message.ID,
	}

	items := buildVisibleUserNotifications([]models.UserNotification{notification}, viewer.ID)

	if len(items) != 1 {
		t.Fatalf("expected the restricted notification to remain visible, got %d items", len(items))
	}
	if items[0].TargetStatus != userNotificationTargetStatusUnavailable {
		t.Fatalf("expected unavailable placeholder, got %q", items[0].TargetStatus)
	}
	if items[0].Message != nil || items[0].Comment != nil || items[0].ParentComment != nil {
		t.Fatal("restricted placeholder must not expose associated content")
	}
}

func TestBuildUserNotificationsUsesDeletedMessagePlaceholderAcrossNotificationTypes(t *testing.T) {
	setupUserNotificationTest(t)
	viewerID := uint(201)
	actorID := uint(202)
	messageID := uint(203)
	commentID := uint(204)
	parentID := uint(205)
	types := []string{
		models.UserNotificationTypeLike,
		models.UserNotificationTypeComment,
		models.UserNotificationTypeReply,
		models.UserNotificationTypeGuestbook,
	}
	notifications := make([]models.UserNotification, 0, len(types))
	for index, notificationType := range types {
		notifications = append(notifications, models.UserNotification{
			ID:              uint(210 + index),
			Type:            notificationType,
			RecipientUserID: viewerID,
			ActorUserID:     &actorID,
			MessageID:       &messageID,
			CommentID:       &commentID,
			ParentCommentID: &parentID,
		})
	}

	items := buildVisibleUserNotifications(notifications, viewerID)

	if len(items) != len(types) {
		t.Fatalf("expected all missing-message notifications to remain visible, got %d", len(items))
	}
	for _, item := range items {
		if item.TargetStatus != userNotificationTargetStatusUnavailable {
			t.Fatalf("expected deleted-message placeholder for %s, got %q", item.Type, item.TargetStatus)
		}
	}
}

func TestBuildUserNotificationsKeepsRetractedLikeFiltered(t *testing.T) {
	db := setupUserNotificationTest(t)
	viewer := models.User{Username: "viewer"}
	actor := models.User{Username: "actor"}
	if err := db.Create(&viewer).Error; err != nil {
		t.Fatalf("create viewer: %v", err)
	}
	if err := db.Create(&actor).Error; err != nil {
		t.Fatalf("create actor: %v", err)
	}
	message := models.Message{Content: "still visible", UserID: viewer.ID, Visibility: "public"}
	if err := db.Create(&message).Error; err != nil {
		t.Fatalf("create message: %v", err)
	}
	notification := models.UserNotification{
		ID:              220,
		Type:            models.UserNotificationTypeLike,
		RecipientUserID: viewer.ID,
		ActorUserID:     &actor.ID,
		MessageID:       &message.ID,
	}

	items := buildVisibleUserNotifications([]models.UserNotification{notification}, viewer.ID)

	if len(items) != 0 {
		t.Fatalf("expected retracted like to stay filtered, got %#v", items)
	}
}

func TestBuildUserNotificationsHidesMissingReplyParentBehindUnavailablePlaceholder(t *testing.T) {
	db := setupUserNotificationTest(t)
	viewer := models.User{Username: "viewer"}
	actor := models.User{Username: "actor"}
	if err := db.Create(&viewer).Error; err != nil {
		t.Fatalf("create viewer: %v", err)
	}
	if err := db.Create(&actor).Error; err != nil {
		t.Fatalf("create actor: %v", err)
	}
	message := models.Message{Content: "visible message", UserID: viewer.ID, Visibility: "public"}
	if err := db.Create(&message).Error; err != nil {
		t.Fatalf("create message: %v", err)
	}
	missingParentID := uint(230)
	reply := models.Comment{MessageID: message.ID, UserID: &actor.ID, Content: "reply content", Visibility: "public", ParentID: &missingParentID}
	if err := db.Create(&reply).Error; err != nil {
		t.Fatalf("create reply: %v", err)
	}
	notification := models.UserNotification{
		ID:              231,
		Type:            models.UserNotificationTypeReply,
		RecipientUserID: viewer.ID,
		ActorUserID:     &actor.ID,
		MessageID:       &message.ID,
		CommentID:       &reply.ID,
		ParentCommentID: &missingParentID,
	}

	items := buildVisibleUserNotifications([]models.UserNotification{notification}, viewer.ID)

	if len(items) != 1 || items[0].TargetStatus != userNotificationTargetStatusUnavailable {
		t.Fatalf("expected missing-parent placeholder, got %#v", items)
	}
	if items[0].Comment != nil {
		t.Fatal("missing-parent placeholder must not expose the surviving reply content")
	}
}

func TestUnavailableNotificationsRetainReadStateAndUnreadCounting(t *testing.T) {
	setupUserNotificationTest(t)
	viewerID := uint(240)
	messageID := uint(241)
	readAt := time.Now().UTC()
	notifications := []models.UserNotification{
		{ID: 242, Type: models.UserNotificationTypeComment, RecipientUserID: viewerID, MessageID: &messageID},
		{ID: 243, Type: models.UserNotificationTypeReply, RecipientUserID: viewerID, MessageID: &messageID, ReadAt: &readAt},
	}

	items := buildVisibleUserNotifications(notifications, viewerID)

	if len(items) != 2 || items[0].Read || !items[1].Read || items[1].ReadAt == nil {
		t.Fatalf("expected placeholders to retain read state, got %#v", items)
	}
	if countUnreadUserNotifications(items) != 1 {
		t.Fatalf("expected one unread placeholder, got %d", countUnreadUserNotifications(items))
	}
}

func TestBuildUserNotificationsKeepsAvailableCommentNavigable(t *testing.T) {
	db := setupUserNotificationTest(t)
	viewer := models.User{Username: "viewer"}
	actor := models.User{Username: "actor"}
	if err := db.Create(&viewer).Error; err != nil {
		t.Fatalf("create viewer: %v", err)
	}
	if err := db.Create(&actor).Error; err != nil {
		t.Fatalf("create actor: %v", err)
	}
	message := models.Message{Content: "visible message", UserID: viewer.ID, Visibility: "public"}
	if err := db.Create(&message).Error; err != nil {
		t.Fatalf("create message: %v", err)
	}
	comment := models.Comment{MessageID: message.ID, UserID: &actor.ID, Content: "visible comment", Visibility: "public"}
	if err := db.Create(&comment).Error; err != nil {
		t.Fatalf("create comment: %v", err)
	}
	notification := models.UserNotification{ID: 250, Type: models.UserNotificationTypeComment, RecipientUserID: viewer.ID, ActorUserID: &actor.ID, MessageID: &message.ID, CommentID: &comment.ID}

	items := buildVisibleUserNotifications([]models.UserNotification{notification}, viewer.ID)

	if len(items) != 1 || items[0].TargetStatus != userNotificationTargetStatusAvailable {
		t.Fatalf("expected available notification, got %#v", items)
	}
	if items[0].Message == nil || items[0].Comment == nil || items[0].TargetURL == "" {
		t.Fatal("available notification should retain its content and jump target")
	}
}

func TestBuildUserNotificationsHidesRestrictedCommentBehindUnavailablePlaceholder(t *testing.T) {
	db := setupUserNotificationTest(t)
	viewer := models.User{Username: "viewer"}
	author := models.User{Username: "author"}
	actor := models.User{Username: "actor"}
	for _, user := range []*models.User{&viewer, &author, &actor} {
		if err := db.Create(user).Error; err != nil {
			t.Fatalf("create user: %v", err)
		}
	}
	message := models.Message{Content: "visible message", UserID: author.ID, Visibility: "public"}
	if err := db.Create(&message).Error; err != nil {
		t.Fatalf("create message: %v", err)
	}
	comment := models.Comment{MessageID: message.ID, UserID: &actor.ID, Content: "restricted comment", Visibility: "private"}
	if err := db.Create(&comment).Error; err != nil {
		t.Fatalf("create comment: %v", err)
	}
	notification := models.UserNotification{ID: 251, Type: models.UserNotificationTypeComment, RecipientUserID: viewer.ID, ActorUserID: &actor.ID, MessageID: &message.ID, CommentID: &comment.ID}

	items := buildVisibleUserNotifications([]models.UserNotification{notification}, viewer.ID)

	if len(items) != 1 || items[0].TargetStatus != userNotificationTargetStatusUnavailable {
		t.Fatalf("expected restricted-comment placeholder, got %#v", items)
	}
	if items[0].Comment != nil || items[0].Message != nil {
		t.Fatal("restricted-comment placeholder must not expose associated content")
	}
}

func TestUnavailableNotificationsSupportSingleAndBulkReadUpdates(t *testing.T) {
	db := setupUserNotificationTest(t)
	viewer := models.User{Username: "viewer"}
	if err := db.Create(&viewer).Error; err != nil {
		t.Fatalf("create viewer: %v", err)
	}
	missingMessageID := uint(260)
	notifications := []models.UserNotification{
		{Type: models.UserNotificationTypeComment, RecipientUserID: viewer.ID, MessageID: &missingMessageID},
		{Type: models.UserNotificationTypeReply, RecipientUserID: viewer.ID, MessageID: &missingMessageID},
	}
	if err := db.Create(&notifications).Error; err != nil {
		t.Fatalf("create notifications: %v", err)
	}
	if err := services.MarkUserNotificationRead(viewer.ID, notifications[0].ID); err != nil {
		t.Fatalf("mark one notification read: %v", err)
	}
	if err := db.Order("id").Find(&notifications).Error; err != nil {
		t.Fatalf("reload notifications: %v", err)
	}
	items := buildVisibleUserNotifications(notifications, viewer.ID)
	if len(items) != 2 || !items[0].Read || items[1].Read {
		t.Fatalf("expected only the selected placeholder to be read, got %#v", items)
	}

	if err := services.MarkAllUserNotificationsRead(viewer.ID); err != nil {
		t.Fatalf("mark all notifications read: %v", err)
	}
	if err := db.Order("id").Find(&notifications).Error; err != nil {
		t.Fatalf("reload notifications after bulk update: %v", err)
	}
	items = buildVisibleUserNotifications(notifications, viewer.ID)
	if countUnreadUserNotifications(items) != 0 {
		t.Fatalf("expected bulk read to include placeholders, got %#v", items)
	}
}
