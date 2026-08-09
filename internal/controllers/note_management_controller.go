package controllers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/internal/authorization"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/services"
)

type noteLifecycleBatchRequest struct {
	IDs    []uint `json:"ids"`
	Reason string `json:"reason"`
}

type noteLifecycleBatchItem struct {
	ID     *uint  `json:"id,omitempty"`
	OK     bool   `json:"ok"`
	Reason string `json:"reason,omitempty"`
}

type noteLifecycleBatchResult struct {
	Succeeded int                      `json:"succeeded"`
	Failed    int                      `json:"failed"`
	Items     []noteLifecycleBatchItem `json:"items"`
}

func noteManagementActor(c *gin.Context) (uint, bool) {
	user, ok := currentReadUser(c)
	return user.ID, ok && user.ID != 0 && user.IsAdmin
}

func parseNoteManagementFilter(c *gin.Context) services.NoteManagementFilter {
	filter := services.NoteManagementFilter{
		Page:       1,
		PageSize:   20,
		Keyword:    strings.TrimSpace(c.Query("keyword")),
		Username:   strings.TrimSpace(c.Query("username")),
		Visibility: strings.TrimSpace(c.Query("visibility")),
		Tag:        strings.TrimPrefix(strings.TrimSpace(c.Query("tag")), "#"),
	}
	if page, err := strconv.Atoi(c.Query("page")); err == nil {
		filter.Page = page
	}
	if pageSize, err := strconv.Atoi(c.Query("pageSize")); err == nil {
		filter.PageSize = pageSize
	}
	if id, err := strconv.ParseUint(strings.TrimSpace(c.Query("id")), 10, 64); err == nil && id > 0 {
		value := uint(id)
		filter.MessageID = &value
	}
	if id, err := strconv.ParseUint(strings.TrimSpace(c.Query("authorId")), 10, 64); err == nil && id > 0 {
		value := uint(id)
		filter.AuthorID = &value
	}
	return filter
}

func writeNoteLifecycleError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, services.ErrMessageNotFound), errors.Is(err, services.ErrMessageNotVisible), errors.Is(err, services.ErrMessageProtected):
		c.JSON(http.StatusNotFound, gin.H{"code": 0, "msg": "笔记不存在"})
	case errors.Is(err, services.ErrMessageNotAuthorized):
		c.JSON(http.StatusForbidden, gin.H{"code": 0, "msg": "无权执行此笔记操作"})
	case errors.Is(err, services.ErrMessageAlreadyTrashed), errors.Is(err, services.ErrMessageNotTrashed):
		c.JSON(http.StatusConflict, gin.H{"code": 0, "msg": "笔记状态不允许执行此操作"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"code": 0, "msg": "笔记操作失败"})
	}
}

func ListAdminNotes(c *gin.Context) {
	actorID, ok := noteManagementActor(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 0, "msg": "未授权访问"})
		return
	}
	db, err := database.GetDB()
	if err != nil {
		writeNoteLifecycleError(c, err)
		return
	}
	result, err := services.ListNoteManagementMessages(db, actorID, parseNoteManagementFilter(c))
	if err != nil {
		writeNoteLifecycleError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "获取笔记管理列表成功", "data": result})
}

func GetAdminNote(c *gin.Context) {
	actorID, ok := noteManagementActor(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 0, "msg": "未授权访问"})
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		writeNoteLifecycleError(c, services.ErrMessageNotFound)
		return
	}
	db, err := database.GetDB()
	if err != nil {
		writeNoteLifecycleError(c, err)
		return
	}
	item, err := services.GetNoteManagementMessageForViewer(db, actorID, uint(id))
	if err != nil {
		writeNoteLifecycleError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "获取笔记详情成功", "data": item})
}

func TrashAdminNote(c *gin.Context) {
	actorID, ok := noteManagementActor(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 0, "msg": "未授权访问"})
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		writeNoteLifecycleError(c, services.ErrMessageNotFound)
		return
	}
	reason := strings.TrimSpace(c.GetHeader("X-Note-Reason"))
	var body struct {
		Reason string `json:"reason"`
	}
	if c.Request.Body != nil && c.ShouldBindJSON(&body) == nil && strings.TrimSpace(body.Reason) != "" {
		reason = strings.TrimSpace(body.Reason)
	}
	db, err := database.GetDB()
	if err != nil {
		writeNoteLifecycleError(c, err)
		return
	}
	var message models.Message
	if err := db.First(&message, uint(id)).Error; err != nil {
		writeNoteLifecycleError(c, services.ErrMessageNotFound)
		return
	}
	if err := services.TrashMessage(db, actorID, uint(id), reason); err != nil {
		writeMessageMutationDeniedAudit(c, authorization.New(db), actorID, authorization.CapabilityNotesTrash, "trash", &message)
		writeNoteLifecycleError(c, err)
		return
	}
	if err := writeMessageMutationSuccessAudit(c, authorization.New(db), actorID, authorization.CapabilityNotesTrash, "trash", &message); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 0, "msg": "写入管理员审计失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "删除成功"})
}

func ListAdminRecycleBin(c *gin.Context) {
	actorID, ok := noteManagementActor(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 0, "msg": "未授权访问"})
		return
	}
	db, err := database.GetDB()
	if err != nil {
		writeNoteLifecycleError(c, err)
		return
	}
	result, err := services.ListRecycleBinMessages(db, actorID, parseNoteManagementFilter(c))
	if err != nil {
		writeNoteLifecycleError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "获取回收站成功", "data": result})
}

