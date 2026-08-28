package controllers

import (
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/dto"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/services"
)

func commentManagementFilter(c *gin.Context) services.CommentManagementFilter {
	page, _ := strconv.Atoi(c.Query("page"))
	pageSize, _ := strconv.Atoi(c.Query("pageSize"))
	var authorID *uint
	if value, err := strconv.ParseUint(strings.TrimSpace(c.Query("authorId")), 10, 64); err == nil && value > 0 {
		id := uint(value)
		authorID = &id
	}
	return services.CommentManagementFilter{
		Page: page, PageSize: pageSize, Keyword: strings.TrimSpace(c.Query("q")),
		Kind: strings.TrimSpace(c.Query("kind")), AuthorID: authorID,
		ReasonCode: strings.TrimSpace(c.Query("reason")),
	}
}

func ListAdminCommentRecycleBin(c *gin.Context) {
	actorID, err := checkAdmin(c)
	if err != nil {
		c.JSON(http.StatusForbidden, dto.Fail[any]("无权限"))
		return
	}
	result, err := services.ListCommentManagement(database.DB, actorID, commentManagementFilter(c), true, false, time.Now().UTC())
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[any]("查询评论回收站失败"))
		return
	}
	c.JSON(http.StatusOK, dto.OK(result, "获取评论回收站成功"))
}

func ListAdminCommentManagement(c *gin.Context) {
	actorID, err := checkAdmin(c)
	if err != nil {
		c.JSON(http.StatusForbidden, dto.Fail[any]("无权限"))
		return
	}
	result, err := services.ListCommentManagement(database.DB, actorID, commentManagementFilter(c), false, false, time.Now().UTC())
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[any]("查询互动失败"))
		return
	}
	c.JSON(http.StatusOK, dto.OK(result, "获取互动管理列表成功"))
}

func ListPersonalInteractions(c *gin.Context) {
	user, err := checkUser(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, dto.Fail[any]("请先登录"))
		return
	}
	result, err := services.ListCommentManagement(database.DB, user.ID, commentManagementFilter(c), false, true, time.Now().UTC())
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[any]("查询互动失败"))
		return
	}
	c.JSON(http.StatusOK, dto.OK(result, "获取我的互动成功"))
}

func ListPersonalCommentRecycleBin(c *gin.Context) {
	user, err := checkUser(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, dto.Fail[any]("请先登录"))
		return
	}
	result, err := services.ListCommentManagement(database.DB, user.ID, commentManagementFilter(c), true, true, time.Now().UTC())
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[any]("查询互动回收站失败"))
		return
	}
	c.JSON(http.StatusOK, dto.OK(result, "获取我的互动回收站成功"))
}

func commentLifecycleID(c *gin.Context) (uint, bool) {
	value, err := strconv.ParseUint(c.Param("id"), 10, 64)
	return uint(value), err == nil && value > 0
}

func writeCommentLifecycleError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, services.ErrCommentNotFound), errors.Is(err, services.ErrCommentNotTrashed):
		c.JSON(http.StatusNotFound, dto.Fail[any]("互动不存在"))
	case errors.Is(err, services.ErrCommentAncestorPending):
		c.JSON(http.StatusConflict, dto.Fail[any]("请先恢复所有仍在回收站中的上级内容"))
	case errors.Is(err, services.ErrCommentUserPurged):
		c.JSON(http.StatusConflict, dto.Fail[any]("该互动已被作者彻底删除"))
	case errors.Is(err, services.ErrCommentNotAuthorized), errors.Is(err, services.ErrCommentProtected):
		c.JSON(http.StatusForbidden, dto.Fail[any]("无权限"))
	default:
		c.JSON(http.StatusInternalServerError, dto.Fail[any]("互动状态更新失败"))
	}
}

func RestorePersonalComment(c *gin.Context) {
	user, err := checkUser(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, dto.Fail[any]("请先登录"))
		return
	}
	id, ok := commentLifecycleID(c)
	if !ok {
		c.JSON(http.StatusBadRequest, dto.Fail[any]("无效的互动ID"))
		return
	}
	if err := services.RestoreComment(database.DB, user.ID, id); err != nil {
		writeCommentLifecycleError(c, err)
		return
	}
	c.JSON(http.StatusOK, dto.OK[any](nil, "互动已恢复"))
}

