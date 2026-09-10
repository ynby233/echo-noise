// User-notification controllers translate the query service's target states
// into the public response shape. Eligibility, counts and pagination belong in
// notification_query_service.go; this file only hydrates the selected page for
// display and preserves unavailable/load-error placeholders.
package controllers

import (
	"encoding/json"
	"fmt"
	"log"
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
	CanInteract bool      `json:"can_interact"`
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

const (
	userNotificationTargetStatusAvailable   = services.UserNotificationTargetStatusAvailable
	userNotificationTargetStatusSnapshot    = services.UserNotificationTargetStatusSnapshot
	userNotificationTargetStatusLoadError   = services.UserNotificationTargetStatusLoadError
	userNotificationTargetStatusUnavailable = services.UserNotificationTargetStatusUnavailable
)

type userNotificationResponse struct {
	ID                  uint                             `json:"id"`
	Type                string                           `json:"type"`
	RecipientUserID     uint                             `json:"recipient_user_id"`
	ActorUserID         *uint                            `json:"actor_user_id,omitempty"`
	Actor               *userNotificationActorResponse   `json:"actor,omitempty"`
	MessageID           *uint                            `json:"message_id,omitempty"`
	CommentID           *uint                            `json:"comment_id,omitempty"`
	ParentCommentID     *uint                            `json:"parent_comment_id,omitempty"`
	Message             *userNotificationMessageResponse `json:"message,omitempty"`
	Comment             *userNotificationCommentResponse `json:"comment,omitempty"`
	ParentComment       *userNotificationCommentResponse `json:"parent_comment,omitempty"`
	TargetStatus        string                           `json:"target_status"`
	TargetTab           string                           `json:"target_tab"`
	TargetURL           string                           `json:"target_url"`
	Read                bool                             `json:"read"`
	ReadAt              *time.Time                       `json:"read_at,omitempty"`
	CreatedAt           time.Time                        `json:"created_at"`
	DeletionEvent       string                           `json:"deletion_event,omitempty"`
	DeletionReason      string                           `json:"deletion_reason,omitempty"`
	DeletionActorLabel  string                           `json:"deletion_actor_label,omitempty"`
	DeletionSnapshots   []services.DeletionSnapshotItem  `json:"deletion_snapshots,omitempty"`
	ScheduledDeletionAt *time.Time                       `json:"scheduled_deletion_at,omitempty"`
	RestoredAt          *time.Time                       `json:"restored_at,omitempty"`
}

func deletionNotificationResponse(notification models.UserNotification, cfg models.SiteConfig) userNotificationResponse {
	snapshots := []services.DeletionSnapshotItem{}
	_ = json.Unmarshal([]byte(notification.DeletionSnapshotJSON), &snapshots)
	now := time.Now().UTC()
	var earliest *time.Time
	for i := range snapshots {
		retention := cfg.CommentRecycleBinRetentionDays
		if snapshots[i].TargetType == "note" {
			retention = cfg.RecycleBinRetentionDays
		}
		deadline := services.CalculateRecycleDeadline(snapshots[i].DeletedAt, retention, now)
		if notification.DeletionEvent == services.DeletionEventPermanentlyDeleted {
			snapshots[i].ScheduledDeletionAt = nil
		} else {
			snapshots[i].ScheduledDeletionAt = deadline.ScheduledDeletionAt
		}
		if snapshots[i].ScheduledDeletionAt != nil && (earliest == nil || snapshots[i].ScheduledDeletionAt.Before(*earliest)) {
			value := *snapshots[i].ScheduledDeletionAt
			earliest = &value
		}
	}
	return userNotificationResponse{
		ID: notification.ID, Type: notification.Type, RecipientUserID: notification.RecipientUserID,
		TargetStatus: userNotificationTargetStatusSnapshot, Read: notification.ReadAt != nil,
		ReadAt: notification.ReadAt, CreatedAt: notification.CreatedAt,
		DeletionEvent: notification.DeletionEvent, DeletionReason: notification.DeletionReason,
		DeletionActorLabel: notification.DeletionActorLabel, DeletionSnapshots: snapshots,
		ScheduledDeletionAt: earliest, RestoredAt: notification.RestoredAt,
	}
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

func notificationMessageResponse(message models.Message, canInteract bool) userNotificationMessageResponse {
	return userNotificationMessageResponse{
		ID:          message.ID,
		Content:     message.Content,
		ImageURL:    strings.TrimSpace(message.ImageURL),
		Visibility:  services.StoredMessageVisibility(message),
		UserID:      message.UserID,
		CreatedAt:   message.CreatedAt,
		IsGuestbook: isGuestbookMessage(message),
		CanInteract: canInteract,
	}
}

func notificationUnavailableResponse(notification models.UserNotification, users map[uint]models.User, targetStatus string) userNotificationResponse {
	return userNotificationResponse{
		ID:              notification.ID,
		Type:            notification.Type,
		RecipientUserID: notification.RecipientUserID,
		ActorUserID:     notification.ActorUserID,
		Actor:           notificationActorForUserID(notification.ActorUserID, users),
		MessageID:       notification.MessageID,
		CommentID:       notification.CommentID,
		ParentCommentID: notification.ParentCommentID,
		TargetStatus:    targetStatus,
		Read:            notification.ReadAt != nil,
		ReadAt:          notification.ReadAt,
		CreatedAt:       notification.CreatedAt,
	}
}

func buildVisibleUserNotifications(notifications []models.UserNotification, viewerID uint) []userNotificationResponse {
	hydration := services.HydrateUserNotifications(database.DB, viewerID, notifications)
	return userNotificationResponses(hydration)
}

func userNotificationResponses(hydration services.UserNotificationHydration) []userNotificationResponse {
	items := make([]userNotificationResponse, 0, len(hydration.Items))
	var retentionConfig models.SiteConfig
	for _, target := range hydration.Items {
		if target.TargetStatus == userNotificationTargetStatusSnapshot {
			_ = database.DB.Table("site_configs").First(&retentionConfig).Error
			break
		}
	}
	for _, target := range hydration.Items {
		notification := target.Notification
		if target.TargetStatus == userNotificationTargetStatusSnapshot {
			items = append(items, deletionNotificationResponse(notification, retentionConfig))
			continue
		}
		if target.TargetStatus != userNotificationTargetStatusAvailable || target.Message == nil {
			items = append(items, notificationUnavailableResponse(notification, hydration.Users, target.TargetStatus))
			continue
		}
		var commentResponse *userNotificationCommentResponse
		if target.Comment != nil {
			response := notificationCommentResponse(*target.Comment, hydration.Users)
			commentResponse = &response
		}
		var parentCommentResponse *userNotificationCommentResponse
		if target.ParentComment != nil {
			response := notificationCommentResponse(*target.ParentComment, hydration.Users)
			parentCommentResponse = &response
		}
		messageSummary := notificationMessageResponse(*target.Message, target.CanInteract)
		items = append(items, userNotificationResponse{
			ID:              notification.ID,
			Type:            notification.Type,
			RecipientUserID: notification.RecipientUserID,
			ActorUserID:     notification.ActorUserID,
			Actor:           notificationActorForUserID(notification.ActorUserID, hydration.Users),
			MessageID:       notification.MessageID,
			CommentID:       notification.CommentID,
			ParentCommentID: notification.ParentCommentID,
			Message:         &messageSummary,
			Comment:         commentResponse,
			ParentComment:   parentCommentResponse,
			TargetStatus:    userNotificationTargetStatusAvailable,
			TargetTab:       target.TargetTab,
			TargetURL:       target.TargetURL,
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
	if err := services.ReconcilePendingResolvedPasswordAlerts(user.ID); err != nil {
		log.Printf("password alert reconciliation failed: user_id=%d trigger=notification_list error_type=%T", user.ID, err)
	}
	page, pageSize := parseNotificationPagination(c)
	pageResult, err := services.QueryUserNotificationPage(database.DB, user.ID, page, pageSize)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[any]("获取通知失败"))
		return
	}
	items := userNotificationResponses(pageResult.Hydration)
	c.JSON(http.StatusOK, dto.OK(gin.H{
		"items":        items,
		"total":        pageResult.Total,
		"unread_count": pageResult.UnreadCount,
		"unreadCount":  pageResult.UnreadCount,
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
	if err := services.ReconcilePendingResolvedPasswordAlerts(user.ID); err != nil {
		log.Printf("password alert reconciliation failed: user_id=%d trigger=notification_unread_count error_type=%T", user.ID, err)
	}
	unreadCount, err := services.CountUnreadUserNotificationsForViewer(database.DB, user.ID)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[any]("获取通知失败"))
		return
	}
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
