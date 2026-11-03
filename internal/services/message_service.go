package services

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/feeds"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/dto"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/repository"
	"github.com/rcy1314/echo-noise/pkg"
	"gorm.io/gorm"
)

var ErrRSSDisabled = errors.New("RSS 已禁用")

const (
	MessageVisibilityPublic   = "public"
	MessageVisibilityUsers    = "users"
	MessageVisibilityContacts = "contacts"
	MessageVisibilityPrivate  = "private"
	MessagePinScopeLatest     = "latest"
	MessagePinScopePersonal   = "personal"
)

var (
	messageFilterTagExtractRegexp = regexp.MustCompile(`#([^\s#\p{P}\p{S}]+)([/?=&][^\s#]*)?`)
	invalidMessageFilterTagRegexp = regexp.MustCompile(`[/?=&]|^(song|video|playlist)\?id=\d+$`)
)

func messageMatchesTag(content string, tag string) bool {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return true
	}
	matches := messageFilterTagExtractRegexp.FindAllStringSubmatch(content, -1)
	for _, match := range matches {
		if len(match) <= 1 || match[1] == "" || invalidMessageFilterTagRegexp.MatchString(match[1]) {
			continue
		}
		if len(match) > 2 && match[2] != "" {
			continue
		}
		if match[1] == tag {
			return true
		}
	}
	return false
}

func NormalizeMessageVisibility(value string, fallbackPrivate bool) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "":
		if fallbackPrivate {
			return MessageVisibilityPrivate, true
		}
		return MessageVisibilityPublic, true
	case MessageVisibilityPublic:
		return MessageVisibilityPublic, true
	case MessageVisibilityUsers, "members", "member", "all_users", "logged_in", "logged-in":
		return MessageVisibilityUsers, true
	case MessageVisibilityContacts:
		return MessageVisibilityContacts, true
	case MessageVisibilityPrivate:
		return MessageVisibilityPrivate, true
	default:
		return "", false
	}
}

func MessageVisibilityRequiresPrivateFlag(visibility string) bool {
	return visibility != MessageVisibilityPublic
}

func ApplyMessageVisibilityForSave(message *models.Message) error {
	if message == nil {
		return nil
	}
	visibility, ok := NormalizeMessageVisibility(message.Visibility, message.Private)
	if !ok {
		return fmt.Errorf("无效的可见范围")
	}
	message.Visibility = visibility
	message.Private = MessageVisibilityRequiresPrivateFlag(visibility)
	return nil
}

func StoredMessageVisibility(message models.Message) string {
	if message.IsTombstone && strings.TrimSpace(message.TombstoneVisibility) != "" {
		visibility, ok := NormalizeMessageVisibility(message.TombstoneVisibility, true)
		if ok {
			return visibility
		}
		return MessageVisibilityPrivate
	}
	visibility, ok := NormalizeMessageVisibility(message.Visibility, message.Private)
	if !ok {
		if message.Private {
			return MessageVisibilityPrivate
		}
		return MessageVisibilityPublic
	}
	if message.Private && visibility == MessageVisibilityPublic {
		return MessageVisibilityPrivate
	}
	return visibility
}

func CanViewMessageNormally(message models.Message, userID *uint) bool {
	if userID != nil && *userID != 0 && message.UserID == *userID {
		return true
	}
	switch StoredMessageVisibility(message) {
	case MessageVisibilityPublic:
		return true
	case MessageVisibilityUsers:
		return userID != nil && *userID != 0
	case MessageVisibilityContacts:
		if userID == nil || *userID == 0 {
			return false
		}
		if !voceChatContactsVisibilityEnabled() || !voceChatContactAuthorEligible(message.UserID) {
			return false
		}
		ok, err := repository.IsFreshVoceChatContact(message.UserID, *userID, time.Now().UTC())
		return err == nil && ok
	case MessageVisibilityPrivate:
		return false
	default:
		return false
	}
}

func CanViewMessage(message models.Message, userID *uint) bool {
	scope, err := ResolveContentReadScope(database.DB, userID)
	return err == nil && scope.CanReadMessage(message)
}

func canViewFreshVoceChatContact(authorID uint, viewerID uint) bool {
	if authorID == 0 || viewerID == 0 {
		return false
	}
	ok, err := repository.IsFreshVoceChatContact(authorID, viewerID, time.Now().UTC())
	return err == nil && ok
}

