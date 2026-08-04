package controllers

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/internal/authorization"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/dto"
	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
)

func authorizationActorID(c *gin.Context) (uint, bool) {
	v, ok := c.Get("user_id")
	if !ok {
		return 0, false
	}
	return commentUint(v)
}

func AuthorizationMe(c *gin.Context) {
	actorID, ok := authorizationActorID(c)
	if !ok || actorID == 0 {
		c.JSON(http.StatusUnauthorized, dto.Fail[any]("未登录"))
		return
	}
	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.Fail[any]("授权服务不可用"))
		return
	}
	var user models.User
	if err := db.Select("id,is_admin").First(&user, actorID).Error; err != nil || !user.IsAdmin {
		c.JSON(http.StatusForbidden, dto.Fail[any]("需要管理员权限"))
		return
	}
	capabilities, err := authorization.New(db).CapabilitiesFor(actorID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.Fail[any]("读取授权失败"))
		return
	}
	c.JSON(http.StatusOK, dto.OK(gin.H{"is_admin": true, "is_primary_admin": actorID == models.PrimaryAdminUserID, "capabilities": capabilities}, "获取当前授权成功"))
}

func AuthorizationCatalog(c *gin.Context) {
	c.JSON(http.StatusOK, dto.OK(authorization.Catalog(), "获取授权目录成功"))
}

func ListAuthorizationAdmins(c *gin.Context) {
	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.Fail[any]("授权服务不可用"))
		return
	}
	var users []models.User
	if err := db.Select("id,username,avatar_url,is_admin").Where("is_admin = ? AND id <> ?", true, models.PrimaryAdminUserID).Order("id ASC").Find(&users).Error; err != nil {
		c.JSON(http.StatusInternalServerError, dto.Fail[any]("读取受托管理员失败"))
		return
	}
	c.JSON(http.StatusOK, dto.OK(users, "获取受托管理员成功"))
}

func GetAuthorizationAdmin(c *gin.Context) {
	target, db, ok := authorizationTarget(c)
	if !ok {
		return
	}
	var grants []models.AdminCapabilityGrant
	if err := db.Where("user_id = ?", target.ID).Order("capability ASC").Find(&grants).Error; err != nil {
		c.JSON(http.StatusInternalServerError, dto.Fail[any]("读取授权失败"))
		return
	}
	capabilities := make([]string, 0, len(grants))
	for _, grant := range grants {
		capabilities = append(capabilities, grant.Capability)
	}
	c.JSON(http.StatusOK, dto.OK(gin.H{"id": target.ID, "username": target.Username, "capabilities": capabilities}, "获取受托管理员授权成功"))
}

