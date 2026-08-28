package services

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/vocechat"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var primaryAdminVoceChatCredentialAlertMu sync.Mutex
var voceChatPasswordChangedAlertMu sync.Mutex
var passwordUpdateIncompleteAlertMu sync.Mutex
var resolvedPasswordAlertCleanupRetry = struct {
	sync.Mutex
	users map[uint]struct{}
}{users: make(map[uint]struct{})}

func setResolvedPasswordAlertCleanupPending(userID uint, pending bool) error {
	if database.DB == nil || userID == 0 {
		return nil
	}
	var err error
	if pending {
		err = database.DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&models.PasswordAlertCleanupTask{UserID: userID}).Error
	} else {
		err = database.DB.Where("user_id = ?", userID).Delete(&models.PasswordAlertCleanupTask{}).Error
	}
	if err != nil {
		return err
	}
	resolvedPasswordAlertCleanupRetry.Lock()
	defer resolvedPasswordAlertCleanupRetry.Unlock()
	if pending {
		resolvedPasswordAlertCleanupRetry.users[userID] = struct{}{}
		return nil
	}
	delete(resolvedPasswordAlertCleanupRetry.users, userID)
	return nil
}

func hasResolvedPasswordAlertCleanupPending(userID uint) (bool, error) {
	if database.DB == nil || userID == 0 {
		return false, nil
	}
	var count int64
	if err := database.DB.Model(&models.PasswordAlertCleanupTask{}).Where("user_id = ?", userID).Count(&count).Error; err != nil {
		return false, err
	}
	pending := count != 0
	resolvedPasswordAlertCleanupRetry.Lock()
	defer resolvedPasswordAlertCleanupRetry.Unlock()
	if pending {
		resolvedPasswordAlertCleanupRetry.users[userID] = struct{}{}
	} else {
		delete(resolvedPasswordAlertCleanupRetry.users, userID)
	}
	return pending, nil
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

func CreatePrimaryAdminVoceChatCredentialAlertOnce() {
	if database.DB == nil {
		return
	}
	primaryAdminVoceChatCredentialAlertMu.Lock()
	defer primaryAdminVoceChatCredentialAlertMu.Unlock()
	var count int64
	_ = database.DB.Model(&models.UserNotification{}).Where("recipient_user_id = ? AND type = ?", models.PrimaryAdminUserID, models.UserNotificationTypeVoceChatCredentials).Count(&count).Error
	if count == 0 {
		_ = database.DB.Create(&models.UserNotification{RecipientUserID: models.PrimaryAdminUserID, Type: models.UserNotificationTypeVoceChatCredentials}).Error
	}
}

func ResolvePrimaryAdminVoceChatCredentialAlert() {
	if database.DB == nil {
		return
	}
	primaryAdminVoceChatCredentialAlertMu.Lock()
	defer primaryAdminVoceChatCredentialAlertMu.Unlock()
	_ = database.DB.Where("recipient_user_id = ? AND type = ?", models.PrimaryAdminUserID, models.UserNotificationTypeVoceChatCredentials).Delete(&models.UserNotification{}).Error
}

func CreateVoceChatPasswordChangedAlertOnce(recipientUserID uint) {
	if database.DB == nil || recipientUserID == 0 || recipientUserID == models.PrimaryAdminUserID {
		return
	}
	if err := setResolvedPasswordAlertCleanupPending(recipientUserID, false); err != nil {
		log.Printf("password alert cleanup cancellation failed: user_id=%d alert_type=vocechat_password_changed error_type=%T", recipientUserID, err)
		return
	}
	voceChatPasswordChangedAlertMu.Lock()
	defer voceChatPasswordChangedAlertMu.Unlock()
	var count int64
	_ = database.DB.Model(&models.UserNotification{}).Where("recipient_user_id = ? AND type = ?", recipientUserID, models.UserNotificationTypeVoceChatPasswordChanged).Count(&count).Error
	if count == 0 {
		_ = database.DB.Create(&models.UserNotification{RecipientUserID: recipientUserID, Type: models.UserNotificationTypeVoceChatPasswordChanged}).Error
	}
}

func ResolveVoceChatPasswordChangedAlert(recipientUserID uint) error {
	if database.DB == nil || recipientUserID == 0 || recipientUserID == models.PrimaryAdminUserID {
		return nil
	}
	voceChatPasswordChangedAlertMu.Lock()
	defer voceChatPasswordChangedAlertMu.Unlock()
	return database.DB.Where("recipient_user_id = ? AND type = ?", recipientUserID, models.UserNotificationTypeVoceChatPasswordChanged).Delete(&models.UserNotification{}).Error
}

func CreatePasswordUpdateIncompleteAlertOnce(recipientUserID uint) error {
	if database.DB == nil || recipientUserID == 0 || recipientUserID == models.PrimaryAdminUserID {
		return nil
	}
	if err := setResolvedPasswordAlertCleanupPending(recipientUserID, false); err != nil {
		return err
	}
	passwordUpdateIncompleteAlertMu.Lock()
	defer passwordUpdateIncompleteAlertMu.Unlock()
	var count int64
	if err := database.DB.Model(&models.UserNotification{}).Where("recipient_user_id = ? AND type = ?", recipientUserID, models.UserNotificationTypePasswordUpdateIncomplete).Count(&count).Error; err != nil {
		return err
	}
	if count != 0 {
		return nil
	}
	return database.DB.Create(&models.UserNotification{RecipientUserID: recipientUserID, Type: models.UserNotificationTypePasswordUpdateIncomplete}).Error
}

func ResolvePasswordUpdateIncompleteAlert(recipientUserID uint) error {
	if database.DB == nil || recipientUserID == 0 || recipientUserID == models.PrimaryAdminUserID {
		return nil
	}
	passwordUpdateIncompleteAlertMu.Lock()
	defer passwordUpdateIncompleteAlertMu.Unlock()
	return database.DB.Where("recipient_user_id = ? AND type = ?", recipientUserID, models.UserNotificationTypePasswordUpdateIncomplete).Delete(&models.UserNotification{}).Error
}

func ReconcileResolvedPasswordAlerts(recipientUserID uint) error {
	if database.DB == nil || recipientUserID == 0 || recipientUserID == models.PrimaryAdminUserID {
		return nil
	}
	if err := setResolvedPasswordAlertCleanupPending(recipientUserID, true); err != nil {
		return err
	}

	var user models.User
	if err := database.DB.Select("id", "voce_chat_sync_status", "voce_chat_sync_error").First(&user, recipientUserID).Error; err != nil {
		return err
	}
	if user.VoceChatSyncStatus == models.VoceChatSyncStatusConflicted && user.VoceChatSyncError == "password_update_incomplete" {
		return setResolvedPasswordAlertCleanupPending(recipientUserID, false)
	}
	if err := ResolveVoceChatPasswordChangedAlert(recipientUserID); err != nil {
		return err
	}
	if err := ResolvePasswordUpdateIncompleteAlert(recipientUserID); err != nil {
		return err
	}
	return setResolvedPasswordAlertCleanupPending(recipientUserID, false)
}

func ReconcilePendingResolvedPasswordAlerts(recipientUserID uint) error {
	pending, err := hasResolvedPasswordAlertCleanupPending(recipientUserID)
	if err != nil {
		return err
	}
	if !pending {
		return nil
	}
	return ReconcileResolvedPasswordAlerts(recipientUserID)
}

func reconcileResolvedPasswordAlertsBestEffort(recipientUserID uint, trigger string) {
	if err := ReconcileResolvedPasswordAlerts(recipientUserID); err != nil {
		log.Printf("password alert reconciliation failed: user_id=%d trigger=%s error_type=%T", recipientUserID, trigger, err)
	}
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

	if IsGuestbookMessage(message) && comment.ParentID == nil {
		if actorID == GuestbookRecipientID() {
			return nil
		}
		var existing models.UserNotification
		result := database.DB.Where("recipient_user_id = ? AND type = ? AND message_id = ? AND comment_id = ?", GuestbookRecipientID(), models.UserNotificationTypeGuestbook, messageID, commentID).Limit(1).Find(&existing)
		if result.Error != nil {
			return result.Error
		}
		if existing.ID != 0 {
			return nil
		}
		if err := createUserNotification(GuestbookRecipientID(), &actorID, models.UserNotificationTypeGuestbook, &messageID, &commentID, nil); err != nil {
			return err
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
	db, err := database.GetDB()
	if err != nil {
		return err
	}
	var notification models.UserNotification
	if err := db.First(&notification, notificationID).Error; err != nil {
		return err
	}

	var recipient models.User
	if err := db.Select("id, username, is_admin, voce_chat_user_id, voce_chat_notification_enabled").First(&recipient, notification.RecipientUserID).Error; err != nil {
		return err
	}
	if !recipient.VoceChatNotificationEnabled {
		return nil
	}

	var siteConfig models.SiteConfig
	if err := db.First(&siteConfig).Error; err != nil {
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
		if isVoceChatSiteTransientFailure(err) {
			recordVoceChatLoginHealth("failed", errors.New("VoceChat 推送服务暂不可用"))
		}
		return err
	}
	if recipientVoceChatUserID == "" {
		return nil
	}

	markdown := buildVoceChatNotificationMarkdown(siteConfig, notification)
	if strings.TrimSpace(markdown) == "" {
		return nil
	}
	if err := client.SendMarkdownToUser(ctx, vcConfig.BotAPIKey, recipientVoceChatUserID, markdown); err != nil {
		if isVoceChatSiteTransientFailure(err) {
			recordVoceChatLoginHealth("failed", errors.New("VoceChat 推送服务暂不可用"))
		}
		return err
	}
	recordVoceChatLoginHealth("ok", nil)
	return nil
}

func resolveNotificationRecipientVoceChatUserID(ctx context.Context, client *vocechat.Client, vcConfig vocechat.Config, recipient models.User) (string, error) {
	if uid := strings.TrimSpace(recipient.VoceChatUserID); uid != "" {
		if recipient.ID == models.PrimaryAdminUserID {
			record, ok, err := vocechat.DefaultPlainPasswordStore().GetUserPassword(recipient.ID)
			if err != nil {
				return "", err
			}
			if !ok || strings.TrimSpace(record.VoceChatPasswordValue()) == "" {
				CreatePrimaryAdminVoceChatCredentialAlertOnce()
				return "", nil
			}
			login, err := voceChatPasswordLogin(ctx, vcConfig, strings.TrimSpace(recipient.VoceChatEmail), record.VoceChatPasswordValue())
			if err != nil {
				if isVoceChatAccountCredentialInvalid(err) {
					CreatePrimaryAdminVoceChatCredentialAlertOnce()
				}
				return "", err
			}
			if login == nil || strconv.FormatInt(login.User.UID, 10) != uid || !strings.EqualFold(strings.TrimSpace(login.User.Email), strings.TrimSpace(recipient.VoceChatEmail)) {
				CreatePrimaryAdminVoceChatCredentialAlertOnce()
				return "", nil
			}
		}
		return uid, nil
	}
	if recipient.ID == models.PrimaryAdminUserID {
		CreatePrimaryAdminVoceChatCredentialAlertOnce()
	}
	return "", nil
}

func buildVoceChatNotificationMarkdown(siteConfig models.SiteConfig, notification models.UserNotification) string {
	siteTitle := strings.TrimSpace(siteConfig.SiteTitle)
	if siteTitle == "" {
		siteTitle = neutralSiteTitle
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
	if link := userNotificationPushURL(siteConfig.SitePublicURL, notification.MessageID, notification.CommentID); link != "" {
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