func PurgePersonalComment(c *gin.Context) {
	user, err := checkUser(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, dto.Fail[any]("请先登录"))
		return
	}
	id, ok := commentLifecycleID(c)
	if !ok {
		c.JSON(http.StatusBadRequest, dto.Fail[any]("无效的互动ID"))
		return
	}
	_, err = services.UserPurgeComment(database.DB, user.ID, id)
	if err != nil {
		writeCommentLifecycleError(c, err)
		return
	}
	c.JSON(http.StatusOK, dto.OK[any](nil, "互动已从你的个人回收站彻底删除"))
}

type contentLifecycleBatchRequest struct {
	IDs []uint `json:"ids"`
}

type contentLifecycleBatchItem struct {
	ID     uint   `json:"id"`
	OK     bool   `json:"ok"`
	Reason string `json:"reason,omitempty"`
}

type contentLifecycleBatchResult struct {
	Succeeded int                         `json:"succeeded"`
	Failed    int                         `json:"failed"`
	Items     []contentLifecycleBatchItem `json:"items"`
}

func parseContentLifecycleBatch(c *gin.Context) ([]uint, bool) {
	var request contentLifecycleBatchRequest
	if err := c.ShouldBindJSON(&request); err != nil || len(request.IDs) == 0 || len(request.IDs) > 100 {
		c.JSON(http.StatusBadRequest, dto.Fail[any]("请明确勾选 1 至 100 条内容"))
		return nil, false
	}
	seen := make(map[uint]bool, len(request.IDs))
	ids := make([]uint, 0, len(request.IDs))
	for _, id := range request.IDs {
		if id == 0 || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		c.JSON(http.StatusBadRequest, dto.Fail[any]("请选择有效内容"))
		return nil, false
	}
	return ids, true
}

func writeContentLifecycleBatch(c *gin.Context, ids []uint, action func(uint) error) {
	result := contentLifecycleBatchResult{Items: make([]contentLifecycleBatchItem, 0, len(ids))}
	for _, id := range ids {
		err := action(id)
		if err == nil {
			result.Succeeded++
			result.Items = append(result.Items, contentLifecycleBatchItem{ID: id, OK: true})
			continue
		}
		result.Failed++
		result.Items = append(result.Items, contentLifecycleBatchItem{ID: id, OK: false, Reason: "当前状态或权限不允许执行此操作"})
	}
	c.JSON(http.StatusOK, dto.OK(result, "批量操作完成"))
}

func writeCommentRestoreBatch(c *gin.Context, ids []uint, actorID uint) {
	result := contentLifecycleBatchResult{Items: make([]contentLifecycleBatchItem, 0, len(ids))}
	pending := append([]uint(nil), ids...)
	for len(pending) > 0 {
		next := make([]uint, 0, len(pending))
		progress := false
		for _, id := range pending {
			err := services.RestoreComment(database.DB, actorID, id)
			if errors.Is(err, services.ErrCommentAncestorPending) {
				next = append(next, id)
				continue
			}
			if err == nil {
				progress = true
				result.Succeeded++
				result.Items = append(result.Items, contentLifecycleBatchItem{ID: id, OK: true})
				continue
			}
			result.Failed++
			result.Items = append(result.Items, contentLifecycleBatchItem{ID: id, OK: false, Reason: "当前状态或权限不允许执行此操作"})
		}
		if len(next) == 0 {
			break
		}
		if !progress {
			for _, id := range next {
				result.Failed++
				result.Items = append(result.Items, contentLifecycleBatchItem{ID: id, OK: false, Reason: "需先恢复仍在回收站中的上级内容"})
			}
			break
		}
		pending = next
	}
	c.JSON(http.StatusOK, dto.OK(result, "批量操作完成"))
}

