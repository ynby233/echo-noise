package controllers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/internal/services"
)

// UploadImage 控制器，调用 service 上传图片
func UploadImage(c *gin.Context) {
    result := services.UploadImage(c)
    if result.Code != 1 {
        c.JSON(http.StatusInternalServerError, result)
        return
    }
    c.JSON(http.StatusOK, result)
}