func ReplaceAuthorizationAdminGrants(c *gin.Context) {
	actorID, ok := authorizationActorID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, dto.Fail[any]("未登录"))
		return
	}
	target, db, ok := authorizationTarget(c)
	if !ok {
		return
	}
	var request struct {
		Capabilities []string `json:"capabilities"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, dto.Fail[any]("请求参数错误"))
		return
	}
	capabilities := make([]authorization.Capability, 0, len(request.Capabilities))
	for _, capability := range request.Capabilities {
		capabilities = append(capabilities, authorization.Capability(strings.TrimSpace(capability)))
	}
	if err := authorization.New(db).ReplaceGrants(actorID, target.ID, capabilities); err != nil {
		c.JSON(http.StatusBadRequest, dto.Fail[any]("保存授权失败"))
		return
	}
	c.JSON(http.StatusOK, dto.OK[any](nil, "保存授权成功"))
}

func authorizationTarget(c *gin.Context) (models.User, *gorm.DB, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 || uint(id) == models.PrimaryAdminUserID {
		c.JSON(http.StatusBadRequest, dto.Fail[any]("无效的受托管理员"))
		return models.User{}, nil, false
	}
	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.Fail[any]("授权服务不可用"))
		return models.User{}, nil, false
	}
	var target models.User
	if err := db.Select("id,username,is_admin").First(&target, uint(id)).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, dto.Fail[any]("用户不存在"))
		} else {
			c.JSON(http.StatusInternalServerError, dto.Fail[any]("读取用户失败"))
		}
		return models.User{}, nil, false
	}
	if !target.IsAdmin {
		c.JSON(http.StatusBadRequest, dto.Fail[any]("目标不是受托管理员"))
		return models.User{}, nil, false
	}
	return target, db, true
}

func ListAdminAuditLogs(c *gin.Context) {
	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.Fail[any]("审计服务不可用"))
		return
	}
	filters, err := parseAdminAuditFilters(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.Fail[any](err.Error()))
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "30"))
	if pageSize < 1 {
		pageSize = 30
	}
	if pageSize > 100 {
		pageSize = 100
	}
	query := applyAdminAuditFilters(db.Model(&models.AdminAuditLog{}), filters)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, dto.Fail[any]("查询审计失败"))
		return
	}
	var logs []models.AdminAuditLog
	if err := query.Order("created_at DESC, id DESC").Limit(pageSize).Offset((page - 1) * pageSize).Find(&logs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, dto.Fail[any]("查询审计失败"))
		return
	}
	c.JSON(http.StatusOK, dto.OK(gin.H{"items": presentAdminAuditLogs(logs), "page": page, "page_size": pageSize, "total": total}, "获取管理员审计成功"))
}

func GetAdminAuditLog(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, dto.Fail[any]("审计记录参数错误"))
		return
	}
	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.Fail[any]("审计服务不可用"))
		return
	}
	var log models.AdminAuditLog
	if err := db.First(&log, uint(id)).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, dto.Fail[any]("审计记录不存在"))
		} else {
			c.JSON(http.StatusInternalServerError, dto.Fail[any]("读取审计失败"))
		}
		return
	}
	actorID, ok := authorizationActorID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, dto.Fail[any]("未登录"))
		return
	}
	if err := authorization.New(db).WriteAudit(models.AdminAuditLog{ActorUserID: actorID, Capability: string(authorization.CapabilityAuditView), Module: "audit", Action: "view_detail", TargetType: "admin_audit_log", TargetID: fmt.Sprint(log.ID), Result: "success", Summary: "viewed administrator audit detail", IP: c.ClientIP(), UserAgent: c.GetHeader("User-Agent"), AuthVia: c.GetString("auth_via")}); err != nil {
		c.JSON(http.StatusInternalServerError, dto.Fail[any]("写入审计失败"))
		return
	}
	c.JSON(http.StatusOK, dto.OK(presentAdminAuditLog(log), "获取管理员审计详情成功"))
}

func GetAdminAuditConfig(c *gin.Context) {
	if _, err := requirePrimaryAdmin(c); err != nil {
		c.JSON(http.StatusForbidden, dto.Fail[any](err.Error()))
		return
	}
	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.Fail[any]("审计服务不可用"))
		return
	}
	config := models.AdminAuditConfig{ID: 1, Enabled: true}
	if err := db.First(&config, 1).Error; err != nil && err != gorm.ErrRecordNotFound {
		c.JSON(http.StatusInternalServerError, dto.Fail[any]("读取审计设置失败"))
		return
	}
	c.JSON(http.StatusOK, dto.OK(gin.H{"enabled": config.Enabled}, "获取审计设置成功"))
}

func UpdateAdminAuditConfig(c *gin.Context) {
	actorID, err := requirePrimaryAdmin(c)
	if err != nil {
		c.JSON(http.StatusForbidden, dto.Fail[any](err.Error()))
		return
	}
	var request struct {
		Enabled *bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&request); err != nil || request.Enabled == nil {
		c.JSON(http.StatusBadRequest, dto.Fail[any]("审计设置参数错误"))
		return
	}
	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.Fail[any]("审计服务不可用"))
		return
	}
	if err := authorization.New(db).SetAuditEnabled(actorID, *request.Enabled); err != nil {
		c.JSON(http.StatusInternalServerError, dto.Fail[any]("保存审计设置失败"))
		return
	}
	c.JSON(http.StatusOK, dto.OK(gin.H{"enabled": *request.Enabled}, "保存审计设置成功"))
}
