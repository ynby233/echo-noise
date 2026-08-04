package controllers

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/internal/authorization"
	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
)

func commentMutationAuditRecord(c *gin.Context, actorID uint, capability authorization.Capability, action, result, summary, reason string, comment *models.Comment) models.AdminAuditLog {
	var ownerID *uint
	if comment.UserID != nil {
		value := *comment.UserID
		ownerID = &value
	}
	return models.AdminAuditLog{
		ActorUserID:       actorID,
		Capability:        string(capability),
		Module:            "comments",
		Action:            action,
		TargetType:        "comment",
		TargetID:          fmt.Sprint(comment.ID),
		TargetOwnerUserID: ownerID,
		Result:            result,
		Summary:           summary,
		Reason:            reason,
		IP:                c.ClientIP(),
		UserAgent:         c.GetHeader("User-Agent"),
		AuthVia:           c.GetString("auth_via"),
	}
}

func persistCommentMutation(c *gin.Context, db *gorm.DB, comment *models.Comment, capability authorization.Capability, action string, remove bool) error {
	var err error
	if remove {
		err = db.Delete(comment).Error
	} else {
		err = db.Save(comment).Error
	}
	if err != nil {
		return err
	}
	actorID, ok := commentAuthUserID(c)
	if !ok || (comment.UserID != nil && *comment.UserID == actorID) {
		return nil
	}
	return authorization.New(db).WriteAudit(commentMutationAuditRecord(c, actorID, capability, action, "success", "comment mutation completed", "", comment))
}
