package controllers

import (
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/services"
)

var (
	messageTagExtractRegexp = regexp.MustCompile(`#([^\s#\p{P}\p{S}]+)([/?=&][^\s#]*)?`)
	invalidMessageTagRegexp = regexp.MustCompile(`[/?=&]|^(song|video|playlist)\?id=\d+$`)
	markdownImageRegexp     = regexp.MustCompile(`!\[.*?\]\((.*?)\)`)
)

func extractMessageTags(content string) []string {
	matches := messageTagExtractRegexp.FindAllStringSubmatch(content, -1)
	tags := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) <= 1 || match[1] == "" || invalidMessageTagRegexp.MatchString(match[1]) {
			continue
		}
		if len(match) > 2 && match[2] != "" {
			continue
		}
		tags = append(tags, match[1])
	}
	return tags
}

func messageHasTag(content string, tag string) bool {
	for _, current := range extractMessageTags(content) {
		if current == tag {
			return true
		}
	}
	return false
}

func isHomeStatsExcludedMessage(content string) bool {
	return services.IsGuestbookMessage(models.Message{Content: content}) ||
		strings.Contains(content, "关于本站") ||
		strings.Contains(content, "友情链接")
}

// GetCurrentUserHomeStats 获取当前登录用户首页个人统计。
func GetCurrentUserHomeStats(c *gin.Context) {
	user, err := checkUser(c)
	if err != nil || user == nil || user.ID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 0, "msg": "未登录或登录已过期"})
		return
	}

	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "数据库连接失败", "data": gin.H{}})
		return
	}

	var messages []models.Message
	if err := db.Select("id", "content", "image_url", "user_id").Where("deleted_at IS NULL AND user_id = ?", user.ID).Find(&messages).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "获取个人统计失败", "data": gin.H{}})
		return
	}

	tagSet := make(map[string]struct{})
	totalMessages := 0
	imageURLs := make([]string, 0, len(messages))
	for _, msg := range messages {
		if isHomeStatsExcludedMessage(msg.Content) {
			continue
		}
		totalMessages++
		for _, tag := range extractMessageTags(msg.Content) {
			tagSet[tag] = struct{}{}
		}
		if strings.TrimSpace(msg.ImageURL) != "" {
			imageURLs = append(imageURLs, msg.ImageURL)
		}
		for _, match := range markdownImageRegexp.FindAllStringSubmatch(msg.Content, -1) {
			if len(match) > 1 && strings.TrimSpace(match[1]) != "" {
				imageURLs = append(imageURLs, match[1])
			}
		}
	}

	// 与最新图集保持同一口径：附件已被删除的图片不再计入，避免统计数虚高。
	availability := newImageAvailability(imageURLs)
	totalImages := 0
	for _, imageURL := range imageURLs {
		if !availability.Has(imageURL) {
			continue
		}
		totalImages++
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 1,
		"data": gin.H{
			"total_messages": totalMessages,
			"total_tags":     len(tagSet),
			"total_images":   totalImages,
		},
	})
}

// GetMessagesByTag 获取指定标签的消息
func GetMessagesByTag(c *gin.Context) {
	tag := c.Param("tag")
	if tag == "" {
		c.JSON(http.StatusOK, gin.H{"code": 1, "data": []models.Message{}})
		return
	}

	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 1, "data": []models.Message{}})
		return
	}

	var messages []models.Message
	// 使用 LIKE 进行初步筛选
	tagPattern := "%#" + tag + "%"
	q := db.Where("content LIKE ?", tagPattern)
	// 作者筛选（可选）
	if aid := c.Query("authorId"); aid != "" {
		if v, err := strconv.ParseUint(aid, 10, 64); err == nil {
			q = q.Where("user_id = ?", uint(v))
		}
	}
	if un := c.Query("username"); un != "" {
		q = q.Where("username = ?", un)
	}
	currentUserID, _ := currentMessageViewer(c)
	q = services.ApplyMessageVisibilityScope(q, currentUserID)
	if err := q.Order("created_at DESC").Find(&messages).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 1, "data": []models.Message{}})
		return
	}
	services.ApplyMessageViewerState(messages, currentUserID)

	// 使用统一标签提取逻辑进行精确匹配
	var filteredMessages []models.Message
	for _, msg := range messages {
		if messageHasTag(msg.Content, tag) {
			filteredMessages = append(filteredMessages, msg)
		}
	}

	c.JSON(http.StatusOK, gin.H{"code": 1, "data": filteredMessages})
}

