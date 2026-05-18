package controllers

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/dto"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/repository"
	"github.com/rcy1314/echo-noise/internal/services"
	"github.com/rcy1314/echo-noise/internal/syncmanager"
	"github.com/rcy1314/echo-noise/pkg"
)

type captchaItem struct {
	Code string
	Exp  int64
}

var captchaStore = struct {
	sync.Mutex
	m map[string]captchaItem
}{m: map[string]captchaItem{}}

func newCaptchaID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("%x", b)
}

func setCaptcha(id, code string, exp int64) {
	captchaStore.Lock()
	defer captchaStore.Unlock()
	captchaStore.m[id] = captchaItem{Code: code, Exp: exp}
}

func getCaptcha(id string) (captchaItem, bool) {
	captchaStore.Lock()
	defer captchaStore.Unlock()
	it, ok := captchaStore.m[id]
	if !ok {
		return captchaItem{}, false
	}
	if it.Exp > 0 && time.Now().Unix() > it.Exp {
		delete(captchaStore.m, id)
		return captchaItem{}, false
	}
	return it, true
}

func deleteCaptcha(id string) {
	captchaStore.Lock()
	defer captchaStore.Unlock()
	delete(captchaStore.m, id)
}

func checkUser(c *gin.Context) (*models.User, error) {
	userID, exists := c.Get("user_id") // 修改 userid 为 user_id
	if !exists {
		return nil, fmt.Errorf(models.UserNotFoundMessage)
	}

	user, err := services.GetUserByID(userID.(uint))
	if err != nil {
		return nil, fmt.Errorf(models.UserNotFoundMessage)
	}
	return user, nil
}

func getLoginExpireDays() int {
	db, err := database.GetDB()
	if err != nil {
		return 3
	}
	var cfg models.SiteConfig
	if err := db.Table("site_configs").First(&cfg).Error; err != nil {
		return 3
	}
	if cfg.LoginExpireDays > 0 {
		return cfg.LoginExpireDays
	}
	return 3
}

func applyLoginSessionExpire(session sessions.Session) {
	days := getLoginExpireDays()
	ttlSeconds := days * 24 * 60 * 60
	session.Set("login_expire_at", time.Now().Add(time.Duration(ttlSeconds)*time.Second).Unix())
}

func Login(c *gin.Context) {
	var loginDto dto.LoginDto
	if err := c.ShouldBindJSON(&loginDto); err != nil {
		c.JSON(http.StatusOK, dto.Fail[any]("参数错误"))
		return
	}

	user, err := services.Login(loginDto)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[any](err.Error()))
		return
	}

	// 隐藏敏感字段
	user.Password = ""

	session := sessions.Default(c)
	session.Clear()
	applyLoginSessionExpire(session)
	session.Set("user_id", user.ID)
	session.Set("username", user.Username)
	session.Set("is_admin", user.IsAdmin)
	if err := session.Save(); err != nil {
		c.JSON(http.StatusOK, dto.Fail[any]("Session 保存失败"))
		return
	}

	c.JSON(http.StatusOK, dto.OK(user, "登录成功"))
}

// 添加登出功能
func Logout(c *gin.Context) {
	session := sessions.Default(c)
	session.Clear()
	session.Save()
	c.JSON(http.StatusOK, dto.OK[any](nil, "登出成功"))
}
func Register(c *gin.Context) {
	// 新增：注册前判断是否允许注册
	db, _ := database.GetDB()
	var setting models.Setting
	allowReg := true
	if err := db.Table("settings").First(&setting).Error; err == nil {
		allowReg = setting.AllowRegistration
	}

	if !allowReg {
		c.JSON(http.StatusOK, dto.Fail[string]("当前不允许注册新用户"))
		return
	}

	var user dto.RegisterDto
	if err := c.ShouldBindJSON(&user); err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](models.InvalidRequestBodyMessage))
		return
	}

	// 优先使用 captcha_id（适配移动端/不依赖 Cookie 的场景）
	if strings.TrimSpace(user.CaptchaId) != "" {
		it, ok := getCaptcha(strings.TrimSpace(user.CaptchaId))
		if !ok {
			c.JSON(http.StatusOK, dto.Fail[string]("验证码已过期"))
			return
		}
		if strings.ToLower(strings.TrimSpace(user.Captcha)) != strings.ToLower(strings.TrimSpace(it.Code)) {
			c.JSON(http.StatusOK, dto.Fail[string]("验证码不正确"))
			return
		}
		deleteCaptcha(strings.TrimSpace(user.CaptchaId))
	} else {
		// 兼容旧逻辑：使用 Session Cookie 存储验证码
		session := sessions.Default(c)
		sc := session.Get("captcha_code")
		se := session.Get("captcha_exp")
		if sc == nil || se == nil {
			c.JSON(http.StatusOK, dto.Fail[string]("验证码已过期"))
			return
		}
		exp, ok := se.(int64)
		if !ok || time.Now().Unix() > exp {
			c.JSON(http.StatusOK, dto.Fail[string]("验证码已过期"))
			return
		}
		if strings.ToLower(user.Captcha) != strings.ToLower(fmt.Sprintf("%v", sc)) {
			c.JSON(http.StatusOK, dto.Fail[string]("验证码不正确"))
			return
		}
	}

	if err := services.Register(user); err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](err.Error()))
		return
	}

	c.JSON(http.StatusOK, dto.OK[any](nil, models.RegisterSuccessMessage))
}

func GetCaptcha(c *gin.Context) {
	letters := []rune("ABCDEFGHJKLMNPQRSTUVWXYZ23456789")
	b := make([]rune, 5)
	for i := range b {
		b[i] = letters[int(time.Now().UnixNano()+int64(i))%len(letters)]
	}
	code := string(b)
	capID := newCaptchaID()
	exp := time.Now().Add(2 * time.Minute).Unix()
	setCaptcha(capID, code, exp)

	svg := fmt.Sprintf("<svg xmlns='http://www.w3.org/2000/svg' width='96' height='40'><rect width='100%%' height='100%%' fill='#0f172a'/><text x='50%%' y='50%%' dominant-baseline='middle' text-anchor='middle' font-family='monospace' font-size='20' fill='#ffffff'>%s</text></svg>", code)
	// 新增：json=1 返回 captcha_id + svg（不依赖 cookie，适配移动端）
	if strings.TrimSpace(c.Query("json")) == "1" {
		c.JSON(http.StatusOK, dto.OK(gin.H{"captcha_id": capID, "svg": svg, "expires_in": 120}, "ok"))
		return
	}

	session := sessions.Default(c)
	session.Set("captcha_code", code)
	session.Set("captcha_exp", exp)
	session.Save()
	c.Data(http.StatusOK, "image/svg+xml", []byte(svg))
}

// GetMessages 处理 GET /messages 请求，返回所有留言
func GetMessages(c *gin.Context) {
	showPrivate := false
	userID, exists := c.Get("user_id")
	if exists {
		user, err := services.GetUserByID(userID.(uint))
		if err == nil && user.IsAdmin {
			showPrivate = true
		}
	}

	messages, err := services.GetAllMessages(showPrivate)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](models.GetAllMessagesFailMessage))
		return
	}
	c.JSON(http.StatusOK, dto.OK(messages, models.GetAllMessagesSuccess))
}

// GetMessage 处理 GET /messages/:id 请求，获取留言详情
func GetMessage(c *gin.Context) {
	// 从 URL 参数获取留言 ID
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](models.InvalidIDMessage))
		return
	}

	// 检查是否显示私密消息
	showPrivate := false
	userID, exists := c.Get("user_id")
	if exists {
		user, err := services.GetUserByID(userID.(uint))
		if err == nil && user.IsAdmin {
			showPrivate = true
		}
	}

	// 调用 Service 层根据 ID 获取留言
	message, err := services.GetMessageByID(uint(id), showPrivate)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](models.GetMessageByIDFailMessage))
		return
	}

	if message == nil {
		c.JSON(http.StatusOK, dto.Fail[string](models.MessageNotFoundMessage))
		return
	}

	// 返回成功响应
	c.JSON(http.StatusOK, dto.OK(message, models.GetMessageByIDSuccess))
}

func GetMessagesByPage(c *gin.Context) {
	var page, pageSize int = 1, 10

	// 尝试从 POST JSON 数据获取分页参数
	var pageRequest dto.PageQueryDto
	if err := c.ShouldBindJSON(&pageRequest); err == nil {
		page = pageRequest.Page
		pageSize = pageRequest.PageSize
	} else {
		// 如果不是 POST JSON，则尝试从 URL 查询参数获取
		if pageStr := c.Query("page"); pageStr != "" {
			if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
				page = p
			}
		}
		if sizeStr := c.Query("pageSize"); sizeStr != "" {
			if s, err := strconv.Atoi(sizeStr); err == nil && s > 0 && s <= 100 {
				pageSize = s
			}
		}
	}

	// 验证分页参数
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}

	// 检查权限并传递用户上下文
	var currentUserID *uint
	isAdmin := false
	// 优先从上下文获取（由中间件设置）
	if uid, exists := c.Get("user_id"); exists {
		u, err := services.GetUserByID(uid.(uint))
		if err == nil {
			id := u.ID
			currentUserID = &id
			isAdmin = u.IsAdmin
		}
	} else {
		// 兼容未使用鉴权中间件的场景：从 session 获取
		session := sessions.Default(c)
		if v := session.Get("user_id"); v != nil {
			switch val := v.(type) {
			case uint:
				id := val
				currentUserID = &id
			case int:
				id := uint(val)
				currentUserID = &id
			case int64:
				id := uint(val)
				currentUserID = &id
			case float64:
				id := uint(val)
				currentUserID = &id
			case string:
				if parsed, err := strconv.ParseUint(val, 10, 64); err == nil {
					id := uint(parsed)
					currentUserID = &id
				}
			}
		}
		if v := session.Get("is_admin"); v != nil {
			switch val := v.(type) {
			case bool:
				isAdmin = val
			case int:
				isAdmin = val != 0
			case int64:
				isAdmin = val != 0
			case float64:
				isAdmin = val != 0
			case string:
				isAdmin = val == "true" || val == "1"
			}
		}
		// 如果仅拿到 user_id，则再查一次用户，确保 isAdmin
		if currentUserID != nil && !isAdmin {
			u, err := services.GetUserByID(*currentUserID)
			if err == nil {
				isAdmin = u.IsAdmin
			}
		}
	}

	// 作者筛选（可选）
	var authorID *uint
	if aid := c.Query("authorId"); aid != "" {
		if v, err := strconv.ParseUint(aid, 10, 64); err == nil {
			vv := uint(v)
			authorID = &vv
		}
	}
	var username *string
	if un := c.Query("username"); strings.TrimSpace(un) != "" {
		u := strings.TrimSpace(un)
		username = &u
	}

	pageQueryResult, err := services.GetMessagesByPage(page, pageSize, currentUserID, isAdmin, authorID, username)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](err.Error()))
		return
	}

	c.JSON(http.StatusOK, dto.OK(pageQueryResult, models.GetMessagesByPageSuccess))
}
func GetStatus(c *gin.Context) {
	status, err := services.GetStatus()
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](models.GetStatusFailMessage))
		return
	}

	c.JSON(http.StatusOK, dto.OK(status, models.GetStatusSuccessMessage))
}

func GetUserProfile(c *gin.Context) {
	username := strings.TrimSpace(c.Query("username"))
	idStr := strings.TrimSpace(c.Query("id"))
	var user *models.User
	var err error
	if username != "" {
		user, err = services.GetUserByUsername(username)
		if err != nil || user == nil {
			c.JSON(http.StatusOK, dto.Fail[string](models.UserNotFoundMessage))
			return
		}
	} else if idStr != "" {
		uid, parseErr := strconv.ParseUint(idStr, 10, 64)
		if parseErr != nil {
			c.JSON(http.StatusOK, dto.Fail[string](models.InvalidIDMessage))
			return
		}
		user, err = services.GetUserByID(uint(uid))
		if err != nil || user == nil {
			c.JSON(http.StatusOK, dto.Fail[string](models.UserNotFoundMessage))
			return
		}
	} else {
		c.JSON(http.StatusOK, dto.Fail[string](models.InvalidRequestBodyMessage))
		return
	}
	var total int64
	if err := database.DB.Model(&models.Message{}).
		Where("user_id = ?", user.ID).
		Where("private = ?", false).
		Where("content NOT LIKE ? AND content NOT LIKE ? AND content NOT LIKE ? AND content NOT LIKE ? AND content NOT LIKE ? AND content NOT LIKE ? AND content NOT LIKE ?",
			"%#guestbook%", "%#留言%", "%留言板%",
			"%#友链%", "%友情链接%",
			"%#关于%", "%关于本站%").
		Count(&total).Error; err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](models.GetAllMessagesFailMessage))
		return
	}
	c.JSON(http.StatusOK, dto.OK(map[string]interface{}{
		"id":             user.ID,
		"username":       user.Username,
		"avatar_url":     strings.TrimSpace(user.AvatarURL),
		"description":    strings.TrimSpace(user.Description),
		"total_messages": int(total),
	}, "获取用户资料成功"))
}

func DeleteMessage(c *gin.Context) {
	id := c.Param("id")
	messageID, err := strconv.ParseUint(id, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 0, "msg": "无效的消息ID"})
		return
	}

	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusOK, dto.Fail[string]("未授权访问"))
		return
	}

	user, err := services.GetUserByID(userID.(uint))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 0, "msg": err.Error()})
		return
	}

	if !user.IsAdmin {
		if err := services.DeleteMessage(uint(messageID), userID.(uint)); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 0, "msg": err.Error()})
			return
		}
	} else {
		if err := services.DeleteMessageByAdmin(uint(messageID)); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 0, "msg": err.Error()})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "删除成功"})
}

func GenerateRSS(c *gin.Context) {
	atom, err := services.GenerateRSS(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.Fail[string](models.GenerateRSSFailMessage))
		return
	}

	c.Data(http.StatusOK, "application/rss+xml; charset=utf-8", []byte(atom))
}