func CanViewVoceChatContactAudience(authorID uint, viewerID uint) bool {
	if authorID == 0 || viewerID == 0 {
		return false
	}
	if authorID == viewerID {
		return true
	}
	if !voceChatContactsVisibilityEnabled() {
		return false
	}
	if !voceChatContactAuthorEligible(authorID) {
		return false
	}
	_ = EnsureVoceChatContactCacheForAuthor(authorID)
	return canViewFreshVoceChatContact(authorID, viewerID)
}

func CanInteractWithMessage(message models.Message, userID *uint) bool {
	scope, err := ResolveContentReadScope(database.DB, userID)
	return err == nil && scope.CanInteractWithMessage(message)
}

func ApplyMessageVisibilityScope(query *gorm.DB, userID *uint) *gorm.DB {
	scope, err := ResolveContentReadScope(database.DB, userID)
	if err != nil {
		return query.Where("1 = 0")
	}
	return scope.ApplyMessageVisibility(query)
}

func voceChatContactsVisibilityEnabled() bool {
	config, err := loadVoceChatSiteConfig()
	return err == nil && config.Enabled && config.ContactsEnabled
}

// GetAllMessages 封装业务逻辑，获取所有笔记
func GetAllMessages(showPrivate bool) ([]models.Message, error) {
	if showPrivate {
		return GetAllMessagesForViewer(nil)
	}
	return GetAllMessagesForViewer(nil)
}

func GetAllMessagesForViewer(userID *uint) ([]models.Message, error) {
	var messages []models.Message
	query := ApplyMessageVisibilityScope(database.DB.Model(&models.Message{}), userID)
	if err := query.Order(messagePinOrder(MessagePinScopeLatest)).Find(&messages).Error; err != nil {
		return nil, fmt.Errorf("获取消息失败: %v", err)
	}
	ApplyMessageViewerState(messages, userID)
	return messages, nil
}

func ApplyMessageViewerState(messages []models.Message, userID *uint) {
	scope, err := ResolveContentReadScope(database.DB, userID)
	if err == nil {
		for index := range messages {
			messages[index].CanInteract = scope.CanInteractWithMessage(messages[index])
		}
	}
	applyMessageLikedState(messages, userID)
}

func applyMessageLikedState(messages []models.Message, userID *uint) {
	if userID == nil || *userID == 0 || len(messages) == 0 {
		return
	}
	messageIDs := make([]uint, 0, len(messages))
	seenIDs := make(map[uint]struct{}, len(messages))
	for _, message := range messages {
		if message.ID == 0 {
			continue
		}
		if _, exists := seenIDs[message.ID]; exists {
			continue
		}
		seenIDs[message.ID] = struct{}{}
		messageIDs = append(messageIDs, message.ID)
	}
	if len(messageIDs) == 0 {
		return
	}
	var likes []models.MessageLike
	if err := database.DB.Select("message_id").Where("user_id = ? AND message_id IN ?", *userID, messageIDs).Find(&likes).Error; err != nil {
		return
	}
	likedIDs := make(map[uint]struct{}, len(likes))
	for _, like := range likes {
		likedIDs[like.MessageID] = struct{}{}
	}
	for index := range messages {
		_, messages[index].Liked = likedIDs[messages[index].ID]
	}
}

// GetMessageByID 根据 ID 获取笔记
func GetMessageByID(id uint, showPrivate bool) (*models.Message, error) {
	message, err := repository.GetMessageByID(id, true)
	if err != nil {
		return nil, fmt.Errorf("获取消息失败: %v", err)
	}

	if message == nil {
		return nil, fmt.Errorf("消息不存在")
	}
	if message.DeletedAt != nil || message.IsTombstone {
		return nil, ErrMessageNotVisible
	}
	if !showPrivate && !CanViewMessage(*message, nil) {
		return nil, fmt.Errorf("无权访问")
	}

	return message, nil
}

