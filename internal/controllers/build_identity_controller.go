package controllers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/internal/buildinfo"
	"github.com/rcy1314/echo-noise/internal/dto"
	"github.com/rcy1314/echo-noise/internal/models"
)

func GetBuildIdentity(c *gin.Context) {
	userID, ok := commentUint(c.GetUint("user_id"))
	if !ok || userID != models.PrimaryAdminUserID {
		c.JSON(http.StatusForbidden, dto.Fail[any]("仅站长可读取构建身份"))
		return
	}
	metadata := buildinfo.CurrentMetadata()
	c.JSON(http.StatusOK, dto.OK(gin.H{
		"build_identity": metadata.Identity,
		"version":        metadata.Version,
		"revision":       metadata.Revision,
		"built_at":       metadata.BuiltAt,
	}, "构建身份读取成功"))
}