func UpdateUser(c *gin.Context) {
	user, err := checkUser(c)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](err.Error()))
		return
	}

	var userdto dto.UserInfoDto
	if err := c.ShouldBindJSON(&userdto); err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](models.InvalidRequestBodyMessage))
		return
	}

	if err := services.UpdateUser(user, userdto); err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](err.Error()))
		return
	}

	c.JSON(http.StatusOK, dto.OK[any](nil, models.UpdateUserSuccessMessage))
}

func ChangePassword(c *gin.Context) {
	var req struct {
		OldPassword string `json:"oldPassword"`
		Password    string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](models.InvalidRequestBodyMessage))
		return
	}

	user, err := checkUser(c)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](err.Error()))
		return
	}

	if err := services.ChangePasswordWithOld(user, req.OldPassword, req.Password); err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](err.Error()))
		return
	}

	c.JSON(http.StatusOK, dto.OK[any](nil, models.ChangePasswordSuccessMessage))
}

// checkAdmin 函数需要重新添加
func checkAdmin(c *gin.Context) (uint, error) {
	userID, exists := c.Get("user_id")
	if !exists {
		return 0, fmt.Errorf("未授权访问")
	}
	user, err := services.GetUserByID(userID.(uint))
	if err != nil {
		return 0, err
	}
	if !user.IsAdmin {
		return 0, fmt.Errorf("需要管理员权限")
	}
	return userID.(uint), nil
}

func UpdateUserAdmin(c *gin.Context) {
	_, err := checkAdmin(c)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](err.Error()))
		return
	}

	idStr := c.Query("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil || id == 1 {
		c.JSON(http.StatusOK, dto.Fail[string](models.InvalidIDMessage))
		return
	}

	currentID, err := checkAdmin(c)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](err.Error()))
		return
	}

	if err := services.UpdateUserAdmin(uint(id), currentID); err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](err.Error()))
		return
	}

	c.JSON(http.StatusOK, dto.OK[any](nil, models.UpdateUserSuccessMessage))
}

func GetUserInfo(c *gin.Context) {
	user, err := checkUser(c)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](err.Error()))
		return
	}

	// 返回副本，避免污染缓存中的用户对象。
	safeUser := *user
	safeUser.Password = ""
	c.JSON(http.StatusOK, dto.OK(safeUser, models.QuerySuccessMessage))
}

func UpdateSetting(c *gin.Context) {
	_, err := checkAdmin(c)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](err.Error()))
		return
	}

	db, _ := database.GetDB()
	var oldSetting models.Setting
	if err := db.Table("settings").First(&oldSetting).Error; err != nil {
		c.JSON(http.StatusOK, dto.Fail[string]("读取原有配置失败"))
		return
	}

	var setting dto.SettingDto
	if err := c.ShouldBindJSON(&setting); err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](models.InvalidRequestBodyMessage))
		return
	}

	if setting.AllowRegistration != nil {
		oldSetting.AllowRegistration = *setting.AllowRegistration
	}

	frontendSettings := setting.FrontendSettings
	settingMap := map[string]interface{}{}
	hasSiteConfigUpdate := false
	if frontendSettings != nil {
		settingMap["frontendSettings"] = frontendSettings
		hasSiteConfigUpdate = true
	}
	if setting.AllowRegistration != nil {
		settingMap["allowRegistration"] = *setting.AllowRegistration
	}
	if setting.SmtpEnabled != nil {
		settingMap["smtpEnabled"] = *setting.SmtpEnabled
		hasSiteConfigUpdate = true
	}
	if setting.SmtpDriver != nil {
		settingMap["smtpDriver"] = *setting.SmtpDriver
		hasSiteConfigUpdate = true
	}
	if setting.SmtpHost != nil {
		settingMap["smtpHost"] = *setting.SmtpHost
		hasSiteConfigUpdate = true
	}
	if setting.SmtpPort != nil {
		settingMap["smtpPort"] = *setting.SmtpPort
		hasSiteConfigUpdate = true
	}
	if setting.SmtpUser != nil {
		settingMap["smtpUser"] = *setting.SmtpUser
		hasSiteConfigUpdate = true
	}
	if setting.SmtpPass != nil {
		settingMap["smtpPass"] = *setting.SmtpPass
		hasSiteConfigUpdate = true
	}
	if setting.SmtpFrom != nil {
		settingMap["smtpFrom"] = *setting.SmtpFrom
		hasSiteConfigUpdate = true
	}
	if setting.SmtpEncryption != nil {
		settingMap["smtpEncryption"] = *setting.SmtpEncryption
		hasSiteConfigUpdate = true
	}
	if setting.SmtpTLS != nil {
		settingMap["smtpTLS"] = *setting.SmtpTLS
		hasSiteConfigUpdate = true
	}

	if setting.StorageEnabled != nil {
		settingMap["storageEnabled"] = *setting.StorageEnabled
		hasSiteConfigUpdate = true
	}
	if setting.StorageConfig != nil {
		settingMap["storageConfig"] = setting.StorageConfig
		hasSiteConfigUpdate = true
	}

	if setting.AttachmentStorageEnabled != nil {
		settingMap["attachmentStorageEnabled"] = *setting.AttachmentStorageEnabled
		hasSiteConfigUpdate = true
	}
	if setting.AttachmentStorageConfig != nil {
		settingMap["attachmentStorageConfig"] = setting.AttachmentStorageConfig
		hasSiteConfigUpdate = true
	}

	if hasSiteConfigUpdate {
		if err := services.UpdateFrontendSetting(0, settingMap); err != nil {
			c.JSON(http.StatusOK, dto.Fail[string]("保存前端配置失败: "+err.Error()))
			return
		}
	}

	if err := db.Table("settings").Save(&oldSetting).Error; err != nil {
		c.JSON(http.StatusOK, dto.Fail[string]("保存配置失败"))
		return
	}

	c.JSON(http.StatusOK, dto.OK[any](nil, models.UpdateSettingSuccessMessage))
}

func GetFrontendConfig(c *gin.Context) {
	config, err := services.GetFrontendConfig()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "获取配置失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 1, "data": config})
}

// SubmitFriendLinkApply 提交友链申请（公开）
func SubmitFriendLinkApply(c *gin.Context) {
	var req struct {
		Title       string `json:"title"`
		Link        string `json:"link"`
		Icon        string `json:"icon"`
		Description string `json:"description"`
		Email       string `json:"email"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, dto.Fail[string]("请求参数错误"))
		return
	}
	req.Link = strings.TrimSpace(req.Link)
	req.Title = strings.TrimSpace(req.Title)
	req.Icon = strings.TrimSpace(req.Icon)
	req.Description = strings.TrimSpace(req.Description)
	req.Email = strings.TrimSpace(req.Email)
	if req.Link == "" {
		c.JSON(http.StatusOK, dto.Fail[string]("网址不能为空"))
		return
	}
	db, _ := database.GetDB()
	apply := models.FriendLinkApply{Title: req.Title, Link: req.Link, Icon: req.Icon, Description: req.Description, Email: req.Email, Status: "pending"}
	if err := db.Create(&apply).Error; err != nil {
		c.JSON(http.StatusOK, dto.Fail[string]("提交失败"))
		return
	}
	var cfg models.SiteConfig
	_ = db.Table("site_configs").First(&cfg).Error
	if cfg.SmtpEnabled && cfg.FriendLinkEmailEnabled {
		to := strings.TrimSpace(cfg.SmtpFrom)
		if to == "" {
			to = strings.TrimSpace(cfg.SmtpUser)
		}
		if to != "" {
			subject := fmt.Sprintf("新的友链申请 - %s", cfg.SiteTitle)
			body := fmt.Sprintf("站点：%s\n标题：%s\n网址：%s\n邮箱：%s\n说明：%s", cfg.SiteTitle, apply.Title, apply.Link, apply.Email, apply.Description)
			_ = models.SendEmail(to, subject, body)
		}
	}
	c.JSON(http.StatusOK, dto.OK(apply, "已提交，待审核"))
}

func ResolveDouyinShortURL(c *gin.Context) {
	vid := strings.TrimSpace(c.Query("vid"))
	raw := strings.TrimSpace(c.Query("url"))
	if vid == "" && raw == "" {
		c.JSON(http.StatusOK, dto.Fail[string]("url 或 vid 不能为空"))
		return
	}

	resolvedURL := ""
	var err error
	if vid == "" {
		vid, resolvedURL, err = resolveDouyinVideoIDFromURL(raw)
		if err != nil {
			c.JSON(http.StatusOK, dto.Fail[string](err.Error()))
			return
		}
	}
	if vid == "" {
		c.JSON(http.StatusOK, dto.Fail[string]("未提取到视频ID"))
		return
	}

	playURL, playErr := fetchDouyinPlayURLByID(vid)
	resp := gin.H{
		"video_id":     vid,
		"resolved_url": resolvedURL,
	}
	if playErr == nil && strings.TrimSpace(playURL) != "" {
		resp["play_url"] = playURL
	}
	c.JSON(http.StatusOK, dto.OK(resp, "解析成功"))
}

// ProxyDouyinVideo 由后端中转抖音视频流，避免前端直连被防盗链策略拦截
func ProxyDouyinVideo(c *gin.Context) {
	vid := strings.TrimSpace(c.Query("vid"))
	raw := strings.TrimSpace(c.Query("url"))
	if vid == "" {
		if raw == "" {
			c.String(http.StatusBadRequest, "url 或 vid 不能为空")
			return
		}
		var err error
		vid, _, err = resolveDouyinVideoIDFromURL(raw)
		if err != nil {
			c.String(http.StatusBadRequest, err.Error())
			return
		}
	}
	if vid == "" {
		c.String(http.StatusBadRequest, "未提取到视频ID")
		return
	}
	playURL, err := fetchDouyinPlayURLByID(vid)
	if err != nil || strings.TrimSpace(playURL) == "" {
		c.String(http.StatusBadGateway, "获取抖音视频地址失败")
		return
	}

	req, _ := http.NewRequest(http.MethodGet, playURL, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (iPhone; CPU iPhone OS 16_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Mobile")
	req.Header.Set("Referer", "https://www.douyin.com/")
	req.Header.Set("Accept", "*/*")
	if r := strings.TrimSpace(c.GetHeader("Range")); r != "" {
		req.Header.Set("Range", r)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		c.String(http.StatusBadGateway, "拉取抖音视频流失败")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		c.String(http.StatusBadGateway, "抖音视频源不可用")
		return
	}

	if v := strings.TrimSpace(resp.Header.Get("Content-Type")); v != "" {
		c.Header("Content-Type", v)
	} else {
		c.Header("Content-Type", "video/mp4")
	}
	for _, h := range []string{
		"Content-Length",
		"Content-Range",
		"Accept-Ranges",
		"Cache-Control",
		"Expires",
		"Last-Modified",
		"ETag",
	} {
		if v := strings.TrimSpace(resp.Header.Get(h)); v != "" {
			c.Header(h, v)
		}
	}

	c.Status(resp.StatusCode)
	if _, err := io.Copy(c.Writer, resp.Body); err != nil {
		c.AbortWithStatus(http.StatusBadGateway)
		return
	}
}

func resolveDouyinVideoIDFromURL(raw string) (string, string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", fmt.Errorf("url 不能为空")
	}
	if regexp.MustCompile(`^\d+$`).MatchString(raw) {
		return raw, "", nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", "", fmt.Errorf("url 格式错误")
	}
	host := strings.ToLower(strings.TrimSpace(u.Host))
	if !strings.Contains(host, "douyin.com") && !strings.Contains(host, "iesdouyin.com") {
		return "", "", fmt.Errorf("仅支持抖音链接")
	}
	client := &http.Client{Timeout: 8 * time.Second}
	req, _ := http.NewRequest(http.MethodGet, raw, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0")
	resp, err := client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("短链解析失败")
	}
	defer resp.Body.Close()
	finalURL := ""
	if resp.Request != nil && resp.Request.URL != nil {
		finalURL = resp.Request.URL.String()
	}
	target := finalURL
	if target == "" {
		target = raw
	}
	rePath := regexp.MustCompile(`/video/(\d+)`)
	if m := rePath.FindStringSubmatch(target); len(m) > 1 {
		return strings.TrimSpace(m[1]), target, nil
	}
	if pu, e := url.Parse(target); e == nil {
		if id := strings.TrimSpace(pu.Query().Get("modal_id")); id != "" {
			return id, target, nil
		}
	}
	return "", target, fmt.Errorf("未提取到视频ID")
}

func fetchDouyinPlayURLByID(videoID string) (string, error) {
	videoID = strings.TrimSpace(videoID)
	if videoID == "" {
		return "", fmt.Errorf("video_id 不能为空")
	}
	apiURL := fmt.Sprintf("https://www.iesdouyin.com/web/api/v2/aweme/iteminfo/?item_ids=%s", url.QueryEscape(videoID))
	client := &http.Client{Timeout: 10 * time.Second}
	req, _ := http.NewRequest(http.MethodGet, apiURL, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("请求抖音视频信息失败")
	}

	var data map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", err
	}
	itemList, _ := data["item_list"].([]interface{})
	if len(itemList) == 0 {
		return "", fmt.Errorf("未获取到抖音视频信息")
	}
	item, _ := itemList[0].(map[string]interface{})
	video, _ := item["video"].(map[string]interface{})
	playAddr, _ := video["play_addr"].(map[string]interface{})
	urlList, _ := playAddr["url_list"].([]interface{})
	for _, it := range urlList {
		u, _ := it.(string)
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}
		// 优先去水印地址
		u = strings.ReplaceAll(u, "playwm", "play")
		return u, nil
	}
	return "", fmt.Errorf("未获取到可播放地址")
}

// ListFriendLinkApplications 管理员查看友链申请列表
func ListFriendLinkApplications(c *gin.Context) {
	_, err := checkAdmin(c)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](err.Error()))
		return
	}
	db, _ := database.GetDB()
	var list []models.FriendLinkApply
	q := strings.TrimSpace(c.Query("q"))
	tx := db.Order("created_at DESC")
	if q != "" {
		tx = tx.Where("title LIKE ? OR link LIKE ?", "%"+q+"%", "%"+q+"%")
	}
	if err := tx.Find(&list).Error; err != nil {
		c.JSON(http.StatusOK, dto.Fail[string]("查询失败"))
		return
	}
	c.JSON(http.StatusOK, dto.OK(list, models.QuerySuccessMessage))
}

// DeleteFriendLinkApplication 管理员删除单条友链申请记录
func DeleteFriendLinkApplication(c *gin.Context) {
	_, err := checkAdmin(c)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](err.Error()))
		return
	}
	idStr := c.Param("id")
	id64, e := strconv.ParseUint(idStr, 10, 64)
	if e != nil || id64 == 0 {
		c.JSON(http.StatusOK, dto.Fail[string](models.InvalidIDMessage))
		return
	}
	db, _ := database.GetDB()
	if err := db.Delete(&models.FriendLinkApply{}, uint(id64)).Error; err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](models.DatabaseErrorMessage))
		return
	}
	c.JSON(http.StatusOK, dto.OK[any](nil, "记录已删除"))
}

// ClearFriendLinkApplications 管理员清空友链申请记录
func ClearFriendLinkApplications(c *gin.Context) {
	_, err := checkAdmin(c)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](err.Error()))
		return
	}
	db, _ := database.GetDB()
	if err := db.Where("1 = 1").Delete(&models.FriendLinkApply{}).Error; err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](models.DatabaseErrorMessage))
		return
	}
	c.JSON(http.StatusOK, dto.OK[any](nil, "申请记录已清空"))
}

// AuditFriendLink 审核友链（通过/拒绝）
func AuditFriendLink(c *gin.Context) {
	_, err := checkAdmin(c)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](err.Error()))
		return
	}
	idStr := c.Param("id")
	id64, e := strconv.ParseUint(idStr, 10, 64)
	if e != nil || id64 == 0 {
		c.JSON(http.StatusOK, dto.Fail[string](models.InvalidIDMessage))
		return
	}
	var req struct {
		Approve  bool   `json:"approve"`
		Feedback string `json:"feedback"`
	}
	if e := c.ShouldBindJSON(&req); e != nil {
		c.JSON(http.StatusOK, dto.Fail[string](models.InvalidRequestBodyMessage))
		return
	}
	db, _ := database.GetDB()
	var apply models.FriendLinkApply
	if err := db.First(&apply, uint(id64)).Error; err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](models.RecordNotFoundMessage))
		return
	}
	var cfg models.SiteConfig
	_ = db.Table("site_configs").First(&cfg).Error
	if req.Approve {
		apply.Status = "approved"
		apply.Feedback = strings.TrimSpace(req.Feedback)
		if err := db.Save(&apply).Error; err != nil {
			c.JSON(http.StatusOK, dto.Fail[string](models.DatabaseErrorMessage))
			return
		}
		link := models.FriendLink{Title: apply.Title, Link: apply.Link, Icon: apply.Icon, Description: apply.Description, Email: apply.Email}
		_ = db.Where("link = ?", link.Link).Delete(&models.FriendLink{})
		if err := db.Create(&link).Error; err != nil {
			c.JSON(http.StatusOK, dto.Fail[string](models.DatabaseErrorMessage))
			return
		}
		if cfg.SmtpEnabled && cfg.FriendLinkEmailEnabled && strings.TrimSpace(apply.Email) != "" {
			subject := fmt.Sprintf("友链申请通过 - %s", cfg.SiteTitle)
			body := fmt.Sprintf("你的友链申请已通过：%s\n%s", apply.Link, strings.TrimSpace(req.Feedback))
			_ = models.SendEmail(strings.TrimSpace(apply.Email), subject, body)
		}
		c.JSON(http.StatusOK, dto.OK(link, "已通过"))
		return
	}
	apply.Status = "rejected"
	apply.Feedback = strings.TrimSpace(req.Feedback)
	if err := db.Save(&apply).Error; err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](models.DatabaseErrorMessage))
		return
	}
	if cfg.SmtpEnabled && cfg.FriendLinkEmailEnabled && strings.TrimSpace(apply.Email) != "" {
		subject := fmt.Sprintf("友链申请未通过 - %s", cfg.SiteTitle)
		body := fmt.Sprintf("很抱歉，你的友链申请未通过。原因：%s", strings.TrimSpace(req.Feedback))
		_ = models.SendEmail(strings.TrimSpace(apply.Email), subject, body)
	}
	c.JSON(http.StatusOK, dto.OK(apply, "已拒绝"))
}

// 获取指定消息的评论列表（内置评论系统）
func GetComments(c *gin.Context) {
	idStr := c.Param("id")
	msgID, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil || msgID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 0, "msg": "无效的消息ID"})
		return
	}
	db, _ := database.GetDB()
	var comments []models.Comment
	if err := db.Where("message_id = ?", msgID).Order("created_at ASC").Find(&comments).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 0, "msg": "获取评论失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 1, "data": comments})
}

// 批量获取评论数量
func GetCommentCounts(c *gin.Context) {
	var req struct {
		IDs []uint `json:"ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 0, "msg": "请求参数错误"})
		return
	}
	db, _ := database.GetDB()
	type row struct {
		MessageID uint  `gorm:"column:message_id"`
		Cnt       int64 `gorm:"column:cnt"`
	}
	var rows []row
	if err := db.Model(&models.Comment{}).
		Select("message_id, COUNT(*) as cnt").
		Where("message_id IN ?", req.IDs).
		Group("message_id").
		Find(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 0, "msg": "获取评论数量失败"})
		return
	}
	// 组装为 id->count 的数组结构
	result := make([]gin.H, 0, len(rows))
	for _, r := range rows {
		result = append(result, gin.H{"id": r.MessageID, "count": r.Cnt})
	}
	c.JSON(http.StatusOK, gin.H{"code": 1, "data": result})
}

