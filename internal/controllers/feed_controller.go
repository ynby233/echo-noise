package controllers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/internal/services"
)

func GenerateRSS(c *gin.Context) {
	rss, err := services.GenerateRSS(c)
	if err != nil {
		if err == services.ErrRSSDisabled {
			c.JSON(http.StatusNotFound, gin.H{"code": 0, "msg": "RSS 已禁用"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"code": 0, "msg": err.Error()})
		return
	}

	c.Header("Content-Type", "application/rss+xml; charset=utf-8")
	c.String(http.StatusOK, rss)
}
