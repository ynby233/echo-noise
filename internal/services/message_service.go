package services

import (
	"errors"
	"fmt"
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
)

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

func CanViewMessage(message models.Message, userID *uint, isAdmin bool) bool {
	if isAdmin {
		return true
	}
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
		if !voceChatContactsVisibilityEnabled() {
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

func CanInteractWithMessage(message models.Message, userID *uint) bool {
	if userID != nil && *userID != 0 && message.UserID != *userID && StoredMessageVisibility(message) == MessageVisibilityContacts {
		_ = EnsureVoceChatContactCacheForAuthor(message.UserID)
	}
	return CanViewMessage(message, userID, false)
}

func ApplyMessageVisibilityScope(query *gorm.DB, userID *uint, isAdmin bool) *gorm.DB {
	if isAdmin {
		return query
	}
	publicSQL := "(private = ? AND (visibility = ? OR visibility = ? OR visibility IS NULL))"
	if userID != nil && *userID != 0 {
		if voceChatContactsVisibilityEnabled() {
			EnsureVoceChatContactCachesForViewer(userID, isAdmin)
			now := time.Now().UTC()
			contactsSQL := "(visibility = ? AND EXISTS (SELECT 1 FROM voce_chat_contact_caches AS vcc WHERE vcc.user_id = messages.user_id AND vcc.contact_user_id = ? AND vcc.last_sync_status = ? AND vcc.expires_at > ?))"
			return query.Where("(user_id = ? OR "+publicSQL+" OR visibility = ? OR "+contactsSQL+")", *userID, false, MessageVisibilityPublic, "", MessageVisibilityUsers, MessageVisibilityContacts, *userID, models.VoceChatContactSyncStatusOK, now)
		}
		return query.Where("(user_id = ? OR "+publicSQL+" OR visibility = ?)", *userID, false, MessageVisibilityPublic, "", MessageVisibilityUsers)
	}
	return query.Where(publicSQL, false, MessageVisibilityPublic, "")
}

func voceChatContactsVisibilityEnabled() bool {
	config, err := loadVoceChatSiteConfig()
	return err == nil && config.Enabled && config.ContactsEnabled
}

// GetAllMessages 封装业务逻辑，获取所有笔记
func GetAllMessages(showPrivate bool) ([]models.Message, error) {
	if showPrivate {
		return GetAllMessagesForViewer(nil, true)
	}
	return GetAllMessagesForViewer(nil, false)
}

func GetAllMessagesForViewer(userID *uint, isAdmin bool) ([]models.Message, error) {
	var messages []models.Message
	query := ApplyMessageVisibilityScope(database.DB.Model(&models.Message{}), userID, isAdmin)
	if err := query.Order("pinned DESC, created_at DESC").Find(&messages).Error; err != nil {
		return nil, fmt.Errorf("获取消息失败: %v", err)
	}
	return messages, nil
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
	if !showPrivate && !CanViewMessage(*message, nil, false) {
		return nil, fmt.Errorf("无权访问")
	}

	return message, nil
}

func GetMessageByIDForViewer(id uint, userID *uint, isAdmin bool) (*models.Message, error) {
	message, err := repository.GetMessageByID(id, true)
	if err != nil {
		return nil, fmt.Errorf("获取消息失败: %v", err)
	}
	if message == nil {
		return nil, fmt.Errorf("消息不存在")
	}
	if !isAdmin && userID != nil && *userID != 0 && message.UserID != *userID && StoredMessageVisibility(*message) == MessageVisibilityContacts {
		_ = EnsureVoceChatContactCacheForAuthor(message.UserID)
	}
	if !CanViewMessage(*message, userID, isAdmin) {
		return nil, fmt.Errorf("无权访问")
	}
	return message, nil
}

// GetMessagesByPage 分页获取笔记（支持作者和日期筛选；管理员查看全部；普通用户可查看公开和自己的私密）
func GetMessagesByPage(page, pageSize int, userID *uint, isAdmin bool, authorID *uint, username *string, date *string) (dto.PageQueryResult, error) {
	// 参数校验
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}

	offset := (page - 1) * pageSize

	// 查询数据库
	var messages []models.Message
	var total int64

	// 基础查询
	q := database.DB.Model(&models.Message{})
	// 作者筛选
	if authorID != nil {
		q = q.Where("user_id = ?", *authorID)
	} else if username != nil && *username != "" {
		q = q.Where("username = ?", *username)
	}
	if date != nil && strings.TrimSpace(*date) != "" {
		day, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(*date), shanghaiLocation())
		if err != nil {
			return dto.PageQueryResult{}, fmt.Errorf("日期格式无效")
		}
		q = q.Where("created_at >= ? AND created_at < ?", day, day.AddDate(0, 0, 1))
	}
	// 隐私过滤
	q = ApplyMessageVisibilityScope(q, userID, isAdmin)

	q.Count(&total)
	if err := q.Limit(pageSize).Offset(offset).Order("pinned DESC, created_at DESC").Find(&messages).Error; err != nil {
		return dto.PageQueryResult{}, err
	}

	// 返回结果
	return dto.PageQueryResult{Total: total, Items: messages}, nil
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
	return repository.CreateMessage(message)
}

// DeleteMessage 根据 ID 删除笔记
func DeleteMessage(id uint, userID uint) error {
	// 获取笔记信息
	message, err := repository.GetMessageByID(id, true)
	if err != nil {
		return err
	}

	// 验证是否为笔记作者
	if message.UserID != userID {
		return fmt.Errorf("无权删除他人的笔记")
	}

	return repository.DeleteMessage(id)
}

