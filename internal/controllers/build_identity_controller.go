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
		c.JSON(http.StatusForbidden, dto.Fail[any]("仅 1 号管理员可读取构建身份"))
		return
	}
	c.JSON(http.StatusOK, dto.OK(gin.H{"build_identity": buildinfo.Current()}, "构建身份读取成功"))
}
