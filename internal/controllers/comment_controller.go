package controllers

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/internal/authorization"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/dto"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/services"
)

// 评论账号鉴权：兼容当前登录逻辑写入的 user_id session，保留中间件注入上下文的可能性。
func commentAuthUserID(c *gin.Context) (uint, bool) {
	if user, ok := currentReadUser(c); ok {
		return user.ID, true
	}
	return 0, false
}

func normalizeCommentVisibility(value string) (string, bool) {
	return services.NormalizeCommentVisibility(value)
}

func normalizedCommentVisibilityOrPublic(value string) string {
	return services.NormalizedCommentVisibilityOrPublic(value)
}

func commentVisibilityRank(value string) int {
	return services.CommentVisibilityRank(value)
}

func defaultCommentVisibilityForMessage(messageVisibility string) string {
	return services.DefaultCommentVisibilityForMessage(messageVisibility)
}

func normalizeRequestedCommentVisibility(value string, messageVisibility string) (string, bool) {
	if strings.TrimSpace(value) == "" {
		return defaultCommentVisibilityForMessage(messageVisibility), true
	}
	return normalizeCommentVisibility(value)
}

func commentVisibilityAllowedForMessage(visibility string, messageVisibility string) bool {
	return services.CommentVisibilityAllowedForMessage(visibility, messageVisibility)
}

func effectiveCommentVisibilityForMessage(visibility string, messageVisibility string) string {
	return services.EffectiveCommentVisibilityForMessage(visibility, messageVisibility)
}

func isGuestbookMessage(message models.Message) bool {
	return services.IsGuestbookMessage(message)
}

func canViewComment(message models.Message, comment models.Comment, commentMap map[uint]models.Comment, viewerID uint, hasViewer bool) bool {
	return services.CanViewCommentInThread(message, comment, commentMap, viewerID, hasViewer)
}

func authorizeCommentMutation(c *gin.Context, message models.Message, comment models.Comment, commentMap map[uint]models.Comment, capability authorization.Capability) authorization.Decision {
	userID, ok := commentAuthUserID(c)
	if !ok {
		return authorization.Decision{Reason: authorization.DenialNotAdministrator}
	}
	if comment.UserID != nil && *comment.UserID == userID {
		return authorization.Decision{Allowed: true}
	}
	db, err := database.GetDB()
	if err != nil {
		return authorization.Decision{Reason: authorization.DenialContentNotReadable}
	}
	decision := services.AuthorizeCommentMutation(db, userID, message, comment, commentMap, capability)
	if !decision.Allowed && decision.Reason != authorization.DenialNotAdministrator {
		authorization.New(db).WriteDeniedBestEffort(commentMutationAuditRecord(c, userID, capability, "mutation", "denied", "comment mutation denied", string(decision.Reason), &comment))
	}
	return decision
}

func commentUint(v any) (uint, bool) {
	switch val := v.(type) {
	case uint:
		return val, true
	case int:
		if val >= 0 {
			return uint(val), true
		}
	case int64:
		if val >= 0 {
			return uint(val), true
		}
	case float64:
		if val >= 0 {
			return uint(val), true
		}
	case string:
		s := strings.TrimSpace(val)
		if s == "" {
			return 0, false
		}
		n, err := strconv.ParseUint(s, 10, 64)
		if err == nil {
			return uint(n), true
		}
	}
	return 0, false
}