func GetMessageByIDForViewer(id uint, userID *uint) (*models.Message, error) {
	message, err := repository.GetMessageByID(id, true)
	if err != nil {
		return nil, fmt.Errorf("获取消息失败: %v", err)
	}
	if message == nil {
		return nil, fmt.Errorf("消息不存在")
	}
	if message.DeletedAt != nil && !message.IsTombstone {
		return nil, ErrMessageNotVisible
	}
	if message.IsTombstone {
		var comments []models.Comment
		if err := database.DB.Where("message_id = ?", message.ID).Find(&comments).Error; err != nil {
			return nil, err
		}
		scope, err := ResolveContentReadScope(database.DB, userID)
		if err != nil {
			return nil, err
		}
		messageForRead := *message
		messageForRead.DeletedAt = nil
		commentMap := CommentMap(comments)
		visibleDescendant := false
		for _, comment := range comments {
			if comment.DeletedAt == nil && !comment.IsTombstone && scope.CanReadComment(messageForRead, comment, commentMap) {
				visibleDescendant = true
				break
			}
		}
		if !visibleDescendant {
			return nil, ErrMessageNotVisible
		}
		message.UserID = 0
		message.Username = ""
		message.Content = ""
		message.ImageURL = ""
		message.CanInteract = false
		message.Visibility = StoredMessageVisibility(*message)
		message.TombstoneVisibility = ""
		return message, nil
	}
	if userID != nil && *userID != 0 && message.UserID != *userID && StoredMessageVisibility(*message) == MessageVisibilityContacts {
		_ = EnsureVoceChatContactCacheForAuthor(message.UserID)
	}
	if !CanViewMessage(*message, userID) {
		return nil, fmt.Errorf("无权访问")
	}
	if userID != nil && *userID != 0 {
		var count int64
		if err := database.DB.Model(&models.MessageLike{}).Where("user_id = ? AND message_id = ?", *userID, message.ID).Count(&count).Error; err == nil {
			message.Liked = count > 0
		}
	}
	scope, scopeErr := ResolveContentReadScope(database.DB, userID)
	if scopeErr == nil {
		message.CanInteract = scope.CanInteractWithMessage(*message)
	}
	return message, nil
}

func normalizeMessagePageParams(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}
	return page, pageSize
}

func normalizeMessagePinScope(pinScope string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(pinScope)) {
	case "", MessagePinScopeLatest:
		return MessagePinScopeLatest, nil
	case MessagePinScopePersonal:
		return MessagePinScopePersonal, nil
	default:
		return "", fmt.Errorf("pinScope 参数无效")
	}
}

func messagePinOrder(pinScope string) string {
	if pinScope == MessagePinScopePersonal {
		return "personal_pinned DESC, CASE WHEN personal_pinned_at IS NULL THEN 1 ELSE 0 END ASC, personal_pinned_at DESC, created_at DESC, id DESC"
	}
	return "pinned DESC, CASE WHEN pinned_at IS NULL THEN 1 ELSE 0 END ASC, pinned_at DESC, created_at DESC, id DESC"
}

// Keep the projection and cursor comparison aligned with messagePinOrder.
// Content is selected only by the exact-tag scan, never by ordinary locate.
type messagePageCursor struct {
	ID        uint
	PagePin   *bool
	PagePinAt *time.Time
	CreatedAt *time.Time
	Content   string
}

const messageTagScanBatchSize = 256

func messagePinColumns(pinScope string) (string, string) {
	if pinScope == MessagePinScopePersonal {
		return "personal_pinned", "personal_pinned_at"
	}
	return "pinned", "pinned_at"
}

func messageCursorProjection(pinScope string) string {
	pin, pinAt := messagePinColumns(pinScope)
	return "id, " + pin + " AS page_pin, " + pinAt + " AS page_pin_at, created_at"
}

// nullableMessageOrderBoundary adds one descending key to a lexicographic
// comparison. The schema permits NULL in legacy pin flags and creation times.
func nullableMessageOrderBoundary(column string, value interface{}, isNull, nullsFirst, before bool, tail string, args []interface{}) (string, []interface{}) {
	if isNull {
		condition := column + " IS NULL AND (" + tail + ")"
		if before != nullsFirst {
			condition = column + " IS NOT NULL OR (" + condition + ")"
		}
		return "(" + condition + ")", args
	}
	op := "<"
	if before {
		op = ">"
	}
	condition := column + " " + op + " ? OR (" + column + " = ? AND (" + tail + "))"
	if before == nullsFirst {
		condition = column + " IS NULL OR " + condition
	}
	return "(" + condition + ")", append([]interface{}{value, value}, args...)
}

