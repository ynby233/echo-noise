package services

import (
	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/config"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/dto"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/pkg"
)

// UploadImage 上传图片
func UploadImage(c *gin.Context) dto.Result[string] {
	userID := c.GetUint("user_id")
	if userID == 0 {
		return dto.Fail[string]("未登录或登录已过期")
	}

	// 允许所有已登录用户上传头像/图片
	_, err := GetUserByID(userID)
	if err != nil {
		return dto.Fail[string](err.Error())
	}

	// 从配置中读取支持的扩展名
	allowedExtensions := config.Config.Upload.AllowedTypes

	// 获取站点配置
	var siteConfig models.SiteConfig
	if err := database.DB.Table("site_configs").First(&siteConfig).Error; err != nil {
		// 如果获取配置失败，使用 nil，将使用本地存储
		// 或者记录错误
	}

	// 调用 pkg 中的图片上传方法
	imageURL, err := pkg.UploadImage(c, allowedExtensions, &siteConfig)
	if err != nil {
		return dto.Fail[string](err.Error())
	}

	return dto.OK(imageURL)
}
