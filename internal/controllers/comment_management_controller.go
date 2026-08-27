package controllers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/dto"
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
