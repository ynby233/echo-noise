package middleware

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/services"
	"github.com/rcy1314/echo-noise/pkg"
	"gorm.io/gorm"
)

// AbortAuthentication distinguishes expired login state from revoked credentials
// and infrastructure failures. It always stops the handler chain.
func AbortAuthentication(c *gin.Context, err error) {
	status, code, message := http.StatusInternalServerError, "authentication_unavailable", "认证服务暂不可用"
	if errors.Is(err, services.ErrBearerLoginExpired) {
		status, code, message = http.StatusUnauthorized, "login_expired", "登录已过期，请重新登录站点或通过登录接口续期后重试。"
	} else if err == nil || errors.Is(err, services.ErrInvalidBearerIdentity) || errors.Is(err, gorm.ErrRecordNotFound) {
		status, code, message = http.StatusUnauthorized, "invalid_credentials", "凭据无效或已撤销，请检查凭据；必要时重新生成并更新配置。"
	}
	c.AbortWithStatusJSON(status, gin.H{"code": 0, "msg": message, "error": code})
}

func resolveSessionIdentity(c *gin.Context) (*models.User, bool, error) {
	if _, exists := c.Get(sessions.DefaultKey); !exists {
		return nil, false, nil
	}
	session := sessions.Default(c)
	value := session.Get("user_id")
	if value == nil {
		return nil, false, nil
	}
	id, ok := toUint(value)
	if !ok || id == 0 {
		return nil, true, services.ErrInvalidBearerIdentity
	}
	db, err := database.GetDB()
	if err != nil {
		return nil, true, err
	}
	var user models.User
	if err := db.First(&user, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			_ = pkg.ClearUserSession(c)
		}
		return nil, true, err
	}
	issuedAt := parseSessionExpireAt(session.Get("login_issued_at"))
	expireAt := parseSessionExpireAt(session.Get("login_expire_at"))
	if services.IsUserLoginExpired(&user, issuedAt, time.Now()) || (issuedAt <= 0 && expireAt > 0 && time.Now().Unix() >= expireAt) {
		_ = pkg.ClearUserSession(c)
		return nil, true, services.ErrBearerLoginExpired
	}
	return &user, true, nil
}

// PublicReadAuthMiddleware validates supplied identity before public handlers.
// Session identity takes precedence; cached read identity never grants writes.
func PublicReadAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		user, present, err := resolveSessionIdentity(c)
		via := "session"
		if !present && c.GetHeader("Authorization") != "" {
			user, _, via, err = services.ResolveBearerIdentity(c.GetHeader("Authorization"), false)
		}
		if err != nil {
			AbortAuthentication(c, err)
			return
		}
		if user != nil {
			c.Set(readIdentityKey, readIdentity{User: *user, Via: via})
		}
		c.Next()
	}
}
