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
		cfg = models.SecurityConfig{AutoBanEnabled: false, AutoBanWindowSeconds: 600, AutoBanThreshold: 10, AutoBanMinutes: 60}
		_ = db.Create(&cfg).Error
	}
	c.JSON(http.StatusOK, dto.OK(cfg, "ok"))
}

type securityConfigReq struct {
	AutoBanEnabled       bool `json:"autoBanEnabled"`
	AutoBanWindowSeconds int  `json:"autoBanWindowSeconds"`
	AutoBanThreshold     int  `json:"autoBanThreshold"`
	AutoBanMinutes       int  `json:"autoBanMinutes"`
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
	if cfg.ID == 0 {
		_ = db.Create(&cfg).Error
	} else {
		_ = db.Save(&cfg).Error
	}
	c.JSON(http.StatusOK, dto.OK(cfg, "已保存"))
}
