package controllers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/internal/dto"
	"github.com/rcy1314/echo-noise/internal/services"
)

func GetInfoFeedItems(c *gin.Context) {
	limit := 0
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}
	if limit > 100 {
		limit = 100
	}
	if limit < 0 {
		limit = 0
	}

	baseURL := requestBaseURL(c)
	items, err := services.LoadInfoFeedItems(baseURL, limit)
	if err != nil && len(items) == 0 {
		c.JSON(http.StatusOK, dto.Fail[string]("加载信息流失败: "+err.Error()))
		return
	}
	c.JSON(http.StatusOK, dto.OK(items, "ok"))
}

func requestBaseURL(c *gin.Context) string {
	scheme := c.GetHeader("X-Forwarded-Proto")
	if scheme == "" {
		if c.Request.TLS != nil {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}
	host := strings.TrimSpace(c.GetHeader("X-Forwarded-Host"))
	if host == "" {
		host = c.Request.Host
	}
	return scheme + "://" + host
}
