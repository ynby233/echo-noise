package controllers

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/dto"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/services"
)

type userNotificationActorResponse struct {
	ID        uint   `json:"id"`
	Username  string `json:"username"`
	AvatarURL string `json:"avatar_url,omitempty"`
}

type userNotificationMessageResponse struct {
	ID          uint      `json:"id"`
	Content     string    `json:"content"`
	ImageURL    string    `json:"image_url,omitempty"`
	Visibility  string    `json:"visibility"`
	UserID      uint      `json:"user_id"`
	CreatedAt   time.Time `json:"created_at"`
	IsGuestbook bool      `json:"is_guestbook"`
}

type userNotificationCommentResponse struct {
	ID        uint                           `json:"id"`
	MessageID uint                           `json:"message_id"`
	UserID    *uint                          `json:"user_id,omitempty"`
	User      *userNotificationActorResponse `json:"user,omitempty"`
	Content   string                         `json:"content"`
	ParentID  *uint                          `json:"parent_id,omitempty"`
	CreatedAt time.Time                      `json:"created_at"`
}

type userNotificationResponse struct {
	ID              uint                             `json:"id"`
	Type            string                           `json:"type"`
	RecipientUserID uint                             `json:"recipient_user_id"`
	ActorUserID     *uint                            `json:"actor_user_id,omitempty"`
	Actor           *userNotificationActorResponse   `json:"actor,omitempty"`
	MessageID       *uint                            `json:"message_id,omitempty"`
	CommentID       *uint                            `json:"comment_id,omitempty"`
	ParentCommentID *uint                            `json:"parent_comment_id,omitempty"`
	Message         *userNotificationMessageResponse `json:"message,omitempty"`
	Comment         *userNotificationCommentResponse `json:"comment,omitempty"`
	ParentComment   *userNotificationCommentResponse `json:"parent_comment,omitempty"`
	TargetTab       string                           `json:"target_tab"`
	TargetURL       string                           `json:"target_url"`
	Read            bool                             `json:"read"`
	ReadAt          *time.Time                       `json:"read_at,omitempty"`
	CreatedAt       time.Time                        `json:"created_at"`
}

func notificationPtrValue(ptr *uint) uint {
	if ptr == nil {
		return 0
	}
	return *ptr
}