func orderCommentBatchIDs(ids []uint, descendantsFirst bool) []uint {
	ordered := append([]uint(nil), ids...)
	if database.DB == nil || len(ordered) < 2 {
		return ordered
	}
	var selected []models.Comment
	if err := database.DB.Where("id IN ?", ordered).Find(&selected).Error; err != nil {
		return ordered
	}
	messageIDs := make([]uint, 0, len(selected))
	seenMessages := map[uint]bool{}
	for _, comment := range selected {
		if !seenMessages[comment.MessageID] {
			seenMessages[comment.MessageID] = true
			messageIDs = append(messageIDs, comment.MessageID)
		}
	}
	var comments []models.Comment
	if len(messageIDs) == 0 || database.DB.Where("message_id IN ?", messageIDs).Find(&comments).Error != nil {
		return ordered
	}
	commentMap := make(map[uint]models.Comment, len(comments))
	for _, comment := range comments {
		commentMap[comment.ID] = comment
	}
	depth := func(id uint) int {
		value := 0
		seen := map[uint]bool{id: true}
		parentID := commentMap[id].ParentID
		for parentID != nil && !seen[*parentID] {
			seen[*parentID] = true
			parent, ok := commentMap[*parentID]
			if !ok {
				break
			}
			value++
			parentID = parent.ParentID
		}
		return value
	}
	sort.SliceStable(ordered, func(left, right int) bool {
		leftDepth, rightDepth := depth(ordered[left]), depth(ordered[right])
		if descendantsFirst {
			return leftDepth > rightDepth
		}
		return leftDepth < rightDepth
	})
	return ordered
}

func BatchTrashPersonalInteractions(c *gin.Context) {
	user, err := checkUser(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, dto.Fail[any]("请先登录"))
		return
	}
	ids, ok := parseContentLifecycleBatch(c)
	if !ok {
		return
	}
	batchID := services.NewCommentLifecycleBatchID()
	writeContentLifecycleBatch(c, orderCommentBatchIDs(ids, false), func(id uint) error {
		_, itemErr := services.TrashCommentTree(database.DB, user.ID, id, services.CommentTrashRequest{ReasonCode: services.CommentDeletionReasonSelf, BatchID: batchID})
		if errors.Is(itemErr, services.ErrCommentAlreadyTrashed) {
			return nil
		}
		return itemErr
	})
}

func BatchRestorePersonalComments(c *gin.Context) {
	user, err := checkUser(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, dto.Fail[any]("请先登录"))
		return
	}
	ids, ok := parseContentLifecycleBatch(c)
	if !ok {
		return
	}
	writeCommentRestoreBatch(c, ids, user.ID)
}

func BatchPurgePersonalComments(c *gin.Context) {
	user, err := checkUser(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, dto.Fail[any]("请先登录"))
		return
	}
	ids, ok := parseContentLifecycleBatch(c)
	if !ok {
		return
	}
	writeContentLifecycleBatch(c, ids, func(id uint) error {
		_, itemErr := services.UserPurgeComment(database.DB, user.ID, id)
		return itemErr
	})
}

func BatchTrashPersonalNotes(c *gin.Context) {
	user, err := checkUser(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, dto.Fail[any]("请先登录"))
		return
	}
	ids, ok := parseContentLifecycleBatch(c)
	if !ok {
		return
	}
	writeContentLifecycleBatch(c, ids, func(id uint) error { return services.TrashMessage(database.DB, user.ID, id, "author batch request") })
}

func BatchRestorePersonalNotes(c *gin.Context) {
	user, err := checkUser(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, dto.Fail[any]("请先登录"))
		return
	}
	ids, ok := parseContentLifecycleBatch(c)
	if !ok {
		return
	}
	writeContentLifecycleBatch(c, ids, func(id uint) error { return services.RestoreMessage(database.DB, user.ID, id) })
}

