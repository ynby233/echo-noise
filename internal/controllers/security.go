package controllers

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/internal/dto"
	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
)

func GetAttackRecords(c *gin.Context) {
	db := models.GetDB()
	if db == nil {
		c.JSON(http.StatusOK, dto.OK([]models.SecurityAttackLog{}, "ok"))
		return
	}

	limit := 200
	if v := strings.TrimSpace(c.Query("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			if n > 0 && n <= 1000 {
				limit = n
			}
		}
	}

	var logs []models.SecurityAttackLog
	_ = db.Order("id desc").Limit(limit).Find(&logs).Error
	c.JSON(http.StatusOK, dto.OK(logs, "ok"))
}

const (
	loginAuditActionLogin  = "login"
	loginAuditActionLogout = "logout"
)

func recordUserLoginAudit(c *gin.Context, user *models.User, action string) error {
	if user == nil || user.ID == 0 || user.IsAdmin {
		return nil
	}
	db := models.GetDB()
	if db == nil {
		return nil
	}
	audit := models.SecurityLoginAudit{
		UserID:    user.ID,
		Username:  strings.TrimSpace(user.Username),
		Action:    normalizeLoginAuditAction(action),
		IP:        c.ClientIP(),
		UserAgent: c.GetHeader("User-Agent"),
	}
	return db.Create(&audit).Error
}

func normalizeLoginAuditAction(action string) string {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case loginAuditActionLogout:
		return loginAuditActionLogout
	default:
		return loginAuditActionLogin
	}
}

func GetLoginAudits(c *gin.Context) {
	db := models.GetDB()
	if db == nil {
		c.JSON(http.StatusOK, dto.OK([]models.SecurityLoginAudit{}, "ok"))
		return
	}

	limit := 200
	if v := strings.TrimSpace(c.Query("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			if n > 0 && n <= 1000 {
				limit = n
			}
		}
	}

	query := db.Order("id desc").Limit(limit)
	if username := strings.TrimSpace(c.Query("username")); username != "" {
		query = query.Where("username = ?", username)
	}
	if ip := strings.TrimSpace(c.Query("ip")); ip != "" {
		query = query.Where("ip = ?", ip)
	}
	if action := strings.ToLower(strings.TrimSpace(c.Query("action"))); action != "" {
		switch normalizeLoginAuditAction(action) {
		case loginAuditActionLogout:
			query = query.Where("action = ?", loginAuditActionLogout)
		default:
			query = query.Where("action = ? OR action IS NULL OR action = ''", loginAuditActionLogin)
		}
	}

	var audits []models.SecurityLoginAudit
	_ = query.Find(&audits).Error
	c.JSON(http.StatusOK, dto.OK(audits, "ok"))
}

func GetAccessLogs(c *gin.Context) {
	db := models.GetDB()
	if db == nil {
		c.JSON(http.StatusOK, dto.OK([]models.SecurityAccessLog{}, "ok"))
		return
	}

	limit := 50
	if v := strings.TrimSpace(c.Query("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			if n > 0 && n <= 200 {
				limit = n
			}
		}
	}

	query := db.Order("id desc").Limit(limit)
	query = applyVisitIdentityFilters(c, query)
	if method := strings.ToUpper(strings.TrimSpace(c.Query("method"))); method != "" {
		query = query.Where("method = ?", method)
	}
	if path := strings.TrimSpace(c.Query("path")); path != "" {
		query = query.Where("path LIKE ?", "%"+path+"%")
	}
	if status := strings.TrimSpace(c.Query("status")); status != "" {
		if n, err := strconv.Atoi(status); err == nil && n >= 100 && n <= 599 {
			query = query.Where("status = ?", n)
		}
	}
	if start, ok := parseAccessLogDate(c.Query("startDate")); ok {
		query = query.Where("created_at >= ?", start)
	}
	if end, ok := parseAccessLogDate(c.Query("endDate")); ok {
		query = query.Where("created_at < ?", end.AddDate(0, 0, 1))
	}

	var logs []models.SecurityAccessLog
	_ = query.Find(&logs).Error
	c.JSON(http.StatusOK, dto.OK(logs, "ok"))
}

func GetSiteVisits(c *gin.Context) {
	db := models.GetDB()
	if db == nil {
		c.JSON(http.StatusOK, dto.OK([]models.SecuritySiteVisitLog{}, "ok"))
		return
	}

	limit := 50
	if v := strings.TrimSpace(c.Query("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			if n > 0 && n <= 200 {
				limit = n
			}
		}
	}

	query := db.Order("id desc").Limit(limit)
	query = applyVisitIdentityFilters(c, query)
	if start, ok := parseAccessLogDate(c.Query("startDate")); ok {
		query = query.Where("created_at >= ?", start)
	}
	if end, ok := parseAccessLogDate(c.Query("endDate")); ok {
		query = query.Where("created_at < ?", end.AddDate(0, 0, 1))
	}

	var logs []models.SecuritySiteVisitLog
	_ = query.Find(&logs).Error
	c.JSON(http.StatusOK, dto.OK(logs, "ok"))
}