// Match messagePinOrder exactly: pin timestamps always put NULL last, while
// pin flags and creation times retain the database's native DESC NULL order.
func messageCursorBoundary(cursor messagePageCursor, pinScope string, before bool, dialect string) (string, []interface{}) {
	pin, pinAt := messagePinColumns(pinScope)
	op := "<"
	if before {
		op = ">"
	}
	tail := "id " + op + " ?"
	args := []interface{}{cursor.ID}
	nullsFirst := dialect == "postgres"
	tail, args = nullableMessageOrderBoundary("created_at", cursor.CreatedAt, cursor.CreatedAt == nil, nullsFirst, before, tail, args)
	tail, args = nullableMessageOrderBoundary(pinAt, cursor.PagePinAt, cursor.PagePinAt == nil, false, before, tail, args)
	return nullableMessageOrderBoundary(pin, cursor.PagePin, cursor.PagePin == nil, nullsFirst, before, tail, args)
}

// scanMessageTagMatches bounds memory to one narrow batch and the caller's
// retained IDs. Keyset traversal avoids increasingly large OFFSET scans.
func scanMessageTagMatches(q *gorm.DB, tag, pinScope string, visit func(uint, int64)) (int64, error) {
	var total int64
	var cursor *messagePageCursor
	for {
		batchQuery := q.Session(&gorm.Session{}).Select(messageCursorProjection(pinScope) + ", content")
		if cursor != nil {
			condition, args := messageCursorBoundary(*cursor, pinScope, false, q.Dialector.Name())
			batchQuery = batchQuery.Where(condition, args...)
		}
		var batch []messagePageCursor
		if err := batchQuery.Order(messagePinOrder(pinScope)).Limit(messageTagScanBatchSize).Find(&batch).Error; err != nil {
			return 0, err
		}
		for _, candidate := range batch {
			if messageMatchesTag(candidate.Content, tag) {
				visit(candidate.ID, total)
				total++
			}
		}
		if len(batch) < messageTagScanBatchSize {
			return total, nil
		}
		last := batch[len(batch)-1]
		last.Content = ""
		cursor = &last
	}
}

func messagePageBaseQuery(userID *uint, authorID *uint, username *string, date *string, keyword *string, tag *string, pinScope string, excludeID *uint) (*gorm.DB, error) {
	normalizedPinScope, err := normalizeMessagePinScope(pinScope)
	if err != nil {
		return nil, err
	}
	if normalizedPinScope == MessagePinScopePersonal && (userID == nil || authorID == nil || *userID != *authorID) {
		return nil, fmt.Errorf("个人作用域仅允许查询当前用户的笔记")
	}
	q := database.DB.Model(&models.Message{})
	if excludeID != nil && *excludeID != 0 {
		q = q.Where("id <> ?", *excludeID)
	}
	if authorID != nil {
		q = q.Where("user_id = ?", *authorID)
	} else if username != nil && strings.TrimSpace(*username) != "" {
		q = q.Where("username = ?", strings.TrimSpace(*username))
	}
	if date != nil && strings.TrimSpace(*date) != "" {
		day, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(*date), shanghaiLocation())
		if err != nil {
			return nil, fmt.Errorf("日期格式无效")
		}
		q = q.Where("created_at >= ? AND created_at < ?", day, day.AddDate(0, 0, 1))
	}
	if keyword != nil && strings.TrimSpace(*keyword) != "" {
		q = q.Where("content LIKE ?", "%"+strings.TrimSpace(*keyword)+"%")
	}
	if tag != nil && strings.TrimSpace(*tag) != "" {
		q = q.Where("content LIKE ?", "%#"+strings.TrimSpace(*tag)+"%")
	}
	return ApplyMessageVisibilityScope(q, userID), nil
}

