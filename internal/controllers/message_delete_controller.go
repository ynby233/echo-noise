package controllers

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/internal/authorization"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/dto"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/services"
)

func DeleteMessage(c *gin.Context) {
	id := c.Param("id")
	messageID, err := strconv.ParseUint(id, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 0, "msg": "无效的消息ID"})
		return
	}

	actorID, ok := commentAuthUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, dto.Fail[string]("未授权访问"))
		return
	}
	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 0, "msg": err.Error()})
		return
	}
	var message models.Message
	if err := db.First(&message, uint(messageID)).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 0, "msg": "消息不存在"})
		return
	}
	authorizer := authorization.New(db)
	isOwner := message.UserID == actorID
	if err := services.TrashMessage(db, actorID, uint(messageID), "author request"); err != nil {
		if !isOwner {
			writeMessageMutationDeniedAuditForError(c, authorizer, actorID, authorization.CapabilityNotesTrash, "trash", &message, err)
		}
		switch {
		case errors.Is(err, services.ErrMessageNotVisible), errors.Is(err, services.ErrMessageProtected), errors.Is(err, services.ErrMessageNotFound):
			c.JSON(http.StatusNotFound, gin.H{"code": 0, "msg": "消息不存在"})
		case errors.Is(err, services.ErrMessageNotAuthorized):
			c.JSON(http.StatusForbidden, gin.H{"code": 0, "msg": "无权删除此消息"})
		case errors.Is(err, services.ErrMessageAlreadyTrashed):
			c.JSON(http.StatusConflict, gin.H{"code": 0, "msg": "消息状态不允许执行此操作"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"code": 0, "msg": "删除失败"})
		}
		return
	}
	if !isOwner {
		if err := writeMessageMutationSuccessAudit(c, authorizer, actorID, authorization.CapabilityNotesTrash, "trash", &message); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 0, "msg": "写入管理员审计失败"})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "删除成功"})
}
