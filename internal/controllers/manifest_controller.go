package controllers

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/internal/services"
)

func GetWebManifest(c *gin.Context) {
	configMap, _ := services.GetFrontendConfig()
	fs := map[string]interface{}{}
	if v, ok := configMap["frontendSettings"].(map[string]interface{}); ok {
		fs = v
	}

	// 读取 PWA 设置（优先用 PWA 字段，否则回退到站点字段）
	pwaEnabled := true
	if v, ok := fs["pwaEnabled"].(bool); ok {
		pwaEnabled = v
	}
	if !pwaEnabled {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	title := ""
	description := ""
	// 站点默认图标使用 SVG
	siteIcon := "/favicon.svg"

	if pwaEnabled {
		if v, ok := fs["pwaTitle"].(string); ok {
			title = strings.TrimSpace(v)
		}
		if v, ok := fs["pwaDescription"].(string); ok {
			description = v
		}
	}
	if title == "" {
		if v, ok := fs["siteTitle"].(string); ok && v != "" {
			title = strings.TrimSpace(v)
		}
	}
	if title == "" {
		title = "个人站点"
	}
	if description == "" {
		if v, ok := fs["description"].(string); ok {
			description = v
		}
	}
	if v, ok := fs["rssFaviconURL"].(string); ok && v != "" {
		siteIcon = v
	}

	// PWA 图标选择：优先 pwaIconURL；否则回退到 SVG
	pwaIcon := "/favicon.svg"
	if v, ok := fs["pwaIconURL"].(string); ok && v != "" {
		pwaIcon = v
	}

	// favicon 类型
	icon := siteIcon
	iconLower := strings.ToLower(icon)
	iconType := "image/svg+xml"
	if strings.HasSuffix(iconLower, ".png") {
		iconType = "image/png"
	}

	// 计算 PWA 图标 sizes 与类型
	pwaLower := strings.ToLower(pwaIcon)
	pwaType := func() string {
		if strings.HasSuffix(pwaLower, ".png") {
			return "image/png"
		}
		if strings.HasSuffix(pwaLower, ".svg") {
			return "image/svg+xml"
		}
		return "image/png"
	}()
	pwaSize := "any"
	if m := regexp.MustCompile(`(\d+)x(\d+)`).FindStringSubmatch(pwaLower); len(m) == 3 {
		pwaSize = m[1] + "x" + m[2]
	}
	manifest := map[string]interface{}{
		"id":               "/",
		"name":             title,
		"short_name":       title,
		"description":      description,
		"start_url":        "/",
		"scope":            "/",
		"lang":             "zh-CN",
		"display":          "standalone",
		"background_color": "#000000",
		"theme_color":      "#000000",
		"icons": []map[string]string{
			{"src": "/android-chrome-192x192.png", "sizes": "192x192", "type": "image/png", "purpose": "any"},
			{"src": "/android-chrome-512x512.png", "sizes": "512x512", "type": "image/png", "purpose": "any"},
			{"src": icon, "sizes": "any", "type": iconType, "purpose": "any"},
			{"src": pwaIcon, "sizes": pwaSize, "type": pwaType, "purpose": "any maskable"},
			{"src": func() string {
				if strings.Contains(pwaLower, "512x512") && strings.HasSuffix(pwaLower, ".png") {
					return pwaIcon
				}
				return "/android-chrome-512x512.png"
			}(), "sizes": "512x512", "type": "image/png", "purpose": "any maskable"},
			{"src": func() string {
				if strings.Contains(pwaLower, "180x180") && strings.HasSuffix(pwaLower, ".png") {
					return pwaIcon
				}
				return "/apple-touch-icon.png"
			}(), "sizes": "180x180", "type": "image/png"},
		},
	}

	c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
	c.Header("Pragma", "no-cache")
	c.Header("Expires", "0")
	b, _ := json.Marshal(manifest)
	c.Data(http.StatusOK, "application/manifest+json; charset=utf-8", b)
}

// GetNotifyConfig 获取推送配置