// GetGuestbookMessageID 获取或创建用于留言板的独立消息ID
func GetGuestbookMessageID(c *gin.Context) {
	db, _ := database.GetDB()
	var msg models.Message
	if err := db.Where("private = ?", false).
		Where("content LIKE ? OR content LIKE ? OR content LIKE ?",
			"%#留言%", "%#guestbook%", "%留言板%").
		Order("created_at ASC").First(&msg).Error; err == nil && msg.ID != 0 {
		c.JSON(http.StatusOK, gin.H{"code": 1, "data": gin.H{"id": msg.ID}})
		return
	}
	var admin models.User
	_ = db.Where("is_admin = ?", true).Order("id ASC").First(&admin).Error
	uid := uint(1)
	if admin.ID != 0 {
		uid = admin.ID
	}
	content := "留言板\n\n此条用于承载全站留言，不会参与普通内容展示。\n\n#留言 #guestbook"
	var existing models.Message
	if err := db.Where("user_id = ? AND content LIKE ?", uid, "%#guestbook%").First(&existing).Error; err == nil && existing.ID != 0 {
		c.JSON(http.StatusOK, gin.H{"code": 1, "data": gin.H{"id": existing.ID}})
		return
	}
	msg = models.Message{Content: content, UserID: uid, Private: false, Pinned: false}
	if err := db.Create(&msg).Error; err != nil || msg.ID == 0 {
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "初始化留言板失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 1, "data": gin.H{"id": msg.ID}})
}

// 提交评论（内置评论系统）
func PostComment(c *gin.Context) {
	idStr := c.Param("id")
	msgID64, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil || msgID64 == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 0, "msg": "无效的消息ID"})
		return
	}
	msgID := uint(msgID64)
	var req struct {
		Nick     string `json:"nick"`
		Mail     string `json:"mail"`
		Link     string `json:"link"`
		Content  string `json:"content"`
		ParentID *uint  `json:"parent_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 0, "msg": "请求参数错误"})
		return
	}
	req.Content = strings.TrimSpace(req.Content)
	if req.Content == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 0, "msg": "评论内容不能为空"})
		return
	}
	db, _ := database.GetDB()
	// 校验消息存在
	var message models.Message
	if err := db.First(&message, msgID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 0, "msg": "消息不存在"})
		return
	}
	// 读取站点配置并根据需要校验登录状态
	var cfg models.SiteConfig
	_ = db.Table("site_configs").First(&cfg).Error
	if cfg.CommentLoginRequired {
		if _, ok := pkg.GetUserSession(c); !ok {
			c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "请登录后评论"})
			return
		}
	}
	comment := models.Comment{
		MessageID: msgID,
		Nick:      strings.TrimSpace(req.Nick),
		Mail:      strings.TrimSpace(req.Mail),
		Link:      strings.TrimSpace(req.Link),
		Content:   req.Content,
		ParentID:  req.ParentID,
	}
	if err := db.Create(&comment).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 0, "msg": "保存评论失败"})
		return
	}
	// 邮件通知
	if cfg.SmtpEnabled && cfg.CommentEmailEnabled {
		siteURL := strings.TrimSpace(cfg.CommentEmailSiteURL)
		if siteURL == "" || !(strings.HasPrefix(siteURL, "http://") || strings.HasPrefix(siteURL, "https://")) {
			scheme := c.Request.Header.Get("X-Forwarded-Proto")
			if scheme == "" {
				scheme = "http"
			}
			host := c.Request.Host
			siteURL = fmt.Sprintf("%s://%s", scheme, host)
		}
		if comment.ParentID == nil || cfg.CommentEmailAdminNotifyAll {
			adminTo := cfg.SmtpFrom
			if adminTo == "" {
				adminTo = cfg.SmtpUser
			}
			prefixAdmin := strings.TrimSpace(cfg.CommentEmailAdminPrefix)
			if prefixAdmin != "" {
				prefixAdmin = prefixAdmin + " "
			}
			subject := fmt.Sprintf("%s新评论通知 - %s", prefixAdmin, cfg.SiteTitle)
			textBody := fmt.Sprintf("站点：%s\n用户：%s\n邮箱：%s\n网址：%s\n内容：\n%s\n\n查看：%s/m/%d", cfg.SiteTitle, comment.Nick, comment.Mail, comment.Link, comment.Content, siteURL, message.ID)
			if strings.TrimSpace(cfg.CommentEmailAdminTemplate) != "" {
				tpl := cfg.CommentEmailAdminTemplate
				tpl = strings.ReplaceAll(tpl, "{site}", cfg.SiteTitle)
				tpl = strings.ReplaceAll(tpl, "{nick}", comment.Nick)
				tpl = strings.ReplaceAll(tpl, "{mail}", comment.Mail)
				tpl = strings.ReplaceAll(tpl, "{link}", comment.Link)
				tpl = strings.ReplaceAll(tpl, "{content}", comment.Content)
				tpl = strings.ReplaceAll(tpl, "{url}", fmt.Sprintf("%s/m/%d", siteURL, message.ID))
				textBody = tpl
			}
			htmlTpl := strings.TrimSpace(cfg.CommentEmailAdminTemplateHTML)
			if htmlTpl != "" {
				htmlTpl = strings.ReplaceAll(htmlTpl, "{site}", cfg.SiteTitle)
				htmlTpl = strings.ReplaceAll(htmlTpl, "{nick}", comment.Nick)
				htmlTpl = strings.ReplaceAll(htmlTpl, "{mail}", comment.Mail)
				htmlTpl = strings.ReplaceAll(htmlTpl, "{link}", comment.Link)
				htmlTpl = strings.ReplaceAll(htmlTpl, "{content}", comment.Content)
				htmlTpl = strings.ReplaceAll(htmlTpl, "{url}", fmt.Sprintf("%s/m/%d", siteURL, message.ID))
				_ = models.SendEmailHTML(adminTo, subject, htmlTpl)
			} else {
				_ = models.SendEmail(adminTo, subject, textBody)
			}
		}
		// 回复通知
		if comment.ParentID != nil {
			var parent models.Comment
			parentMail := ""
			if err := db.First(&parent, *comment.ParentID).Error; err == nil {
				parentMail = strings.TrimSpace(parent.Mail)
			}
			if parentMail != "" && strings.TrimSpace(comment.Mail) != parentMail {
				prefixReply := strings.TrimSpace(cfg.CommentEmailReplyPrefix)
				if prefixReply != "" {
					prefixReply = prefixReply + " "
				}
				replySubject := fmt.Sprintf("%s你的评论有新回复 - %s", prefixReply, cfg.SiteTitle)
				textTpl := fmt.Sprintf("用户 %s 回复了你的评论：\n\n原评论：%s\n回复内容：%s\n\n查看：%s/m/%d", comment.Nick, parent.Content, comment.Content, siteURL, message.ID)
				if strings.TrimSpace(cfg.CommentEmailReplyTemplate) != "" {
					tpl := cfg.CommentEmailReplyTemplate
					tpl = strings.ReplaceAll(tpl, "{site}", cfg.SiteTitle)
					tpl = strings.ReplaceAll(tpl, "{nick}", comment.Nick)
					tpl = strings.ReplaceAll(tpl, "{content}", comment.Content)
					tpl = strings.ReplaceAll(tpl, "{url}", fmt.Sprintf("%s/m/%d", siteURL, message.ID))
					textTpl = tpl
				}
				htmlTpl := strings.TrimSpace(cfg.CommentEmailReplyTemplateHTML)
				if htmlTpl != "" {
					htmlTpl = strings.ReplaceAll(htmlTpl, "{site}", cfg.SiteTitle)
					htmlTpl = strings.ReplaceAll(htmlTpl, "{nick}", comment.Nick)
					htmlTpl = strings.ReplaceAll(htmlTpl, "{content}", comment.Content)
					htmlTpl = strings.ReplaceAll(htmlTpl, "{url}", fmt.Sprintf("%s/m/%d", siteURL, message.ID))
					_ = models.SendEmailHTMLWithFrom(parentMail, replySubject, htmlTpl, strings.TrimSpace(cfg.CommentEmailReplyName))
				} else {
					_ = models.SendEmailWithFrom(parentMail, replySubject, textTpl, strings.TrimSpace(cfg.CommentEmailReplyName))
				}
			}
		}
	}
	c.JSON(http.StatusOK, gin.H{"code": 1, "data": comment, "msg": "评论已发布"})
}

// 删除评论（管理员）
func DeleteComment(c *gin.Context) {
	msgIDStr := c.Param("id")
	cidStr := c.Param("cid")
	msgID, err1 := strconv.ParseUint(msgIDStr, 10, 64)
	cid, err2 := strconv.ParseUint(cidStr, 10, 64)
	if err1 != nil || err2 != nil || msgID == 0 || cid == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 0, "msg": "无效的ID"})
		return
	}
	isAdmin, _ := c.Get("is_admin")
	if b, ok := isAdmin.(bool); !ok || !b {
		c.JSON(http.StatusForbidden, gin.H{"code": 0, "msg": "无权限"})
		return
	}
	db, _ := database.GetDB()
	// 确认评论属于该消息
	var cm models.Comment
	if err := db.First(&cm, cid).Error; err != nil || cm.MessageID != uint(msgID) {
		c.JSON(http.StatusNotFound, gin.H{"code": 0, "msg": "评论不存在"})
		return
	}
	if err := db.Delete(&cm).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 0, "msg": "删除失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "已删除"})
}

func BackfillCommentParents(c *gin.Context) {
	isAdmin, _ := c.Get("is_admin")
	if b, ok := isAdmin.(bool); !ok || !b {
		c.JSON(http.StatusForbidden, gin.H{"code": 0, "msg": "无权限"})
		return
	}
	db, _ := database.GetDB()
	var all []models.Comment
	if err := db.Order("message_id ASC").Order("created_at ASC").Find(&all).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 0, "msg": "查询失败"})
		return
	}
	type key struct{ mid uint }
	groups := map[uint][]models.Comment{}
	for _, cm := range all {
		groups[cm.MessageID] = append(groups[cm.MessageID], cm)
	}
	mention := regexp.MustCompile(`^@([^\s:：]+)`)
	updated := 0
	for mid, list := range groups {
		for i := range list {
			cmt := list[i]
			if cmt.ParentID != nil {
				continue
			}
			m := mention.FindStringSubmatch(strings.TrimSpace(cmt.Content))
			if len(m) < 2 {
				continue
			}
			nick := strings.TrimSpace(m[1])
			var candidates []models.Comment
			for j := range list {
				if list[j].ID == cmt.ID {
					continue
				}
				if strings.TrimSpace(list[j].Nick) == nick {
					candidates = append(candidates, list[j])
				}
			}
			if len(candidates) == 0 {
				continue
			}
			ct := cmt.CreatedAt
			var parent *models.Comment
			var earlier []models.Comment
			for _, cand := range candidates {
				if !cand.CreatedAt.After(ct) {
					earlier = append(earlier, cand)
				}
			}
			if len(earlier) > 0 {
				sort.Slice(earlier, func(a, b int) bool { return earlier[a].CreatedAt.After(earlier[b].CreatedAt) })
				p := earlier[0]
				parent = &p
			} else {
				sort.Slice(candidates, func(a, b int) bool { return candidates[a].CreatedAt.After(candidates[b].CreatedAt) })
				p := candidates[0]
				parent = &p
			}
			if parent != nil {
				pid := parent.ID
				if err := db.Model(&models.Comment{}).Where("id = ? AND message_id = ?", cmt.ID, mid).Update("parent_id", pid).Error; err == nil {
					updated++
				}
			}
		}
	}
	c.JSON(http.StatusOK, gin.H{"code": 1, "data": gin.H{"updated": updated}})
}

// 列出所有内置评论（管理员，支持搜索与分页）
func ListComments(c *gin.Context) {
	_, err := checkAdmin(c)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](err.Error()))
		return
	}
	q := strings.TrimSpace(c.Query("q"))
	page, _ := strconv.Atoi(c.Query("page"))
	if page <= 0 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(c.Query("pageSize"))
	if pageSize <= 0 || pageSize > 200 {
		pageSize = 30
	}
	db, _ := database.GetDB()
	tx := db.Model(&models.Comment{})
	if q != "" {
		like := "%" + q + "%"
		tx = tx.Where("nick LIKE ? OR content LIKE ? OR mail LIKE ? OR link LIKE ?", like, like, like, like)
	}
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		c.JSON(http.StatusOK, dto.Fail[string]("查询失败"))
		return
	}
	var list []models.Comment
	if err := tx.Order("created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&list).Error; err != nil {
		c.JSON(http.StatusOK, dto.Fail[string]("查询失败"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 1, "data": gin.H{"total": total, "items": list}})
}

// 动态生成 Web Manifest
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
	title := "说说笔记"
	description := ""
	// 站点默认图标使用 SVG
	siteIcon := "/favicon.svg"

	if pwaEnabled {
		if v, ok := fs["pwaTitle"].(string); ok && v != "" {
			title = v
		}
		if v, ok := fs["pwaDescription"].(string); ok {
			description = v
		}
	}
	if title == "说说笔记" {
		if v, ok := fs["siteTitle"].(string); ok && v != "" {
			title = v
		}
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
		"name":             title,
		"short_name":       title,
		"description":      description,
		"start_url":        "/",
		"display":          "standalone",
		"background_color": "#000000",
		"theme_color":      "#000000",
		"icons": []map[string]string{
			{"src": icon, "sizes": "any", "type": iconType},
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

func UpdateMessage(c *gin.Context) {
	// 获取消息ID
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 0, "msg": "消息ID不能为空"})
		return
	}

	// 检查用户权限
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 0, "msg": "未授权访问"})
		return
	}

	var req struct {
		Content *string `json:"content"`
		Private *bool   `json:"private"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 0, "msg": "请求参数错误"})
		return
	}

	// 检查消息是否存在并且属于当前用户
	messageID, err := strconv.ParseUint(id, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 0, "msg": "无效的消息ID"})
		return
	}

	// 获取用户信息
	user, err := services.GetUserByID(userID.(uint))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 0, "msg": "获取用户信息失败"})
		return
	}

	// 检查消息所有权或管理员权限
	message, err := services.GetMessageByID(uint(messageID), true)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 0, "msg": "消息不存在"})
		return
	}

	if !user.IsAdmin && message.UserID != userID.(uint) {
		c.JSON(http.StatusForbidden, gin.H{"code": 0, "msg": "无权限修改此消息"})
		return
	}

	updated, err := services.UpdateMessage(uint(messageID), req.Content, req.Private)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 0, "msg": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "更新成功", "data": updated})

	// 即时模式触发云同步（防抖）
	syncmanager.Trigger()
}

