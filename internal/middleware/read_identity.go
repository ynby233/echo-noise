package middleware

import (
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/services"
	"github.com/rcy1314/echo-noise/pkg"
)

const readIdentityKey = "verified_read_identity"

type readIdentity struct {
	User models.User
	Via  string
}

// ResolveReadIdentity shares a verified reader with access logging. It never
// sets user_id, is_admin, or auth_via: read identity cannot authorize writes.
func ResolveReadIdentity(c *gin.Context) (models.User, string, bool) {
	if value, exists := c.Get(readIdentityKey); exists {
		if identity, ok := value.(readIdentity); ok {
			return identity.User, identity.Via, true
		}
	}
	remember := func(user models.User, via string) (models.User, string, bool) {
		c.Set(readIdentityKey, readIdentity{User: user, Via: via})
		return user, via, true
	}
	db, err := database.GetDB()
	if err != nil {
		return models.User{}, "unknown", false
	}
	if value, exists := c.Get("user_id"); exists {
		id, ok := toUint(value)
		if !ok || id == 0 {
			return models.User{}, "unknown", false
		}
		var user models.User
		if db.First(&user, id).Error == nil {
			via := c.GetString("auth_via")
			if via != "session" && via != "token" && via != "ai_token" {
				via = "unknown"
			}
			return remember(user, via)
		}
		return models.User{}, "unknown", false
	}
	if _, exists := c.Get(sessions.DefaultKey); exists {
		session := sessions.Default(c)
		if id, ok := toUint(session.Get("user_id")); ok && id > 0 {
			var user models.User
			if db.First(&user, id).Error == nil {
				issuedAt := parseSessionExpireAt(session.Get("login_issued_at"))
				expireAt := parseSessionExpireAt(session.Get("login_expire_at"))
				if !services.IsUserLoginExpired(&user, issuedAt, time.Now()) && !(issuedAt <= 0 && expireAt > 0 && time.Now().Unix() > expireAt) {
					return remember(user, "session")
				}
			}
			_ = pkg.ClearUserSession(c)
		}
	}
	user, _, via, err := services.ResolveBearerIdentity(c.GetHeader("Authorization"), false)
	if err != nil || user == nil {
		return models.User{}, "unknown", false
	}
	return remember(*user, via)
}

// SessionOnlyAuthMiddleware deliberately excludes every bearer fallback.
func SessionOnlyAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, exists := c.Get(sessions.DefaultKey); !exists {
			c.AbortWithStatus(401)
			return
		}
		if id, ok := toUint(sessions.Default(c).Get("user_id")); !ok || id == 0 {
			c.AbortWithStatus(401)
			return
		}
		SessionAuthMiddleware()(c)
	}
}
