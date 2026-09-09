package controllers

import (
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/internal/authorization"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/dto"
	"github.com/rcy1314/echo-noise/internal/middleware"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/services"
	"github.com/rcy1314/echo-noise/internal/syncmanager"
)

func UpdateMessage(c *gin.Context) {
	// 获取消息ID
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 0, "msg": "消息ID不能为空"})
		return
	}

	// 检查用户权限
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 0, "msg": "未授权访问"})
		return
	}

	var req struct {
		Content    *string `json:"content"`
		Private    *bool   `json:"private"`
		Visibility *string `json:"visibility"`
		CreatedAt  *string `json:"created_at"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 0, "msg": "请求参数错误"})
		return
	}

	createdAt, err := parseMessageCreatedAt(req.CreatedAt)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 0, "msg": err.Error()})
		return
	}

	// 检查消息是否存在并且属于当前用户
	messageID, err := strconv.ParseUint(id, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 0, "msg": "无效的消息ID"})
		return
	}

	// 检查消息所有权或管理员权限
	message, err := services.GetMessageByID(uint(messageID), true)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 0, "msg": "消息不存在"})
		return
	}

	actorID, ok := commentUint(userID)
	if !ok || actorID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 0, "msg": "未授权访问"})
		return
	}
	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 0, "msg": "授权服务不可用"})
		return
	}
	authorizer := authorization.New(db)
	requireMutation := func(capability authorization.Capability, messageText string) bool {
		if decision := services.AuthorizeMessageMutation(db, actorID, *message, capability); !decision.Allowed {
			writeMessageMutationDeniedAuditForDecision(c, authorizer, actorID, capability, "update", message, decision)
			if decision.Reason == authorization.DenialContentNotReadable {
				c.JSON(http.StatusNotFound, gin.H{"code": 0, "msg": "消息不存在"})
			} else {
				c.JSON(http.StatusForbidden, gin.H{"code": 0, "msg": messageText})
			}
			return false
		}
		return true
	}
	if createdAt != nil {
		if !requireMutation(authorization.CapabilityNotesPublishTime, "无权限调整发布时间") {
			return
		}
	}
	if message.UserID != actorID {
		if req.Content != nil && !requireMutation(authorization.CapabilityNotesEdit, "无权限修改此消息") {
			return
		}
		if (req.Private != nil || req.Visibility != nil) && !requireMutation(authorization.CapabilityNotesVisibility, "无权限调整可见范围") {
			return
		}
	}

	updated, err := services.UpdateMessage(uint(messageID), req.Content, req.Private, req.Visibility, createdAt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 0, "msg": err.Error()})
		return
	}
	if message.UserID != actorID {
		capability := authorization.CapabilityNotesEdit
		if req.Content == nil && (req.Private != nil || req.Visibility != nil) {
			capability = authorization.CapabilityNotesVisibility
		} else if req.Content == nil && createdAt != nil {
			capability = authorization.CapabilityNotesPublishTime
		}
		if err := writeMessageMutationSuccessAudit(c, authorizer, actorID, capability, "update", message); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 0, "msg": "写入管理员审计失败"})
			return
		}
		middleware.MarkSemanticAuditWritten(c)
	}

	// Mutation results replace the rendered note, so they need the same
	// viewer-scoped interaction decision as list/read responses.
	updated.CanInteract = services.CanInteractWithMessage(*updated, &actorID)
	c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "更新成功", "data": updated})

	// 即时模式触发云同步（防抖）
	syncmanager.Trigger()
}

// 更新消息置顶状态
func UpdateMessagePinned(c *gin.Context) {
	// Compatibility alias: the legacy route is still a global-pin operation,
	// so it must pass the same administrator authorization and audit path.
	UpdateMessageGlobalPin(c)
}

// 点赞接口：POST /api/messages/:id/like
func IncrementMessageLike(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 0, "msg": "消息ID不能为空"})
		return
	}
	messageID, err := strconv.ParseUint(id, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 0, "msg": "无效的消息ID"})
		return
	}
	userID, ok := commentAuthUserID(c)
	if !ok || userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 0, "msg": "请先登录后再点赞"})
		return
	}
	created, count, err := services.IncrementLikeCount(uint(messageID), userID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": err.Error()})
		return
	}
	if created {
		if err := services.CreateNotificationForLike(uint(messageID), userID); err != nil {
			log.Printf("创建点赞通知失败: %v", err)
		}
	}
	c.JSON(http.StatusOK, gin.H{"code": 1, "data": map[string]interface{}{"liked": true, "like_count": count}})
}

// 点赞切换：POST /api/messages/:id/like/toggle
func ToggleMessageLike(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 0, "msg": "消息ID不能为空"})
		return
	}
	messageID, err := strconv.ParseUint(id, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 0, "msg": "无效的消息ID"})
		return
	}

	userID, ok := commentAuthUserID(c)
	if !ok || userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 0, "msg": "请先登录后再点赞"})
		return
	}
	uid := userID
	liked, count, err := services.ToggleLike(uint(messageID), &uid, "")
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": err.Error()})
		return
	}
	if liked {
		if err := services.CreateNotificationForLike(uint(messageID), userID); err != nil {
			log.Printf("创建点赞通知失败: %v", err)
		}
	}
	c.JSON(http.StatusOK, gin.H{"code": 1, "data": map[string]interface{}{"liked": liked, "like_count": count}})
}
func GetMessagesCalendar(c *gin.Context) {
	currentUserID, _ := currentMessageViewer(c)

	var authorID *uint
	if aid := strings.TrimSpace(c.Query("authorId")); aid != "" {
		if v, err := strconv.ParseUint(aid, 10, 64); err == nil && v > 0 {
			vv := uint(v)
			authorID = &vv
		}
	}

	calendarData, err := services.GetMessagesGroupByDate(currentUserID, authorID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 1,
		"data": calendarData,
	})
}
func SearchMessages(c *gin.Context) {
	// 从查询参数获取数据
	keyword := c.Query("keyword")
	page := 1
	pageSize := 10

	// 尝试解析页码和每页数量
	if pageStr := c.Query("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}
	if sizeStr := c.Query("pageSize"); sizeStr != "" {
		if s, err := strconv.Atoi(sizeStr); err == nil && s > 0 {
			pageSize = s
		}
	}

	currentUserID, _ := currentMessageViewer(c)

	// 可选作者筛选
	var authorID *uint
	if aid := c.Query("authorId"); aid != "" {
		if v, err := strconv.ParseUint(aid, 10, 64); err == nil {
			vv := uint(v)
			authorID = &vv
		}
	}
	var username *string
	if un := c.Query("username"); strings.TrimSpace(un) != "" {
		u := strings.TrimSpace(un)
		username = &u
	}

	result, err := services.SearchMessages(keyword, page, pageSize, currentUserID, authorID, username)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code": 0,
			"msg":  err.Error(),
			"data": nil,
		})
		return
	}

	// 直接构造符合前端期望的JSON格式
	c.JSON(http.StatusOK, gin.H{
		"code": 1,
		"msg":  "搜索成功",
		"data": result,
	})
}

func parseMessageCreatedAt(raw *string) (*time.Time, error) {
	if raw == nil {
		return nil, nil
	}
	value := strings.TrimSpace(*raw)
	if value == "" {
		return nil, fmt.Errorf("发布时间不能为空")
	}

	if t, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return &t, nil
	}

	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("Asia/Shanghai", 8*60*60)
	}
	for _, layout := range []string{
		"2006-01-02T15:04:05",
		"2006-01-02T15:04",
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
	} {
		if t, err := time.ParseInLocation(layout, value, loc); err == nil {
			return &t, nil
		}
	}

	return nil, fmt.Errorf("发布时间格式错误")
}

// 保留这个新版本的 PostMessage 函数
func PostMessage(c *gin.Context) {
	// 解析请求数据
	var request struct {
		Content    string  `json:"content"`
		Private    bool    `json:"private"`
		Visibility string  `json:"visibility"`
		ImageURL   string  `json:"image_url"`
		VideoURL   string  `json:"video_url"` // 新增视频字段
		Notify     *bool   `json:"notify"`
		CreatedAt  *string `json:"created_at"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusOK, dto.Fail[string]("内容不能为空"))
		return
	}

	createdAt, err := parseMessageCreatedAt(request.CreatedAt)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](err.Error()))
		return
	}

	// 验证用户身份
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusOK, dto.Fail[string]("未授权访问"))
		return
	}
	user, err := services.GetUserByID(userID.(uint))
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[string]("获取用户信息失败"))
		return
	}
	var siteCfg models.SiteConfig
	_ = database.DB.Table("site_configs").First(&siteCfg).Error
	viaStr := c.GetString("auth_via")
	shouldNotify := shouldNotifyPublishedMessage(siteCfg.NotifyEnabled, user.IsAdmin, viaStr, request.Notify)
	if shouldNotify {
		actorID, ok := commentUint(userID)
		db, dbErr := database.GetDB()
		if !ok || dbErr != nil || !authorization.New(db).Authorize(actorID, authorization.CapabilityNotificationsManage, nil).Allowed {
			c.JSON(http.StatusForbidden, dto.Fail[string]("无权限发送发布通知"))
			return
		}
	}
	if createdAt != nil {
		actorID, ok := commentUint(userID)
		db, dbErr := database.GetDB()
		if !ok || dbErr != nil || !authorization.New(db).Authorize(actorID, authorization.CapabilityNotesPublishTime, nil).Allowed {
			c.JSON(http.StatusOK, dto.Fail[string]("仅管理员可以指定发布时间"))
			return
		}
	}

	// 创建消息
	message := &models.Message{
		Content:    request.Content,
		Private:    request.Private,
		Visibility: request.Visibility,
		ImageURL:   request.ImageURL,
		UserID:     userID.(uint),
	}
	if createdAt != nil {
		message.CreatedAt = *createdAt
	}

	if err := services.CreateMessage(message); err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](err.Error()))
		return
	}

	// 推送策略仅对管理员生效：会话发布需显式 notify=true，管理员 token 发布跟随总开关自动推送。
	if shouldNotify {
		notifyConfig := models.GetNotifyConfig()
		if notifyConfig != nil {
			// 提取内容中的第一张图片链接
			var firstImageURL string
			var firstVideoURL string
			var formattedContent string

			// 如果已有上传的图片，优先使用
			if request.ImageURL != "" {
				firstImageURL = request.ImageURL
			}
			// 如果已有上传的视频，优先使用
			if request.VideoURL != "" {
				firstVideoURL = request.VideoURL
			}

			cleanContent, extractedImages := models.ExtractImageURLsFromMarkdown(request.Content)
			if firstImageURL == "" && len(extractedImages) > 0 {
				firstImageURL = extractedImages[0]
			}

			// 从 Markdown 内容中提取第一段视频（如 [video](url)）
			videoRegex := regexp.MustCompile(`\[video\]\(([^)]+)\)`)
			videoMatches := videoRegex.FindAllStringSubmatch(request.Content, -1)
			if firstVideoURL == "" && len(videoMatches) > 0 {
				firstVideoURL = videoMatches[0][1]
			}

			formattedContent = cleanContent

			// 处理长内容，如果超过4000字符，进行截断
			const maxContentLength = 4000
			var truncatedContent string
			if len(formattedContent) > maxContentLength {
				truncatedContent = formattedContent[:maxContentLength] + "...\n(内容过长，已截断)"
			} else {
				truncatedContent = formattedContent
			}

			// 格式化内容，处理Markdown语法
			headingRegex := regexp.MustCompile(`(?m)^(#{1,6})\s+(.+)$`)
			truncatedContent = headingRegex.ReplaceAllString(truncatedContent, "$1 $2")

			// 准备图片和视频数组
			var images []string
			var videos []string
			if request.ImageURL != "" {
				images = append(images, request.ImageURL)
			}
			if len(extractedImages) > 0 {
				images = append(images, extractedImages...)
			}
			if firstVideoURL != "" {
				videos = []string{firstVideoURL}
			}

			go func() {
				// Webhook
				if notifyConfig.WebhookEnabled && notifyConfig.WebhookURL != "" {
					models.SendWebhook(truncatedContent)
				}

				// Telegram
				if notifyConfig.TelegramEnabled && notifyConfig.TelegramToken != "" && notifyConfig.TelegramChatID != "" {
					const telegramMaxText = 4096
					const telegramMaxCaption = 1024

					isPublicURL := func(url string) bool {
						return strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://")
					}

					// 推送图片
					if len(images) > 0 {
						allPublic := true
						for _, img := range images {
							if !isPublicURL(img) {
								allPublic = false
								break
							}
						}

						if allPublic {
							caption := formattedContent
							if len([]rune(caption)) > telegramMaxCaption {
								msg := caption
								if len([]rune(msg)) > telegramMaxText {
									msg = string([]rune(msg)[:telegramMaxText]) + "...\n(内容过长，已截断)"
								}
								if err := models.SendTelegramMessage(msg); err != nil {
									sendTelegramErrorNotify(c, err)
								}
								caption = ""
							}

							if len(images) == 1 {
								if err := models.SendTelegramPhotoWithCaption(images[0], caption); err != nil {
									sendTelegramErrorNotify(c, err)
								}
							} else {
								if err := models.SendTelegramMediaGroupWithCaption(images, caption); err != nil {
									sendTelegramErrorNotify(c, err)
								}
							}
						} else {
							msg := formattedContent
							for _, img := range images {
								msg += "\n[图片] " + img
							}
							if len([]rune(msg)) > telegramMaxText {
								msg = string([]rune(msg)[:telegramMaxText]) + "...\n(内容过长，已截断)"
							}
							if err := models.SendTelegramMessage(msg); err != nil {
								sendTelegramErrorNotify(c, err)
							}
						}
					}

					// 推送视频
					if len(videos) > 0 {
						if isPublicURL(videos[0]) {
							caption := formattedContent
							if len(caption) > telegramMaxCaption {
								caption = caption[:telegramMaxCaption] + "...\n(内容过长，已截断)"
							}
							err := models.SendTelegramVideoWithCaption(videos[0], caption)
							if err != nil {
								sendTelegramErrorNotify(c, err)
							}
						} else {
							msg := formattedContent + "\n[视频] " + videos[0]
							if len(msg) > telegramMaxText {
								msg = msg[:telegramMaxText] + "...\n(内容过长，已截断)"
							}
							err := models.SendTelegramMessage(msg)
							if err != nil {
								sendTelegramErrorNotify(c, err)
							}
						}
					}

					// 没有图片和视频，直接发文本
					if len(images) == 0 && len(videos) == 0 {
						if len(formattedContent) > telegramMaxText {
							sendTelegramErrorNotify(c, fmt.Errorf("Telegram 文本内容超出最大长度（%d 字符）", telegramMaxText))
						} else {
							err := models.SendTelegramMessage(formattedContent)
							if err != nil {
								sendTelegramErrorNotify(c, err)
							}
						}
					}
				}

				// 企业微信
				if notifyConfig.WeworkEnabled && notifyConfig.WeworkKey != "" {
					const weworkMaxLength = 2000
					var weworkContent string
					if len(formattedContent) > weworkMaxLength {
						weworkContent = formattedContent[:weworkMaxLength] + "...\n(内容过长，已截断)"
					} else {
						weworkContent = formattedContent
					}
					models.SendWework(weworkContent, images)
				}

				// 飞书
				if notifyConfig.FeishuEnabled && notifyConfig.FeishuWebhook != "" {
					const feishuMaxLength = 2000
					var feishuContent string
					if len(formattedContent) > feishuMaxLength {
						feishuContent = formattedContent[:feishuMaxLength] + "...\n(内容过长，已截断)"
					} else {
						feishuContent = formattedContent
					}
					models.SendFeishu(feishuContent)
				}
			}()
		}
	}

	c.JSON(http.StatusOK, dto.OK(message, "发布成功"))

	// 即时模式触发云同步（防抖）
	syncmanager.Trigger()
}

func shouldNotifyPublishedMessage(siteEnabled bool, isAdmin bool, authVia string, requested *bool) bool {
	if !siteEnabled || !isAdmin {
		return false
	}
	if authVia == "token" {
		return true
	}
	return requested != nil && *requested
}
