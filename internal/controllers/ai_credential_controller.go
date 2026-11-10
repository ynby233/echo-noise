package controllers

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/dto"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/services"
	"gorm.io/gorm"
)

func aiCredentialSessionUser(c *gin.Context) (*models.User, bool) {
	id, ok := c.Get("user_id")
	userID, valid := id.(uint)
	if c.GetString("auth_via") != "session" || !ok || !valid || userID == 0 {
		c.JSON(http.StatusUnauthorized, dto.Fail[any]("请使用会话登录管理 AI 凭据"))
		return nil, false
	}
	db, err := database.GetDB()
	var user models.User
	if err != nil || db.First(&user, userID).Error != nil {
		c.JSON(http.StatusUnauthorized, dto.Fail[any]("未登录或账号不存在"))
		return nil, false
	}
	return &user, true
}

func ListAICredentials(c *gin.Context) {
	user, ok := aiCredentialSessionUser(c)
	if !ok {
		return
	}
	credentials, err := services.ListAICredentials(user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.Fail[any]("获取 AI 凭据失败"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 1, "data": credentials})
}

func CreateAICredential(c *gin.Context) {
	user, ok := aiCredentialSessionUser(c)
	if !ok {
		return
	}
	var input struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, dto.Fail[any]("参数错误"))
		return
	}
	credential, token, err := services.CreateAICredential(user.ID, input.Name)
	if err != nil {
		if errors.Is(err, services.ErrInvalidAICredentialName) {
			c.JSON(http.StatusBadRequest, dto.Fail[any](err.Error()))
		} else {
			c.JSON(http.StatusInternalServerError, dto.Fail[any]("创建 AI 凭据失败"))
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 1, "data": gin.H{
		"id": credential.ID, "name": credential.Name,
		"created_at": credential.CreatedAt, "token": token,
	}})
}

func RevokeAICredential(c *gin.Context) {
	user, ok := aiCredentialSessionUser(c)
	if !ok {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, strconv.IntSize)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, dto.Fail[any]("凭据 ID 无效"))
		return
	}
	if err := services.RevokeAICredential(user.ID, uint(id)); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, dto.Fail[any]("凭据不存在"))
		} else {
			c.JSON(http.StatusInternalServerError, dto.Fail[any]("撤销 AI 凭据失败"))
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 1})
}

func bearerIdentityResponse(user *models.User, credential *models.AICredential, via string) gin.H {
	var credentialID uint
	if credential != nil {
		credentialID = credential.ID
	}
	return gin.H{"user_id": user.ID, "username": user.Username, "auth_via": via, "credential_id": credentialID}
}

// GetAuthIdentity intentionally ignores session cookies and Gin identity context.
func GetAuthIdentity(c *gin.Context) {
	user, credential, via, err := services.ResolveBearerIdentity(c.GetHeader("Authorization"), false)
	if err != nil {
		c.JSON(http.StatusUnauthorized, dto.Fail[any]("无效的凭据或登录已过期"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 1, "data": bearerIdentityResponse(user, credential, via)})
}

// AILogin renews an AI credential owner's login state without creating a session
// or exposing the owner's ordinary token in the response.
func AILogin(c *gin.Context) {
	var input dto.LoginDto
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, dto.Fail[any]("参数错误"))
		return
	}
	user, credential, err := services.LoginWithAICredential(c.GetHeader("Authorization"), input)
	if err != nil {
		c.JSON(http.StatusUnauthorized, dto.Fail[any]("凭据、用户名或密码无效"))
		return
	}
	c.Set("user_id", user.ID)
	c.Set("username", user.Username)
	c.Set("is_admin", user.IsAdmin)
	c.Set("auth_via", "ai_token")
	c.Set("credential_id", credential.ID)
	_ = recordUserLoginAudit(c, user, loginAuditActionLogin)
	c.JSON(http.StatusOK, gin.H{"code": 1, "data": bearerIdentityResponse(user, credential, "ai_token")})
}