// GetMessagesByPage 按当前查看者的正常可见性与获授权隐藏读取范围分页获取笔记，并支持作者、日期等筛选。
func GetMessagesByPage(page, pageSize int, userID *uint, authorID *uint, username *string, date *string, keyword *string, tag *string, pinScope string, excludeID *uint) (dto.PageQueryResult, error) {
	page, pageSize = normalizeMessagePageParams(page, pageSize)
	offset := (page - 1) * pageSize
	normalizedPinScope, err := normalizeMessagePinScope(pinScope)
	if err != nil {
		return dto.PageQueryResult{}, err
	}

	q, err := messagePageBaseQuery(userID, authorID, username, date, keyword, tag, normalizedPinScope, excludeID)
	if err != nil {
		return dto.PageQueryResult{}, err
	}

	var messages []models.Message
	var total int64
	if tag != nil && strings.TrimSpace(*tag) != "" {
		pageIDs := make([]uint, 0, pageSize)
		total, err = scanMessageTagMatches(q, *tag, normalizedPinScope, func(id uint, index int64) {
			if index/int64(pageSize)+1 == int64(page) {
				pageIDs = append(pageIDs, id)
			}
		})
		if err != nil {
			return dto.PageQueryResult{}, err
		}
		messages = []models.Message{}
		if len(pageIDs) > 0 {
			if err := q.Session(&gorm.Session{}).Where("id IN ?", pageIDs).Order(messagePinOrder(normalizedPinScope)).Find(&messages).Error; err != nil {
				return dto.PageQueryResult{}, err
			}
		}
		ApplyMessageViewerState(messages, userID)
		return dto.PageQueryResult{Total: total, Items: messages}, nil
	}
	if err := q.Count(&total).Error; err != nil {
		return dto.PageQueryResult{}, err
	}
	if err := q.Limit(pageSize).Offset(offset).Order(messagePinOrder(normalizedPinScope)).Find(&messages).Error; err != nil {
		return dto.PageQueryResult{}, err
	}
	ApplyMessageViewerState(messages, userID)

	return dto.PageQueryResult{Total: total, Items: messages}, nil
}

func LocateMessagePage(messageID uint, pageSize int, userID *uint, authorID *uint, username *string, date *string, keyword *string, tag *string, pinScope string, excludeID *uint) (dto.MessagePageLocateResult, error) {
	_, pageSize = normalizeMessagePageParams(1, pageSize)
	normalizedPinScope, err := normalizeMessagePinScope(pinScope)
	if err != nil {
		return dto.MessagePageLocateResult{}, err
	}

	q, err := messagePageBaseQuery(userID, authorID, username, date, keyword, tag, normalizedPinScope, excludeID)
	if err != nil {
		return dto.MessagePageLocateResult{}, err
	}
	var target messagePageCursor
	projection := messageCursorProjection(normalizedPinScope)
	hasTag := tag != nil && strings.TrimSpace(*tag) != ""
	if hasTag {
		projection += ", content"
	}
	if err := q.Session(&gorm.Session{}).Select(projection).Where("id = ?", messageID).Take(&target).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return dto.MessagePageLocateResult{}, fmt.Errorf("消息不存在或无权访问")
		}
		return dto.MessagePageLocateResult{}, err
	}

	var total, targetIndex int64
	if hasTag {
		if !messageMatchesTag(target.Content, *tag) {
			return dto.MessagePageLocateResult{}, fmt.Errorf("消息不存在或无权访问")
		}
		targetIndex = -1
		total, err = scanMessageTagMatches(q, *tag, normalizedPinScope, func(id uint, index int64) {
			if id == messageID {
				targetIndex = index
			}
		})
		if err != nil {
			return dto.MessagePageLocateResult{}, err
		}
		if targetIndex < 0 {
			return dto.MessagePageLocateResult{}, fmt.Errorf("消息不存在或无权访问")
		}
	} else {
		if err := q.Session(&gorm.Session{}).Count(&total).Error; err != nil {
			return dto.MessagePageLocateResult{}, err
		}
		condition, args := messageCursorBoundary(target, normalizedPinScope, true, q.Dialector.Name())
		if err := q.Session(&gorm.Session{}).Where(condition, args...).Count(&targetIndex).Error; err != nil {
			return dto.MessagePageLocateResult{}, err
		}
	}

	return dto.MessagePageLocateResult{
		MessageID: messageID,
		Page:      int(targetIndex/int64(pageSize)) + 1,
		PageSize:  pageSize,
		Total:     total,
	}, nil
}