// 获取指定消息的评论列表（内置评论系统）
func GetComments(c *gin.Context) {
	idStr := c.Param("id")
	msgID, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil || msgID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 0, "msg": "无效的消息ID"})
		return
	}
	db, _ := database.GetDB()
	var message models.Message
	if err := db.First(&message, msgID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 0, "msg": "消息不存在"})
		return
	}
	var comments []models.Comment
	if err := db.Where("message_id = ?", msgID).Order("created_at ASC").Find(&comments).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 0, "msg": "获取评论失败"})
		return
	}
	var actorID *uint
	if user, ok := currentReadUser(c); ok {
		id := user.ID
		actorID = &id
	}
	scope, scopeErr := services.ResolveContentReadScope(db, actorID)
	messageForRead := message
	if message.IsTombstone {
		messageForRead.DeletedAt = nil
	}
	if scopeErr != nil || !scope.CanReadMessage(messageForRead) {
		c.JSON(http.StatusNotFound, gin.H{"code": 0, "msg": "消息不存在"})
		return
	}
	commentMap := services.CommentMap(comments)
	visibleActive := make(map[uint]models.Comment)
	requiredTombstones := make(map[uint]bool)
	for _, comment := range comments {
		if comment.DeletedAt == nil && !comment.IsTombstone && scope.CanReadComment(messageForRead, comment, commentMap) {
			comment.CanInteract = scope.CanInteractWithComment(messageForRead, comment, commentMap)
			visibleActive[comment.ID] = comment
			seen := map[uint]bool{comment.ID: true}
			parentID := comment.ParentID
			for parentID != nil && !seen[*parentID] {
				seen[*parentID] = true
				parent, ok := commentMap[*parentID]
				if !ok {
					break
				}
				if parent.IsTombstone {
					requiredTombstones[parent.ID] = true
				}
				parentID = parent.ParentID
			}
		}
	}
	visibleComments := make([]models.Comment, 0, len(visibleActive)+len(requiredTombstones))
	for _, comment := range comments {
		if visible, ok := visibleActive[comment.ID]; ok {
			visibleComments = append(visibleComments, visible)
			continue
		}
		if comment.IsTombstone && requiredTombstones[comment.ID] {
			comment.Content = ""
			comment.UserID = nil
			comment.User = nil
			comment.CanInteract = false
			comment.Visibility = ""
			comment.TombstoneVisibility = ""
			visibleComments = append(visibleComments, comment)
		}
	}
	userIDs := make([]uint, 0)
	seenUserIDs := map[uint]bool{}
	for _, comment := range visibleComments {
		if comment.UserID != nil && *comment.UserID > 0 {
			if !seenUserIDs[*comment.UserID] {
				seenUserIDs[*comment.UserID] = true
				userIDs = append(userIDs, *comment.UserID)
			}
		}
	}
	if len(userIDs) > 0 {
		var users []models.User
		if err := db.Select("id, username, avatar_url").Where("id IN ?", userIDs).Find(&users).Error; err == nil {
			userInfo := map[uint]models.CommentUserInfo{}
			for _, user := range users {
				userInfo[user.ID] = models.CommentUserInfo{
					ID:        user.ID,
					Username:  strings.TrimSpace(user.Username),
					AvatarURL: strings.TrimSpace(user.AvatarURL),
				}
			}
			for i := range visibleComments {
				if visibleComments[i].UserID == nil {
					continue
				}
				if info, ok := userInfo[*visibleComments[i].UserID]; ok {
					visibleComments[i].User = &info
				}
			}
		}
	}
	c.JSON(http.StatusOK, gin.H{"code": 1, "data": visibleComments})
}

