package middleware

import (
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/services"
)

var mobileSetupExemptPaths = map[string]struct{}{
	"/api/setup/status": {},
	"/api/setup/owner":  {},
}

// MobileSetupGate applies only to the embedded Android backend. Static assets
// remain available so the setup UI can render, while every other API fails
// closed until an explicit valid ID 1 administrator exists.
func MobileSetupGate() gin.HandlerFunc {
	return func(c *gin.Context) {
		if strings.TrimSpace(os.Getenv("NOISE_MOBILE")) != "1" || !strings.HasPrefix(c.Request.URL.Path, "/api") {
			c.Next()
			return
		}
		if c.Request.Method == http.MethodOptions {
			c.Next()
			return
		}
		if _, exempt := mobileSetupExemptPaths[c.Request.URL.Path]; exempt {
			c.Next()
			return
		}

		db, err := database.GetDB()
		if err != nil {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"code": 0, "msg": "初始化状态不可用"})
			return
		}
		state, err := services.MobileSetupStateForDB(db)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"code": 0, "msg": "初始化状态读取失败"})
			return
		}
		switch state {
		case services.MobileSetupReady:
			c.Next()
		case services.MobileSetupRequired:
			c.AbortWithStatusJSON(http.StatusPreconditionRequired, gin.H{"code": 0, "msg": "请先创建初始 1 号管理员", "data": gin.H{"setup_state": state}})
		default:
			c.AbortWithStatusJSON(http.StatusLocked, gin.H{"code": 0, "msg": "数据库状态异常，初始化已锁定", "data": gin.H{"setup_state": state}})
		}
	}
}