// CreateMessage 发布一条笔记
// 允许所有注册登陆用户发布信息
func CreateMessage(message *models.Message) error {
	user, err := GetUserByID(message.UserID)
	if err != nil {
		return err
	}

	// 删除管理员权限检查，允许所有登录用户发布信息
	message.Username = user.Username // 获取用户名
	if err := ApplyMessageVisibilityForSave(message); err != nil {
		return err
	}
	contactsAllowed := false
	if StoredMessageVisibility(*message) == MessageVisibilityContacts {
		_ = RefreshVoceChatContactCacheForAuthor(message.UserID)
		contactsAllowed = voceChatContactsVisibilityEnabled() && voceChatContactAuthorEligible(message.UserID)
	}
	publishedAt := time.Now().UTC()
	var notificationIDs []uint
	if err := repository.CreateMessage(message, func(tx *gorm.DB) error {
		var err error
		notificationIDs, err = createAuthorSubscriptionNotifications(tx, *message, publishedAt, contactsAllowed)
		return err
	}); err != nil {
		return err
	}
	for _, notificationID := range notificationIDs {
		queueUserNotificationPush(notificationID)
	}
	if len(notificationIDs) > 0 {
		WakeWebPushDispatcher()
	}
	return nil
}

// DeleteMessage 根据 ID 删除笔记
func DeleteMessage(id uint, userID uint) error {
	return TrashMessage(database.DB, userID, id, "author request")
}

// DeleteMessageByAdmin is retained only as a non-destructive compatibility
// symbol. Administrator deletion must go through TrashMessage with an actor ID.
func DeleteMessageByAdmin(id uint) error {
	return fmt.Errorf("administrator deletion requires actor identity for recycle-bin lifecycle")
}

func GenerateRSS(c *gin.Context) (string, error) {
	rssConfig, err := GetRSSConfig()
	if err != nil {
		return "", fmt.Errorf("获取 RSS 配置失败: %v", err)
	}
	if !rssConfig.Enabled || len(rssConfig.MemberIDs) == 0 {
		return "", ErrRSSDisabled
	}

	messages, err := repository.GetPublicMessagesByUserIDs(rssConfig.MemberIDs)
	if err != nil {
		return "", fmt.Errorf("获取消息失败: %v", err)
	}

	// 判断请求协议
	schema := "http"
	if c.Request.TLS != nil {
		schema = "https"
	}

	// RSS links must reflect the address that requested the feed. Do not retain
	// deployment-specific hosts or invent a port that was absent from Host.
	baseURL := schema + "://" + c.Request.Host

	// 确保URL末尾没有斜杠
	baseURL = strings.TrimSuffix(baseURL, "/")

	feed := &feeds.Feed{
		Title: rssConfig.Title,
		Link: &feeds.Link{
			Href: baseURL + "/",
		},
		Image: &feeds.Image{
			Url: baseURL + rssConfig.FaviconURL,
		},
		Description: rssConfig.Description,
		Author: &feeds.Author{
			Name: rssConfig.AuthorName,
		},
		Updated: time.Now(),
	}

	for _, msg := range messages {
		// 处理内容
		content := msg.Content
		if msg.ImageURL != "" {
			imageURL := baseURL + "/api" + msg.ImageURL
			content = fmt.Sprintf("![图片](%s)\n\n%s", imageURL, content)
		}

		// 渲染 Markdown
		htmlContent := pkg.MdToHTML([]byte(content))

		// 生成标题
		title := msg.Username
		if firstLine := pkg.GetFirstLine(msg.Content); firstLine != "" {
			title = firstLine
		}

		// 生成前端页面 URL
		pageURL := baseURL + "/#/messages/" + fmt.Sprintf("%d", msg.ID)

		item := &feeds.Item{
			Title:       title,
			Link:        &feeds.Link{Href: pageURL},
			Description: string(htmlContent),
			Author:      &feeds.Author{Name: msg.Username},
			Created:     msg.CreatedAt,
			Id:          pageURL,
		}

		feed.Items = append(feed.Items, item)
	}

	rss, err := feed.ToRss()
	if err != nil {
		return "", err
	}

	return rss, nil
}

// contains 辅助函数，用于检查字符串中是否包含指定子串
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