// 更新消息置顶状态
func UpdateMessagePinned(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 0, "msg": "消息ID不能为空"})
		return
	}

	// 身份校验
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 0, "msg": "未授权访问"})
		return
	}

	// 请求体
	var req struct {
		Pinned bool `json:"pinned"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 0, "msg": "请求参数错误"})
		return
	}

	// 解析ID
	messageID, err := strconv.ParseUint(id, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 0, "msg": "无效的消息ID"})
		return
	}

	// 获取用户信息
	user, err := services.GetUserByID(userID.(uint))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 0, "msg": "获取用户信息失败"})
		return
	}

	// 获取消息并校验权限
	message, err := services.GetMessageByID(uint(messageID), true)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 0, "msg": "消息不存在"})
		return
	}

	if !user.IsAdmin && message.UserID != userID.(uint) {
		c.JSON(http.StatusForbidden, gin.H{"code": 0, "msg": "无权限操作该消息"})
		return
	}

	// 更新置顶状态
	if err := services.UpdateMessagePinned(uint(messageID), req.Pinned); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "更新成功"})
}

// 点赞接口：POST /api/messages/:id/like （无需登录）
func IncrementMessageLike(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 0, "msg": "消息ID不能为空"})
		return
	}
	messageID, err := strconv.ParseUint(id, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 0, "msg": "无效的消息ID"})
		return
	}
	count, err := services.IncrementLikeCount(uint(messageID))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 1, "data": map[string]int{"like_count": count}})
}

// 点赞切换：POST /api/messages/:id/like/toggle
func ToggleMessageLike(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 0, "msg": "消息ID不能为空"})
		return
	}
	messageID, err := strconv.ParseUint(id, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 0, "msg": "无效的消息ID"})
		return
	}

	// 用户或会话
	var uid *uint
	if user, ok := pkg.GetUserSession(c); ok {
		u := user.ID
		uid = &u
	}
	sessionID := ""
	if cookie, _ := c.Request.Cookie("ech0_session"); cookie != nil {
		sessionID = cookie.Value
	}
	// 兼容匿名用户：若无会话则使用自定义 Cookie 维持点赞状态
	if sessionID == "" {
		if ck, _ := c.Request.Cookie("like_sid"); ck != nil && ck.Value != "" {
			sessionID = ck.Value
		} else {
			sid := models.GenerateToken(32)
			http.SetCookie(c.Writer, &http.Cookie{
				Name:     "like_sid",
				Value:    sid,
				Path:     "/",
				MaxAge:   86400 * 365,
				HttpOnly: true,
				SameSite: http.SameSiteLaxMode,
			})
			sessionID = sid
		}
	}
	liked, count, err := services.ToggleLike(uint(messageID), uid, sessionID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 1, "data": map[string]interface{}{"liked": liked, "like_count": count}})
}
func GetMessagesCalendar(c *gin.Context) {
	// 改为调用 services 层方法
	calendarData, err := services.GetMessagesGroupByDate()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 1,
		"data": calendarData,
	})
}
func SearchMessages(c *gin.Context) {
	// 从查询参数获取数据
	keyword := c.Query("keyword")
	page := 1
	pageSize := 10

	// 尝试解析页码和每页数量
	if pageStr := c.Query("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}
	if sizeStr := c.Query("pageSize"); sizeStr != "" {
		if s, err := strconv.Atoi(sizeStr); err == nil && s > 0 {
			pageSize = s
		}
	}

	// 默认只搜索公开内容
	showPrivate := false

	// 检查用户是否为管理员，管理员可以看到私密内容
	userID, exists := c.Get("user_id")
	if exists {
		user, err := services.GetUserByID(userID.(uint))
		if err == nil && user.IsAdmin {
			showPrivate = true
		}
	}

	// 可选作者筛选
	var authorID *uint
	if aid := c.Query("authorId"); aid != "" {
		if v, err := strconv.ParseUint(aid, 10, 64); err == nil {
			vv := uint(v)
			authorID = &vv
		}
	}
	var username *string
	if un := c.Query("username"); strings.TrimSpace(un) != "" {
		u := strings.TrimSpace(un)
		username = &u
	}

	result, err := services.SearchMessages(keyword, page, pageSize, showPrivate, authorID, username)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code": 0,
			"msg":  err.Error(),
			"data": nil,
		})
		return
	}

	// 直接构造符合前端期望的JSON格式
	c.JSON(http.StatusOK, gin.H{
		"code": 1,
		"msg":  "搜索成功",
		"data": result,
	})
}
func GetUserToken(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusOK, dto.Fail[any]("未授权访问"))
		return
	}

	token, err := services.GetUserToken(userID.(uint))
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[any]("获取Token失败"))
		return
	}

	c.JSON(http.StatusOK, dto.OK(gin.H{
		"token": token,
	}, "获取成功"))
}

func RegenerateUserToken(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusOK, dto.Fail[any]("未授权访问"))
		return
	}

	token, err := services.RegenerateUserToken(userID.(uint))
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[any]("更新Token失败"))
		return
	}

	c.JSON(http.StatusOK, dto.OK(gin.H{
		"token": token,
	}, "更新成功"))
}

// RefreshRSS 刷新 RSS 内容
func RefreshRSS(c *gin.Context) {
	// 重新生成 RSS
	_, err := services.GenerateRSS(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 0,
			"msg":  "RSS 刷新失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 1,
		"msg":  "RSS 已刷新",
	})
}

// 检查版本更新
func CheckVersion(c *gin.Context) {
	client := &http.Client{Timeout: 5 * time.Second}
	type tagInfo struct{ Name, LastUpdated string }
	latest := tagInfo{}

	get := func(url string, v any) error {
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		req = req.WithContext(ctx)
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		return json.NewDecoder(resp.Body).Decode(v)
	}

	type result struct {
		ok   bool
		info tagInfo
	}
	ch := make(chan result, 3)
	go func() {
		var v struct {
			Name        string `json:"name"`
			LastUpdated string `json:"last_updated"`
		}
		if get("https://hub.docker.com/v2/repositories/noise233/echo-noise/tags/latest", &v) == nil && strings.TrimSpace(v.LastUpdated) != "" {
			ch <- result{true, tagInfo{v.Name, v.LastUpdated}}
			return
		}
		ch <- result{false, tagInfo{}}
	}()
	go func() {
		var v struct {
			Results []struct {
				Name        string `json:"name"`
				LastUpdated string `json:"last_updated"`
			} `json:"results"`
		}
		if get("https://hub.docker.com/v2/repositories/noise233/echo-noise/tags?page_size=1&ordering=last_updated", &v) == nil && len(v.Results) > 0 && strings.TrimSpace(v.Results[0].LastUpdated) != "" {
			r := v.Results[0]
			ch <- result{true, tagInfo{r.Name, r.LastUpdated}}
			return
		}
		ch <- result{false, tagInfo{}}
	}()
	go func() {
		var v struct {
			TagName     string `json:"tag_name"`
			PublishedAt string `json:"published_at"`
		}
		if get("https://api.github.com/repos/noise233/echo-noise/releases/latest", &v) == nil && strings.TrimSpace(v.PublishedAt) != "" {
			ch <- result{true, tagInfo{v.TagName, v.PublishedAt}}
			return
		}
		ch <- result{false, tagInfo{}}
	}()
	for i := 0; i < 3; i++ {
		r := <-ch
		if r.ok {
			latest = r.info
			break
		}
	}
	if strings.TrimSpace(latest.LastUpdated) == "" {
		cur := strings.TrimSpace(os.Getenv("ECHO_NOISE_VERSION"))
		if cur == "" {
			cur = strings.TrimSpace(os.Getenv("APP_VERSION"))
		}
		if cur == "" {
			cur = strings.TrimSpace(os.Getenv("IMAGE_TAG"))
		}
		if cur == "" {
			cur = "latest"
		}
		c.JSON(http.StatusOK, gin.H{"code": 1, "data": gin.H{"hasUpdate": false, "lastUpdateTime": time.Now().Format(time.RFC3339), "currentTag": cur}})
		return
	}
	cur := strings.TrimSpace(os.Getenv("ECHO_NOISE_VERSION"))
	if cur == "" {
		cur = strings.TrimSpace(os.Getenv("APP_VERSION"))
	}
	if cur == "" {
		cur = strings.TrimSpace(os.Getenv("IMAGE_TAG"))
	}
	if cur == "" {
		cur = "latest"
	}
	var curUpdated string
	if strings.ToLower(cur) == "latest" {
		curUpdated = strings.TrimSpace(latest.LastUpdated)
	} else {
		if resp, err := client.Get("https://hub.docker.com/v2/repositories/noise233/echo-noise/tags/" + cur); err == nil {
			defer resp.Body.Close()
			var curTag struct {
				Name        string `json:"name"`
				LastUpdated string `json:"last_updated"`
			}
			if json.NewDecoder(resp.Body).Decode(&curTag) == nil {
				curUpdated = strings.TrimSpace(curTag.LastUpdated)
			}
		}
		if strings.TrimSpace(curUpdated) == "" {
			if resp, err := client.Get("https://api.github.com/repos/noise233/echo-noise/releases/tags/" + cur); err == nil {
				defer resp.Body.Close()
				var rel struct {
					PublishedAt string `json:"published_at"`
				}
				if json.NewDecoder(resp.Body).Decode(&rel) == nil {
					curUpdated = strings.TrimSpace(rel.PublishedAt)
				}
			}
		}
	}
	latestTime, err := time.Parse(time.RFC3339, latest.LastUpdated)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "解析时间失败"})
		return
	}
	var hasUpdate bool
	if curUpdated != "" {
		curTime, err := time.Parse(time.RFC3339, curUpdated)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "解析时间失败"})
			return
		}
		hasUpdate = latestTime.After(curTime)
	} else {
		hasUpdate = true
	}
	c.JSON(http.StatusOK, gin.H{"code": 1, "data": gin.H{"hasUpdate": hasUpdate, "lastUpdateTime": latest.LastUpdated, "currentTag": cur}})
}

// 获取当前运行版本（优先读取容器环境变量或镜像标签）
func GetVersion(c *gin.Context) {
	v := strings.TrimSpace(os.Getenv("ECHO_NOISE_VERSION"))
	if v == "" {
		v = strings.TrimSpace(os.Getenv("APP_VERSION"))
	}
	if v == "" {
		v = strings.TrimSpace(os.Getenv("IMAGE_TAG"))
	}
	if v == "" {
		v = "latest"
	}
	if strings.ToLower(v) == "latest" {
		client := &http.Client{Timeout: 5 * time.Second}
		var resp struct {
			Results []struct {
				Name        string `json:"name"`
				LastUpdated string `json:"last_updated"`
			} `json:"results"`
		}
		req, err := http.NewRequest("GET", "https://hub.docker.com/v2/repositories/noise233/echo-noise/tags?page_size=1&ordering=last_updated", nil)
		if err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			req = req.WithContext(ctx)
			if r, e := client.Do(req); e == nil {
				defer r.Body.Close()
				if json.NewDecoder(r.Body).Decode(&resp) == nil && len(resp.Results) > 0 && strings.TrimSpace(resp.Results[0].Name) != "" {
					v = strings.TrimSpace(resp.Results[0].Name)
				}
			}
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"code": 1,
		"data": gin.H{
			"version": v,
		},
	})
}

func UpdateVersion(c *gin.Context) {
	_, err := checkAdmin(c)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](err.Error()))
		return
	}

	hasUpdate, _, _, chkErr := computeUpgradeInfo()
	if chkErr != nil {
		c.JSON(http.StatusOK, dto.Fail[string]("版本检测失败: "+chkErr.Error()))
		return
	}
	if !hasUpdate {
		c.JSON(http.StatusOK, dto.Fail[string]("已是最新版，无需升级"))
		return
	}

	var logs bytes.Buffer
	shellArgs := func() (string, []string) {
		if _, err := exec.LookPath("bash"); err == nil {
			return "bash", []string{"-lc"}
		}
		return "sh", []string{"-c"}
	}
	run := func(timeout time.Duration, cmdStr string) error {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		prog, args := shellArgs()
		cmd := exec.CommandContext(ctx, prog, append(args, cmdStr)...)
		cmd.Env = os.Environ()
		cmd.Stdout = &logs
		cmd.Stderr = &logs
		return cmd.Run()
	}

	image := strings.TrimSpace(os.Getenv("UPDATE_IMAGE"))
	if image == "" {
		image = "noise233/echo-noise:latest"
	}
	name := strings.TrimSpace(os.Getenv("CONTAINER_NAME"))
	if name == "" {
		name = strings.TrimSpace(os.Getenv("ECH0_CONTAINER_NAME"))
	}
	if name == "" {
		name = "Ech0-Noise"
	}
	hostPort := strings.TrimSpace(os.Getenv("HTTP_PORT"))
	if hostPort == "" {
		hostPort = "1314"
	}
	wd, _ := os.Getwd()
	dataDir := strings.TrimSpace(os.Getenv("DATA_DIR"))
	if dataDir == "" {
		candidates := []string{"/opt/data", filepath.Join(wd, "data"), "/data"}
		for _, d := range candidates {
			if info, err := os.Stat(d); err == nil && info.IsDir() {
				dataDir = d
				break
			}
		}
		if dataDir == "" {
			dataDir = filepath.Join(wd, "data")
			_ = os.MkdirAll(dataDir, 0755)
		}
	}

	if err := run(10*time.Second, "docker --version"); err != nil {
		custom := strings.TrimSpace(os.Getenv("DESKTOP_UPDATE_CMD"))
		if custom == "" {
			c.JSON(http.StatusOK, dto.Fail[string]("Docker 未就绪: "+err.Error()))
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		cmd := exec.CommandContext(ctx, "bash", "-lc", custom)
		cmd.Env = os.Environ()
		cmd.Stdout = &logs
		cmd.Stderr = &logs
		if err := cmd.Run(); err != nil {
			c.JSON(http.StatusOK, dto.Fail[string]("桌面端更新失败: "+err.Error()))
			return
		}
		out := logs.String()
		if len(out) > 4000 {
			out = out[len(out)-4000:]
		}
		c.JSON(http.StatusOK, dto.OK[string](out, "桌面端已更新"))
		return
	}
	// 检测是否在Docker Compose环境中运行
	isComposeMode := false
	if os.Getenv("DOCKER_ENVIRONMENT") == "compose" {
		isComposeMode = true
	}

	dockerHost := strings.TrimSpace(os.Getenv("DOCKER_HOST"))
	dockerCmd := "docker"
	if dockerHost != "" {
		dockerCmd = "docker -H '" + dockerHost + "'"
	}
	if dockerHost == "" {
		if _, err := os.Stat("/var/run/docker.sock"); err != nil {
			c.JSON(http.StatusOK, dto.Fail[string]("Docker 未就绪: 缺少 /var/run/docker.sock"))
			return
		}
	}

	if isComposeMode {
		composeCmd := "docker compose"
		if err := run(10*time.Second, composeCmd+" version"); err != nil {
			if err2 := run(10*time.Second, "docker-compose --version"); err2 == nil {
				composeCmd = "docker-compose"
			} else {
				c.JSON(http.StatusOK, dto.Fail[string]("Docker Compose 未就绪: "+err.Error()))
				return
			}
		}
		if err := run(2*time.Minute, composeCmd+" pull"); err != nil {
			c.JSON(http.StatusOK, dto.Fail[string]("拉取镜像失败: "+err.Error()))
			return
		}
		if err := run(2*time.Minute, composeCmd+" up -d --force-recreate"); err != nil {
			c.JSON(http.StatusOK, dto.Fail[string]("重启服务失败: "+err.Error()))
			return
		}
	} else {
		// 标准Docker模式更新流程
		if err := run(2*time.Minute, dockerCmd+" pull "+image); err != nil {
			c.JSON(http.StatusOK, dto.Fail[string]("拉取镜像失败: "+err.Error()))
			return
		}
		_ = run(30*time.Second, dockerCmd+" ps -a --filter name=^"+name+"$ --format '{{.ID}}' | xargs -r "+dockerCmd+" stop")
		_ = run(30*time.Second, dockerCmd+" ps -a --filter name=^"+name+"$ --format '{{.ID}}' | xargs -r "+dockerCmd+" rm")
		runCmd := dockerCmd + " run -d --name " + name + " -p " + hostPort + ":1314 -v '" + dataDir + ":/app/data' --restart unless-stopped " + image
		if err := run(2*time.Minute, runCmd); err != nil {
			c.JSON(http.StatusOK, dto.Fail[string]("启动新容器失败: "+err.Error()))
			return
		}
		_ = run(30*time.Second, dockerCmd+" image prune -f || true")
	}

	out := logs.String()
	if len(out) > 4000 {
		out = out[len(out)-4000:]
	}
	c.JSON(http.StatusOK, dto.OK[string](out, "容器已升级并重启（数据已保留）"))
}

func UpdateVersionStream(c *gin.Context) {
	if _, err := checkAdmin(c); err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](err.Error()))
		return
	}
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusOK, dto.Fail[string]("当前服务器不支持流式输出"))
		return
	}
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	write := func(m map[string]any) {
		b, _ := json.Marshal(m)
		_, _ = c.Writer.Write([]byte("data: " + string(b) + "\n\n"))
		flusher.Flush()
	}

	write(map[string]any{"type": "info", "message": "开始升级流程"})
	hasUpdate, latestTime, curTag, chkErr := computeUpgradeInfo()
	if chkErr != nil {
		write(map[string]any{"type": "error", "message": "版本检测失败: " + chkErr.Error()})
		return
	}
	write(map[string]any{"type": "info", "message": fmt.Sprintf("当前版本 %s，最新发布时间 %s", curTag, latestTime)})
	if !hasUpdate {
		write(map[string]any{"type": "info", "message": "已是最新版，无需升级"})
		write(map[string]any{"type": "done", "message": "no-upgrade"})
		return
	}

	var step = func(progress int, msg string) {
		write(map[string]any{"type": "progress", "progress": progress, "message": msg})
	}

	shellArgs := func() (string, []string) {
		if _, err := exec.LookPath("bash"); err == nil {
			return "bash", []string{"-lc"}
		}
		return "sh", []string{"-c"}
	}
	runStreaming := func(timeout time.Duration, label, cmdStr string) error {
		step(0, "执行: "+label)
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		prog, args := shellArgs()
		cmd := exec.CommandContext(ctx, prog, append(args, cmdStr)...)
		stdout, _ := cmd.StdoutPipe()
		stderr, _ := cmd.StderrPipe()
		if err := cmd.Start(); err != nil {
			return err
		}

		done := make(chan struct{}, 2)
		go func() {
			defer func() { done <- struct{}{} }()
			scanner := bufio.NewScanner(stdout)
			for scanner.Scan() {
				line := scanner.Text()
				write(map[string]any{"type": "log", "message": fmt.Sprintf("[%s] %s", label, line)})
			}
		}()
		go func() {
			defer func() { done <- struct{}{} }()
			scanner := bufio.NewScanner(stderr)
			for scanner.Scan() {
				line := scanner.Text()
				write(map[string]any{"type": "log", "message": fmt.Sprintf("[%s] %s", label, line)})
			}
		}()
		err := cmd.Wait()
		<-done
		<-done
		return err
	}

	image := strings.TrimSpace(os.Getenv("UPDATE_IMAGE"))
	if image == "" {
		image = "noise233/echo-noise:latest"
	}
	name := strings.TrimSpace(os.Getenv("CONTAINER_NAME"))
	if name == "" {
		name = strings.TrimSpace(os.Getenv("ECH0_CONTAINER_NAME"))
	}
	if name == "" {
		name = "Ech0-Noise"
	}
	hostPort := strings.TrimSpace(os.Getenv("HTTP_PORT"))
	if hostPort == "" {
		hostPort = "1314"
	}
	wd, _ := os.Getwd()
	dataDir := strings.TrimSpace(os.Getenv("DATA_DIR"))
	if dataDir == "" {
		candidates := []string{"/opt/data", filepath.Join(wd, "data"), "/data"}
		for _, d := range candidates {
			if info, err := os.Stat(d); err == nil && info.IsDir() {
				dataDir = d
				break
			}
		}
		if dataDir == "" {
			dataDir = filepath.Join(wd, "data")
			_ = os.MkdirAll(dataDir, 0755)
		}
	}

	isComposeMode := false
	if os.Getenv("DOCKER_ENVIRONMENT") == "compose" {
		isComposeMode = true
	}

	if err := runStreaming(10*time.Second, "docker", "docker --version"); err != nil {
		if custom := strings.TrimSpace(os.Getenv("DESKTOP_UPDATE_CMD")); custom != "" {
			step(20, "桌面端更新执行...")
			if err2 := runStreaming(5*time.Minute, "desktop", custom); err2 != nil {
				write(map[string]any{"type": "error", "message": "桌面端更新失败: " + err2.Error()})
				return
			}
			write(map[string]any{"type": "success", "message": "桌面端已更新"})
			write(map[string]any{"type": "done", "message": "desktop-updated"})
			return
		}
		write(map[string]any{"type": "error", "message": "Docker 未就绪: " + err.Error()})
		return
	}
	dockerHost := strings.TrimSpace(os.Getenv("DOCKER_HOST"))
	dockerCmd := "docker"
	if dockerHost != "" {
		dockerCmd = "docker -H '" + dockerHost + "'"
	}
	if dockerHost == "" {
		if _, err := os.Stat("/var/run/docker.sock"); err != nil {
			write(map[string]any{"type": "error", "message": "Docker 未就绪: 缺少 /var/run/docker.sock"})
			return
		}
	}

	if isComposeMode {
		composeCmd := "docker compose"
		if err := runStreaming(10*time.Second, "compose", composeCmd+" version"); err != nil {
			if err2 := runStreaming(10*time.Second, "compose", "docker-compose --version"); err2 == nil {
				composeCmd = "docker-compose"
			} else {
				write(map[string]any{"type": "error", "message": "Docker Compose 未就绪: " + err.Error()})
				return
			}
		}
		step(30, "拉取镜像...")
		if err := runStreaming(3*time.Minute, "compose", composeCmd+" pull"); err != nil {
			write(map[string]any{"type": "error", "message": "拉取镜像失败: " + err.Error()})
			return
		}
		step(70, "重启服务...")
		if err := runStreaming(2*time.Minute, "compose", composeCmd+" up -d --force-recreate"); err != nil {
			write(map[string]any{"type": "error", "message": "重启服务失败: " + err.Error()})
			return
		}
		write(map[string]any{"type": "success", "message": "容器已升级并重启（数据已保留）"})
		step(100, "完成")
		write(map[string]any{"type": "done", "message": "ok"})
		return
	}
	step(25, "拉取镜像...")
	if err := runStreaming(3*time.Minute, "pull", dockerCmd+" pull "+image); err != nil {
		write(map[string]any{"type": "error", "message": "拉取镜像失败: " + err.Error()})
		return
	}
	step(45, "停止旧容器...")
	_ = runStreaming(30*time.Second, "stop", dockerCmd+" ps -a --filter name=^"+name+"$ --format '{{.ID}}' | xargs -r "+dockerCmd+" stop")
	step(55, "移除旧容器...")
	_ = runStreaming(30*time.Second, "rm", dockerCmd+" ps -a --filter name=^"+name+"$ --format '{{.ID}}' | xargs -r "+dockerCmd+" rm")
	step(75, "启动新容器...")
	runCmd := dockerCmd + " run -d --name " + name + " -p " + hostPort + ":1314 -v '" + dataDir + ":/app/data' --restart unless-stopped " + image
	if err := runStreaming(2*time.Minute, "run", runCmd); err != nil {
		write(map[string]any{"type": "error", "message": "启动新容器失败: " + err.Error()})
		return
	}
	step(90, "清理旧镜像...")
	_ = runStreaming(30*time.Second, "prune", dockerCmd+" image prune -f || true")

	write(map[string]any{"type": "success", "message": "容器已升级并重启（数据已保留）"})
	step(100, "完成")
	write(map[string]any{"type": "done", "message": "ok"})
}

// 版本升级逻辑仅通过容器镜像更新，保留数据卷；非容器桌面端由 DESKTOP_UPDATE_CMD 处理

// GetNotifyConfig 获取推送配置
func GetNotifyConfig(c *gin.Context) {
	_, err := checkAdmin(c)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](err.Error()))
		return
	}

	config := models.GetNotifyConfig()
	if config == nil {
		// 如果配置不存在，返回空配置（所有字段默认值）
		config = &models.NotifyConfig{
			WebhookEnabled:           false,
			WebhookURL:               "",
			TelegramEnabled:          false,
			TelegramToken:            "",
			TelegramChatID:           "",
			WeworkEnabled:            false,
			WeworkKey:                "",
			FeishuEnabled:            false,
			FeishuWebhook:            "",
			FeishuSecret:             "",
			TwitterEnabled:           false,
			TwitterApiKey:            "",
			TwitterApiSecret:         "",
			TwitterAccessToken:       "",
			TwitterAccessTokenSecret: "",
			CustomHttpEnabled:        false,
			CustomHttpUrl:            "",
			CustomHttpMethod:         "",
			CustomHttpHeaders:        "",
			CustomHttpBody:           "",
		}
	}
	c.JSON(http.StatusOK, dto.OK(config, "获取成功"))
}

// SaveNotifyConfig 保存推送配置
func SaveNotifyConfig(c *gin.Context) {
	_, err := checkAdmin(c)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](err.Error()))
		return
	}

	var config models.NotifyConfig
	if err := c.ShouldBindJSON(&config); err != nil {
		c.JSON(http.StatusOK, dto.Fail[string]("无效的配置数据"))
		return
	}
	// Twitter校验
	if config.TwitterEnabled {
		if config.TwitterApiKey == "" || config.TwitterApiSecret == "" || config.TwitterAccessToken == "" || config.TwitterAccessTokenSecret == "" {
			c.JSON(http.StatusOK, dto.Fail[string]("Twitter配置不完整"))
			return
		}
	}
	// 自定义HTTP校验
	if config.CustomHttpEnabled {
		if config.CustomHttpUrl == "" {
			c.JSON(http.StatusOK, dto.Fail[string]("自定义HTTP URL不能为空"))
			return
		}
	}
	// 根据启用状态验证配置
	if config.WebhookEnabled {
		if config.WebhookURL == "" {
			c.JSON(http.StatusOK, dto.Fail[string]("Webhook URL 不能为空"))
			return
		}
	}
	if config.TelegramEnabled {
		if config.TelegramToken == "" || config.TelegramChatID == "" {
			c.JSON(http.StatusOK, dto.Fail[string]("Telegram 配置不完整"))
			return
		}
	}
	if config.WeworkEnabled {
		if config.WeworkKey == "" {
			c.JSON(http.StatusOK, dto.Fail[string]("企业微信 Key 不能为空"))
			return
		}
	}
	if config.FeishuEnabled {
		if config.FeishuWebhook == "" {
			c.JSON(http.StatusOK, dto.Fail[string]("飞书 Webhook 不能为空"))
			return
		}
	}

	if err := models.SaveNotifyConfig(config); err != nil {
		c.JSON(http.StatusOK, dto.Fail[string]("保存配置失败: "+err.Error()))
		return
	}

	savedConfig := models.GetNotifyConfig()
	c.JSON(http.StatusOK, dto.OK(savedConfig, "配置已更新"))
}

// TestNotify 测试推送
func TestNotify(c *gin.Context) {
	_, err := checkAdmin(c)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](err.Error()))
		return
	}

	var request struct {
		Type string `json:"type" binding:"required"`
		To   string `json:"to"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusOK, dto.Fail[string]("无效的请求参数"))
		return
	}

	testMsg := "这是一条测试消息 - " + time.Now().Format("2006-01-02 15:04:05")
	var emptyImages []string

	var testErr error
	switch request.Type {
	case "webhook":
		testErr = models.SendWebhook(testMsg)
	case "telegram":
		testErr = models.SendTelegram(testMsg, emptyImages)
	case "wework":
		testErr = models.SendWework(testMsg, emptyImages)
	case "feishu":
		testErr = models.SendFeishu(testMsg)
	case "twitter":
		testErr = models.SendTwitter(testMsg)
	case "customHttp":
		testErr = models.SendCustomHttp(testMsg)
	case "email":
		to := strings.TrimSpace(request.To)
		if to == "" {
			db, _ := database.GetDB()
			var cfg models.SiteConfig
			_ = db.Table("site_configs").First(&cfg).Error
			if cfg.SmtpFrom != "" {
				to = cfg.SmtpFrom
			} else {
				to = cfg.SmtpUser
			}
		}
		testErr = models.SendTestEmail(to)
	default:
		c.JSON(http.StatusOK, dto.Fail[string]("不支持的推送类型"))
		return
	}

	if testErr != nil {
		c.JSON(http.StatusOK, dto.Fail[string](fmt.Sprintf("推送测试失败: %v", testErr)))
		return
	}

	c.JSON(http.StatusOK, dto.OK[any](nil, "推送测试已发送"))
}