func BatchTrashAdminComments(c *gin.Context) {
	actorID, err := checkAdmin(c)
	if err != nil {
		c.JSON(http.StatusForbidden, dto.Fail[any]("无权限"))
		return
	}
	ids, ok := parseContentLifecycleBatch(c)
	if !ok {
		return
	}
	batchID := services.NewCommentLifecycleBatchID()
	writeContentLifecycleBatch(c, orderCommentBatchIDs(ids, false), func(id uint) error {
		_, itemErr := services.TrashCommentTree(database.DB, actorID, id, services.CommentTrashRequest{ReasonCode: services.CommentDeletionReasonModeration, BatchID: batchID})
		if errors.Is(itemErr, services.ErrCommentAlreadyTrashed) {
			return nil
		}
		return itemErr
	})
}

func BatchRestoreAdminComments(c *gin.Context) {
	actorID, err := checkAdmin(c)
	if err != nil {
		c.JSON(http.StatusForbidden, dto.Fail[any]("无权限"))
		return
	}
	ids, ok := parseContentLifecycleBatch(c)
	if !ok {
		return
	}
	writeCommentRestoreBatch(c, ids, actorID)
}

func BatchPermanentlyDeleteAdminComments(c *gin.Context) {
	actorID, err := checkAdmin(c)
	if err != nil {
		c.JSON(http.StatusForbidden, dto.Fail[any]("无权限"))
		return
	}
	ids, ok := parseContentLifecycleBatch(c)
	if !ok {
		return
	}
	writeContentLifecycleBatch(c, orderCommentBatchIDs(ids, true), func(id uint) error { return services.PermanentlyDeleteComment(database.DB, actorID, id) })
}

func RestoreAdminComment(c *gin.Context) {
	actorID, err := checkAdmin(c)
	if err != nil {
		c.JSON(http.StatusForbidden, dto.Fail[any]("无权限"))
		return
	}
	id, ok := commentLifecycleID(c)
	if !ok {
		c.JSON(http.StatusBadRequest, dto.Fail[any]("无效的互动ID"))
		return
	}
	if err := services.RestoreComment(database.DB, actorID, id); err != nil {
		writeCommentLifecycleError(c, err)
		return
	}
	c.JSON(http.StatusOK, dto.OK[any](nil, "互动已恢复"))
}

func PermanentlyDeleteAdminComment(c *gin.Context) {
	actorID, err := checkAdmin(c)
	if err != nil {
		c.JSON(http.StatusForbidden, dto.Fail[any]("无权限"))
		return
	}
	id, ok := commentLifecycleID(c)
	if !ok {
		c.JSON(http.StatusBadRequest, dto.Fail[any]("无效的互动ID"))
		return
	}
	if err := services.PermanentlyDeleteComment(database.DB, actorID, id); err != nil {
		writeCommentLifecycleError(c, err)
		return
	}
	c.JSON(http.StatusOK, dto.OK[any](nil, "互动已永久删除"))
}

func ListPersonalNoteRecycleBin(c *gin.Context) {
	user, err := checkUser(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, dto.Fail[any]("请先登录"))
		return
	}
	filter := parseNoteManagementFilter(c)
	result, err := services.ListPersonalRecycleBinMessages(database.DB, user.ID, filter, time.Now().UTC())
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[any]("查询笔记回收站失败"))
		return
	}
	c.JSON(http.StatusOK, dto.OK(result, "获取我的笔记回收站成功"))
}

func ListPersonalNotes(c *gin.Context) {
	user, err := checkUser(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, dto.Fail[any]("请先登录"))
		return
	}
	result, err := services.ListPersonalMessages(database.DB, user.ID, parseNoteManagementFilter(c))
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[any]("查询个人笔记失败"))
		return
	}
	c.JSON(http.StatusOK, dto.OK(result, "获取我的笔记成功"))
}

func RestorePersonalNote(c *gin.Context) {
	user, err := checkUser(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, dto.Fail[any]("请先登录"))
		return
	}
	id, ok := commentLifecycleID(c)
	if !ok {
		c.JSON(http.StatusBadRequest, dto.Fail[any]("无效的笔记ID"))
		return
	}
	if err := services.RestoreMessage(database.DB, user.ID, id); err != nil {
		c.JSON(http.StatusConflict, dto.Fail[any]("笔记恢复失败"))
		return
	}
	c.JSON(http.StatusOK, dto.OK[any](nil, "笔记已恢复"))
}