// UpdateMessage 更新消息字段
func UpdateMessage(messageID uint, content *string, private *bool, visibility *string, createdAt *time.Time) (*models.Message, error) {
	message, err := repository.GetMessageByID(messageID, true)
	if err != nil {
		return nil, fmt.Errorf("获取消息失败: %v", err)
	}
	if message == nil {
		return nil, fmt.Errorf("消息不存在")
	}
	if message.DeletedAt != nil || message.IsTombstone {
		return nil, ErrMessageNotVisible
	}
	if content != nil {
		c := strings.TrimSpace(*content)
		if c == "" && strings.TrimSpace(message.ImageURL) == "" {
			return nil, fmt.Errorf(models.CannotBeEmptyMessage)
		}
		message.Content = c
	}
	if visibility != nil {
		message.Visibility = *visibility
		if err := ApplyMessageVisibilityForSave(message); err != nil {
			return nil, err
		}
	} else if private != nil {
		message.Private = *private
		message.Visibility = ""
		if err := ApplyMessageVisibilityForSave(message); err != nil {
			return nil, err
		}
	}
	if createdAt != nil {
		message.CreatedAt = *createdAt
	}
	if err := database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(message).Error; err != nil {
			return err
		}
		return models.SyncLocalAttachmentGrants(tx, message)
	}); err != nil {
		return nil, fmt.Errorf("更新消息失败: %v", err)
	}
	return message, nil
}

// SetGlobalPin updates the compatibility pinned column, whose explicit meaning
// is the site's global pin state. The caller supplies the transaction when the
// update must be committed together with its audit record.
func SetGlobalPin(db *gorm.DB, messageID uint, pinned bool) error {
	if db == nil {
		return fmt.Errorf("数据库未初始化")
	}
	var message models.Message
	if err := db.Select("id", "deleted_at").First(&message, messageID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("消息不存在")
		}
		return fmt.Errorf("读取消息失败: %v", err)
	}
	if message.DeletedAt != nil {
		return ErrMessageNotVisible
	}
	result := db.Model(&models.Message{}).Where("id = ? AND deleted_at IS NULL", messageID).Update("pinned", pinned)
	if result.Error == nil {
		if pinned {
			result = db.Model(&models.Message{}).Where("id = ?", messageID).Update("pinned_at", time.Now().UTC())
		} else {
			result = db.Exec("UPDATE messages SET pinned_at = NULL WHERE id = ?", messageID)
		}
	}
	if result.Error != nil {
		return fmt.Errorf("更新全站置顶状态失败: %v", result.Error)
	}
	return nil
}

// SetPersonalPin updates only the author's personal pin state.
func SetPersonalPin(db *gorm.DB, messageID uint, pinned bool) error {
	if db == nil {
		return fmt.Errorf("数据库未初始化")
	}
	var message models.Message
	if err := db.Select("id", "deleted_at").First(&message, messageID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("消息不存在")
		}
		return fmt.Errorf("读取消息失败: %v", err)
	}
	if message.DeletedAt != nil {
		return ErrMessageNotVisible
	}
	result := db.Model(&models.Message{}).Where("id = ? AND deleted_at IS NULL", messageID).Update("personal_pinned", pinned)
	if result.Error == nil {
		if pinned {
			result = db.Model(&models.Message{}).Where("id = ?", messageID).Update("personal_pinned_at", time.Now().UTC())
		} else {
			result = db.Exec("UPDATE messages SET personal_pinned_at = NULL WHERE id = ?", messageID)
		}
	}
	if result.Error != nil {
		return fmt.Errorf("更新个人置顶状态失败: %v", result.Error)
	}
	return nil
}

// UpdateMessagePinned is retained for internal callers as the old global-pin
// compatibility operation. HTTP callers must pass the global authorization
// checks before reaching this function.
func UpdateMessagePinned(messageID uint, pinned bool) error {
	return SetGlobalPin(database.DB, messageID, pinned)
}

// IncrementLikeCount 登录用户点赞；已点赞时保持现状。
func IncrementLikeCount(messageID uint, userID uint) (bool, int, error) {
	if userID == 0 {
		return false, 0, fmt.Errorf("请先登录后再点赞")
	}
	currentUserID := userID
	message, err := GetMessageByIDForViewer(messageID, &currentUserID)
	if err != nil {
		return false, 0, err
	}
	if !CanInteractWithMessage(*message, &currentUserID) {
		return false, 0, fmt.Errorf("无权点赞该内容")
	}

	created := false
	var existing models.MessageLike
	err = database.DB.Where("message_id = ? AND user_id = ?", messageID, userID).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		like := models.MessageLike{MessageID: messageID, UserID: &currentUserID}
		if err := database.DB.Create(&like).Error; err != nil {
			return false, 0, err
		}
		created = true
	} else if err != nil {
		return false, 0, err
	}

	count, err := syncMessageLikeCount(messageID)
	return created, count, err
}