// GetAllTags 获取所有标签列表
func GetAllTags(c *gin.Context) {
	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 1, "data": []map[string]interface{}{}})
		return
	}

	// 添加缓存控制头
	c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
	c.Header("Pragma", "no-cache")
	c.Header("Expires", "0")

	var messages []models.Message
	currentUserID, _ := currentMessageViewer(c)
	q := services.ApplyMessageVisibilityScope(db.Model(&models.Message{}).Select("content", "private", "visibility", "user_id"), currentUserID)
	if err := q.Order("created_at DESC").Find(&messages).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 1, "data": []map[string]interface{}{}})
		return
	}

	// 提取并统计标签
	tagMap := make(map[string]int)

	for _, msg := range messages {
		for _, tag := range extractMessageTags(msg.Content) {
			tagMap[tag]++
		}
	}

	// 转换为数组格式并按计数倒序排序
	var tagList []map[string]interface{}
	for tag, count := range tagMap {
		tagList = append(tagList, map[string]interface{}{
			"name":  tag,
			"count": count,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"code":      1,
		"data":      tagList,
		"timestamp": time.Now().Unix(), // 添加时间戳
	})
}

// GetAllImages 获取所有图片列表.
func GetAllImages(c *gin.Context) {
	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 1, "data": []map[string]interface{}{}})
		return
	}

	var messages []models.Message
	q := db.Select("id", "content", "image_url", "created_at", "private", "visibility", "user_id").Order("created_at DESC")
	viewerID, hasViewer := resolveImageViewer(c)
	var currentUserID *uint
	if hasViewer {
		id := viewerID
		currentUserID = &id
	}
	q = services.ApplyMessageVisibilityScope(q, currentUserID)
	if err := q.Find(&messages).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 1, "data": []map[string]interface{}{}})
		return
	}

	type ImageInfo struct {
		ID        uint      `json:"id"`
		ImageURL  string    `json:"image_url"`
		CreatedAt time.Time `json:"created_at"`
	}

	candidates := make([]ImageInfo, 0, len(messages))
	for _, msg := range messages {
		if msg.ImageURL != "" {
			candidates = append(candidates, ImageInfo{
				ID:        msg.ID,
				ImageURL:  msg.ImageURL,
				CreatedAt: msg.CreatedAt,
			})
		}

		matches := markdownImageRegexp.FindAllStringSubmatch(msg.Content, -1)
		for _, match := range matches {
			if len(match) > 1 {
				candidates = append(candidates, ImageInfo{
					ID:        msg.ID,
					ImageURL:  match[1],
					CreatedAt: msg.CreatedAt,
				})
			}
		}
	}

	// 附件已被删除的图片不再返回，避免图集出现无法加载的占位图并让计数虚高。
	candidateURLs := make([]string, 0, len(candidates))
	for _, image := range candidates {
		candidateURLs = append(candidateURLs, image.ImageURL)
	}
	availability := newImageAvailability(candidateURLs)
	allImages := make([]ImageInfo, 0, len(candidates))
	for _, image := range candidates {
		if !availability.Has(image.ImageURL) {
			continue
		}
		allImages = append(allImages, image)
	}

	c.JSON(http.StatusOK, gin.H{"code": 1, "data": allImages})
}

func resolveImageViewer(c *gin.Context) (uint, bool) {
	if user, ok := currentReadUser(c); ok {
		return user.ID, true
	}
	return 0, false
}

// GetMessagePage 处理消息详情页请求
func GetMessagePage(c *gin.Context) {
	id := c.Param("id")
	messageID, err := strconv.ParseUint(id, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 0, "msg": "无效的消息ID"})
		return
	}

	currentUserID, _ := currentMessageViewer(c)
	message, err := services.GetMessagePage(uint(messageID), currentUserID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 0, "msg": "消息不存在"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 1,
		"data": message,
	})
}
