package middleware

import (
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/internal/authorization"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/dto"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/services"
	"net/http"
	"strings"
	"time"
)

func SessionAuthMiddleware() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		session := sessions.Default(ctx)
		userID := session.Get("user_id")
		expireAt := parseSessionExpireAt(session.Get("login_expire_at"))
		issuedAt := parseSessionExpireAt(session.Get("login_issued_at"))
		if sessionUserID, ok := toUint(userID); ok && sessionUserID > 0 {
			db, err := database.GetDB()
			var sessionUser models.User
			if err != nil || db.First(&sessionUser, sessionUserID).Error != nil || services.IsUserLoginExpired(&sessionUser, issuedAt, time.Now()) || (issuedAt <= 0 && expireAt > 0 && time.Now().Unix() > expireAt) {
				session.Clear()
				_ = session.Save()
				ctx.JSON(http.StatusUnauthorized, dto.Fail[any]("未登录或登录已过期"))
				ctx.Abort()
				return
			}
			ctx.Set("user_id", sessionUser.ID)
			ctx.Set("username", sessionUser.Username)
			ctx.Set("is_admin", sessionUser.IsAdmin)
			ctx.Set("auth_via", "session")
			ctx.Next()
			return
		}

		if userID == nil {
			// Bearer Token 回退仅保留给管理员，避免普通用户的持久化 token 绕过登录过期时间。
			if authenticateAdminBearerToken(ctx) {
				ctx.Next()
				return
			}
			// 定义公共路由
			publicPaths := []string{
				"/api/messages/page",
				"/api/messages/",
				"/api/messages",
				"/api/messages/search",
				"/api/messages/tags",
				"/api/messages/tags/",
				"/api/messages/images",
				"/api/messages/calendar",
				"/api/frontend/config",
				"/api/status",
				"/api/version/check",
			}

			// 检查是否是公共路由
			for _, path := range publicPaths {
				if strings.HasPrefix(ctx.Request.URL.Path, path) {
					ctx.Set("user_id", uint(0))
					ctx.Next()
					return
				}
			}

			ctx.JSON(http.StatusUnauthorized, dto.Fail[any]("未登录或登录已过期"))
			ctx.Abort()
			return
		}
		ctx.JSON(http.StatusUnauthorized, dto.Fail[any]("未登录或登录已过期"))
		ctx.Abort()
	}
}

func authenticateAdminBearerToken(ctx *gin.Context) bool {
	auth := ctx.GetHeader("Authorization")
	if strings.TrimSpace(auth) == "" {
		return false
	}
	var token string
	if strings.HasPrefix(auth, "Bearer ") {
		token = strings.TrimPrefix(auth, "Bearer ")
	} else {
		token = auth
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return false
	}
	db, err := database.GetDB()
	if err != nil {
		return false
	}
	var user models.User
	if err := db.Where("token = ?", token).First(&user).Error; err != nil || user.ID == 0 || user.Token == "" || !user.IsAdmin {
		return false
	}
	issuedAt := int64(0)
	if user.LoginIssuedAt != nil {
		issuedAt = user.LoginIssuedAt.Unix()
	}
	if services.IsUserLoginExpired(&user, issuedAt, time.Now()) {
		return false
	}
	ctx.Set("user_id", user.ID)
	ctx.Set("username", user.Username)
	ctx.Set("is_admin", true)
	ctx.Set("auth_via", "token")
	return true
}

func parseSessionExpireAt(v interface{}) int64 {
	switch x := v.(type) {
	case int64:
		return x
	case int:
		return int64(x)
	case float64:
		return int64(x)
	case string:
		s := strings.TrimSpace(x)
		if s == "" {
			return 0
		}
		var n int64
		for i := 0; i < len(s); i++ {
			c := s[i]
			if c < '0' || c > '9' {
				return 0
			}
			n = n*10 + int64(c-'0')
		}
		return n
	default:
		return 0
	}
}

func AdminAuthMiddleware() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		if v, ok := ctx.Get("user_id"); ok {
			if uid, ok := toUint(v); ok {
				db, err := database.GetDB()
				if err == nil {
					var user models.User
					if err := db.Select("id,is_admin").First(&user, uid).Error; err == nil && user.IsAdmin {
						ctx.Set("is_admin", true)
						ctx.Next()
						return
					}
				}
			}
		}

		ctx.JSON(http.StatusForbidden, dto.Fail[any]("需要管理员权限"))
		ctx.Abort()
	}
}

