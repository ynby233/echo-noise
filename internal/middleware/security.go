package middleware

import (
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/internal/dto"
	"github.com/rcy1314/echo-noise/internal/models"
)

func getAutoBanConfig() models.SecurityConfig {
	autoCfgMu.RLock()
	if autoCfg != nil && time.Now().Before(autoCfgExp) {
		cfg := *autoCfg
		autoCfgMu.RUnlock()
		return cfg
	}
	autoCfgMu.RUnlock()

	cfg := models.SecurityConfig{AutoBanEnabled: false, AutoBanWindowSeconds: 600, AutoBanThreshold: 10, AutoBanMinutes: 60}
	db := models.GetDB()
	if db != nil {
		var dbCfg models.SecurityConfig
		if err := db.Order("id asc").First(&dbCfg).Error; err == nil {
			cfg = dbCfg
		}
	}

	autoCfgMu.Lock()
	autoCfg = &cfg
	autoCfgExp = time.Now().Add(15 * time.Second)
	autoCfgMu.Unlock()
	return cfg
}

func recordAndMaybeAutoBan(ip string) {
	if ip == "" || isLocalIP(ip) {
		return
	}
	cfg := getAutoBanConfig()
	if !cfg.AutoBanEnabled {
		return
	}
	if cfg.AutoBanWindowSeconds <= 0 {
		return
	}
	if cfg.AutoBanThreshold <= 0 {
		return
	}

	now := time.Now()
	window := time.Duration(cfg.AutoBanWindowSeconds) * time.Second

	autoMu.Lock()
	it := autoCount[ip]
	if it.Start.IsZero() || now.Sub(it.Start) > window {
		it = struct{ Count int; Start time.Time }{Count: 0, Start: now}
	}
	it.Count++
	autoCount[ip] = it
	count := it.Count
	autoMu.Unlock()

	if count < cfg.AutoBanThreshold {
		return
	}

	db := models.GetDB()
	if db == nil {
		return
	}

	var until *time.Time
	if cfg.AutoBanMinutes > 0 {
		t := now.Add(time.Duration(cfg.AutoBanMinutes) * time.Minute)
		until = &t
	}

	// upsert ban
	var existing models.SecurityIPBan
	if err := db.Where("ip = ?", ip).First(&existing).Error; err == nil {
		existing.Reason = "auto-ban"
		existing.Until = until
		_ = db.Save(&existing).Error
	} else {
		_ = db.Create(&models.SecurityIPBan{IP: ip, Reason: "auto-ban", Until: until}).Error
	}

	// 立刻让 banCache 生效
	banMu.Lock()
	banCache[ip] = banCacheItem{Banned: true, Exp: time.Now().Add(30 * time.Second)}
	banMu.Unlock()

	// reset counter after banning
	autoMu.Lock()
	delete(autoCount, ip)
	autoMu.Unlock()
}

type banCacheItem struct {
	Banned bool
	Exp    time.Time
}

var (
	banMu    sync.RWMutex
	banCache = map[string]banCacheItem{}

	autoMu     sync.Mutex
	autoCount  = map[string]struct{ Count int; Start time.Time }{}
	autoCfgMu  sync.RWMutex
	autoCfg    *models.SecurityConfig
	autoCfgExp time.Time

	// 常见敏感文件/目录扫描路径（只要命中就拒绝）
	suspiciousPathRegexes = []*regexp.Regexp{
		regexp.MustCompile(`(?i)^/(\.env|\.git|\.svn|\.hg|\.DS_Store)(/|$)`),
		regexp.MustCompile(`(?i)^/(internal|config|cmd|pkg|scripts|vendor|node_modules)(/|$)`),
		regexp.MustCompile(`(?i)^/(docker-compose\.ya?ml|dockerfile|go\.mod|go\.sum)(/|$)`),
		regexp.MustCompile(`(?i)^/(wp-admin|wp-login\.php|xmlrpc\.php)(/|$)`),
		regexp.MustCompile(`(?i)\.(php|asp|aspx|jsp)(\?|$)`),
		regexp.MustCompile(`(?i)^/(actuator|swagger|swagger-ui|v2/api-docs)(/|$)`),
	}
)

