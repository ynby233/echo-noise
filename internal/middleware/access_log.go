package middleware

import (
	"context"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
)

const (
	accessLogConfigTTL     = 5 * time.Second
	accessLogBatchInterval = 100 * time.Millisecond
	accessLogBatchSize     = 100
	accessLogQueueSize     = 4096
)

var accessLogConfigCache = struct {
	sync.Mutex
	db               *gorm.DB
	expiresAt        time.Time
	accessLogEnabled bool
	siteVisitEnabled bool
}{}

type accessLogQueueItem struct {
	db        *gorm.DB
	accessLog *models.SecurityAccessLog
	siteVisit *models.SecuritySiteVisitLog
	flush     chan error
}

var (
	accessLogQueueOnce sync.Once
	accessLogQueue     chan accessLogQueueItem
	droppedAccessLogs  atomic.Uint64
)

func AccessLogMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		accessEnabled, siteVisitEnabled := accessLogSettings()
		recordAccess := accessEnabled && !shouldSkipAccessLog(c.Request.Method, c.Request.URL.Path)
		recordSiteVisit := siteVisitEnabled && shouldRecordSiteVisitRequest(c)
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

func accessLogSettings() (bool, bool) {
	db := models.GetDB()
	if db == nil {
		return false, false
	}
	now := time.Now()
	accessLogConfigCache.Lock()
	defer accessLogConfigCache.Unlock()
	if accessLogConfigCache.db == db && now.Before(accessLogConfigCache.expiresAt) {
		return accessLogConfigCache.accessLogEnabled, accessLogConfigCache.siteVisitEnabled
	}
	var cfg models.SecurityConfig
	accessEnabled := false
	siteVisitEnabled := false
	if err := db.Select("access_log_enabled", "site_visit_log_enabled").Order("id asc").First(&cfg).Error; err == nil {
		accessEnabled = cfg.AccessLogEnabled
		siteVisitEnabled = cfg.SiteVisitLogEnabled
	}
	accessLogConfigCache.db = db
	accessLogConfigCache.expiresAt = now.Add(accessLogConfigTTL)
	accessLogConfigCache.accessLogEnabled = accessEnabled
	accessLogConfigCache.siteVisitEnabled = siteVisitEnabled
	return accessEnabled, siteVisitEnabled
}

func InvalidateAccessLogConfigCache() {
	accessLogConfigCache.Lock()
	accessLogConfigCache.db = nil
	accessLogConfigCache.expiresAt = time.Time{}
	accessLogConfigCache.accessLogEnabled = false
	accessLogConfigCache.siteVisitEnabled = false
	accessLogConfigCache.Unlock()
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
	enqueueAccessLog(accessLogQueueItem{db: db, accessLog: &log})
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
	enqueueAccessLog(accessLogQueueItem{db: db, siteVisit: &log})
}

func enqueueAccessLog(item accessLogQueueItem) {
	queue := ensureAccessLogQueue()
	select {
	case queue <- item:
	default:
		dropped := droppedAccessLogs.Add(1)
		if dropped == 1 || dropped%1000 == 0 {
			log.Printf("访问日志异步队列已满，累计丢弃 %d 条日志", dropped)
		}
	}
}

func ensureAccessLogQueue() chan accessLogQueueItem {
	accessLogQueueOnce.Do(func() {
		accessLogQueue = make(chan accessLogQueueItem, accessLogQueueSize)
		go runAccessLogWriter(accessLogQueue)
	})
	return accessLogQueue
}

func runAccessLogWriter(queue <-chan accessLogQueueItem) {
	ticker := time.NewTicker(accessLogBatchInterval)
	defer ticker.Stop()
	batch := make([]accessLogQueueItem, 0, accessLogBatchSize)
	flush := func() error {
		err := writeAccessLogBatch(batch)
		batch = batch[:0]
		return err
	}
	for {
		select {
		case item := <-queue:
			if item.flush != nil {
				item.flush <- flush()
				continue
			}
			batch = append(batch, item)
			if len(batch) >= accessLogBatchSize {
				if err := flush(); err != nil {
					log.Printf("批量写入访问日志失败: %v", err)
				}
			}
		case <-ticker.C:
			if len(batch) > 0 {
				if err := flush(); err != nil {
					log.Printf("批量写入访问日志失败: %v", err)
				}
			}
		}
	}
}

func writeAccessLogBatch(batch []accessLogQueueItem) error {
	if len(batch) == 0 {
		return nil
	}
	accessByDB := make(map[*gorm.DB][]models.SecurityAccessLog)
	visitsByDB := make(map[*gorm.DB][]models.SecuritySiteVisitLog)
	for _, item := range batch {
		if item.db == nil {
			continue
		}
		if item.accessLog != nil {
			accessByDB[item.db] = append(accessByDB[item.db], *item.accessLog)
		}
		if item.siteVisit != nil {
			visitsByDB[item.db] = append(visitsByDB[item.db], *item.siteVisit)
		}
	}
	var firstErr error
	for db, logs := range accessByDB {
		if err := db.CreateInBatches(&logs, accessLogBatchSize).Error; err != nil && firstErr == nil {
			firstErr = err
		}
	}
	for db, visits := range visitsByDB {
		if err := db.CreateInBatches(&visits, accessLogBatchSize).Error; err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func FlushAccessLogs(ctx context.Context) error {
	done := make(chan error, 1)
	item := accessLogQueueItem{flush: done}
	select {
	case ensureAccessLogQueue() <- item:
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
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
