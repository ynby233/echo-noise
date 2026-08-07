package controllers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/internal/authorization"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/services"
	"gorm.io/gorm"
)

type messagePinRequest struct {
	Pinned bool `json:"pinned"`
}

func parseMessagePinRequest(c *gin.Context) (uint, messagePinRequest, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 0, "msg": "无效的消息ID"})
		return 0, messagePinRequest{}, false
	}
	var request messagePinRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 0, "msg": "请求参数错误"})
		return 0, messagePinRequest{}, false
	}
	return uint(id), request, true
}

func currentPinActor(c *gin.Context) (uint, bool) {
	value, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 0, "msg": "未授权访问"})
		return 0, false
	}
	actorID, ok := commentUint(value)
	if !ok || actorID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 0, "msg": "未授权访问"})
		return 0, false
	}
	return actorID, true
}

func globalPinAction(pinned bool) string {
	if pinned {
		return "set_global_pin"
	}
	return "unset_global_pin"
}

func globalPinAuditRecord(c *gin.Context, actorID uint, action, result, summary string, message *models.Message, before, after bool) models.AdminAuditLog {
	record := messageMutationAuditRecord(c, actorID, authorization.CapabilityNotesPinGlobal, action, result, summary, message)
	changes, _ := json.Marshal(map[string]map[string]bool{
		"pinned": {"before": before, "after": after},
	})
	record.ChangesJSON = string(changes)
	return record
}

func writeGlobalPinAuditBestEffort(c *gin.Context, db *gorm.DB, actorID uint, action, result, summary string, message *models.Message, before, after bool) {
	if db == nil {
		return
	}
	authorization.New(db).WriteAuditBestEffort(globalPinAuditRecord(c, actorID, action, result, summary, message, before, after))
}

func pinStateData(message *models.Message) gin.H {
	return gin.H{
		"pinned":          message.Pinned,
		"personal_pinned": message.PersonalPinned,
	}
}

// UpdateMessageGlobalPin is the explicit administrator-only global pin route.
func UpdateMessageGlobalPin(c *gin.Context) {
	messageID, request, ok := parseMessagePinRequest(c)
	if !ok {
		return
	}
	actorID, ok := currentPinActor(c)
	if !ok {
		return
	}
	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 0, "msg": "授权服务不可用"})
		return
	}
	message, err := services.GetMessageByID(messageID, true)
	if err != nil || message == nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 0, "msg": "消息不存在"})
		return
	}

	action := globalPinAction(request.Pinned)
	decision := authorization.New(db).Authorize(actorID, authorization.CapabilityNotesPinGlobal, &message.UserID)
	if !decision.Allowed {
		writeGlobalPinAuditBestEffort(c, db, actorID, action, "denied", "global pin request denied", message, message.Pinned, message.Pinned)
		c.JSON(http.StatusForbidden, gin.H{"code": 0, "msg": "无权限操作该消息"})
		return
	}

	// Admin list queries intentionally include restricted notes so that the
	// admin can manage the full dataset. Pinning must still respect the
	// visibility the actor would have as an ordinary logged-in viewer.
	scope, scopeErr := services.ResolveContentReadScope(db, &actorID)
	if scopeErr != nil {
		writeGlobalPinAuditBestEffort(c, db, actorID, action, "failure", "global pin visibility check failed", message, message.Pinned, message.Pinned)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 0, "msg": "授权服务不可用"})
		return
	}
	if !scope.CanReadMessage(*message) {
		writeGlobalPinAuditBestEffort(c, db, actorID, action, "denied", "global pin target is outside actor read scope", message, message.Pinned, message.Pinned)
		c.JSON(http.StatusNotFound, gin.H{"code": 0, "msg": "消息不存在"})
		return
	}
	if !scope.CanInteractWithMessage(*message) {
		writeGlobalPinAuditBestEffort(c, db, actorID, action, "denied", "global pin target is outside actor visibility", message, message.Pinned, message.Pinned)
		c.JSON(http.StatusForbidden, gin.H{"code": 0, "msg": "不能置顶当前账号按正常可见性不可见的笔记"})
		return
	}

	before := message.Pinned
	var updated models.Message
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := services.SetGlobalPin(tx, messageID, request.Pinned); err != nil {
			return err
		}
		if err := tx.First(&updated, messageID).Error; err != nil {
			return err
		}
		return authorization.New(tx).WriteAudit(globalPinAuditRecord(c, actorID, action, "success", "global pin mutation completed", &updated, before, updated.Pinned))
	})
	if err != nil {
		writeGlobalPinAuditBestEffort(c, db, actorID, action, "failure", "global pin mutation failed", message, before, before)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 0, "msg": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "更新成功", "data": pinStateData(&updated)})
}

// UpdateMessagePersonalPin is an authenticated owner-only personal pin route.
func UpdateMessagePersonalPin(c *gin.Context) {
	messageID, request, ok := parseMessagePinRequest(c)
	if !ok {
		return
	}
	actorID, ok := currentPinActor(c)
	if !ok {
		return
	}
	message, err := services.GetMessageByID(messageID, true)
	if err != nil || message == nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 0, "msg": "消息不存在"})
		return
	}
	if message.UserID != actorID {
		c.JSON(http.StatusForbidden, gin.H{"code": 0, "msg": "只能操作自己的笔记"})
		return
	}
	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 0, "msg": "数据库不可用"})
		return
	}
	if err := services.SetPersonalPin(db, messageID, request.Pinned); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 0, "msg": err.Error()})
		return
	}
	updated, err := services.GetMessageByID(messageID, true)
	if err != nil || updated == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 0, "msg": "读取更新后的消息失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "更新成功", "data": pinStateData(updated)})
}