func uniqueNotificationIDs(values []uint) []uint {
	seen := map[uint]bool{}
	out := make([]uint, 0, len(values))
	for _, value := range values {
		if value == 0 || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func notificationPairKey(messageID uint, userID uint) string {
	return fmt.Sprintf("%d:%d", messageID, userID)
}

func notificationActorForUserID(userID *uint, users map[uint]models.User) *userNotificationActorResponse {
	if userID == nil || *userID == 0 {
		return nil
	}
	if user, ok := users[*userID]; ok {
		username := strings.TrimSpace(user.Username)
		if username == "" {
			username = fmt.Sprintf("用户%d", user.ID)
		}
		return &userNotificationActorResponse{ID: user.ID, Username: username, AvatarURL: strings.TrimSpace(user.AvatarURL)}
	}
	return &userNotificationActorResponse{ID: *userID, Username: fmt.Sprintf("用户%d", *userID)}
}

func notificationCommentResponse(comment models.Comment, users map[uint]models.User) userNotificationCommentResponse {
	return userNotificationCommentResponse{
		ID:        comment.ID,
		MessageID: comment.MessageID,
		UserID:    comment.UserID,
		User:      notificationActorForUserID(comment.UserID, users),
		Content:   comment.Content,
		ParentID:  comment.ParentID,
		CreatedAt: comment.CreatedAt,
	}
}

func notificationMessageResponse(message models.Message) userNotificationMessageResponse {
	return userNotificationMessageResponse{
		ID:          message.ID,
		Content:     message.Content,
		ImageURL:    strings.TrimSpace(message.ImageURL),
		Visibility:  services.StoredMessageVisibility(message),
		UserID:      message.UserID,
		CreatedAt:   message.CreatedAt,
		IsGuestbook: isGuestbookMessage(message),
	}
}

func notificationTarget(message models.Message, commentID *uint, notificationType string) (string, string) {
	tab := "latest"
	if isGuestbookMessage(message) && notificationType == models.UserNotificationTypeGuestbook {
		tab = "comment"
	}
	query := fmt.Sprintf("/?tab=%s&message_id=%d", tab, message.ID)
	if commentID != nil && *commentID != 0 {
		query += fmt.Sprintf("&comment_id=%d", *commentID)
	}
	return tab, query
}

func buildVisibleUserNotifications(notifications []models.UserNotification, viewerID uint, isAdmin bool) []userNotificationResponse {
	if len(notifications) == 0 {
		return []userNotificationResponse{}
	}
	db := database.DB
	messageIDs := make([]uint, 0, len(notifications))
	commentIDs := make([]uint, 0, len(notifications)*2)
	actorIDs := make([]uint, 0, len(notifications))
	likeMessageIDs := make([]uint, 0)
	likeActorIDs := make([]uint, 0)
	for _, notification := range notifications {
		if notification.MessageID != nil {
			messageIDs = append(messageIDs, *notification.MessageID)
		}
		if notification.CommentID != nil {
			commentIDs = append(commentIDs, *notification.CommentID)
		}
		if notification.ParentCommentID != nil {
			commentIDs = append(commentIDs, *notification.ParentCommentID)
		}
		if notification.ActorUserID != nil {
			actorIDs = append(actorIDs, *notification.ActorUserID)
			if notification.Type == models.UserNotificationTypeLike && notification.MessageID != nil {
				likeMessageIDs = append(likeMessageIDs, *notification.MessageID)
				likeActorIDs = append(likeActorIDs, *notification.ActorUserID)
			}
		}
	}

	messageMap := map[uint]models.Message{}
	messageIDs = uniqueNotificationIDs(messageIDs)
	if len(messageIDs) > 0 {
		var messages []models.Message
		if err := db.Where("id IN ?", messageIDs).Find(&messages).Error; err == nil {
			for _, message := range messages {
				messageMap[message.ID] = message
			}
		}
	}

	commentMap := map[uint]models.Comment{}
	commentIDs = uniqueNotificationIDs(commentIDs)
	if len(commentIDs) > 0 {
		var comments []models.Comment
		if err := db.Where("id IN ?", commentIDs).Find(&comments).Error; err == nil {
			for _, comment := range comments {
				commentMap[comment.ID] = comment
				if comment.ParentID != nil {
					commentIDs = append(commentIDs, *comment.ParentID)
				}
			}
		}
		commentIDs = uniqueNotificationIDs(commentIDs)
		if len(commentIDs) > len(commentMap) {
			var comments []models.Comment
			if err := db.Where("id IN ?", commentIDs).Find(&comments).Error; err == nil {
				for _, comment := range comments {
					commentMap[comment.ID] = comment
				}
			}
		}
	}

	userIDs := append([]uint{}, actorIDs...)
	for _, message := range messageMap {
		userIDs = append(userIDs, message.UserID)
	}
	for _, comment := range commentMap {
		if comment.UserID != nil {
			userIDs = append(userIDs, *comment.UserID)
		}
	}
	users := map[uint]models.User{}
	userIDs = uniqueNotificationIDs(userIDs)
	if len(userIDs) > 0 {
		var loadedUsers []models.User
		if err := db.Select("id, username, avatar_url").Where("id IN ?", userIDs).Find(&loadedUsers).Error; err == nil {
			for _, user := range loadedUsers {
				users[user.ID] = user
			}
		}
	}

	activeLikePairs := map[string]bool{}
	likeMessageIDs = uniqueNotificationIDs(likeMessageIDs)
	likeActorIDs = uniqueNotificationIDs(likeActorIDs)
	if len(likeMessageIDs) > 0 && len(likeActorIDs) > 0 {
		var likes []models.MessageLike
		if err := db.Where("message_id IN ? AND user_id IN ?", likeMessageIDs, likeActorIDs).Find(&likes).Error; err == nil {
			for _, like := range likes {
				if like.UserID != nil {
					activeLikePairs[notificationPairKey(like.MessageID, *like.UserID)] = true
				}
			}
		}
	}

	viewerIDPtr := viewerID
	items := make([]userNotificationResponse, 0, len(notifications))
	for _, notification := range notifications {
		messageID := notificationPtrValue(notification.MessageID)
		message, ok := messageMap[messageID]
		if !ok || !services.CanViewMessage(message, &viewerIDPtr, isAdmin) {
			continue
		}

		var commentResponse *userNotificationCommentResponse
		var parentCommentResponse *userNotificationCommentResponse
		valid := true
		switch notification.Type {
		case models.UserNotificationTypeLike:
			actorID := notificationPtrValue(notification.ActorUserID)
			if actorID == 0 || !activeLikePairs[notificationPairKey(message.ID, actorID)] {
				valid = false
			}
		case models.UserNotificationTypeComment, models.UserNotificationTypeReply, models.UserNotificationTypeGuestbook:
			commentID := notificationPtrValue(notification.CommentID)
			comment, ok := commentMap[commentID]
			if !ok || comment.MessageID != message.ID {
				valid = false
				break
			}
			var parent *models.Comment
			if comment.ParentID != nil {
				loaded, ok := commentMap[*comment.ParentID]
				if !ok || loaded.MessageID != message.ID {
					valid = false
					break
				}
				parent = &loaded
			}
			if notification.ParentCommentID != nil {
				loaded, ok := commentMap[*notification.ParentCommentID]
				if !ok || loaded.MessageID != message.ID {
					valid = false
					break
				}
				parent = &loaded
			}
			if notification.Type == models.UserNotificationTypeReply && comment.ParentID == nil {
				valid = false
				break
			}
			if notification.Type == models.UserNotificationTypeGuestbook && (!isGuestbookMessage(message) || comment.ParentID != nil) {
				valid = false
				break
			}
			if notification.Type == models.UserNotificationTypeComment && comment.ParentID != nil {
				valid = false
				break
			}
			if !canViewComment(message, comment, parent, viewerID, true, isAdmin) {
				valid = false
				break
			}
			cr := notificationCommentResponse(comment, users)
			commentResponse = &cr
			if parent != nil {
				pr := notificationCommentResponse(*parent, users)
				parentCommentResponse = &pr
			}
		default:
			valid = false
		}
		if !valid {
			continue
		}

		targetTab, targetURL := notificationTarget(message, notification.CommentID, notification.Type)
		messageSummary := notificationMessageResponse(message)
		items = append(items, userNotificationResponse{
			ID:              notification.ID,
			Type:            notification.Type,
			RecipientUserID: notification.RecipientUserID,
			ActorUserID:     notification.ActorUserID,
			Actor:           notificationActorForUserID(notification.ActorUserID, users),
			MessageID:       notification.MessageID,
			CommentID:       notification.CommentID,
			ParentCommentID: notification.ParentCommentID,
			Message:         &messageSummary,
			Comment:         commentResponse,
			ParentComment:   parentCommentResponse,
			TargetTab:       targetTab,
			TargetURL:       targetURL,
			Read:            notification.ReadAt != nil,
			ReadAt:          notification.ReadAt,
			CreatedAt:       notification.CreatedAt,
		})
	}
	return items
}

func parseNotificationPagination(c *gin.Context) (int, int) {
	page, _ := strconv.Atoi(c.Query("page"))
	if page <= 0 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(c.Query("pageSize"))
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}

func countUnreadUserNotifications(items []userNotificationResponse) int {
	count := 0
	for _, item := range items {
		if !item.Read {
			count++
		}
	}
	return count
}

func ListUserNotifications(c *gin.Context) {
	user, err := checkUser(c)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[any](err.Error()))
		return
	}
	page, pageSize := parseNotificationPagination(c)
	var notifications []models.UserNotification
	if err := database.DB.Where("recipient_user_id = ?", user.ID).Order("created_at DESC, id DESC").Find(&notifications).Error; err != nil {
		c.JSON(http.StatusOK, dto.Fail[any]("获取通知失败"))
		return
	}
	visibleItems := buildVisibleUserNotifications(notifications, user.ID, user.IsAdmin)
	total := len(visibleItems)
	unreadCount := countUnreadUserNotifications(visibleItems)
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	items := visibleItems[start:end]
	c.JSON(http.StatusOK, dto.OK(gin.H{
		"items":        items,
		"total":        total,
		"unread_count": unreadCount,
		"unreadCount":  unreadCount,
		"page":         page,
		"pageSize":     pageSize,
	}, "获取成功"))
}