func applyVisitIdentityFilters(c *gin.Context, query *gorm.DB) *gorm.DB {
	if ip := strings.TrimSpace(c.Query("ip")); ip != "" {
		query = query.Where("ip = ?", ip)
	}
	username := strings.TrimSpace(c.Query("username"))
	visitorOnly := isVisitorKeyword(username)
	if visitorFlag := strings.ToLower(strings.TrimSpace(c.Query("visitor"))); visitorFlag == "1" || visitorFlag == "true" {
		visitorOnly = true
	}
	if visitorOnly {
		query = query.Where("user_id = 0")
	} else if username != "" {
		query = query.Where("username LIKE ?", "%"+username+"%")
	}
	if userIDs := parseAccessLogUserIDs(c.Query("user_ids")); len(userIDs) > 0 {
		query = query.Where("user_id IN ?", userIDs)
	} else if userID := strings.TrimSpace(c.Query("user_id")); userID != "" {
		if n, err := strconv.ParseUint(userID, 10, 64); err == nil {
			query = query.Where("user_id = ?", uint(n))
		}
	}
	return query
}

func isVisitorKeyword(value string) bool {
	s := strings.ToLower(strings.TrimSpace(value))
	return s == "访客" || s == "游客" || s == "guest" || s == "visitor"
}

func parseAccessLogUserIDs(value string) []uint {
	parts := strings.Split(strings.TrimSpace(value), ",")
	ids := make([]uint, 0, len(parts))
	seen := map[uint]bool{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		n, err := strconv.ParseUint(part, 10, 64)
		if err != nil {
			continue
		}
		id := uint(n)
		if seen[id] {
			continue
		}
		ids = append(ids, id)
		seen[id] = true
	}
	return ids
}

func parseAccessLogDate(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.Local
	}
	parsed, err := time.ParseInLocation("2006-01-02", value, loc)
	if err != nil {
		return time.Time{}, false
	}
	return parsed, true
}

func ClearAccessLogs(c *gin.Context) {
	db := models.GetDB()
	if db == nil {
		c.JSON(http.StatusOK, dto.OK[any](nil, "已清空"))
		return
	}
	_ = db.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&models.SecurityAccessLog{}).Error
	c.JSON(http.StatusOK, dto.OK[any](nil, "已清空"))
}

func ClearSiteVisits(c *gin.Context) {
	db := models.GetDB()
	if db == nil {
		c.JSON(http.StatusOK, dto.OK[any](nil, "已清空"))
		return
	}
	_ = db.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&models.SecuritySiteVisitLog{}).Error
	c.JSON(http.StatusOK, dto.OK[any](nil, "已清空"))
}

func ClearAttackRecords(c *gin.Context) {
	db := models.GetDB()
	if db == nil {
		c.JSON(http.StatusOK, dto.OK[any](nil, "已清空"))
		return
	}
	_ = db.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&models.SecurityAttackLog{}).Error
	c.JSON(http.StatusOK, dto.OK[any](nil, "已清空"))
}

type banReq struct {
	IP      string `json:"ip"`
	Reason  string `json:"reason"`
	Minutes int    `json:"minutes"`
}

func cleanupExpiredBans(db *gorm.DB) {
	if db == nil {
		return
	}
	now := time.Now()
	_ = db.Where("until IS NOT NULL AND until <= ?", now).Delete(&models.SecurityIPBan{}).Error
}

func GetIPBans(c *gin.Context) {
	db := models.GetDB()
	if db == nil {
		c.JSON(http.StatusOK, dto.OK([]models.SecurityIPBan{}, "ok"))
		return
	}
	cleanupExpiredBans(db)
	var bans []models.SecurityIPBan
	_ = db.Order("id desc").Find(&bans).Error
	c.JSON(http.StatusOK, dto.OK(bans, "ok"))
}

