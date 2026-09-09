package controllers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/pkg"
)

func UploadVideo(c *gin.Context) {
	// 获取站点配置
	db, _ := database.GetDB()
	var siteConfig models.SiteConfig
	if err := db.First(&siteConfig).Error; err != nil {
		// 如果获取配置失败，使用空配置（默认本地存储）
		siteConfig = models.SiteConfig{}
	}

	// 支持的视频 MIME 类型
	allowedMimeTypes := []string{"video/mp4", "video/webm", "video/quicktime", "video/x-msvideo"}

	videoURL, err := pkg.UploadVideo(c, allowedMimeTypes, &siteConfig)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": err.Error()})
		return
	}

	// 返回视频访问路径
	c.JSON(http.StatusOK, gin.H{
		"code": 1,
		"msg":  "上传成功",
		"data": videoURL,
	})
}

// 上传音频
func UploadAudio(c *gin.Context) {
	db, _ := database.GetDB()
	var siteConfig models.SiteConfig
	if err := db.First(&siteConfig).Error; err != nil {
		siteConfig = models.SiteConfig{}
	}

	allowedMimeTypes := []string{"audio/webm", "audio/ogg", "audio/mpeg", "audio/mp4", "audio/wav", "audio/x-wav", "audio/flac", "audio/x-flac"}

	audioURL, err := pkg.UploadAudio(c, allowedMimeTypes, &siteConfig)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 1,
		"msg":  "上传成功",
		"data": audioURL,
	})
}

// ResetDefaultData 重置/初始化默认数据