func isLocalIP(ip string) bool {
	ip = strings.TrimSpace(ip)
	if ip == "127.0.0.1" || ip == "::1" {
		return true
	}
	if strings.HasPrefix(ip, "192.168.") || strings.HasPrefix(ip, "10.") {
		return true
	}
	if strings.HasPrefix(ip, "172.") {
		parts := strings.Split(ip, ".")
		if len(parts) >= 2 {
			s := parts[1]
			if len(s) == 2 {
				if s[0] == '1' {
					return s[1] >= '6' && s[1] <= '9'
				}
				if s[0] == '2' {
					return true
				}
				if s[0] == '3' {
					return s[1] >= '0' && s[1] <= '1'
				}
			}
			if s == "16" || s == "17" || s == "18" || s == "19" || s == "20" || s == "21" || s == "22" || s == "23" || s == "24" || s == "25" || s == "26" || s == "27" || s == "28" || s == "29" || s == "30" || s == "31" {
				return true
			}
		}
	}
	return false
}

func isBannedIP(ip string) bool {
	if ip == "" {
		return false
	}
	if isLocalIP(ip) {
		return false
	}

	banMu.RLock()
	it, ok := banCache[ip]
	banMu.RUnlock()
	if ok {
		if time.Now().Before(it.Exp) {
			return it.Banned
		}
		banMu.Lock()
		delete(banCache, ip)
		banMu.Unlock()
	}

	db := models.GetDB()
	if db == nil {
		banMu.Lock()
		banCache[ip] = banCacheItem{Banned: false, Exp: time.Now().Add(30 * time.Second)}
		banMu.Unlock()
		return false
	}

	var ban models.SecurityIPBan
	err := db.Where("ip = ?", ip).Order("id desc").First(&ban).Error
	if err != nil {
		banMu.Lock()
		banCache[ip] = banCacheItem{Banned: false, Exp: time.Now().Add(30 * time.Second)}
		banMu.Unlock()
		return false
	}
	if ban.Until != nil && time.Now().After(*ban.Until) {
		banMu.Lock()
		banCache[ip] = banCacheItem{Banned: false, Exp: time.Now().Add(30 * time.Second)}
		banMu.Unlock()
		return false
	}

	banMu.Lock()
	banCache[ip] = banCacheItem{Banned: true, Exp: time.Now().Add(30 * time.Second)}
	banMu.Unlock()
	return true
}

func isSuspiciousPath(path string) bool {
	if path == "" {
		return false
	}

	// 对正常功能必须放行的路径做快速排除，避免误伤
	allowPrefixes := []string{
		"/api",
		"/_nuxt/",
		"/assets/",
		"/favicon",
		"/android-chrome",
		"/sw.js",
		"/manifest.json",
		"/manifest.webmanifest",
		"/rss",
		"/m/",
		"/video/",
		"/api/images",
		"/api/video",
	}
	for _, p := range allowPrefixes {
		if strings.HasPrefix(path, p) {
			return false
		}
	}

	for _, re := range suspiciousPathRegexes {
		if re.MatchString(path) {
			return true
		}
	}
	return false
}

// SecurityMiddleware
// - 拦截敏感路径扫描（访问核心文件/目录）
// - 不影响正常 API、静态资源、RSS、MCP（MCP 走 /api）
func SecurityMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		
		// 白名单机制：如果是本地 IP，则直接放行，不记录攻击日志，也不检查是否被 ban
		if isLocalIP(ip) {
			c.Next()
			return
		}

		if isBannedIP(ip) {
			c.JSON(http.StatusForbidden, dto.Fail[any]("禁止访问"))
			c.Abort()
			return
		}

		p := c.Request.URL.Path
		if isSuspiciousPath(p) {
			db := models.GetDB()
			if db != nil {
				_ = db.Create(&models.SecurityAttackLog{
					IP:     ip,
					Method: c.Request.Method,
					Path:   p,
					UA:     c.GetHeader("User-Agent"),
				}).Error
			}

			recordAndMaybeAutoBan(ip)

			c.JSON(http.StatusForbidden, dto.Fail[any]("禁止访问"))
			c.Abort()
			return
		}

		c.Next()
	}
}
