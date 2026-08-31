package controllers

import (
	"errors"
	"net/http"
	"os"
	"strings"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/dto"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/services"
)

func mobileStandaloneEnabled() bool {
	return strings.TrimSpace(os.Getenv("NOISE_MOBILE")) == "1"
}

func establishLoginSession(c *gin.Context, user *models.User) error {
	session := sessions.Default(c)
	session.Clear()
	applyLoginSessionExpire(session, user)
	session.Set("user_id", user.ID)
	session.Set("username", user.Username)
	session.Set("is_admin", user.IsAdmin)
	webPushSessionID, err := newWebPushSessionID()
	if err != nil {
		return err
	}
	session.Set(webPushSessionKey, webPushSessionID)
	return session.Save()
}

func GetMobileSetupStatus(c *gin.Context) {
	if !mobileStandaloneEnabled() {
		c.Status(http.StatusNotFound)
		return
	}
	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, dto.Fail[any]("初始化状态不可用"))
		return
	}
	state, err := services.MobileSetupStateForDB(db)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, dto.Fail[any]("初始化状态读取失败"))
		return
	}
	c.JSON(http.StatusOK, dto.OK(gin.H{"setup_state": state}, "初始化状态读取成功"))
}

func InitializeMobileSiteOwner(c *gin.Context) {
	if !mobileStandaloneEnabled() {
		c.Status(http.StatusNotFound)
		return
	}
	var input dto.MobileSetupOwnerDto
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, dto.Fail[any](models.InvalidRequestBodyMessage))
		return
	}
	if input.Password != input.ConfirmPassword {
		c.JSON(http.StatusBadRequest, dto.Fail[any]("两次输入的密码不一致"))
		return
	}
	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, dto.Fail[any]("数据库未初始化"))
		return
	}
	owner, err := services.InitializeMobileSiteOwner(db, input.Username, input.Password)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, services.ErrMobileSetupAlreadyCompleted) || errors.Is(err, services.ErrMobileSetupInvalidDatabase) {
			status = http.StatusConflict
		}
		c.JSON(status, dto.Fail[any](err.Error()))
		return
	}
	if err := establishLoginSession(c, owner); err != nil {
		c.JSON(http.StatusInternalServerError, dto.Fail[any]("管理员已创建，但自动登录失败，请返回登录页手动登录"))
		return
	}
	_ = recordUserLoginAudit(c, owner, loginAuditActionLogin)

	safeOwner := *owner
	safeOwner.Password = ""
	safeOwner.Token = ""
	c.JSON(http.StatusOK, dto.OK(&safeOwner, "初始管理员创建成功"))
}
