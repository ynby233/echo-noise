package controllers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/internal/authorization"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/services"
	"gorm.io/gorm"
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
		Sort:       strings.TrimSpace(c.Query("sort")),
	}
	if value, err := time.Parse("2006-01-02", strings.TrimSpace(c.Query("createdFrom"))); err == nil {
		filter.CreatedFrom = &value
	}
	if value, err := time.Parse("2006-01-02", strings.TrimSpace(c.Query("createdTo"))); err == nil {
		end := value.Add(24 * time.Hour)
		filter.CreatedTo = &end
	}
	if value := strings.TrimSpace(c.Query("pinned")); value == "true" || value == "false" {
		parsed := value == "true"
		filter.Pinned = &parsed
	}
	if value := strings.TrimSpace(c.Query("hasAttachment")); value == "true" || value == "false" {
		parsed := value == "true"
		filter.HasAttachment = &parsed
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
	auditRecord := messageMutationAuditRecord(c, actorID, authorization.CapabilityNotesTrash, "trash", "success", "message mutation completed", &message)
	if err := services.TrashMessageWithAudit(db, actorID, uint(id), reason, func(tx *gorm.DB) error {
		return authorization.New(tx).WriteAudit(auditRecord)
	}); err != nil {
		writeMessageMutationDeniedAudit(c, authorization.New(db), actorID, authorization.CapabilityNotesTrash, "trash", &message)
		writeNoteLifecycleError(c, err)
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
	auditRecord := messageMutationAuditRecord(c, actorID, authorization.CapabilityNotesRestore, "restore", "success", "message mutation completed", &message)
	if err := services.RestoreMessageWithAudit(db, actorID, uint(id), func(tx *gorm.DB) error {
		return authorization.New(tx).WriteAudit(auditRecord)
	}); err != nil {
		writeMessageMutationDeniedAudit(c, authorization.New(db), actorID, authorization.CapabilityNotesRestore, "restore", &message)
		writeNoteLifecycleError(c, err)
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
	auditRecord := messageMutationAuditRecord(c, actorID, authorization.CapabilityNotesDelete, "permanent_delete", "success", "message mutation completed", &message)
	if err := services.PermanentlyDeleteMessageWithAudit(db, actorID, uint(id), "manual permanent deletion", func(tx *gorm.DB) error {
		return authorization.New(tx).WriteAudit(auditRecord)
	}); err != nil {
		writeMessageMutationDeniedAudit(c, authorization.New(db), actorID, authorization.CapabilityNotesDelete, "permanent_delete", &message)
		writeNoteLifecycleError(c, err)
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
		capability := authorization.CapabilityNotesTrash
		actionName := "trash"
		if action == "restore" {
			capability = authorization.CapabilityNotesRestore
			actionName = "restore"
		} else if action == "permanent-delete" {
			capability = authorization.CapabilityNotesDelete
			actionName = "permanent_delete"
		}
		var auditRecord models.AdminAuditLog
		if messageErr == nil {
			auditRecord = messageMutationAuditRecord(c, actorID, capability, actionName, "success", "message mutation completed", &message)
		}
		switch action {
		case "trash":
			itemErr = services.TrashMessageWithAudit(db, actorID, id, req.Reason, func(tx *gorm.DB) error {
				if messageErr != nil {
					return nil
				}
				return authorization.New(tx).WriteAudit(auditRecord)
			})
		case "restore":
			itemErr = services.RestoreMessageWithAudit(db, actorID, id, func(tx *gorm.DB) error {
				if messageErr != nil {
					return nil
				}
				return authorization.New(tx).WriteAudit(auditRecord)
			})
		case "permanent-delete":
			itemErr = services.PermanentlyDeleteMessageWithAudit(db, actorID, id, req.Reason, func(tx *gorm.DB) error {
				if messageErr != nil {
					return nil
				}
				return authorization.New(tx).WriteAudit(auditRecord)
			})
		}
		if itemErr == nil {
		}
		if itemErr == nil {
			idCopy := id
			result.Succeeded++
			result.Items = append(result.Items, noteLifecycleBatchItem{ID: &idCopy, OK: true})
			continue
		}
		if messageErr == nil {
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
