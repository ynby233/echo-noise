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
	limit := parseInfoFeedLimit(c)

	items, err := services.LoadInfoFeedItems(limit)
	if err != nil && len(items) == 0 {
		c.JSON(http.StatusOK, dto.Fail[string]("加载信息流失败: "+err.Error()))
		return
	}
	c.JSON(http.StatusOK, dto.OK(items, "ok"))
}

func RefreshInfoFeedItems(c *gin.Context) {
	items, err := services.RefreshInfoFeedItems(parseInfoFeedLimit(c))
	if err != nil && len(items) == 0 {
		c.JSON(http.StatusOK, dto.Fail[string]("刷新信息流失败: "+err.Error()))
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, dto.OK(items, "ok"))
}

func parseInfoFeedLimit(c *gin.Context) int {
	limit := 0
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}
	if limit > 100 {
		return 100
	}
	if limit < 0 {
		return 0
	}
	return limit
}
