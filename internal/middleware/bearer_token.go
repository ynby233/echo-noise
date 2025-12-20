package middleware

import (
    "strings"
    "github.com/gin-gonic/gin"
    "github.com/rcy1314/echo-noise/internal/dto"
    "github.com/rcy1314/echo-noise/internal/models"
    "github.com/rcy1314/echo-noise/internal/database"
)

func BearerTokenAuthMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        auth := c.GetHeader("Authorization")
        if auth == "" {
            c.JSON(401, dto.Fail[any]("未提供认证信息"))
            c.Abort()
            return
        }

        // 检查 Bearer token 格式
        parts := strings.Split(auth, " ")
        if len(parts) != 2 || parts[0] != "Bearer" {
            c.JSON(401, dto.Fail[any]("认证格式错误"))
            c.Abort()
            return
        }

        token := parts[1]
        
        // 验证 token
        db, err := database.GetDB()
        if err != nil {
            c.JSON(401, dto.Fail[any]("系统错误"))
            c.Abort()
            return
        }

        var user models.User
        if err := db.Where("token = ?", token).First(&user).Error; err != nil {
            c.JSON(401, dto.Fail[any]("无效的token"))
            c.Abort()
            return
        }

        if user.Token == "" {
            c.JSON(401, dto.Fail[any]("token已失效"))
            c.Abort()
            return
        }

        // 将用户信息存储到上下文中
        c.Set("user_id", user.ID)
        c.Set("username", user.Username)
        c.Set("is_admin", user.IsAdmin)
        c.Next()
    }
}