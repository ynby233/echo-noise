package services

import (
	"fmt"
	"log"
	"strings"

	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
)

const notificationQueryBatchSize = 400

const (
	UserNotificationTargetStatusAvailable   = "available"
	UserNotificationTargetStatusSnapshot    = "snapshot"
	UserNotificationTargetStatusLoadError   = "load_error"
	UserNotificationTargetStatusUnavailable = "unavailable"
)

type UserNotificationTargetState struct {
	Notification  models.UserNotification
	Message       *models.Message
	Comment       *models.Comment
	ParentComment *models.Comment
	TargetStatus  string
	TargetTab     string
	TargetURL     string
	CanInteract   bool
}

type UserNotificationHydration struct {
	Items []UserNotificationTargetState
	Users map[uint]models.User
}

type notificationLikePair struct {
	messageID uint
	actorID   uint
}

type UserNotificationPage struct {
	Hydration   UserNotificationHydration
	Total       int
	UnreadCount int
}

func notificationLikePairKey(messageID uint, actorID uint) string {
	return fmt.Sprintf("%d:%d", messageID, actorID)
}

func uniqueNotificationModelIDs(ids []uint) []uint {
	seen := make(map[uint]struct{}, len(ids))
	unique := make([]uint, 0, len(ids))
	for _, id := range ids {
		if id == 0 {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
	}
	return unique
}

func loadNotificationEligibilityMessages(db *gorm.DB, notifications []models.UserNotification) (map[uint]models.Message, error) {
	ids := make([]uint, 0, len(notifications))
	for _, notification := range notifications {
		if notification.Type == models.UserNotificationTypeLike && notification.MessageID != nil {
			ids = append(ids, *notification.MessageID)
		}
	}
	ids = uniqueNotificationModelIDs(ids)
	messages := make(map[uint]models.Message, len(ids))
	for start := 0; start < len(ids); start += notificationQueryBatchSize {
		end := start + notificationQueryBatchSize
		if end > len(ids) {
			end = len(ids)
		}
		var batch []models.Message
		if err := db.Select("id", "user_id", "private", "visibility", "deleted_at", "is_tombstone", "tombstone_visibility").Where("id IN ?", ids[start:end]).Find(&batch).Error; err != nil {
			return nil, err
		}
		for _, message := range batch {
			messages[message.ID] = message
		}
	}
	return messages, nil
}

func loadNotificationMessages(db *gorm.DB, notifications []models.UserNotification) (map[uint]models.Message, error) {
	ids := make([]uint, 0, len(notifications))
	for _, notification := range notifications {
		if notification.MessageID != nil {
			ids = append(ids, *notification.MessageID)
		}
	}
	ids = uniqueNotificationModelIDs(ids)
	messages := make(map[uint]models.Message, len(ids))
	for start := 0; start < len(ids); start += notificationQueryBatchSize {
		end := start + notificationQueryBatchSize
		if end > len(ids) {
			end = len(ids)
		}
		var batch []models.Message
		if err := db.Where("id IN ?", ids[start:end]).Find(&batch).Error; err != nil {
			return nil, err
		}
		for _, message := range batch {
			messages[message.ID] = message
		}
	}
	return messages, nil
}

func loadNotificationCommentClosure(db *gorm.DB, notifications []models.UserNotification) (map[uint]models.Comment, error) {
	pending := make([]uint, 0, len(notifications)*2)
	for _, notification := range notifications {
		switch notification.Type {
		case models.UserNotificationTypeComment, models.UserNotificationTypeReply, models.UserNotificationTypeGuestbook:
			if notification.CommentID != nil {
				pending = append(pending, *notification.CommentID)
			}
			if notification.ParentCommentID != nil {
				pending = append(pending, *notification.ParentCommentID)
			}
		}
	}
	comments := make(map[uint]models.Comment, len(pending))
	requested := map[uint]struct{}{}
	for {
		pending = uniqueNotificationModelIDs(pending)
		batchIDs := make([]uint, 0, len(pending))
		for _, id := range pending {
			if _, exists := requested[id]; !exists {
				requested[id] = struct{}{}
				batchIDs = append(batchIDs, id)
			}
		}
		if len(batchIDs) == 0 {
			break
		}
		pending = pending[:0]
		for start := 0; start < len(batchIDs); start += notificationQueryBatchSize {
			end := start + notificationQueryBatchSize
			if end > len(batchIDs) {
				end = len(batchIDs)
			}
			var batch []models.Comment
			if err := db.Where("id IN ?", batchIDs[start:end]).Find(&batch).Error; err != nil {
				return nil, err
			}
			for _, comment := range batch {
				comments[comment.ID] = comment
				if comment.ParentID != nil {
					pending = append(pending, *comment.ParentID)
				}
			}
		}
	}
	return comments, nil
}

func loadNotificationUsers(db *gorm.DB, notifications []models.UserNotification, messages map[uint]models.Message, comments map[uint]models.Comment) map[uint]models.User {
	ids := make([]uint, 0, len(notifications)+len(messages)+len(comments))
	for _, notification := range notifications {
		if notification.ActorUserID != nil {
			ids = append(ids, *notification.ActorUserID)
		}
	}
	for _, message := range messages {
		ids = append(ids, message.UserID)
	}
	for _, comment := range comments {
		if comment.UserID != nil {
			ids = append(ids, *comment.UserID)
		}
	}
	ids = uniqueNotificationModelIDs(ids)
	users := make(map[uint]models.User, len(ids))
	for start := 0; start < len(ids); start += notificationQueryBatchSize {
		end := start + notificationQueryBatchSize
		if end > len(ids) {
			end = len(ids)
		}
		var batch []models.User
		if err := db.Select("id", "username", "avatar_url").Where("id IN ?", ids[start:end]).Find(&batch).Error; err != nil {
			continue
		}
		for _, user := range batch {
			users[user.ID] = user
		}
	}
	return users
}

func uniqueNotificationLikePairs(notifications []models.UserNotification) []notificationLikePair {
	seen := make(map[string]struct{}, len(notifications))
	pairs := make([]notificationLikePair, 0, len(notifications))
	for _, notification := range notifications {
		if notification.Type != models.UserNotificationTypeLike || notification.MessageID == nil || notification.ActorUserID == nil || *notification.MessageID == 0 || *notification.ActorUserID == 0 {
			continue
		}
		pair := notificationLikePair{messageID: *notification.MessageID, actorID: *notification.ActorUserID}
		key := notificationLikePairKey(pair.messageID, pair.actorID)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		pairs = append(pairs, pair)
	}
	return pairs
}

func loadActiveNotificationLikePairs(db *gorm.DB, notifications []models.UserNotification) (map[string]bool, error) {
	pairs := uniqueNotificationLikePairs(notifications)
	active := make(map[string]bool, len(pairs))
	// Each pair contributes two parameters. Keeping batches small avoids SQLite's
	// parameter ceiling while retaining one exact query for the common case.
	const pairsPerBatch = notificationQueryBatchSize / 2
	for start := 0; start < len(pairs); start += pairsPerBatch {
		end := start + pairsPerBatch
		if end > len(pairs) {
			end = len(pairs)
		}
		clauses := make([]string, 0, end-start)
		args := make([]any, 0, (end-start)*2)
		for _, pair := range pairs[start:end] {
			clauses = append(clauses, "(message_id = ? AND user_id = ?)")
			args = append(args, pair.messageID, pair.actorID)
		}
		var likes []models.MessageLike
		if err := db.Select("message_id", "user_id").Where(strings.Join(clauses, " OR "), args...).Find(&likes).Error; err != nil {
			return nil, err
		}
		for _, like := range likes {
			if like.UserID != nil {
				active[notificationLikePairKey(like.MessageID, *like.UserID)] = true
			}
		}
	}
	return active, nil
}

func filterEligibleUserNotifications(db *gorm.DB, scope ContentReadScope, scopeErr error, notifications []models.UserNotification) ([]models.UserNotification, error, error) {
	if len(notifications) == 0 {
		return []models.UserNotification{}, nil, nil
	}
	messages, messageErr := loadNotificationEligibilityMessages(db, notifications)
	activeLikes, likeErr := loadActiveNotificationLikePairs(db, notifications)
	eligible := make([]models.UserNotification, 0, len(notifications))
	messageReadable := make(map[uint]bool, len(messages))
	messageReadabilityKnown := make(map[uint]bool, len(messages))
	for _, notification := range notifications {
		if notification.Type != models.UserNotificationTypeLike || scopeErr != nil || messageErr != nil || likeErr != nil || notification.MessageID == nil {
			eligible = append(eligible, notification)
			continue
		}
		message, exists := messages[*notification.MessageID]
		if !exists {
			eligible = append(eligible, notification)
			continue
		}
		readable, known := messageReadable[message.ID], messageReadabilityKnown[message.ID]
		if !known {
			readable = scope.CanReadMessage(message)
			messageReadable[message.ID] = readable
			messageReadabilityKnown[message.ID] = true
		}
		if !readable {
			eligible = append(eligible, notification)
			continue
		}
		if notification.ActorUserID != nil && activeLikes[notificationLikePairKey(message.ID, *notification.ActorUserID)] {
			eligible = append(eligible, notification)
		}
	}
	return eligible, messageErr, likeErr
}

func userNotificationTarget(message models.Message, commentID *uint, notificationType string) (string, string) {
	tab := "latest"
	if IsGuestbookMessage(message) && notificationType == models.UserNotificationTypeGuestbook {
		tab = "comment"
	}
	query := fmt.Sprintf("/?tab=%s&message_id=%d", tab, message.ID)
	if commentID != nil && *commentID != 0 {
		query += fmt.Sprintf("&comment_id=%d", *commentID)
	}
	return tab, query
}

// HydrateUserNotifications resolves page-only content after selection. Query
// failures become load-error placeholders so one unavailable association does
// not discard the notification history.
func hydrateUserNotifications(db *gorm.DB, viewerScope ContentReadScope, viewerScopeErr error, notifications []models.UserNotification, eligibilityMessageErr error, eligibilityLikeErr error) UserNotificationHydration {
	result := UserNotificationHydration{Items: []UserNotificationTargetState{}, Users: map[uint]models.User{}}
	if len(notifications) == 0 {
		return result
	}
	messages, messageErr := loadNotificationMessages(db, notifications)
	if eligibilityMessageErr != nil {
		messageErr = eligibilityMessageErr
		messages = map[uint]models.Message{}
	}
	if messageErr != nil {
		log.Printf("加载通知关联笔记失败: %v", messageErr)
		messages = map[uint]models.Message{}
	}
	comments, commentErr := loadNotificationCommentClosure(db, notifications)
	if commentErr != nil {
		log.Printf("加载通知关联评论失败: %v", commentErr)
		comments = map[uint]models.Comment{}
	}
	activeLikes, likeErr := loadActiveNotificationLikePairs(db, notifications)
	if eligibilityLikeErr != nil {
		likeErr = eligibilityLikeErr
		activeLikes = map[string]bool{}
	}
	if likeErr != nil {
		log.Printf("加载通知关联点赞失败: %v", likeErr)
		activeLikes = map[string]bool{}
	}
	result.Users = loadNotificationUsers(db, notifications, messages, comments)
	messageReadable := make(map[uint]bool, len(messages))
	messageReadabilityKnown := make(map[uint]bool, len(messages))
	messageInteractive := make(map[uint]bool, len(messages))
	for _, notification := range notifications {
		state := UserNotificationTargetState{Notification: notification}
		if notification.Type == models.UserNotificationTypeContentDeletion {
			state.TargetStatus = UserNotificationTargetStatusSnapshot
			result.Items = append(result.Items, state)
			continue
		}
		switch notification.Type {
		case models.UserNotificationTypeVoceChatCredentials, models.UserNotificationTypeVoceChatPasswordChanged, models.UserNotificationTypePasswordUpdateIncomplete:
			state.TargetStatus = UserNotificationTargetStatusUnavailable
			result.Items = append(result.Items, state)
			continue
		}
		if messageErr != nil {
			state.TargetStatus = UserNotificationTargetStatusLoadError
			result.Items = append(result.Items, state)
			continue
		}
		if notification.MessageID == nil || *notification.MessageID == 0 {
			state.TargetStatus = UserNotificationTargetStatusUnavailable
			result.Items = append(result.Items, state)
			continue
		}
		message, exists := messages[*notification.MessageID]
		if !exists || viewerScopeErr != nil {
			state.TargetStatus = UserNotificationTargetStatusUnavailable
			result.Items = append(result.Items, state)
			continue
		}
		readable, known := messageReadable[message.ID], messageReadabilityKnown[message.ID]
		if !known {
			readable = viewerScope.CanReadMessage(message)
			messageReadable[message.ID] = readable
			messageReadabilityKnown[message.ID] = true
			messageInteractive[message.ID] = viewerScope.CanInteractWithMessage(message)
		}
		if !readable {
			state.TargetStatus = UserNotificationTargetStatusUnavailable
			result.Items = append(result.Items, state)
			continue
		}
		if notification.Type == models.UserNotificationTypeLike && likeErr != nil {
			state.TargetStatus = UserNotificationTargetStatusLoadError
			result.Items = append(result.Items, state)
			continue
		}
		if notification.Type != models.UserNotificationTypeLike && commentErr != nil {
			state.TargetStatus = UserNotificationTargetStatusLoadError
			result.Items = append(result.Items, state)
			continue
		}

		valid := true
		switch notification.Type {
		case models.UserNotificationTypeLike:
			if notification.ActorUserID == nil || !activeLikes[notificationLikePairKey(message.ID, *notification.ActorUserID)] {
				valid = false
			}
		case models.UserNotificationTypeComment, models.UserNotificationTypeReply, models.UserNotificationTypeGuestbook:
			if notification.CommentID == nil {
				valid = false
				break
			}
			comment, ok := comments[*notification.CommentID]
			if !ok || comment.MessageID != message.ID {
				valid = false
				break
			}
			var parent *models.Comment
			if comment.ParentID != nil {
				loaded, ok := comments[*comment.ParentID]
				if !ok || loaded.MessageID != message.ID {
					valid = false
					break
				}
				parentCopy := loaded
				parent = &parentCopy
			}
			if notification.ParentCommentID != nil {
				loaded, ok := comments[*notification.ParentCommentID]
				if !ok || loaded.MessageID != message.ID {
					valid = false
					break
				}
				parentCopy := loaded
				parent = &parentCopy
			}
			if notification.Type == models.UserNotificationTypeReply && comment.ParentID == nil {
				valid = false
			}
			if notification.Type == models.UserNotificationTypeGuestbook && (!IsGuestbookMessage(message) || comment.ParentID != nil) {
				valid = false
			}
			if notification.Type == models.UserNotificationTypeComment && comment.ParentID != nil {
				valid = false
			}
			if valid && !viewerScope.CanReadComment(message, comment, comments) {
				valid = false
			}
			if valid {
				commentCopy := comment
				state.Comment = &commentCopy
				state.ParentComment = parent
			}
		default:
			valid = false
		}
		if !valid {
			if notification.Type == models.UserNotificationTypeLike {
				continue
			}
			state.TargetStatus = UserNotificationTargetStatusUnavailable
			result.Items = append(result.Items, state)
			continue
		}
		messageCopy := message
		state.Message = &messageCopy
		state.CanInteract = messageInteractive[message.ID]
		state.TargetTab, state.TargetURL = userNotificationTarget(message, notification.CommentID, notification.Type)
		state.TargetStatus = UserNotificationTargetStatusAvailable
		result.Items = append(result.Items, state)
	}
	return result
}

func HydrateUserNotifications(db *gorm.DB, viewerID uint, notifications []models.UserNotification) UserNotificationHydration {
	viewerIDCopy := viewerID
	scope, scopeErr := ResolveContentReadScope(db, &viewerIDCopy)
	return hydrateUserNotifications(db, scope, scopeErr, notifications, nil, nil)
}

// CountUnreadUserNotificationsForViewer applies the same retracted-like rule
// as the notification page without constructing page-only response content.
func CountUnreadUserNotificationsForViewer(db *gorm.DB, viewerID uint) (int, error) {
	var notifications []models.UserNotification
	if err := db.Select("id", "recipient_user_id", "actor_user_id", "type", "message_id", "read_at", "created_at").Where("recipient_user_id = ? AND read_at IS NULL", viewerID).Order("created_at DESC, id DESC").Find(&notifications).Error; err != nil {
		return 0, err
	}
	viewerIDCopy := viewerID
	scope, scopeErr := ResolveContentReadScope(db, &viewerIDCopy)
	eligible, _, _ := filterEligibleUserNotifications(db, scope, scopeErr, notifications)
	return len(eligible), nil
}

// QueryUserNotificationPage first evaluates the only filtering rule (a
// retracted like on an otherwise readable note) from lightweight rows. Full
// notification payloads are loaded only for the final page.
func QueryUserNotificationPage(db *gorm.DB, viewerID uint, page int, pageSize int) (UserNotificationPage, error) {
	var candidates []models.UserNotification
	if err := db.Select("id", "recipient_user_id", "actor_user_id", "type", "message_id", "comment_id", "parent_comment_id", "read_at", "created_at").Where("recipient_user_id = ?", viewerID).Order("created_at DESC, id DESC").Find(&candidates).Error; err != nil {
		return UserNotificationPage{}, err
	}
	viewerIDCopy := viewerID
	scope, scopeErr := ResolveContentReadScope(db, &viewerIDCopy)
	eligible, eligibilityMessageErr, eligibilityLikeErr := filterEligibleUserNotifications(db, scope, scopeErr, candidates)
	result := UserNotificationPage{Total: len(eligible)}
	for _, notification := range eligible {
		if notification.ReadAt == nil {
			result.UnreadCount++
		}
	}
	start := (page - 1) * pageSize
	if start < 0 {
		start = 0
	}
	if start > len(eligible) {
		start = len(eligible)
	}
	end := start + pageSize
	if end > len(eligible) {
		end = len(eligible)
	}
	pageCandidates := eligible[start:end]
	if len(pageCandidates) == 0 {
		result.Hydration = UserNotificationHydration{Items: []UserNotificationTargetState{}, Users: map[uint]models.User{}}
		return result, nil
	}
	ids := make([]uint, 0, len(pageCandidates))
	for _, notification := range pageCandidates {
		ids = append(ids, notification.ID)
	}
	var hydrated []models.UserNotification
	if err := db.Where("id IN ?", ids).Find(&hydrated).Error; err != nil {
		return UserNotificationPage{}, err
	}
	byID := make(map[uint]models.UserNotification, len(hydrated))
	for _, notification := range hydrated {
		byID[notification.ID] = notification
	}
	pageNotifications := make([]models.UserNotification, 0, len(pageCandidates))
	for _, candidate := range pageCandidates {
		if notification, exists := byID[candidate.ID]; exists {
			pageNotifications = append(pageNotifications, notification)
		}
	}
	result.Hydration = hydrateUserNotifications(db, scope, scopeErr, pageNotifications, eligibilityMessageErr, eligibilityLikeErr)
	return result, nil
}