// 批量获取评论数量
func GetCommentCounts(c *gin.Context) {
	var req struct {
		IDs []uint `json:"ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 0, "msg": "请求参数错误"})
		return
	}
	db, _ := database.GetDB()
	var messages []models.Message
	if err := db.Where("id IN ?", req.IDs).Find(&messages).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 0, "msg": "获取评论数量失败"})
		return
	}
	messageMap := make(map[uint]models.Message, len(messages))
	for _, message := range messages {
		messageMap[message.ID] = message
	}
	var comments []models.Comment
	if err := db.Where("message_id IN ?", req.IDs).Order("created_at ASC").Find(&comments).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 0, "msg": "获取评论数量失败"})
		return
	}
	var actorID *uint
	if user, ok := currentReadUser(c); ok {
		id := user.ID
		actorID = &id
	}
	scope, scopeErr := services.ResolveContentReadScope(db, actorID)
	if scopeErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 0, "msg": "获取评论数量失败"})
		return
	}
	counts := make(map[uint]int64)
	commentMap := services.CommentMap(comments)
	for _, comment := range comments {
		if comment.ParentID != nil {
			continue
		}
		message, ok := messageMap[comment.MessageID]
		if !ok {
			continue
		}
		if scope.CanReadComment(message, comment, commentMap) {
			counts[comment.MessageID]++
		}
	}
	result := make([]gin.H, 0, len(counts))
	for _, id := range req.IDs {
		if cnt, ok := counts[id]; ok {
			result = append(result, gin.H{"id": id, "count": cnt})
		}
	}
	c.JSON(http.StatusOK, gin.H{"code": 1, "data": result})
}

// GetGuestbookMessageID 获取或创建用于留言板的独立消息ID
func GetGuestbookMessageID(c *gin.Context) {
	db, _ := database.GetDB()
	descriptor, err := services.EnsureGuestbook(db)
	if err != nil || descriptor.MessageID == 0 {
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "初始化留言板失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 1, "data": gin.H{"id": descriptor.MessageID}})
}

// 提交评论（内置评论系统）
func PostComment(c *gin.Context) {
	idStr := c.Param("id")
	msgID64, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil || msgID64 == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 0, "msg": "无效的消息ID"})
		return
	}
	msgID := uint(msgID64)
	var req struct {
		Content    string `json:"content"`
		Visibility string `json:"visibility"`
		ParentID   *uint  `json:"parent_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 0, "msg": "请求参数错误"})
		return
	}
	req.Content = strings.TrimSpace(req.Content)
	if req.Content == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 0, "msg": "评论内容不能为空"})
		return
	}
	db, _ := database.GetDB()
	// 校验消息存在
	var message models.Message
	if err := db.First(&message, msgID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 0, "msg": "消息不存在"})
		return
	}
	// 评论/留言/回复统一强制绑定当前登录账号，不再信任前端提交的昵称/邮箱/网址。
	userID, ok := commentAuthUserID(c)
	if !ok {
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "请登录后评论"})
		return
	}
	var currentUser models.User
	if err := db.Select("id, username, avatar_url, email").First(&currentUser, userID).Error; err != nil || currentUser.ID == 0 {
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "账号不存在或已失效"})
		return
	}
	viewerID := currentUser.ID
	scope, scopeErr := services.ResolveContentReadScope(db, &viewerID)
	if scopeErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 0, "msg": "授权服务不可用"})
		return
	}
	if !scope.CanReadMessage(message) {
		c.JSON(http.StatusNotFound, gin.H{"code": 0, "msg": "消息不存在"})
		return
	}
	if !scope.CanInteractWithMessage(message) {
		c.JSON(http.StatusForbidden, gin.H{"code": 0, "msg": "无权限评论该内容"})
		return
	}
	messageVisibility := services.StoredMessageVisibility(message)
	visibility, ok := normalizeRequestedCommentVisibility(req.Visibility, messageVisibility)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"code": 0, "msg": "无效的可见范围"})
		return
	}
	if !commentVisibilityAllowedForMessage(visibility, messageVisibility) {
		c.JSON(http.StatusBadRequest, gin.H{"code": 0, "msg": "评论可见范围不能宽于当前笔记"})
		return
	}
	var notificationParent *models.Comment
	if req.ParentID != nil {
		var parent models.Comment
		if err := db.First(&parent, *req.ParentID).Error; err != nil || parent.MessageID != msgID {
			c.JSON(http.StatusBadRequest, gin.H{"code": 0, "msg": "回复目标不存在"})
			return
		}
		notificationParent = &parent
		commentMap, err := services.LoadCommentMapForMessage(msgID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 0, "msg": "获取评论失败"})
			return
		}
		commentMap[parent.ID] = parent
		if !scope.CanInteractWithComment(message, parent, commentMap) {
			c.JSON(http.StatusForbidden, gin.H{"code": 0, "msg": "无权限回复该内容"})
			return
		}
		parentVisibility := services.EffectiveCommentVisibilityInThread(parent, messageVisibility, commentMap)
		if commentVisibilityRank(visibility) > commentVisibilityRank(parentVisibility) {
			if strings.TrimSpace(req.Visibility) == "" {
				visibility = parentVisibility
			} else {
				c.JSON(http.StatusBadRequest, gin.H{"code": 0, "msg": "回复可见范围不能宽于被回复内容"})
				return
			}
		}
	}
	commentUserID := currentUser.ID
	commentUsername := strings.TrimSpace(currentUser.Username)
	if commentUsername == "" {
		commentUsername = fmt.Sprintf("用户%d", currentUser.ID)
	}
	comment := models.Comment{
		MessageID:  msgID,
		UserID:     &commentUserID,
		Content:    req.Content,
		Visibility: visibility,
		ParentID:   req.ParentID,
	}
	if err := db.Create(&comment).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 0, "msg": "保存评论失败"})
		return
	}
	comment.User = &models.CommentUserInfo{ID: currentUser.ID, Username: commentUsername, AvatarURL: strings.TrimSpace(currentUser.AvatarURL)}
	if err := services.CreateNotificationsForComment(message, comment, notificationParent); err != nil {
		log.Printf("创建站内评论通知失败: %v", err)
	}
	c.JSON(http.StatusOK, gin.H{"code": 1, "data": comment, "msg": "评论已发布"})
}