// RequireCapability checks the current database state on every request. It
// deliberately does not trust a session or bearer-token is_admin snapshot, so
// a revoked grant cannot survive until login expiry.
func RequireCapability(capability authorization.Capability) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		value, exists := ctx.Get("user_id")
		actorID, ok := toUint(value)
		if !exists || !ok || actorID == 0 {
			ctx.JSON(http.StatusUnauthorized, dto.Fail[any]("未登录"))
			ctx.Abort()
			return
		}
		db, err := database.GetDB()
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, dto.Fail[any]("授权服务不可用"))
			ctx.Abort()
			return
		}
		authorizer := authorization.New(db)
		decision := authorizer.Authorize(actorID, capability, nil)
		if !decision.Allowed {
			definition, _ := authorization.DefinitionFor(capability)
			authorizer.WriteDeniedBestEffort(models.AdminAuditLog{
				ActorUserID: actorID, Capability: string(capability), Module: definition.Module,
				Action: ctx.Request.Method, TargetType: "route", TargetID: ctx.FullPath(),
				Result: "denied", Summary: "capability request denied", Reason: string(decision.Reason),
				IP: ctx.ClientIP(), UserAgent: ctx.GetHeader("User-Agent"), AuthVia: ctx.GetString("auth_via"),
			})
			ctx.JSON(http.StatusForbidden, dto.Fail[any]("无权执行此操作"))
			ctx.Abort()
			return
		}
		ctx.Next()
		if ctx.IsAborted() || ctx.Request.Method == http.MethodGet || ctx.Request.Method == http.MethodHead || ctx.Request.Method == http.MethodOptions {
			return
		}
		if SemanticAuditWritten(ctx) {
			return
		}
		definition, _ := authorization.DefinitionFor(capability)
		result := "success"
		summary := "administrative write completed"
		if ctx.Writer.Status() >= http.StatusBadRequest {
			result = "failure"
			summary = "administrative write failed"
		}
		authorizer.WriteDeniedBestEffort(models.AdminAuditLog{
			ActorUserID: actorID, Capability: string(capability), Module: definition.Module,
			Action: ctx.Request.Method, TargetType: "route", TargetID: ctx.FullPath(),
			Result: result, Summary: summary, IP: ctx.ClientIP(), UserAgent: ctx.GetHeader("User-Agent"), AuthVia: ctx.GetString("auth_via"),
		})
	}
}

const semanticAuditContextKey = "semantic_admin_audit_written"

// MarkSemanticAuditWritten prevents RequireCapability from appending a second,
// route-only success record after a handler has committed a richer object-level
// audit record. Access logging remains unaffected.
func MarkSemanticAuditWritten(ctx *gin.Context) {
	if ctx != nil {
		ctx.Set(semanticAuditContextKey, true)
	}
}

func SemanticAuditWritten(ctx *gin.Context) bool {
	if ctx == nil {
		return false
	}
	value, exists := ctx.Get(semanticAuditContextKey)
	written, ok := value.(bool)
	return exists && ok && written
}

func toUint(v any) (uint, bool) {
	switch val := v.(type) {
	case uint:
		return val, true
	case int:
		if val >= 0 {
			return uint(val), true
		}
	case int64:
		if val >= 0 {
			return uint(val), true
		}
	case float64:
		if val >= 0 {
			return uint(val), true
		}
	case string:
		if s := strings.TrimSpace(val); s != "" {
			var n uint64
			for i := 0; i < len(s); i++ {
				c := s[i]
				if c < '0' || c > '9' {
					return 0, false
				}
				n = n*10 + uint64(c-'0')
			}
			return uint(n), true
		}
	}
	return 0, false
}

func TokenAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		if auth == "" {
			c.JSON(http.StatusUnauthorized, dto.Fail[any]("未提供认证信息"))
			c.Abort()
			return
		}

		// 提取 token
		var token string
		if strings.HasPrefix(auth, "Bearer ") {
			token = strings.TrimPrefix(auth, "Bearer ")
		} else {
			token = auth
		}

		db, err := database.GetDB()
		if err != nil {
			c.JSON(http.StatusUnauthorized, dto.Fail[any]("系统错误"))
			c.Abort()
			return
		}

		// 查询用户
		var user models.User
		if err := db.Where("token = ?", token).First(&user).Error; err != nil {
			c.JSON(http.StatusUnauthorized, dto.Fail[any]("无效的token"))
			c.Abort()
			return
		}

		// 检查 token 是否为空
		if user.Token == "" {
			c.JSON(http.StatusUnauthorized, dto.Fail[any]("token已失效"))
			c.Abort()
			return
		}
		issuedAt := int64(0)
		if user.LoginIssuedAt != nil {
			issuedAt = user.LoginIssuedAt.Unix()
		}
		if services.IsUserLoginExpired(&user, issuedAt, time.Now()) {
			c.JSON(http.StatusUnauthorized, dto.Fail[any]("未登录或登录已过期"))
			c.Abort()
			return
		}

		// 设置用户信息到上下文
		c.Set("user_id", user.ID)
		c.Set("username", user.Username)
		c.Set("is_admin", user.IsAdmin)
		c.Set("auth_via", "token")
		c.Next()
	}
}