func AddIPBan(c *gin.Context) {
	var req banReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, dto.Fail[any]("参数错误"))
		return
	}
	ip := strings.TrimSpace(req.IP)
	if ip == "" {
		c.JSON(http.StatusOK, dto.Fail[any]("IP不能为空"))
		return
	}
	if ip == "127.0.0.1" || ip == "::1" {
		c.JSON(http.StatusOK, dto.Fail[any]("禁止封禁本机IP"))
		return
	}

	db := models.GetDB()
	if db == nil {
		c.JSON(http.StatusOK, dto.Fail[any]("数据库未初始化"))
		return
	}

	var until *time.Time
	if req.Minutes > 0 {
		t := time.Now().Add(time.Duration(req.Minutes) * time.Minute)
		until = &t
	}

	ban := models.SecurityIPBan{IP: ip, Reason: strings.TrimSpace(req.Reason), Until: until}
	// upsert
	var existing models.SecurityIPBan
	if err := db.Where("ip = ?", ip).First(&existing).Error; err == nil {
		existing.Reason = ban.Reason
		existing.Until = ban.Until
		_ = db.Save(&existing).Error
		c.JSON(http.StatusOK, dto.OK(existing, "已封禁"))
		return
	}
	if err := db.Create(&ban).Error; err != nil {
		c.JSON(http.StatusOK, dto.Fail[any](err.Error()))
		return
	}
	c.JSON(http.StatusOK, dto.OK(ban, "已封禁"))
}

func RemoveIPBan(c *gin.Context) {
	ip := strings.TrimSpace(c.Query("ip"))
	if ip == "" {
		c.JSON(http.StatusOK, dto.Fail[any]("ip不能为空"))
		return
	}
	db := models.GetDB()
	if db == nil {
		c.JSON(http.StatusOK, dto.OK[any](nil, "已解封"))
		return
	}
	_ = db.Where("ip = ?", ip).Delete(&models.SecurityIPBan{}).Error
	c.JSON(http.StatusOK, dto.OK[any](nil, "已解封"))
}

func GetSecurityConfig(c *gin.Context) {
	db := models.GetDB()
	if db == nil {
		c.JSON(http.StatusOK, dto.OK(models.SecurityConfig{}, "ok"))
		return
	}
	var cfg models.SecurityConfig
	if err := db.Order("id asc").First(&cfg).Error; err != nil {
		cfg = models.SecurityConfig{AutoBanEnabled: false, AutoBanWindowSeconds: 600, AutoBanThreshold: 10, AutoBanMinutes: 60, AccessLogEnabled: false, SiteVisitLogEnabled: false}
		_ = db.Create(&cfg).Error
	}
	c.JSON(http.StatusOK, dto.OK(cfg, "ok"))
}

type securityConfigReq struct {
	AutoBanEnabled       bool `json:"autoBanEnabled"`
	AutoBanWindowSeconds int  `json:"autoBanWindowSeconds"`
	AutoBanThreshold     int  `json:"autoBanThreshold"`
	AutoBanMinutes       int  `json:"autoBanMinutes"`
	AccessLogEnabled     bool `json:"accessLogEnabled"`
	SiteVisitLogEnabled  bool `json:"siteVisitLogEnabled"`
}

func UpdateSecurityConfig(c *gin.Context) {
	var req securityConfigReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, dto.Fail[any]("参数错误"))
		return
	}
	if req.AutoBanWindowSeconds <= 0 {
		req.AutoBanWindowSeconds = 600
	}
	if req.AutoBanWindowSeconds > 86400 {
		req.AutoBanWindowSeconds = 86400
	}
	if req.AutoBanThreshold <= 0 {
		req.AutoBanThreshold = 10
	}
	if req.AutoBanThreshold > 1000 {
		req.AutoBanThreshold = 1000
	}
	if req.AutoBanMinutes < 0 {
		req.AutoBanMinutes = 0
	}
	if req.AutoBanMinutes > 525600 {
		req.AutoBanMinutes = 525600
	}

	db := models.GetDB()
	if db == nil {
		c.JSON(http.StatusOK, dto.Fail[any]("数据库未初始化"))
		return
	}
	var cfg models.SecurityConfig
	if err := db.Order("id asc").First(&cfg).Error; err != nil {
		cfg = models.SecurityConfig{}
	}
	cfg.AutoBanEnabled = req.AutoBanEnabled
	cfg.AutoBanWindowSeconds = req.AutoBanWindowSeconds
	cfg.AutoBanThreshold = req.AutoBanThreshold
	cfg.AutoBanMinutes = req.AutoBanMinutes
	cfg.AccessLogEnabled = req.AccessLogEnabled
	cfg.SiteVisitLogEnabled = req.SiteVisitLogEnabled
	if cfg.ID == 0 {
		_ = db.Create(&cfg).Error
	} else {
		_ = db.Save(&cfg).Error
	}
	c.JSON(http.StatusOK, dto.OK(cfg, "已保存"))
}