// 更新评论：管理员可更新任意评论，普通用户仅可更新自己发布的评论/留言/回复。
func UpdateComment(c *gin.Context) {
	msgIDStr := c.Param("id")
	cidStr := c.Param("cid")
	msgID, err1 := strconv.ParseUint(msgIDStr, 10, 64)
	cid, err2 := strconv.ParseUint(cidStr, 10, 64)
	if err1 != nil || err2 != nil || msgID == 0 || cid == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 0, "msg": "无效的ID"})
		return
	}
	var req struct {
		Content    string `json:"content"`
		Visibility string `json:"visibility"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 0, "msg": "请求参数错误"})
		return
	}
	req.Content = strings.TrimSpace(req.Content)
	if req.Content == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 0, "msg": "评论内容不能为空"})
		return
	}
	db, _ := database.GetDB()
	var cm models.Comment
	if err := db.First(&cm, cid).Error; err != nil || cm.MessageID != uint(msgID) {
		c.JSON(http.StatusNotFound, gin.H{"code": 0, "msg": "评论不存在"})
		return
	}
	if cm.DeletedAt != nil || cm.IsTombstone || cm.UserPurgedAt != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 0, "msg": "评论不存在"})
		return
	}
	var message models.Message
	if err := db.First(&message, uint(msgID)).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 0, "msg": "消息不存在"})
		return
	}
	commentMap, err := services.LoadCommentMapForMessage(uint(msgID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 0, "msg": "获取评论失败"})
		return
	}
	actorID, actorOK := commentAuthUserID(c)
	isOwner := actorOK && cm.UserID != nil && *cm.UserID == actorID
	messageVisibility := services.StoredMessageVisibility(message)
	visibility := cm.Visibility
	if strings.TrimSpace(req.Visibility) != "" {
		var ok bool
		visibility, ok = normalizeCommentVisibility(req.Visibility)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"code": 0, "msg": "无效的可见范围"})
			return
		}
	} else if strings.TrimSpace(visibility) == "" {
		visibility = defaultCommentVisibilityForMessage(messageVisibility)
	} else {
		var ok bool
		visibility, ok = normalizeCommentVisibility(visibility)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"code": 0, "msg": "无效的可见范围"})
			return
		}
	}
	if !commentVisibilityAllowedForMessage(visibility, messageVisibility) {
		c.JSON(http.StatusBadRequest, gin.H{"code": 0, "msg": "评论可见范围不能宽于当前笔记"})
		return
	}
	if cm.ParentID != nil {
		var parent models.Comment
		if err := db.First(&parent, *cm.ParentID).Error; err == nil {
			commentMap[parent.ID] = parent
			parentVisibility := services.EffectiveCommentVisibilityInThread(parent, messageVisibility, commentMap)
			if commentVisibilityRank(visibility) > commentVisibilityRank(parentVisibility) {
				if strings.TrimSpace(req.Visibility) == "" {
					visibility = parentVisibility
				} else {
					c.JSON(http.StatusBadRequest, gin.H{"code": 0, "msg": "回复可见范围不能宽于被回复内容"})
					return
				}
			}
		}
	}
	contentChanged := cm.Content != req.Content
	visibilityChanged := services.NormalizedCommentVisibilityOrPublic(cm.Visibility) != visibility
	for _, requirement := range []struct {
		changed    bool
		capability authorization.Capability
	}{
		{contentChanged, authorization.CapabilityCommentsEdit},
		{visibilityChanged, authorization.CapabilityCommentsChangeVisibility},
	} {
		if !requirement.changed || isOwner {
			continue
		}
		decision := authorizeCommentMutation(c, message, cm, commentMap, requirement.capability)
		if !decision.Allowed {
			if decision.Reason == authorization.DenialContentNotReadable {
				c.JSON(http.StatusNotFound, gin.H{"code": 0, "msg": "评论不存在"})
			} else {
				c.JSON(http.StatusForbidden, gin.H{"code": 0, "msg": "无权限"})
			}
			return
		}
	}
	cm.Content = req.Content
	cm.Visibility = visibility
	auditCapability := authorization.CapabilityCommentsEdit
	if visibilityChanged && !contentChanged {
		auditCapability = authorization.CapabilityCommentsChangeVisibility
	}
	if err := persistCommentMutation(c, db, &cm, auditCapability, "edit", false); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 0, "msg": "更新失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 1, "data": cm, "msg": "已更新"})
}

// 删除评论：管理员可删除任意评论，普通用户仅可删除自己发布的评论/留言/回复。
func DeleteComment(c *gin.Context) {
	msgIDStr := c.Param("id")
	cidStr := c.Param("cid")
	msgID, err1 := strconv.ParseUint(msgIDStr, 10, 64)
	cid, err2 := strconv.ParseUint(cidStr, 10, 64)
	if err1 != nil || err2 != nil || msgID == 0 || cid == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 0, "msg": "无效的ID"})
		return
	}
	db, _ := database.GetDB()
	// 确认评论属于该消息
	var cm models.Comment
	if err := db.First(&cm, cid).Error; err != nil || cm.MessageID != uint(msgID) {
		c.JSON(http.StatusNotFound, gin.H{"code": 0, "msg": "评论不存在"})
		return
	}
	var message models.Message
	if err := db.First(&message, uint(msgID)).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 0, "msg": "消息不存在"})
		return
	}
	actorID, ok := commentAuthUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 0, "msg": "请先登录"})
		return
	}
	result, err := services.TrashCommentTree(db, actorID, cm.ID, services.CommentTrashRequest{})
	if err != nil {
		if errors.Is(err, services.ErrCommentNotAuthorized) || errors.Is(err, services.ErrCommentProtected) {
			reason := authorization.DenialMissingGrant
			if errors.Is(err, services.ErrCommentProtected) {
				reason = authorization.DenialProtectedContent
			}
			authorization.New(db).WriteDeniedBestEffort(models.AdminAuditLog{
				ActorUserID: actorID, Capability: string(authorization.CapabilityCommentsTrash), Module: "comments",
				Action: "trash", TargetType: "comment", TargetID: fmt.Sprint(cm.ID), TargetOwnerUserID: cm.UserID,
				Result: "denied", Summary: "interaction trash denied", Reason: string(reason),
			})
		}
		if errors.Is(err, services.ErrCommentNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"code": 0, "msg": "评论不存在"})
		} else if errors.Is(err, services.ErrCommentNotAuthorized) || errors.Is(err, services.ErrCommentProtected) {
			c.JSON(http.StatusForbidden, gin.H{"code": 0, "msg": "无权限"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 0, "msg": "移入回收站失败"})
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "已移入回收站", "data": result})
}

// 列出所有内置评论（管理员，支持搜索与分页）
func ListComments(c *gin.Context) {
	actor, err := checkAdmin(c)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](err.Error()))
		return
	}
	q := strings.TrimSpace(c.Query("q"))
	page, _ := strconv.Atoi(c.Query("page"))
	if page <= 0 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(c.Query("pageSize"))
	if pageSize <= 0 || pageSize > 200 {
		pageSize = 30
	}
	db, _ := database.GetDB()
	actorID := actor
	scope, err := services.ResolveContentReadScope(db, &actorID)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[string]("查询失败"))
		return
	}
	tx := db.Model(&models.Comment{}).Joins("LEFT JOIN users ON users.id = comments.user_id").Where("comments.deleted_at IS NULL AND comments.is_tombstone = ?", false)
	if q != "" {
		like := "%" + q + "%"
		tx = tx.Where("comments.content LIKE ? OR users.username LIKE ?", like, like)
	}
	var candidates []models.Comment
	if err := tx.Select("comments.*").Order("comments.created_at DESC").Find(&candidates).Error; err != nil {
		c.JSON(http.StatusOK, dto.Fail[string]("查询失败"))
		return
	}
	messageIDs := make([]uint, 0)
	seenMessageIDs := make(map[uint]struct{})
	for _, comment := range candidates {
		if _, exists := seenMessageIDs[comment.MessageID]; exists {
			continue
		}
		seenMessageIDs[comment.MessageID] = struct{}{}
		messageIDs = append(messageIDs, comment.MessageID)
	}
	var messages []models.Message
	if len(messageIDs) > 0 {
		if err := db.Where("id IN ?", messageIDs).Find(&messages).Error; err != nil {
			c.JSON(http.StatusOK, dto.Fail[string]("查询失败"))
			return
		}
	}
	messageByID := make(map[uint]models.Message, len(messages))
	commentMaps := make(map[uint]map[uint]models.Comment, len(messages))
	for _, message := range messages {
		messageByID[message.ID] = message
	}
	if len(messageIDs) > 0 {
		var threadComments []models.Comment
		if err := db.Where("message_id IN ?", messageIDs).Find(&threadComments).Error; err != nil {
			c.JSON(http.StatusOK, dto.Fail[string]("查询失败"))
			return
		}
		commentsByMessage := make(map[uint][]models.Comment)
		for _, comment := range threadComments {
			commentsByMessage[comment.MessageID] = append(commentsByMessage[comment.MessageID], comment)
		}
		for messageID, comments := range commentsByMessage {
			commentMaps[messageID] = services.CommentMap(comments)
		}
	}
	visible := make([]models.Comment, 0, len(candidates))
	for _, comment := range candidates {
		message, exists := messageByID[comment.MessageID]
		if exists && scope.CanReadComment(message, comment, commentMaps[comment.MessageID]) {
			comment.CanInteract = scope.CanInteractWithComment(message, comment, commentMaps[comment.MessageID])
			visible = append(visible, comment)
		}
	}
	total := int64(len(visible))
	start := (page - 1) * pageSize
	if start > len(visible) {
		start = len(visible)
	}
	end := start + pageSize
	if end > len(visible) {
		end = len(visible)
	}
	list := visible[start:end]
	userIDs := make([]uint, 0)
	for _, item := range list {
		if item.UserID != nil {
			userIDs = append(userIDs, *item.UserID)
		}
	}
	if len(userIDs) > 0 {
		var users []models.User
		if err := db.Select("id, username, avatar_url").Where("id IN ?", userIDs).Find(&users).Error; err == nil {
			userInfo := map[uint]models.CommentUserInfo{}
			for _, user := range users {
				userInfo[user.ID] = models.CommentUserInfo{ID: user.ID, Username: strings.TrimSpace(user.Username), AvatarURL: strings.TrimSpace(user.AvatarURL)}
			}
			for i := range list {
				if list[i].UserID == nil {
					continue
				}
				if info, ok := userInfo[*list[i].UserID]; ok {
					list[i].User = &info
				}
			}
		}
	}
	c.JSON(http.StatusOK, gin.H{"code": 1, "data": gin.H{"total": total, "items": list}})
}
