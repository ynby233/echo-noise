package middleware

import (
	"strings"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/internal/models"
)

func AccessLogMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		recordAccess := !shouldSkipAccessLog(c.Request.Method, c.Request.URL.Path) && isAccessLogEnabled()
		recordSiteVisit := shouldRecordSiteVisitRequest(c) && isSiteVisitLogEnabled()
		if !recordAccess && !recordSiteVisit {
			c.Next()
			return
		}

		start := time.Now()
		c.Next()
		if recordAccess {
			recordAccessLog(c, time.Since(start))
		}
		if recordSiteVisit && responseAllowsSiteVisitLog(c) {
			recordSiteVisitLog(c)
		}
	}
}

func isAccessLogEnabled() bool {
	db := models.GetDB()
	if db == nil {
		return false
	}
	var cfg models.SecurityConfig
	if err := db.Select("access_log_enabled").Order("id asc").First(&cfg).Error; err != nil {
		return false
	}
	return cfg.AccessLogEnabled
}

func isSiteVisitLogEnabled() bool {
	db := models.GetDB()
	if db == nil {
		return false
	}
	var cfg models.SecurityConfig
	if err := db.Select("site_visit_log_enabled").Order("id asc").First(&cfg).Error; err != nil {
		return false
	}
	return cfg.SiteVisitLogEnabled
}

func shouldSkipAccessLog(method string, path string) bool {
	if strings.EqualFold(method, "OPTIONS") {
		return true
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return false
	}

	skipPrefixes := []string{
		"/_nuxt/",
		"/assets/",
		"/css/",
		"/js/",
		"/fonts/",
		"/images/",
		"/uploads/",
		"/video/",
		"/api/images",
		"/api/video",
		"/api/security/access-logs",
		"/api/security/site-visits",
	}
	for _, prefix := range skipPrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}

	skipExact := map[string]bool{
		"/favicon.ico":          true,
		"/manifest.json":        true,
		"/manifest.webmanifest": true,
		"/sw.js":                true,
		"/api/manifest":         true,
	}
	if skipExact[path] {
		return true
	}

	staticExts := []string{".css", ".js", ".map", ".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg", ".ico", ".woff", ".woff2", ".ttf", ".eot", ".mp3", ".mp4", ".webm"}
	lowerPath := strings.ToLower(path)
	for _, ext := range staticExts {
		if strings.HasSuffix(lowerPath, ext) {
			return true
		}
	}
	return false
}

func shouldRecordSiteVisitRequest(c *gin.Context) bool {
	if c == nil || c.Request == nil {
		return false
	}
	if !strings.EqualFold(c.Request.Method, "GET") {
		return false
	}
	path := strings.TrimSpace(c.Request.URL.Path)
	if path != "/" && path != "/index.html" {
		return false
	}
	return headerAcceptsHTML(c.GetHeader("Accept"))
}

func headerAcceptsHTML(value string) bool {
	for _, part := range strings.Split(value, ",") {
		mediaType := strings.TrimSpace(strings.SplitN(part, ";", 2)[0])
		if strings.EqualFold(mediaType, "text/html") {
			return true
		}
	}
	return false
}

func responseAllowsSiteVisitLog(c *gin.Context) bool {
	status := c.Writer.Status()
	if status == 0 {
		status = 200
	}
	return status >= 200 && status < 400
}

func recordAccessLog(c *gin.Context, duration time.Duration) {
	db := models.GetDB()
	if db == nil {
		return
	}

	userID, username, isAdmin := resolveAccessLogUser(c)
	status := c.Writer.Status()
	if status == 0 {
		status = 200
	}

	log := models.SecurityAccessLog{
		IP:         trimLogField(c.ClientIP(), 191),
		Method:     trimLogField(strings.ToUpper(c.Request.Method), 20),
		Path:       trimLogField(c.Request.URL.Path, 1024),
		Status:     status,
		UserID:     userID,
		Username:   trimLogField(username, 191),
		IsAdmin:    isAdmin,
		UserAgent:  trimLogField(c.GetHeader("User-Agent"), 2048),
		Referer:    trimLogField(c.GetHeader("Referer"), 2048),
		DurationMS: duration.Milliseconds(),
	}
	_ = db.Create(&log).Error
}

func recordSiteVisitLog(c *gin.Context) {
	db := models.GetDB()
	if db == nil {
		return
	}
	userID, username, isAdmin := resolveAccessLogUser(c)
	log := models.SecuritySiteVisitLog{
		IP:        trimLogField(c.ClientIP(), 191),
		UserID:    userID,
		Username:  trimLogField(username, 191),
		IsAdmin:   isAdmin,
		UserAgent: trimLogField(c.GetHeader("User-Agent"), 2048),
		Referer:   trimLogField(c.GetHeader("Referer"), 2048),
	}
	_ = db.Create(&log).Error
}

func resolveAccessLogUser(c *gin.Context) (uint, string, bool) {
	var userID uint
	var username string
	var isAdmin bool

	if v, ok := c.Get("user_id"); ok {
		if id, ok := toUint(v); ok {
			userID = id
		}
	}
	if v, ok := c.Get("username"); ok {
		username = toString(v)
	}
	if v, ok := c.Get("is_admin"); ok {
		isAdmin = toBool(v)
	}

	if userID == 0 {
		session := sessions.Default(c)
		if v := session.Get("user_id"); v != nil {
			if id, ok := toUint(v); ok {
				userID = id
			}
		}
		if username == "" {
			username = toString(session.Get("username"))
		}
		if !isAdmin {
			isAdmin = toBool(session.Get("is_admin"))
		}
	}

	return userID, strings.TrimSpace(username), isAdmin
}

func toString(v any) string {
	switch val := v.(type) {
	case string:
		return strings.TrimSpace(val)
	case []byte:
		return strings.TrimSpace(string(val))
	default:
		return ""
	}
}

func toBool(v any) bool {
	switch val := v.(type) {
	case bool:
		return val
	case string:
		s := strings.ToLower(strings.TrimSpace(val))
		return s == "true" || s == "1" || s == "yes"
	default:
		return false
	}
}

func trimLogField(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || len(value) <= limit {
		return value
	}
	cut := 0
	for i := range value {
		if i > limit {
			break
		}
		cut = i
	}
	if cut <= 0 {
		return ""
	}
	return value[:cut]
}
