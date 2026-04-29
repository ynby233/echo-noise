package pkg

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/internal/models"
)

const (
	UserKey = "user"
)

// 初始化 Session
func InitSession(r *gin.Engine) {
	secret := sessionSecretFromEnv()
	store := cookie.NewStore([]byte(secret))

	// Keep cookie lifetime long enough; actual login expiration is enforced server-side.
	maxAge := 86400 * 3650
	if v := strings.TrimSpace(os.Getenv("SESSION_MAX_AGE")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxAge = n
		}
	}

	secure := inferSecureCookieDefault()
	if v := strings.TrimSpace(os.Getenv("COOKIE_SECURE")); v != "" {
		secure = strings.EqualFold(v, "true") || v == "1"
	}

	sameSite := parseSameSite(strings.TrimSpace(os.Getenv("COOKIE_SAMESITE")))
	store.Options(sessions.Options{
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   secure,
		SameSite: sameSite,
	})
	r.Use(sessions.Sessions("ech0_session", store))
}

func sessionSecretFromEnv() string {
	secret := strings.TrimSpace(os.Getenv("SESSION_SECRET"))
	if secret == "" {
		secret = strings.TrimSpace(os.Getenv("NOISE_SESSION_SECRET"))
	}
	if secret != "" {
		if len(secret) < 32 {
			log.Printf("警告: SESSION_SECRET 长度建议至少 32 字符")
		}
		return secret
	}

	// 安全回退：若未配置密钥，使用进程内随机值，避免固定弱密钥
	secret = randomHex(32)
	log.Printf("警告: 未设置 SESSION_SECRET，已使用临时随机密钥；重启后会话将失效")
	return secret
}

func inferSecureCookieDefault() bool {
	env := strings.ToLower(strings.TrimSpace(os.Getenv("APP_ENV")))
	ginMode := strings.ToLower(strings.TrimSpace(os.Getenv("GIN_MODE")))
	return env == "prod" || env == "production" || ginMode == "release"
}

func parseSameSite(v string) http.SameSite {
	switch strings.ToLower(v) {
	case "strict":
		return http.SameSiteStrictMode
	case "none":
		return http.SameSiteNoneMode
	case "default":
		return http.SameSiteDefaultMode
	default:
		return http.SameSiteLaxMode
	}
}

func randomHex(length int) string {
	if length <= 0 {
		length = 32
	}
	buf := make([]byte, (length+1)/2)
	if _, err := rand.Read(buf); err != nil {
		return "temporary-session-secret-change-me"
	}
	return hex.EncodeToString(buf)[:length]
}

// 保存用户会话
func SaveUserSession(c *gin.Context, user models.User) error {
	session := sessions.Default(c)
	session.Clear() // 清除旧数据

	// 存储用户信息
	session.Set(UserKey, map[string]interface{}{
		"id":       float64(user.ID),
		"username": user.Username,
		"is_admin": user.IsAdmin,
	})

	return session.Save()
}

// 获取用户会话
func GetUserSession(c *gin.Context) (models.User, bool) {
	session := sessions.Default(c)
	val := session.Get(UserKey)
	if val == nil {
		return models.User{}, false
	}

	// 类型断言和转换
	userMap, ok := val.(map[string]interface{})
	if !ok {
		return models.User{}, false
	}

	// 安全地获取和转换值
	id, ok := userMap["id"].(float64)
	if !ok {
		return models.User{}, false
	}

	username, ok := userMap["username"].(string)
	if !ok {
		return models.User{}, false
	}

	isAdmin, ok := userMap["is_admin"].(bool)
	if !ok {
		return models.User{}, false
	}

	user := models.User{
		ID:       uint(id),
		Username: username,
		IsAdmin:  isAdmin,
	}
	return user, true
}

// 清除用户会话
func ClearUserSession(c *gin.Context) error {
	session := sessions.Default(c)
	session.Clear()
	return session.Save()
}
