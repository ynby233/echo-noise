package services

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/vocechat"
	"gorm.io/gorm"
)

func isGuestbookNotificationMessage(message models.Message) bool {
	content := strings.ToLower(strings.TrimSpace(message.Content))
	return strings.Contains(content, "#guestbook") || strings.Contains(content, "#留言") || strings.Contains(content, "留言板")
}

func createUserNotification(recipientUserID uint, actorUserID *uint, notificationType string, messageID *uint, commentID *uint, parentCommentID *uint) error {
	if recipientUserID == 0 {
		return nil
	}
	if actorUserID != nil && *actorUserID == recipientUserID {
		return nil
	}
	notification := models.UserNotification{
		RecipientUserID: recipientUserID,
		ActorUserID:     actorUserID,
		Type:            notificationType,
		MessageID:       messageID,
		CommentID:       commentID,
		ParentCommentID: parentCommentID,
	}
	if err := database.DB.Create(&notification).Error; err != nil {
		return err
	}
	queueUserNotificationPush(notification.ID)
	return nil
}

func refreshLikeNotification(recipientUserID uint, actorUserID uint, messageID uint) error {
	if recipientUserID == 0 || actorUserID == 0 || recipientUserID == actorUserID || messageID == 0 {
		return nil
	}
	actorID := actorUserID
	msgID := messageID
	var existing models.UserNotification
	err := database.DB.Where("recipient_user_id = ? AND actor_user_id = ? AND type = ? AND message_id = ?", recipientUserID, actorUserID, models.UserNotificationTypeLike, messageID).First(&existing).Error
	if err == nil && existing.ID != 0 {
		now := time.Now()
		if err := database.DB.Model(&existing).Updates(map[string]interface{}{
			"read_at":    nil,
			"created_at": now,
			"updated_at": now,
		}).Error; err != nil {
			return err
		}
		queueUserNotificationPush(existing.ID)
		return nil
	}
	if err != nil && err != gorm.ErrRecordNotFound {
		return err
	}
	return createUserNotification(recipientUserID, &actorID, models.UserNotificationTypeLike, &msgID, nil, nil)
}

func CreateNotificationForLike(messageID uint, actorUserID uint) error {
	if messageID == 0 || actorUserID == 0 {
		return nil
	}
	var message models.Message
	if err := database.DB.Select("id, user_id").First(&message, messageID).Error; err != nil {
		return err
	}
	return refreshLikeNotification(message.UserID, actorUserID, message.ID)
}

func CreateNotificationsForComment(message models.Message, comment models.Comment, parent *models.Comment) error {
	if message.ID == 0 || comment.ID == 0 || comment.UserID == nil || *comment.UserID == 0 {
		return nil
	}
	actorID := *comment.UserID
	messageID := message.ID
	commentID := comment.ID
	commentMap, err := LoadCommentMapForMessage(message.ID)
	if err != nil {
		return err
	}
	commentMap[comment.ID] = comment
	if parent != nil {
		commentMap[parent.ID] = *parent
	}

	if isGuestbookNotificationMessage(message) && comment.ParentID == nil {
		var admins []models.User
		if err := database.DB.Select("id").Where("is_admin = ?", true).Find(&admins).Error; err != nil {
			return err
		}
		for _, admin := range admins {
			if admin.ID == 0 || admin.ID == actorID {
				continue
			}
			if err := createUserNotification(admin.ID, &actorID, models.UserNotificationTypeGuestbook, &messageID, &commentID, nil); err != nil {
				return err
			}
		}
		return nil
	}

	if comment.ParentID != nil {
		parentID := *comment.ParentID
		if parent != nil {
			parentID = parent.ID
			if parent.UserID != nil && *parent.UserID != 0 && CanViewCommentInThread(message, comment, commentMap, *parent.UserID, true, false) {
				return createUserNotification(*parent.UserID, &actorID, models.UserNotificationTypeReply, &messageID, &commentID, &parentID)
			}
		}
		return nil
	}

	if CanViewCommentInThread(message, comment, commentMap, message.UserID, true, false) {
		return createUserNotification(message.UserID, &actorID, models.UserNotificationTypeComment, &messageID, &commentID, nil)
	}
	return nil
}

func MarkUserNotificationRead(userID uint, notificationID uint) error {
	if userID == 0 || notificationID == 0 {
		return nil
	}
	now := time.Now()
	return database.DB.Model(&models.UserNotification{}).
		Where("id = ? AND recipient_user_id = ? AND read_at IS NULL", notificationID, userID).
		Update("read_at", &now).Error
}

func MarkAllUserNotificationsRead(userID uint) error {
	if userID == 0 {
		return nil
	}
	now := time.Now()
	return database.DB.Model(&models.UserNotification{}).
		Where("recipient_user_id = ? AND read_at IS NULL", userID).
		Update("read_at", &now).Error
}

func queueUserNotificationPush(notificationID uint) {
	if notificationID == 0 {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := sendUserNotificationToVoceChat(ctx, notificationID); err != nil {
			log.Printf("VoceChat 通知推送失败: %v", err)
		}
	}()
}