func GetAdminRecycleBinNote(c *gin.Context) {
	actorID, ok := noteManagementActor(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 0, "msg": "未授权访问"})
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		writeNoteLifecycleError(c, services.ErrMessageNotFound)
		return
	}
	db, err := database.GetDB()
	if err != nil {
		writeNoteLifecycleError(c, err)
		return
	}
	message, err := services.GetRecycleBinMessageForViewer(db, actorID, uint(id))
	if err != nil {
		writeNoteLifecycleError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "获取回收站笔记详情成功", "data": message})
}

func RestoreAdminRecycleBinNote(c *gin.Context) {
	actorID, ok := noteManagementActor(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 0, "msg": "未授权访问"})
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		writeNoteLifecycleError(c, services.ErrMessageNotFound)
		return
	}
	db, err := database.GetDB()
	if err != nil {
		writeNoteLifecycleError(c, err)
		return
	}
	var message models.Message
	if err := db.First(&message, uint(id)).Error; err != nil {
		writeNoteLifecycleError(c, services.ErrMessageNotFound)
		return
	}
	if err := services.RestoreMessage(db, actorID, uint(id)); err != nil {
		writeMessageMutationDeniedAudit(c, authorization.New(db), actorID, authorization.CapabilityNotesRestore, "restore", &message)
		writeNoteLifecycleError(c, err)
		return
	}
	if err := writeMessageMutationSuccessAudit(c, authorization.New(db), actorID, authorization.CapabilityNotesRestore, "restore", &message); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 0, "msg": "写入管理员审计失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "恢复成功"})
}

func PermanentlyDeleteAdminRecycleBinNote(c *gin.Context) {
	actorID, ok := noteManagementActor(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 0, "msg": "未授权访问"})
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		writeNoteLifecycleError(c, services.ErrMessageNotFound)
		return
	}
	db, err := database.GetDB()
	if err != nil {
		writeNoteLifecycleError(c, err)
		return
	}
	var message models.Message
	if err := db.First(&message, uint(id)).Error; err != nil {
		writeNoteLifecycleError(c, services.ErrMessageNotFound)
		return
	}
	if err := services.PermanentlyDeleteMessage(db, actorID, uint(id), "manual permanent deletion"); err != nil {
		writeMessageMutationDeniedAudit(c, authorization.New(db), actorID, authorization.CapabilityNotesDelete, "permanent_delete", &message)
		writeNoteLifecycleError(c, err)
		return
	}
	if err := writeMessageMutationSuccessAudit(c, authorization.New(db), actorID, authorization.CapabilityNotesDelete, "permanent_delete", &message); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 0, "msg": "写入管理员审计失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "永久删除成功"})
}

func runNoteLifecycleBatch(c *gin.Context, action string) {
	actorID, ok := noteManagementActor(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 0, "msg": "未授权访问"})
		return
	}
	var req noteLifecycleBatchRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) == 0 || len(req.IDs) > 100 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 0, "msg": "请明确勾选 1 至 100 条笔记"})
		return
	}
	db, err := database.GetDB()
	if err != nil {
		writeNoteLifecycleError(c, err)
		return
	}
	result := noteLifecycleBatchResult{Items: make([]noteLifecycleBatchItem, 0, len(req.IDs))}
	for _, id := range req.IDs {
		if id == 0 {
			result.Failed++
			result.Items = append(result.Items, noteLifecycleBatchItem{OK: false, Reason: "无效笔记"})
			continue
		}
		var message models.Message
		messageErr := db.First(&message, id).Error
		var itemErr error
		switch action {
		case "trash":
			itemErr = services.TrashMessage(db, actorID, id, req.Reason)
		case "restore":
			itemErr = services.RestoreMessage(db, actorID, id)
		case "permanent-delete":
			itemErr = services.PermanentlyDeleteMessage(db, actorID, id, req.Reason)
		}
		if itemErr == nil {
			if messageErr == nil {
				capability := authorization.CapabilityNotesTrash
				actionName := "trash"
				switch action {
				case "restore":
					capability = authorization.CapabilityNotesRestore
					actionName = "restore"
				case "permanent-delete":
					capability = authorization.CapabilityNotesDelete
					actionName = "permanent_delete"
				}
				if auditErr := writeMessageMutationSuccessAudit(c, authorization.New(db), actorID, capability, actionName, &message); auditErr != nil {
					itemErr = auditErr
				}
			}
		}
		if itemErr == nil {
			idCopy := id
			result.Succeeded++
			result.Items = append(result.Items, noteLifecycleBatchItem{ID: &idCopy, OK: true})
			continue
		}
		if messageErr == nil {
			capability := authorization.CapabilityNotesTrash
			actionName := "trash"
			switch action {
			case "restore":
				capability = authorization.CapabilityNotesRestore
				actionName = "restore"
			case "permanent-delete":
				capability = authorization.CapabilityNotesDelete
				actionName = "permanent_delete"
			}
			writeMessageMutationDeniedAudit(c, authorization.New(db), actorID, capability, actionName, &message)
		}
		result.Failed++
		if errors.Is(itemErr, services.ErrMessageNotVisible) || errors.Is(itemErr, services.ErrMessageProtected) || errors.Is(itemErr, services.ErrMessageNotAuthorized) {
			result.Items = append(result.Items, noteLifecycleBatchItem{OK: false, Reason: "操作失败"})
		} else {
			idCopy := id
			result.Items = append(result.Items, noteLifecycleBatchItem{ID: &idCopy, OK: false, Reason: "笔记状态不允许执行此操作"})
		}
	}
	c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "批量操作完成", "data": result})
}

func BatchTrashAdminNotes(c *gin.Context)        { runNoteLifecycleBatch(c, "trash") }
func BatchRestoreAdminRecycleBin(c *gin.Context) { runNoteLifecycleBatch(c, "restore") }
func BatchPermanentDeleteAdminRecycleBin(c *gin.Context) {
	runNoteLifecycleBatch(c, "permanent-delete")
}