// DeleteMessageByAdmin 管理员删除笔记（无需验证作者）
func DeleteMessageByAdmin(id uint) error {
	return repository.DeleteMessage(id)
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

	// 处理域名和端口
	requestHost := c.Request.Host
	var baseURL string

	// 从配置获取站点URL，如果没有则使用请求的host
	configURL := ""

	// 检查请求来源是否为反向代理域名
	if strings.Contains(requestHost, "note.noisework.cn") {
		// 如果是从反向代理域名访问的，使用完整的反向代理域名
		baseURL = "https://note.noisework.cn"
	} else if configURL != "" {
		// 使用配置的URL
		baseURL = configURL
		// 确保配置的URL包含正确的端口
		if !strings.Contains(baseURL, ":") && requestHost != "note.noisework.cn" {
			// 从请求中提取端口
			parts := strings.Split(requestHost, ":")
			if len(parts) == 2 && parts[1] == "1314" {
				baseURL = baseURL + ":1314"
			}
		}
	} else {
		// 如果没有配置，确保使用完整的请求地址（包括端口）
		if strings.Contains(requestHost, ":") {
			baseURL = schema + "://" + requestHost
		} else {
			// 默认添加1314端口，如果是直接IP或域名访问
			baseURL = schema + "://" + requestHost + ":1314"
		}
	}

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
	if err := database.DB.Save(message).Error; err != nil {
		return nil, fmt.Errorf("更新消息失败: %v", err)
	}
	return message, nil
}

// UpdateMessagePinned 更新消息置顶状态
func UpdateMessagePinned(messageID uint, pinned bool) error {
	message, err := repository.GetMessageByID(messageID, true)
	if err != nil {
		return fmt.Errorf("获取消息失败: %v", err)
	}
	if message == nil {
		return fmt.Errorf("消息不存在")
	}
	message.Pinned = pinned
	if err := database.DB.Save(message).Error; err != nil {
		return fmt.Errorf("更新置顶状态失败: %v", err)
	}
	return nil
}

// IncrementLikeCount 点赞计数加一
func IncrementLikeCount(messageID uint) (int, error) {
	message, err := GetMessageByIDForViewer(messageID, nil, false)
	if err != nil {
		return 0, fmt.Errorf("获取消息失败: %v", err)
	}
	if message == nil {
		return 0, fmt.Errorf("消息不存在")
	}
	message.LikeCount = message.LikeCount + 1
	if err := database.DB.Save(message).Error; err != nil {
		return 0, fmt.Errorf("更新点赞失败: %v", err)
	}
	return message.LikeCount, nil
}

// ToggleLike 根据 session 或用户切换点赞状态
func ToggleLike(messageID uint, userID *uint, sessionID string, isAdmin bool) (bool, int, error) {
	if sessionID == "" && (userID == nil || *userID == 0) {
		return false, 0, fmt.Errorf("缺少会话或用户信息")
	}
	var currentUserID *uint
	if userID != nil && *userID != 0 {
		currentUserID = userID
	}
	message, err := GetMessageByIDForViewer(messageID, currentUserID, isAdmin)
	if err != nil {
		return false, 0, err
	}
	if !CanInteractWithMessage(*message, currentUserID) {
		return false, 0, fmt.Errorf("无权点赞该内容")
	}
	// 查询是否已有点赞
	var existing models.MessageLike
	q := database.DB.Where("message_id = ?", messageID)
	if userID != nil && *userID != 0 {
		q = q.Where("user_id = ?", *userID)
	} else {
		q = q.Where("session_id = ?", sessionID)
	}
	if err := q.First(&existing).Error; err == nil && existing.ID != 0 {
		// 已点赞 -> 取消
		if err := database.DB.Delete(&existing).Error; err != nil {
			return false, 0, err
		}
	} else {
		// 未点赞 -> 新增
		like := models.MessageLike{MessageID: messageID, SessionID: sessionID}
		if userID != nil && *userID != 0 {
			like.UserID = userID
		}
		if err := database.DB.Create(&like).Error; err != nil {
			return false, 0, err
		}
	}
	// 重新统计总数并同步
	var cnt int64
	if err := database.DB.Model(&models.MessageLike{}).Where("message_id = ?", messageID).Count(&cnt).Error; err != nil {
		return false, 0, err
	}
	if err := database.DB.Model(&models.Message{}).Where("id = ?", messageID).Update("like_count", cnt).Error; err != nil {
		return false, int(cnt), err
	}
	// 再查一次是否点赞状态
	var check models.MessageLike
	q2 := database.DB.Where("message_id = ?", messageID)
	if userID != nil && *userID != 0 {
		q2 = q2.Where("user_id = ?", *userID)
	} else {
		q2 = q2.Where("session_id = ?", sessionID)
	}
	liked := q2.First(&check).Error == nil && check.ID != 0
	return liked, int(cnt), nil
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

func GetMessagesGroupByDate(userID *uint, isAdmin bool, authorID *uint) ([]MessageDateCount, error) {
	type createdAtRow struct {
		CreatedAt time.Time `json:"created_at"`
	}

	var rows []createdAtRow
	q := database.DB.Table("messages")
	if authorID != nil {
		q = q.Where("user_id = ?", *authorID)
	}
	q = ApplyMessageVisibilityScope(q, userID, isAdmin)
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
func GetMessagePage(id uint, userID *uint, isAdmin bool) (*models.Message, error) {
	return GetMessageByIDForViewer(id, userID, isAdmin)
}

func SearchMessages(keyword string, page, pageSize int, userID *uint, isAdmin bool, authorID *uint, username *string) (dto.PageQueryResult, error) {
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
	query = ApplyMessageVisibilityScope(query, userID, isAdmin)

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

	// 确保返回的数据结构符合前端期望
	return dto.PageQueryResult{
		Total: total,
		Items: messages,
	}, nil
}