// 保留这个新版本的 PostMessage 函数
func PostMessage(c *gin.Context) {
	// 解析请求数据
	var request struct {
		Content  string `json:"content"`
		Private  bool   `json:"private"`
		ImageURL string `json:"image_url"`
		VideoURL string `json:"video_url"` // 新增视频字段
		Notify   *bool  `json:"notify"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusOK, dto.Fail[string]("内容不能为空"))
		return
	}

	// 验证用户身份
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusOK, dto.Fail[string]("未授权访问"))
		return
	}

	// 创建消息
	message := &models.Message{
		Content:  request.Content,
		Private:  request.Private,
		ImageURL: request.ImageURL,
		UserID:   userID.(uint),
	}

	if err := services.CreateMessage(message); err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](err.Error()))
		return
	}

	// 处理推送逻辑
	// 后台推送总开关：SiteConfig.NotifyEnabled
	var siteCfg models.SiteConfig
	_ = database.DB.Table("site_configs").First(&siteCfg).Error

	// 推送策略：
	// - session（编辑器/后台页面会话）发布：保持原有语义，只有显式 notify=true 才推送
	// - token（API/扩展/MCP）发布：当后台推送总开关开启时自动推送（忽略客户端 notify 字段）
	shouldNotify := false
	if siteCfg.NotifyEnabled {
		via, _ := c.Get("auth_via")
		viaStr, _ := via.(string)
		if viaStr == "token" {
			shouldNotify = true
		} else {
			// session 或未标记：保持原逻辑，必须显式 notify=true
			shouldNotify = (request.Notify != nil && *request.Notify)
		}
	}

	if shouldNotify {
		notifyConfig := models.GetNotifyConfig()
		if notifyConfig != nil {
			// 提取内容中的第一张图片链接
			var firstImageURL string
			var firstVideoURL string
			var formattedContent string

			// 如果已有上传的图片，优先使用
			if request.ImageURL != "" {
				firstImageURL = request.ImageURL
			}
			// 如果已有上传的视频，优先使用
			if request.VideoURL != "" {
				firstVideoURL = request.VideoURL
			}

			cleanContent, extractedImages := models.ExtractImageURLsFromMarkdown(request.Content)
			if firstImageURL == "" && len(extractedImages) > 0 {
				firstImageURL = extractedImages[0]
			}

			// 从 Markdown 内容中提取第一段视频（如 [video](url)）
			videoRegex := regexp.MustCompile(`\[video\]\(([^)]+)\)`)
			videoMatches := videoRegex.FindAllStringSubmatch(request.Content, -1)
			if firstVideoURL == "" && len(videoMatches) > 0 {
				firstVideoURL = videoMatches[0][1]
			}

			formattedContent = cleanContent

			// 处理长内容，如果超过4000字符，进行截断
			const maxContentLength = 4000
			var truncatedContent string
			if len(formattedContent) > maxContentLength {
				truncatedContent = formattedContent[:maxContentLength] + "...\n(内容过长，已截断)"
			} else {
				truncatedContent = formattedContent
			}

			// 格式化内容，处理Markdown语法
			headingRegex := regexp.MustCompile(`(?m)^(#{1,6})\s+(.+)$`)
			truncatedContent = headingRegex.ReplaceAllString(truncatedContent, "$1 $2")

			// 准备图片和视频数组
			var images []string
			var videos []string
			if request.ImageURL != "" {
				images = append(images, request.ImageURL)
			}
			if len(extractedImages) > 0 {
				images = append(images, extractedImages...)
			}
			if firstVideoURL != "" {
				videos = []string{firstVideoURL}
			}

			go func() {
				// Webhook
				if notifyConfig.WebhookEnabled && notifyConfig.WebhookURL != "" {
					models.SendWebhook(truncatedContent)
				}

				// Telegram
				if notifyConfig.TelegramEnabled && notifyConfig.TelegramToken != "" && notifyConfig.TelegramChatID != "" {
					const telegramMaxText = 4096
					const telegramMaxCaption = 1024

					isPublicURL := func(url string) bool {
						return strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://")
					}

					// 推送图片
					if len(images) > 0 {
						allPublic := true
						for _, img := range images {
							if !isPublicURL(img) {
								allPublic = false
								break
							}
						}

						if allPublic {
							caption := formattedContent
							if len([]rune(caption)) > telegramMaxCaption {
								msg := caption
								if len([]rune(msg)) > telegramMaxText {
									msg = string([]rune(msg)[:telegramMaxText]) + "...\n(内容过长，已截断)"
								}
								if err := models.SendTelegramMessage(msg); err != nil {
									sendTelegramErrorNotify(c, err)
								}
								caption = ""
							}

							if len(images) == 1 {
								if err := models.SendTelegramPhotoWithCaption(images[0], caption); err != nil {
									sendTelegramErrorNotify(c, err)
								}
							} else {
								if err := models.SendTelegramMediaGroupWithCaption(images, caption); err != nil {
									sendTelegramErrorNotify(c, err)
								}
							}
						} else {
							msg := formattedContent
							for _, img := range images {
								msg += "\n[图片] " + img
							}
							if len([]rune(msg)) > telegramMaxText {
								msg = string([]rune(msg)[:telegramMaxText]) + "...\n(内容过长，已截断)"
							}
							if err := models.SendTelegramMessage(msg); err != nil {
								sendTelegramErrorNotify(c, err)
							}
						}
					}

					// 推送视频
					if len(videos) > 0 {
						if isPublicURL(videos[0]) {
							caption := formattedContent
							if len(caption) > telegramMaxCaption {
								caption = caption[:telegramMaxCaption] + "...\n(内容过长，已截断)"
							}
							err := models.SendTelegramVideoWithCaption(videos[0], caption)
							if err != nil {
								sendTelegramErrorNotify(c, err)
							}
						} else {
							msg := formattedContent + "\n[视频] " + videos[0]
							if len(msg) > telegramMaxText {
								msg = msg[:telegramMaxText] + "...\n(内容过长，已截断)"
							}
							err := models.SendTelegramMessage(msg)
							if err != nil {
								sendTelegramErrorNotify(c, err)
							}
						}
					}

					// 没有图片和视频，直接发文本
					if len(images) == 0 && len(videos) == 0 {
						if len(formattedContent) > telegramMaxText {
							sendTelegramErrorNotify(c, fmt.Errorf("Telegram 文本内容超出最大长度（%d 字符）", telegramMaxText))
						} else {
							err := models.SendTelegramMessage(formattedContent)
							if err != nil {
								sendTelegramErrorNotify(c, err)
							}
						}
					}
				}

				// 企业微信
				if notifyConfig.WeworkEnabled && notifyConfig.WeworkKey != "" {
					const weworkMaxLength = 2000
					var weworkContent string
					if len(formattedContent) > weworkMaxLength {
						weworkContent = formattedContent[:weworkMaxLength] + "...\n(内容过长，已截断)"
					} else {
						weworkContent = formattedContent
					}
					models.SendWework(weworkContent, images)
				}

				// 飞书
				if notifyConfig.FeishuEnabled && notifyConfig.FeishuWebhook != "" {
					const feishuMaxLength = 2000
					var feishuContent string
					if len(formattedContent) > feishuMaxLength {
						feishuContent = formattedContent[:feishuMaxLength] + "...\n(内容过长，已截断)"
					} else {
						feishuContent = formattedContent
					}
					models.SendFeishu(feishuContent)
				}
			}()
		}
	}

	c.JSON(http.StatusOK, dto.OK(message, "发布成功"))

	// 即时模式触发云同步（防抖）
	syncmanager.Trigger()
}