func GetUserNotificationUnreadCount(c *gin.Context) {
	user, err := checkUser(c)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[any](err.Error()))
		return
	}
	var notifications []models.UserNotification
	if err := database.DB.Where("recipient_user_id = ? AND read_at IS NULL", user.ID).Order("created_at DESC, id DESC").Find(&notifications).Error; err != nil {
		c.JSON(http.StatusOK, dto.Fail[any]("获取通知失败"))
		return
	}
	visibleItems := buildVisibleUserNotifications(notifications, user.ID, user.IsAdmin)
	unreadCount := countUnreadUserNotifications(visibleItems)
	c.JSON(http.StatusOK, dto.OK(gin.H{"unread_count": unreadCount, "unreadCount": unreadCount}, "获取成功"))
}

func MarkUserNotificationRead(c *gin.Context) {
	user, err := checkUser(c)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[any](err.Error()))
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, dto.Fail[any]("无效的通知ID"))
		return
	}
	if err := services.MarkUserNotificationRead(user.ID, uint(id)); err != nil {
		c.JSON(http.StatusOK, dto.Fail[any]("更新通知失败"))
		return
	}
	c.JSON(http.StatusOK, dto.OK[any](nil, "已读"))
}

func MarkAllUserNotificationsRead(c *gin.Context) {
	user, err := checkUser(c)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[any](err.Error()))
		return
	}
	if err := services.MarkAllUserNotificationsRead(user.ID); err != nil {
		c.JSON(http.StatusOK, dto.Fail[any]("更新通知失败"))
		return
	}
	c.JSON(http.StatusOK, dto.OK[any](nil, "已全部已读"))
}
