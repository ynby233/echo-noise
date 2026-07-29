package controllers

import (
	"errors"
	"testing"
	"time"

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
	if err := db.AutoMigrate(&models.User{}, &models.Message{}, &models.MessageLike{}, &models.Comment{}, &models.UserNotification{}); err != nil {
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

	items := buildVisibleUserNotifications([]models.UserNotification{notification}, viewerID, false)

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

	items := buildVisibleUserNotifications([]models.UserNotification{notification}, viewer.ID, false)

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

	items := buildVisibleUserNotifications([]models.UserNotification{notification}, viewer.ID, false)

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

	items := buildVisibleUserNotifications([]models.UserNotification{notification}, viewerID, false)

	if len(items) != 1 {
		t.Fatalf("expected the notification to remain visible, got %d items", len(items))
	}
	if items[0].TargetStatus != userNotificationTargetStatusMessageDeleted {
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

	items := buildVisibleUserNotifications([]models.UserNotification{notification}, viewer.ID, false)

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

	items := buildVisibleUserNotifications([]models.UserNotification{notification}, viewer.ID, false)

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

	items := buildVisibleUserNotifications(notifications, viewerID, false)

	if len(items) != len(types) {
		t.Fatalf("expected all missing-message notifications to remain visible, got %d", len(items))
	}
	for _, item := range items {
		if item.TargetStatus != userNotificationTargetStatusMessageDeleted {
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

	items := buildVisibleUserNotifications([]models.UserNotification{notification}, viewer.ID, false)

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

	items := buildVisibleUserNotifications([]models.UserNotification{notification}, viewer.ID, false)

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

	items := buildVisibleUserNotifications(notifications, viewerID, false)

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

	items := buildVisibleUserNotifications([]models.UserNotification{notification}, viewer.ID, false)

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

	items := buildVisibleUserNotifications([]models.UserNotification{notification}, viewer.ID, false)

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
	items := buildVisibleUserNotifications(notifications, viewer.ID, false)
	if len(items) != 2 || !items[0].Read || items[1].Read {
		t.Fatalf("expected only the selected placeholder to be read, got %#v", items)
	}

	if err := services.MarkAllUserNotificationsRead(viewer.ID); err != nil {
		t.Fatalf("mark all notifications read: %v", err)
	}
	if err := db.Order("id").Find(&notifications).Error; err != nil {
		t.Fatalf("reload notifications after bulk update: %v", err)
	}
	items = buildVisibleUserNotifications(notifications, viewer.ID, false)
	if countUnreadUserNotifications(items) != 0 {
		t.Fatalf("expected bulk read to include placeholders, got %#v", items)
	}
}