// 上传视频
func UploadVideo(c *gin.Context) {
	// 获取站点配置
	db, _ := database.GetDB()
	var siteConfig models.SiteConfig
	if err := db.First(&siteConfig).Error; err != nil {
		// 如果获取配置失败，使用空配置（默认本地存储）
		siteConfig = models.SiteConfig{}
	}

	// 支持的视频 MIME 类型
	allowedMimeTypes := []string{"video/mp4", "video/webm", "video/quicktime", "video/x-msvideo"}

	videoURL, err := pkg.UploadVideo(c, allowedMimeTypes, &siteConfig)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": err.Error()})
		return
	}

	// 返回视频访问路径
	c.JSON(http.StatusOK, gin.H{
		"code": 1,
		"msg":  "上传成功",
		"data": videoURL,
	})
}

// ResetDefaultData 重置/初始化默认数据
func ResetDefaultData(c *gin.Context) {
	_, err := checkAdmin(c)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](err.Error()))
		return
	}

	if err := services.SeedDefaultData(); err != nil {
		c.JSON(http.StatusOK, dto.Fail[string]("重置失败: "+err.Error()))
		return
	}

	c.JSON(http.StatusOK, dto.OK[any](nil, "重置成功"))
}
func SendNotify(c *gin.Context) {
	var request struct {
		Content string   `json:"content"`
		Images  []string `json:"images"`
		Format  string   `json:"format"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 0, "msg": "请求参数错误"})
		return
	}

	// 验证内容不为空
	if request.Content == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 0, "msg": "推送内容不能为空"})
		return
	}

	// 获取推送配置
	config := models.GetNotifyConfig()
	if config == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 0, "msg": "推送配置不存在"})
		return
	}

	// 并发处理所有启用的推送渠道
	type notifyResult struct {
		Success bool   `json:"success"`
		Error   string `json:"error,omitempty"`
	}
	results := map[string]notifyResult{}
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Telegram
	if config.TelegramEnabled {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := models.SendTelegram(request.Content, request.Images)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				results["telegram"] = notifyResult{Success: false, Error: err.Error()}
			} else {
				results["telegram"] = notifyResult{Success: true}
			}
		}()
	}

	// 企业微信
	if config.WeworkEnabled {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := models.SendWework(request.Content, request.Images)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				results["wework"] = notifyResult{Success: false, Error: err.Error()}
			} else {
				results["wework"] = notifyResult{Success: true}
			}
		}()
	}

	// 飞书
	if config.FeishuEnabled {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := models.SendFeishu(request.Content)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				results["feishu"] = notifyResult{Success: false, Error: err.Error()}
			} else {
				results["feishu"] = notifyResult{Success: true}
			}
		}()
	}

	// Webhook
	if config.WebhookEnabled {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := models.SendWebhook(request.Content)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				results["webhook"] = notifyResult{Success: false, Error: err.Error()}
			} else {
				results["webhook"] = notifyResult{Success: true}
			}
		}()
	}
	// Twitter
	if config.TwitterEnabled {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Twitter 字数限制 280
			tweet := request.Content
			if len([]rune(tweet)) > 280 {
				tweet = string([]rune(tweet)[:280]) + "...(内容截断)"
			}
			err := models.SendTwitter(tweet)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				results["twitter"] = notifyResult{Success: false, Error: err.Error()}
			} else {
				results["twitter"] = notifyResult{Success: true}
			}
		}()
	}

	// 自定义 HTTP
	if config.CustomHttpEnabled {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := models.SendCustomHttp(request.Content)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				results["customHttp"] = notifyResult{Success: false, Error: err.Error()}
			} else {
				results["customHttp"] = notifyResult{Success: true}
			}
		}()
	}

	// 等待所有推送完成
	wg.Wait()

	anyFail := false
	for _, r := range results {
		if !r.Success {
			anyFail = true
			break
		}
	}
	if anyFail {
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "部分推送失败", "data": results})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "推送成功", "data": results})
}
func EmailTest(c *gin.Context) {
	_, err := checkAdmin(c)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](err.Error()))
		return
	}
	var req struct {
		To string `json:"to" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, dto.Fail[string]("无效的请求参数"))
		return
	}
	if req.To == "" {
		c.JSON(http.StatusOK, dto.Fail[string]("收件人不能为空"))
		return
	}
	if err := models.SendTestEmail(req.To); err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](err.Error()))
		return
	}
	c.JSON(http.StatusOK, dto.OK[any](nil, "测试邮件已发送"))
}
func PasswordForgot(c *gin.Context) {
	var req struct {
		Account string `json:"account" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, dto.Fail[string]("无效的请求参数"))
		return
	}
	db, _ := database.GetDB()
	var cfg models.SiteConfig
	if err := db.Table("site_configs").First(&cfg).Error; err != nil {
		c.JSON(http.StatusOK, dto.Fail[string]("系统配置读取失败"))
		return
	}
	if !cfg.SmtpEnabled {
		c.JSON(http.StatusOK, dto.Fail[string]("邮件未开启"))
		return
	}
	account := strings.TrimSpace(req.Account)
	if account == "" {
		c.JSON(http.StatusOK, dto.Fail[string]("账号不能为空"))
		return
	}
	var user *models.User
	var err error
	if strings.Contains(account, "@") {
		user, err = repository.GetUserByEmail(account)
		if err != nil || user == nil {
			c.JSON(http.StatusOK, dto.Fail[string]("用户不存在"))
			return
		}
	} else {
		user, err = services.GetUserByUsername(account)
		if err != nil || user == nil {
			c.JSON(http.StatusOK, dto.Fail[string]("用户不存在"))
			return
		}
	}
	if strings.TrimSpace(user.Email) == "" || !user.EmailVerified {
		c.JSON(http.StatusOK, dto.Fail[string]("未绑定邮箱或未验证"))
		return
	}
	to := strings.TrimSpace(user.Email)
	temp := models.GenerateToken(16)
	if user != nil {
		hashed := models.HashPassword(temp)
		if strings.TrimSpace(hashed) == "" {
			c.JSON(http.StatusOK, dto.Fail[string]("生成临时密码失败"))
			return
		}
		if e := repository.UpdateUserField(user.ID, "password", hashed); e != nil {
			c.JSON(http.StatusOK, dto.Fail[string]("更新密码失败"))
			return
		}
	}
	subject := "密码重置通知"
	body := "您的临时密码为: " + temp + "\n请使用该密码登录后尽快在后台修改为新密码。"
	if err := models.SendEmail(to, subject, body); err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](err.Error()))
		return
	}
	c.JSON(http.StatusOK, dto.OK[any](nil, "重置邮件已发送"))
}

// GitHub OAuth 登录
func GithubLogin(c *gin.Context) {
	db, _ := database.GetDB()
	var cfg models.SiteConfig
	if err := db.Table("site_configs").First(&cfg).Error; err != nil || !cfg.GithubOAuthEnabled {
		c.JSON(http.StatusOK, dto.Fail[string]("未开启 GitHub 登录"))
		return
	}
	clientID := strings.TrimSpace(cfg.GithubClientId)
	callback := strings.TrimSpace(cfg.GithubCallbackURL)
	if clientID == "" || callback == "" {
		c.JSON(http.StatusOK, dto.Fail[string]("GitHub 登录参数不完整"))
		return
	}
	authURL := fmt.Sprintf("https://github.com/login/oauth/authorize?client_id=%s&redirect_uri=%s&scope=user:email", clientID, callback)
	c.Redirect(http.StatusFound, authURL)
}

func GithubCallback(c *gin.Context) {
	code := c.Query("code")
	if code == "" {
		c.JSON(http.StatusOK, dto.Fail[string]("回调参数缺失"))
		return
	}
	db, _ := database.GetDB()
	var cfg models.SiteConfig
	if err := db.Table("site_configs").First(&cfg).Error; err != nil || !cfg.GithubOAuthEnabled {
		c.JSON(http.StatusOK, dto.Fail[string]("未开启 GitHub 登录"))
		return
	}
	clientID := strings.TrimSpace(cfg.GithubClientId)
	clientSecret := strings.TrimSpace(cfg.GithubClientSecret)
	if clientID == "" || clientSecret == "" {
		c.JSON(http.StatusOK, dto.Fail[string]("GitHub 登录参数不完整"))
		return
	}
	// 交换令牌
	tokenReq, _ := http.NewRequest("POST", "https://github.com/login/oauth/access_token", strings.NewReader(fmt.Sprintf("client_id=%s&client_secret=%s&code=%s", clientID, clientSecret, code)))
	tokenReq.Header.Set("Accept", "application/json")
	tokenResp, err := http.DefaultClient.Do(tokenReq)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[string]("访问 GitHub 失败"))
		return
	}
	defer tokenResp.Body.Close()
	var tokenData struct {
		AccessToken string `json:"access_token"`
	}
	_ = json.NewDecoder(tokenResp.Body).Decode(&tokenData)
	if tokenData.AccessToken == "" {
		c.JSON(http.StatusOK, dto.Fail[string]("获取令牌失败"))
		return
	}
	// 获取用户信息
	userReq, _ := http.NewRequest("GET", "https://api.github.com/user", nil)
	userReq.Header.Set("Authorization", "Bearer "+tokenData.AccessToken)
	userResp, err := http.DefaultClient.Do(userReq)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[string]("获取用户信息失败"))
		return
	}
	defer userResp.Body.Close()
	var gh struct {
		Login string `json:"login"`
		ID    int64  `json:"id"`
	}
	_ = json.NewDecoder(userResp.Body).Decode(&gh)
	if gh.Login == "" {
		c.JSON(http.StatusOK, dto.Fail[string]("获取用户信息失败"))
		return
	}
	// 查找或创建用户
	user, _ := services.GetUserByUsername(gh.Login)
	isNew := false
	if user == nil {
		// 检查是否允许注册
		var setting models.Setting
		allowReg := true
		if err := db.Table("settings").First(&setting).Error; err == nil {
			allowReg = setting.AllowRegistration
		}
		if !allowReg {
			c.JSON(http.StatusOK, dto.Fail[string]("站点已关闭注册"))
			return
		}
		// 创建用户，密码随机
		pwd := models.GenerateToken(16)
		hashed := models.HashPassword(pwd)
		newUser := models.User{Username: gh.Login, Password: hashed, IsAdmin: false, Token: models.GenerateToken(32)}
		if err := database.DB.Create(&newUser).Error; err != nil {
			c.JSON(http.StatusOK, dto.Fail[string]("创建用户失败"))
			return
		}
		user = &newUser
		isNew = true
	}
	// 自动识别并绑定 GitHub 邮箱
	emailReq, _ := http.NewRequest("GET", "https://api.github.com/user/emails", nil)
	emailReq.Header.Set("Authorization", "Bearer "+tokenData.AccessToken)
	emailResp, err := http.DefaultClient.Do(emailReq)
	if err == nil {
		defer emailResp.Body.Close()
		var emails []struct {
			Email    string `json:"email"`
			Primary  bool   `json:"primary"`
			Verified bool   `json:"verified"`
		}
		_ = json.NewDecoder(emailResp.Body).Decode(&emails)
		pick := ""
		for _, e := range emails {
			if e.Primary && e.Verified && e.Email != "" {
				pick = e.Email
				break
			}
		}
		if pick == "" {
			for _, e := range emails {
				if e.Verified && e.Email != "" {
					pick = e.Email
					break
				}
			}
		}
		if pick != "" {
			user.Email = pick
			user.EmailVerified = true
			_ = repository.UpdateUser(user)
		}
	}
	// 设置会话
	session := sessions.Default(c)
	session.Clear()
	applyLoginSessionExpire(session)
	session.Set("user_id", user.ID)
	session.Set("username", user.Username)
	session.Set("is_admin", user.IsAdmin)
	if err := session.Save(); err != nil {
		c.JSON(http.StatusOK, dto.Fail[any]("Session 保存失败"))
		return
	}
	// 跳转
	if isNew {
		c.Redirect(http.StatusFound, "/")
	} else {
		c.Redirect(http.StatusFound, "/status")
	}
}

func BindEmail(c *gin.Context) {
	user, err := checkUser(c)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[any](err.Error()))
		return
	}
	db, _ := database.GetDB()
	var cfg models.SiteConfig
	if err := db.Table("site_configs").First(&cfg).Error; err != nil || !cfg.SmtpEnabled {
		c.JSON(http.StatusOK, dto.Fail[any]("邮件未开启"))
		return
	}
	var req struct {
		Email string `json:"email"`
	}
	if e := c.ShouldBindJSON(&req); e != nil || strings.TrimSpace(req.Email) == "" {
		c.JSON(http.StatusOK, dto.Fail[any]("邮箱不能为空"))
		return
	}
	code := fmt.Sprintf("%06d", time.Now().UnixNano()%1000000)
	exp := time.Now().Add(10 * time.Minute)
	user.EmailPending = strings.TrimSpace(req.Email)
	user.EmailVerifyCode = code
	user.EmailVerifyExpires = &exp
	if e := repository.UpdateUser(user); e != nil {
		c.JSON(http.StatusOK, dto.Fail[any]("保存失败"))
		return
	}
	if e := models.SendEmail(user.EmailPending, "邮箱绑定验证码", "验证码: "+code+"，10分钟内有效"); e != nil {
		c.JSON(http.StatusOK, dto.Fail[any](e.Error()))
		return
	}
	c.JSON(http.StatusOK, dto.OK[any](nil, "验证码已发送"))
}

func VerifyEmail(c *gin.Context) {
	user, err := checkUser(c)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[any](err.Error()))
		return
	}
	var req struct {
		Code string `json:"code"`
	}
	if e := c.ShouldBindJSON(&req); e != nil || strings.TrimSpace(req.Code) == "" {
		c.JSON(http.StatusOK, dto.Fail[any]("验证码不能为空"))
		return
	}
	if user.EmailVerifyExpires == nil || time.Now().After(*user.EmailVerifyExpires) {
		c.JSON(http.StatusOK, dto.Fail[any]("验证码已过期"))
		return
	}
	if strings.TrimSpace(req.Code) != strings.TrimSpace(user.EmailVerifyCode) {
		c.JSON(http.StatusOK, dto.Fail[any]("验证码错误"))
		return
	}
	if strings.TrimSpace(user.EmailPending) == "" {
		c.JSON(http.StatusOK, dto.Fail[any]("无待绑定邮箱"))
		return
	}
	user.Email = user.EmailPending
	user.EmailPending = ""
	user.EmailVerified = true
	user.EmailVerifyCode = ""
	user.EmailVerifyExpires = nil
	if e := repository.UpdateUser(user); e != nil {
		c.JSON(http.StatusOK, dto.Fail[any]("更新失败"))
		return
	}
	c.JSON(http.StatusOK, dto.OK[any](nil, "邮箱已绑定"))
}

func SendChangeEmailCode(c *gin.Context) {
	user, err := checkUser(c)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[any](err.Error()))
		return
	}
	if strings.TrimSpace(user.Email) == "" || !user.EmailVerified {
		c.JSON(http.StatusOK, dto.Fail[any]("未绑定邮箱或未验证"))
		return
	}
	db, _ := database.GetDB()
	var cfg models.SiteConfig
	if err := db.Table("site_configs").First(&cfg).Error; err != nil || !cfg.SmtpEnabled {
		c.JSON(http.StatusOK, dto.Fail[any]("邮件未开启"))
		return
	}
	code := fmt.Sprintf("%06d", time.Now().UnixNano()%1000000)
	exp := time.Now().Add(10 * time.Minute)
	user.EmailVerifyCode = code
	user.EmailVerifyExpires = &exp
	if e := repository.UpdateUser(user); e != nil {
		c.JSON(http.StatusOK, dto.Fail[any]("保存失败"))
		return
	}
	if e := models.SendEmail(strings.TrimSpace(user.Email), "更换邮箱验证码", "验证码: "+code+"，10分钟内有效"); e != nil {
		c.JSON(http.StatusOK, dto.Fail[any](e.Error()))
		return
	}
	c.JSON(http.StatusOK, dto.OK[any](nil, "验证码已发送"))
}

func ChangeEmail(c *gin.Context) {
	user, err := checkUser(c)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[any](err.Error()))
		return
	}
	var req struct {
		Code     string `json:"code"`
		NewEmail string `json:"newEmail"`
	}
	if e := c.ShouldBindJSON(&req); e != nil || strings.TrimSpace(req.Code) == "" || strings.TrimSpace(req.NewEmail) == "" {
		c.JSON(http.StatusOK, dto.Fail[any]("参数错误"))
		return
	}
	if user.EmailVerifyExpires == nil || time.Now().After(*user.EmailVerifyExpires) {
		c.JSON(http.StatusOK, dto.Fail[any]("验证码已过期"))
		return
	}
	if strings.TrimSpace(req.Code) != strings.TrimSpace(user.EmailVerifyCode) {
		c.JSON(http.StatusOK, dto.Fail[any]("验证码错误"))
		return
	}
	// 第一步验证通过旧邮箱验证码，进入第二步：向新邮箱发送验证码并挂起
	newEmail := strings.TrimSpace(req.NewEmail)
	code2 := fmt.Sprintf("%06d", time.Now().UnixNano()%1000000)
	exp2 := time.Now().Add(10 * time.Minute)
	user.EmailPending = newEmail
	user.EmailVerified = false
	user.EmailVerifyCode = code2
	user.EmailVerifyExpires = &exp2
	if e := repository.UpdateUser(user); e != nil {
		c.JSON(http.StatusOK, dto.Fail[any]("保存失败"))
		return
	}
	if e := models.SendEmail(newEmail, "新邮箱验证", "验证码: "+code2+"，10分钟内有效"); e != nil {
		c.JSON(http.StatusOK, dto.Fail[any](e.Error()))
		return
	}
	c.JSON(http.StatusOK, dto.OK[any](nil, "已向新邮箱发送验证码，请完成验证"))
}

// 删除用户
func DeleteUser(c *gin.Context) {
	currentID, err := checkAdmin(c)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](err.Error()))
		return
	}

	userIDStr := c.Query("id")
	if userIDStr == "" {
		c.JSON(http.StatusOK, dto.Fail[string]("缺少用户ID"))
		return
	}
	id, _ := strconv.Atoi(userIDStr)

	// 不允许删除自己
	if uint(id) == currentID {
		c.JSON(http.StatusOK, dto.Fail[string]("不允许删除当前登录用户"))
		return
	}
	// 至少保留一位管理员：当删除目标是管理员时检查数量
	target, err := repository.GetUserByID(uint(id))
	if err == nil && target.IsAdmin {
		cnt, err := repository.CountAdmins()
		if err != nil {
			c.JSON(http.StatusOK, dto.Fail[string]("校验管理员数量失败"))
			return
		}
		if cnt <= 1 {
			c.JSON(http.StatusOK, dto.Fail[string]("系统至少保留一位管理员，无法删除最后一位管理员"))
			return
		}
	}
	if err := repository.DeleteUser(uint(id)); err != nil {
		c.JSON(http.StatusOK, dto.Fail[string]("删除失败"))
		return
	}
	c.JSON(http.StatusOK, dto.OK[any](nil, "已删除用户"))
}

// 管理员重置任意用户密码
func AdminResetPassword(c *gin.Context) {
	_, err := checkAdmin(c)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](err.Error()))
		return
	}
	var req struct {
		ID       uint   `json:"id"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.ID == 0 || strings.TrimSpace(req.Password) == "" {
		c.JSON(http.StatusOK, dto.Fail[string](models.InvalidRequestBodyMessage))
		return
	}
	user, err := services.GetUserByID(req.ID)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](models.UserNotFoundMessage))
		return
	}
	if err := services.ChangePassword(user, dto.UserInfoDto{Password: req.Password}); err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](err.Error()))
		return
	}
	c.JSON(http.StatusOK, dto.OK[any](nil, models.ChangePasswordSuccessMessage))
}
func sendTelegramErrorNotify(c *gin.Context, err error) {
	log.Printf("Telegram 推送失败: %v", err)
}
func computeUpgradeInfo() (bool, string, string, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	type tagInfo struct{ Name, LastUpdated string }
	latest := tagInfo{}
	get := func(url string, v any) error {
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		req = req.WithContext(ctx)
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		return json.NewDecoder(resp.Body).Decode(v)
	}
	type result struct {
		ok   bool
		info tagInfo
	}
	ch := make(chan result, 3)
	go func() {
		var v struct {
			Name, LastUpdated string `json:"name" json:"last_updated"`
		}
		if get("https://hub.docker.com/v2/repositories/noise233/echo-noise/tags/latest", &v) == nil && strings.TrimSpace(v.LastUpdated) != "" {
			ch <- result{true, tagInfo{v.Name, v.LastUpdated}}
			return
		}
		ch <- result{false, tagInfo{}}
	}()
	go func() {
		var v struct {
			Results []struct {
				Name, LastUpdated string `json:"name" json:"last_updated"`
			} `json:"results"`
		}
		if get("https://hub.docker.com/v2/repositories/noise233/echo-noise/tags?page_size=1&ordering=last_updated", &v) == nil && len(v.Results) > 0 && strings.TrimSpace(v.Results[0].LastUpdated) != "" {
			r := v.Results[0]
			ch <- result{true, tagInfo{r.Name, r.LastUpdated}}
			return
		}
		ch <- result{false, tagInfo{}}
	}()
	go func() {
		var v struct {
			TagName, PublishedAt string `json:"tag_name" json:"published_at"`
		}
		if get("https://api.github.com/repos/noise233/echo-noise/releases/latest", &v) == nil && strings.TrimSpace(v.PublishedAt) != "" {
			ch <- result{true, tagInfo{v.TagName, v.PublishedAt}}
			return
		}
		ch <- result{false, tagInfo{}}
	}()
	for i := 0; i < 3; i++ {
		r := <-ch
		if r.ok {
			latest = r.info
			break
		}
	}
	if strings.TrimSpace(latest.LastUpdated) == "" {
		cur := strings.TrimSpace(os.Getenv("ECHO_NOISE_VERSION"))
		if cur == "" {
			cur = strings.TrimSpace(os.Getenv("APP_VERSION"))
		}
		if cur == "" {
			cur = strings.TrimSpace(os.Getenv("IMAGE_TAG"))
		}
		if cur == "" {
			cur = "latest"
		}
		return false, time.Now().Format(time.RFC3339), cur, nil
	}
	cur := strings.TrimSpace(os.Getenv("ECHO_NOISE_VERSION"))
	if cur == "" {
		cur = strings.TrimSpace(os.Getenv("APP_VERSION"))
	}
	if cur == "" {
		cur = strings.TrimSpace(os.Getenv("IMAGE_TAG"))
	}
	if cur == "" {
		cur = "latest"
	}
	var curUpdated string
	if strings.ToLower(cur) == "latest" {
		curUpdated = strings.TrimSpace(latest.LastUpdated)
	} else {
		if resp, err := client.Get("https://hub.docker.com/v2/repositories/noise233/echo-noise/tags/" + cur); err == nil {
			defer resp.Body.Close()
			var curTag struct {
				Name, LastUpdated string `json:"name" json:"last_updated"`
			}
			if json.NewDecoder(resp.Body).Decode(&curTag) == nil {
				curUpdated = strings.TrimSpace(curTag.LastUpdated)
			}
		}
		if strings.TrimSpace(curUpdated) == "" {
			if resp, err := client.Get("https://api.github.com/repos/noise233/echo-noise/releases/tags/" + cur); err == nil {
				defer resp.Body.Close()
				var rel struct {
					PublishedAt string `json:"published_at"`
				}
				if json.NewDecoder(resp.Body).Decode(&rel) == nil {
					curUpdated = strings.TrimSpace(rel.PublishedAt)
				}
			}
		}
	}
	latestTime, err := time.Parse(time.RFC3339, latest.LastUpdated)
	if err != nil {
		return false, "", cur, err
	}
	var hasUpdate bool
	if curUpdated != "" {
		curTime, err := time.Parse(time.RFC3339, curUpdated)
		if err != nil {
			return false, "", cur, err
		}
		hasUpdate = latestTime.After(curTime)
	} else {
		hasUpdate = true
	}
	return hasUpdate, latest.LastUpdated, cur, nil
}
func SyncStatic(c *gin.Context) {
	if _, err := checkAdmin(c); err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](err.Error()))
		return
	}
	wd, _ := os.Getwd()
	webDir := filepath.Join(wd, "web")
	outDir := filepath.Join(webDir, ".output", "public")
	{
		var stderr bytes.Buffer
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		cmd := exec.CommandContext(ctx, "bash", "-lc", "cd web && npm run generate")
		cmd.Env = os.Environ()
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			msg := strings.TrimSpace(stderr.String())
			if msg == "" {
				msg = err.Error()
			}
			c.JSON(http.StatusOK, dto.Fail[string]("前端构建失败: "+msg))
			return
		}
	}
	pubDir := filepath.Join(wd, "public")
	_ = os.RemoveAll(pubDir)
	_ = os.MkdirAll(pubDir, 0755)
	if _, err := exec.LookPath("rsync"); err == nil {
		var stderr bytes.Buffer
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		cmd := exec.CommandContext(ctx, "bash", "-lc", "rsync -a --delete '"+outDir+"/' '"+pubDir+"/'")
		cmd.Env = os.Environ()
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			msg := strings.TrimSpace(stderr.String())
			if msg == "" {
				msg = err.Error()
			}
			c.JSON(http.StatusOK, dto.Fail[string]("静态资源同步失败: "+msg))
			return
		}
	} else {
		if err := copyDir(outDir, pubDir); err != nil {
			c.JSON(http.StatusOK, dto.Fail[string]("静态资源同步失败: "+err.Error()))
			return
		}
	}
	c.JSON(http.StatusOK, dto.OK[any](gin.H{"public": pubDir}, "静态资源已同步"))
}

func GetRuntimeEnv(c *gin.Context) {
	isContainer := func() bool {
		if _, err := os.Stat("/.dockerenv"); err == nil {
			return true
		}
		b, _ := os.ReadFile("/proc/1/cgroup")
		s := strings.ToLower(string(b))
		if strings.Contains(s, "docker") || strings.Contains(s, "containerd") || strings.Contains(s, "kubepods") {
			return true
		}
		return false
	}()
	wd, _ := os.Getwd()
	outDir := filepath.Join(wd, "web", ".output", "public")
	pubDir := filepath.Join(wd, "public")
	c.JSON(http.StatusOK, gin.H{
		"code": 1,
		"data": gin.H{
			"isContainer":         isContainer,
			"staticSyncAvailable": !isContainer,
			"outDir":              outDir,
			"publicDir":           pubDir,
		},
	})
}