// ToggleLike 根据登录用户切换点赞状态。
func ToggleLike(messageID uint, userID *uint, _ string) (bool, int, error) {
	if userID == nil || *userID == 0 {
		return false, 0, fmt.Errorf("请先登录后再点赞")
	}
	currentUserID := *userID
	message, err := GetMessageByIDForViewer(messageID, &currentUserID)
	if err != nil {
		return false, 0, err
	}
	if !CanInteractWithMessage(*message, &currentUserID) {
		return false, 0, fmt.Errorf("无权点赞该内容")
	}

	var existing models.MessageLike
	err = database.DB.Where("message_id = ? AND user_id = ?", messageID, currentUserID).First(&existing).Error
	if err == nil && existing.ID != 0 {
		if err := database.DB.Delete(&existing).Error; err != nil {
			return false, 0, err
		}
	} else if errors.Is(err, gorm.ErrRecordNotFound) {
		like := models.MessageLike{MessageID: messageID, UserID: &currentUserID}
		if err := database.DB.Create(&like).Error; err != nil {
			return false, 0, err
		}
	} else if err != nil {
		return false, 0, err
	}

	count, err := syncMessageLikeCount(messageID)
	if err != nil {
		return false, count, err
	}

	var check models.MessageLike
	liked := database.DB.Where("message_id = ? AND user_id = ?", messageID, currentUserID).First(&check).Error == nil && check.ID != 0
	return liked, count, nil
}

func syncMessageLikeCount(messageID uint) (int, error) {
	var cnt int64
	if err := database.DB.Model(&models.MessageLike{}).Where("message_id = ?", messageID).Count(&cnt).Error; err != nil {
		return 0, err
	}
	if err := database.DB.Model(&models.Message{}).Where("id = ?", messageID).Update("like_count", cnt).Error; err != nil {
		return int(cnt), err
	}
	return int(cnt), nil
}
func shanghaiLocation() *time.Location {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.FixedZone("Asia/Shanghai", 8*60*60)
	}
	return loc
}

type MessageDateCount struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}

func GetMessagesGroupByDate(userID *uint, authorID *uint) ([]MessageDateCount, error) {
	type createdAtRow struct {
		CreatedAt time.Time `json:"created_at"`
	}

	var rows []createdAtRow
	q := database.DB.Table("messages")
	if authorID != nil {
		q = q.Where("user_id = ?", *authorID)
	}
	q = ApplyMessageVisibilityScope(q, userID)
	if err := q.
		Select("created_at").
		Order("created_at DESC").
		Scan(&rows).Error; err != nil {
		fmt.Printf("获取消息日历数据失败: %v\n", err)
		return nil, err
	}

	results := make([]MessageDateCount, 0)
	counts := make(map[string]int)
	loc := shanghaiLocation()
	for _, row := range rows {
		date := row.CreatedAt.In(loc).Format("2006-01-02")
		if _, ok := counts[date]; !ok {
			results = append(results, MessageDateCount{Date: date})
		}
		counts[date]++
	}
	for i := range results {
		results[i].Count = counts[results[i].Date]
	}

	return results, nil
}

// GetMessagePage 获取消息详情页
func GetMessagePage(id uint, userID *uint) (*models.Message, error) {
	return GetMessageByIDForViewer(id, userID)
}

func SearchMessages(keyword string, page, pageSize int, userID *uint, authorID *uint, username *string) (dto.PageQueryResult, error) {
	// 参数校验
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}

	// 直接使用服务层实现
	query := database.DB.Model(&models.Message{}).
		Select("id, content, created_at, username, image_url, private, visibility, user_id").
		Where("content LIKE ?", "%"+keyword+"%")
	// 作者筛选
	if authorID != nil {
		query = query.Where("user_id = ?", *authorID)
	} else if username != nil && *username != "" {
		query = query.Where("username = ?", *username)
	}
	query = ApplyMessageVisibilityScope(query, userID)

	var total int64
	var messages []models.Message

	err := query.Count(&total).
		Limit(pageSize).
		Offset((page - 1) * pageSize).
		Order("created_at DESC").
		Find(&messages).Error

	if err != nil {
		return dto.PageQueryResult{}, err
	}
	ApplyMessageViewerState(messages, userID)

	// 确保返回的数据结构符合前端期望
	return dto.PageQueryResult{
		Total: total,
		Items: messages,
	}, nil
}
