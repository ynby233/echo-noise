package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/internal/authorization"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/dto"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/services"
	"github.com/rcy1314/echo-noise/internal/updates"
)

func SessionAuthMiddleware() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		session := sessions.Default(ctx)
		userID := session.Get("user_id")
		sessionUser, present, err := resolveSessionIdentity(ctx)
		if err != nil {
			AbortAuthentication(ctx, err)
			return
		}
		if present {
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
			if ctx.IsAborted() {
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

			AbortAuthentication(ctx, services.ErrInvalidBearerIdentity)
			return
		}
		AbortAuthentication(ctx, services.ErrInvalidBearerIdentity)
	}
}

// legacyBearerHeader preserves raw ordinary tokens only on the two legacy
// middleware paths. AI credentials and public identity reads require Bearer.
func legacyBearerHeader(header string) string {
	parts := strings.Fields(header)
	if len(parts) == 1 && !strings.HasPrefix(parts[0], "noise_ai_") {
		return "Bearer " + parts[0]
	}
	return header
}

func authenticateAdminBearerToken(ctx *gin.Context) bool {
	user, _, via, err := services.ResolveBearerIdentity(legacyBearerHeader(ctx.GetHeader("Authorization")), false)
	if err != nil {
		if ctx.GetHeader("Authorization") != "" {
			AbortAuthentication(ctx, err)
		}
		return false
	}
	if user == nil || via != "token" || !user.IsAdmin {
		return false
	}
	ctx.Set("user_id", user.ID)
	ctx.Set("username", user.Username)
	ctx.Set("is_admin", user.IsAdmin)
	ctx.Set("auth_via", via)
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
		user, credential, via, err := services.ResolveBearerIdentity(legacyBearerHeader(c.GetHeader("Authorization")), false)
		if err != nil || user == nil {
			AbortAuthentication(c, err)
			return
		}
		c.Set("user_id", user.ID)
		c.Set("username", user.Username)
		c.Set("is_admin", user.IsAdmin)
		c.Set("auth_via", via)
		if credential != nil {
			c.Set("credential_id", credential.ID)
		}
		c.Next()
	}
}

// UpdateExecutorAuthMiddleware accepts the dedicated executor credential only
// on the executor route group. User sessions and administrator bearer tokens
// are deliberately not valid here.
func UpdateExecutorAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authorizationHeader := strings.TrimSpace(c.GetHeader("Authorization"))
		if !strings.HasPrefix(authorizationHeader, "Bearer ") {
			c.JSON(http.StatusUnauthorized, dto.Fail[any]("执行器凭据无效"))
			c.Abort()
			return
		}
		token := strings.TrimSpace(strings.TrimPrefix(authorizationHeader, "Bearer "))
		if token == "" {
			c.JSON(http.StatusUnauthorized, dto.Fail[any]("执行器凭据无效"))
			c.Abort()
			return
		}
		db, err := database.GetDB()
		if err != nil {
			c.JSON(http.StatusUnauthorized, dto.Fail[any]("执行器凭据无效"))
			c.Abort()
			return
		}
		service := updates.NewTaskService(db)
		var credential models.UpdateExecutorCredential
		if c.Request.Method == http.MethodPost && c.FullPath() == "/api/updates/executor/tasks/:id/events" {
			credential, err = service.AuthenticateTaskReport(token, c.Param("id"))
		} else {
			credential, err = service.Authenticate(token)
		}
		if err != nil {
			c.JSON(http.StatusUnauthorized, dto.Fail[any]("执行器凭据无效"))
			c.Abort()
			return
		}
		c.Set("executor_credential_id", credential.ID)
		c.Set("auth_via", "executor")
		c.Next()
	}
}