func sendUserNotificationToVoceChat(ctx context.Context, notificationID uint) error {
	var notification models.UserNotification
	if err := database.DB.First(&notification, notificationID).Error; err != nil {
		return err
	}

	var recipient models.User
	if err := database.DB.Select("id, username, is_admin, voce_chat_user_id, voce_chat_notification_enabled").First(&recipient, notification.RecipientUserID).Error; err != nil {
		return err
	}
	if !recipient.VoceChatNotificationEnabled {
		return nil
	}

	var siteConfig models.SiteConfig
	if err := database.DB.First(&siteConfig).Error; err != nil {
		return err
	}
	vcConfig := vocechat.FromSiteConfig(siteConfig)
	if !vcConfig.IsNotificationReady() {
		return nil
	}
	client, err := vocechat.NewClient(vcConfig)
	if err != nil {
		return err
	}
	recipientVoceChatUserID, err := resolveNotificationRecipientVoceChatUserID(ctx, client, vcConfig, recipient)
	if err != nil {
		return err
	}
	if recipientVoceChatUserID == "" {
		return nil
	}

	markdown := buildVoceChatNotificationMarkdown(siteConfig, notification)
	if strings.TrimSpace(markdown) == "" {
		return nil
	}
	return client.SendMarkdownToUser(ctx, vcConfig.BotAPIKey, recipientVoceChatUserID, markdown)
}

func resolveNotificationRecipientVoceChatUserID(ctx context.Context, client *vocechat.Client, vcConfig vocechat.Config, recipient models.User) (string, error) {
	if uid := strings.TrimSpace(recipient.VoceChatUserID); uid != "" {
		return uid, nil
	}
	if recipient.ID != models.PrimaryAdminUserID {
		return "", nil
	}
	adminEmail := strings.TrimSpace(vcConfig.AdminUsername)
	if adminEmail == "" || !vcConfig.HasAdminCredential() || client == nil {
		return "", nil
	}
	tokenManager := vocechat.NewAdminTokenManager(client, vcConfig)
	apiKey, err := tokenManager.GetToken(ctx)
	if err != nil {
		return "", err
	}
	users, err := client.ListUsers(ctx, apiKey)
	if err != nil {
		return "", err
	}
	for _, user := range users {
		if user.UID > 0 && strings.EqualFold(strings.TrimSpace(user.Email), adminEmail) {
			return strconv.FormatInt(user.UID, 10), nil
		}
	}
	return "", nil
}

func buildVoceChatNotificationMarkdown(siteConfig models.SiteConfig, notification models.UserNotification) string {
	siteTitle := strings.TrimSpace(siteConfig.SiteTitle)
	if siteTitle == "" {
		siteTitle = "Echo Noise"
	}
	actorName := notificationActorName(notification.ActorUserID)
	title := userNotificationPushTitle(notification.Type, actorName)
	if title == "" {
		return ""
	}

	snippet := userNotificationPushSnippet(notification)
	lines := []string{fmt.Sprintf("**%s 通知**", siteTitle), "", title}
	if snippet != "" {
		lines = append(lines, "", "> "+strings.ReplaceAll(snippet, "\n", "\n> "))
	}
	if link := userNotificationPushURL(siteConfig.CommentEmailSiteURL, notification.MessageID, notification.CommentID); link != "" {
		lines = append(lines, "", fmt.Sprintf("[查看通知](%s)", link))
	}
	return strings.Join(lines, "\n")
}

func notificationActorName(actorUserID *uint) string {
	if actorUserID == nil || *actorUserID == 0 {
		return "有人"
	}
	var actor models.User
	if err := database.DB.Select("id, username").First(&actor, *actorUserID).Error; err != nil || strings.TrimSpace(actor.Username) == "" {
		return "有人"
	}
	return strings.TrimSpace(actor.Username)
}

func userNotificationPushTitle(notificationType string, actorName string) string {
	switch notificationType {
	case models.UserNotificationTypeComment:
		return fmt.Sprintf("%s 评论了你的动态", actorName)
	case models.UserNotificationTypeReply:
		return fmt.Sprintf("%s 回复了你", actorName)
	case models.UserNotificationTypeGuestbook:
		return fmt.Sprintf("%s 发表了留言", actorName)
	case models.UserNotificationTypeLike:
		return fmt.Sprintf("%s 点赞了你的动态", actorName)
	default:
		return ""
	}
}

func userNotificationPushSnippet(notification models.UserNotification) string {
	if notification.CommentID != nil && *notification.CommentID != 0 {
		var comment models.Comment
		if err := database.DB.Select("id, content").First(&comment, *notification.CommentID).Error; err == nil {
			return compactNotificationText(comment.Content, 120)
		}
	}
	if notification.MessageID != nil && *notification.MessageID != 0 {
		var message models.Message
		if err := database.DB.Select("id, content").First(&message, *notification.MessageID).Error; err == nil {
			return compactNotificationText(message.Content, 120)
		}
	}
	return ""
}

func compactNotificationText(text string, limit int) string {
	text = strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	if text == "" || limit <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + "..."
}

func userNotificationPushURL(base string, messageID *uint, commentID *uint) string {
	base = strings.TrimSpace(base)
	if base == "" {
		return ""
	}
	u, err := url.Parse(base)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	if u.Path == "" {
		u.Path = "/"
	}
	q := u.Query()
	q.Set("tab", "notifications")
	if messageID != nil && *messageID != 0 {
		q.Set("message_id", fmt.Sprintf("%d", *messageID))
	}
	if commentID != nil && *commentID != 0 {
		q.Set("comment_id", fmt.Sprintf("%d", *commentID))
	}
	u.RawQuery = q.Encode()
	return u.String()
}
