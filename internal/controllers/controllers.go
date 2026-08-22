package controllers

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/internal/authorization"
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

func currentMessageViewer(c *gin.Context) (*uint, bool) {
	if user, ok := currentReadUser(c); ok {
		id := user.ID
		return &id, user.IsAdmin
	}
	return nil, false
}

func messageMutationAuditRecord(c *gin.Context, actorID uint, capability authorization.Capability, action, result, summary string, message *models.Message) models.AdminAuditLog {
	ownerID := message.UserID
	return models.AdminAuditLog{
		ActorUserID:       actorID,
		Capability:        string(capability),
		Module:            "notes",
		Action:            action,
		TargetType:        "message",
		TargetID:          fmt.Sprint(message.ID),
		TargetOwnerUserID: &ownerID,
		Result:            result,
		Summary:           summary,
		IP:                c.ClientIP(),
		UserAgent:         c.GetHeader("User-Agent"),
		AuthVia:           c.GetString("auth_via"),
	}
}

func messageMutationDeniedAuditRecord(c *gin.Context, actorID uint, capability authorization.Capability, action string, message *models.Message, reason authorization.DenialReason) models.AdminAuditLog {
	record := messageMutationAuditRecord(c, actorID, capability, action, "denied", "capability request denied", message)
	if reason != authorization.DenialNone {
		record.Reason = string(reason)
	}
	if reason == authorization.DenialContentNotReadable || reason == authorization.DenialProtectedContent {
		record.TargetID = ""
		record.TargetOwnerUserID = nil
	}
	return record
}

func writeMessageMutationDeniedAudit(c *gin.Context, authorizer *authorization.Authorizer, actorID uint, capability authorization.Capability, action string, message *models.Message) {
	decision := authorizer.Authorize(actorID, capability, nil)
	if decision.Reason == authorization.DenialNotAdministrator {
		return
	}
	authorizer.WriteDeniedBestEffort(messageMutationDeniedAuditRecord(c, actorID, capability, action, message, decision.Reason))
}

func writeMessageMutationDeniedAuditForDecision(c *gin.Context, authorizer *authorization.Authorizer, actorID uint, capability authorization.Capability, action string, message *models.Message, decision authorization.Decision) {
	if decision.Reason == authorization.DenialNotAdministrator {
		return
	}
	authorizer.WriteDeniedBestEffort(messageMutationDeniedAuditRecord(c, actorID, capability, action, message, decision.Reason))
}

func writeMessageMutationDeniedAuditForError(c *gin.Context, authorizer *authorization.Authorizer, actorID uint, capability authorization.Capability, action string, message *models.Message, cause error) {
	if decision := authorizer.Authorize(actorID, capability, nil); decision.Reason == authorization.DenialNotAdministrator {
		return
	}
	if errors.Is(cause, services.ErrMessageNotVisible) {
		authorizer.WriteDeniedBestEffort(messageMutationDeniedAuditRecord(c, actorID, capability, action, message, authorization.DenialContentNotReadable))
		return
	}
	if errors.Is(cause, services.ErrMessageProtected) {
		authorizer.WriteDeniedBestEffort(messageMutationDeniedAuditRecord(c, actorID, capability, action, message, authorization.DenialProtectedContent))
		return
	}
	writeMessageMutationDeniedAudit(c, authorizer, actorID, capability, action, message)
}

func writeMessageMutationSuccessAudit(c *gin.Context, authorizer *authorization.Authorizer, actorID uint, capability authorization.Capability, action string, message *models.Message) error {
	return authorizer.WriteAudit(messageMutationAuditRecord(c, actorID, capability, action, "success", "message mutation completed", message))
}

// currentReadUser resolves identity for public read endpoints. Session/context
// identity wins; a valid Bearer token is a read-only fallback and is not copied
// into Gin context, so it cannot accidentally authorize session-protected writes.
func currentReadUser(c *gin.Context) (models.User, bool) {
	if value, exists := c.Get("user_id"); exists {
		uid, ok := commentUint(value)
		if !ok || uid == 0 {
			return models.User{}, false
		}
		if user, err := services.GetUserByID(uid); err == nil && user != nil && user.ID != 0 {
			return *user, true
		}
	}

	session := sessions.Default(c)
	if uid, ok := commentUint(session.Get("user_id")); ok && uid > 0 {
		if user, err := services.GetUserByID(uid); err == nil && user != nil && user.ID != 0 {
			expireAt := parseReadSessionExpireAt(session.Get("login_expire_at"))
			issuedAt := parseReadSessionExpireAt(session.Get("login_issued_at"))
			if services.IsUserLoginExpired(user, issuedAt, time.Now()) || (issuedAt <= 0 && expireAt > 0 && time.Now().Unix() > expireAt) {
				session.Clear()
				_ = session.Save()
			} else {
				return *user, true
			}
		}
	}

	parts := strings.Fields(strings.TrimSpace(c.GetHeader("Authorization")))
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return models.User{}, false
	}
	token := strings.TrimSpace(parts[1])
	if token == "" || strings.EqualFold(token, "null") {
		return models.User{}, false
	}
	db, err := database.GetDB()
	if err != nil {
		return models.User{}, false
	}
	var user models.User
	if err := db.Where("token = ? AND token <> ''", token).First(&user).Error; err != nil || user.ID == 0 {
		return models.User{}, false
	}
	issuedAt := int64(0)
	if user.LoginIssuedAt != nil {
		issuedAt = user.LoginIssuedAt.Unix()
	}
	if services.IsUserLoginExpired(&user, issuedAt, time.Now()) {
		return models.User{}, false
	}
	return user, true
}

func parseReadSessionExpireAt(value interface{}) int64 {
	switch expireAt := value.(type) {
	case int64:
		return expireAt
	case int:
		return int64(expireAt)
	case uint:
		return int64(expireAt)
	case uint64:
		if expireAt > uint64(^uint64(0)>>1) {
			return 0
		}
		return int64(expireAt)
	case float64:
		return int64(expireAt)
	case string:
		parsed, _ := strconv.ParseInt(strings.TrimSpace(expireAt), 10, 64)
		return parsed
	default:
		return 0
	}
}

const (
	defaultLoginExpireDays  = 3
	defaultLoginExpireHours = 0
	maxLoginExpireDays      = 31
	maxLoginExpireHours     = 24
)

func normalizeLoginExpireConfig(days int, hours int) (int, int) {
	if days < 0 {
		days = 0
	}
	if hours < 0 {
		hours = 0
	}
	if days > maxLoginExpireDays {
		return maxLoginExpireDays, maxLoginExpireHours
	}
	if hours > maxLoginExpireHours {
		hours = maxLoginExpireHours
	}
	return days, hours
}

func getLoginExpireDuration() time.Duration {
	return getLoginExpireDurationForUser(nil)
}

func getLoginExpireDurationForUser(user *models.User) time.Duration {
	return services.LoginExpireDurationForUser(user)
}

func applyLoginSessionExpire(session sessions.Session, user *models.User) {
	duration := getLoginExpireDurationForUser(user)
	if duration <= 0 {
		session.Set("login_expire_at", int64(0))
		session.Set("login_issued_at", time.Now().Unix())
		return
	}
	issuedAt := time.Now()
	session.Set("login_issued_at", issuedAt.Unix())
	session.Set("login_expire_at", issuedAt.Add(duration).Unix())
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
	applyLoginSessionExpire(session, user)
	session.Set("user_id", user.ID)
	session.Set("username", user.Username)
	session.Set("is_admin", user.IsAdmin)
	if err := session.Save(); err != nil {
		c.JSON(http.StatusOK, dto.Fail[any]("Session 保存失败"))
		return
	}
	_ = recordUserLoginAudit(c, user, loginAuditActionLogin)

	c.JSON(http.StatusOK, dto.OK(user, "登录成功"))
}

// 添加登出功能
func Logout(c *gin.Context) {
	session := sessions.Default(c)
	recordSessionLogoutAudit(c, session)
	session.Clear()
	session.Save()
	c.JSON(http.StatusOK, dto.OK[any](nil, "登出成功"))
}

func recordSessionLogoutAudit(c *gin.Context, session sessions.Session) {
	if session == nil {
		return
	}
	userID, ok := commentUint(session.Get("user_id"))
	if !ok || userID == 0 {
		return
	}
	username := ""
	if v := session.Get("username"); v != nil {
		username = strings.TrimSpace(fmt.Sprintf("%v", v))
	}
	user := models.User{ID: userID, Username: username}
	if isAdmin, ok := session.Get("is_admin").(bool); ok {
		user.IsAdmin = isAdmin
	}
	if user.Username == "" || !user.IsAdmin {
		if loaded, err := services.GetUserByID(userID); err == nil && loaded != nil {
			user.Username = loaded.Username
			user.IsAdmin = loaded.IsAdmin
		}
	}
	_ = recordUserLoginAudit(c, &user, loginAuditActionLogout)
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

	result, err := services.RegisterWithResult(user)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](err.Error()))
		return
	}

	message := models.RegisterSuccessMessage
	if result.AutoApproved {
		message = models.RegisterAutoApprovedMessage
	}
	c.JSON(http.StatusOK, dto.OK(result, message))
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
	currentUserID, isAdmin := currentMessageViewer(c)
	messages, err := services.GetAllMessagesForViewer(currentUserID, isAdmin)
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

	currentUserID, isAdmin := currentMessageViewer(c)
	message, err := services.GetMessageByIDForViewer(uint(id), currentUserID, isAdmin)
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

func LocateMessagePage(c *gin.Context) {
	var request dto.MessagePageLocateDto
	_ = c.ShouldBindJSON(&request)

	if request.MessageID == 0 {
		idStr := strings.TrimSpace(c.Query("messageId"))
		if idStr == "" {
			idStr = strings.TrimSpace(c.Query("id"))
		}
		if id, err := strconv.ParseUint(idStr, 10, 64); err == nil {
			request.MessageID = uint(id)
		}
	}
	if request.MessageID == 0 {
		c.JSON(http.StatusOK, dto.Fail[string](models.InvalidIDMessage))
		return
	}
	if request.PageSize == 0 {
		if sizeStr := c.Query("pageSize"); sizeStr != "" {
			if size, err := strconv.Atoi(sizeStr); err == nil {
				request.PageSize = size
			}
		}
	}
	if request.AuthorID == nil {
		if aid := c.Query("authorId"); aid != "" {
			if v, err := strconv.ParseUint(aid, 10, 64); err == nil {
				vv := uint(v)
				request.AuthorID = &vv
			}
		}
	}
	if request.ExcludeID == nil {
		if eid := c.Query("excludeId"); eid != "" {
			if v, err := strconv.ParseUint(eid, 10, 64); err == nil {
				vv := uint(v)
				request.ExcludeID = &vv
			}
		}
	}
	if request.Username == nil {
		if un := strings.TrimSpace(c.Query("username")); un != "" {
			request.Username = &un
		}
	}
	if queryDate := strings.TrimSpace(c.Query("date")); queryDate != "" {
		request.Date = queryDate
	} else {
		request.Date = strings.TrimSpace(request.Date)
	}
	if keyword := strings.TrimSpace(c.Query("keyword")); keyword != "" {
		request.Keyword = keyword
	} else {
		request.Keyword = strings.TrimSpace(request.Keyword)
	}
	if tag := strings.TrimSpace(c.Query("tag")); tag != "" {
		request.Tag = strings.TrimPrefix(tag, "#")
	} else {
		request.Tag = strings.TrimPrefix(strings.TrimSpace(request.Tag), "#")
	}
	if pinScope := strings.TrimSpace(c.Query("pinScope")); pinScope != "" {
		request.PinScope = pinScope
	} else {
		request.PinScope = strings.TrimSpace(request.PinScope)
	}

	currentUserID, isAdmin := currentMessageViewer(c)
	if strings.EqualFold(request.PinScope, services.MessagePinScopePersonal) && (currentUserID == nil || request.AuthorID == nil || *currentUserID != *request.AuthorID) {
		c.JSON(http.StatusForbidden, dto.Fail[string]("个人作用域仅允许查询当前用户的笔记"))
		return
	}
	location, err := services.LocateMessagePage(request.MessageID, request.PageSize, currentUserID, isAdmin, request.AuthorID, request.Username, &request.Date, &request.Keyword, &request.Tag, request.PinScope, request.ExcludeID)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](err.Error()))
		return
	}
	c.JSON(http.StatusOK, dto.OK(location, models.GetMessagesByPageSuccess))
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

	// Session、Bearer 与 Token 路由统一通过同一个实时身份解析器，
	// 避免过期 Session 抢占有效 Bearer 身份。
	currentUserID, isAdmin := currentMessageViewer(c)

	// 作者筛选（可选）
	var authorID *uint
	if aid := c.Query("authorId"); aid != "" {
		if v, err := strconv.ParseUint(aid, 10, 64); err == nil {
			vv := uint(v)
			authorID = &vv
		}
	}
	if authorID == nil && pageRequest.AuthorID != nil {
		authorID = pageRequest.AuthorID
	}
	if pageRequest.ExcludeID == nil {
		if eid := c.Query("excludeId"); eid != "" {
			if v, err := strconv.ParseUint(eid, 10, 64); err == nil {
				vv := uint(v)
				pageRequest.ExcludeID = &vv
			}
		}
	}
	var username *string
	if un := c.Query("username"); strings.TrimSpace(un) != "" {
		u := strings.TrimSpace(un)
		username = &u
	}
	if username == nil && pageRequest.Username != nil && strings.TrimSpace(*pageRequest.Username) != "" {
		u := strings.TrimSpace(*pageRequest.Username)
		username = &u
	}
	if queryDate := strings.TrimSpace(c.Query("date")); queryDate != "" {
		pageRequest.Date = queryDate
	} else {
		pageRequest.Date = strings.TrimSpace(pageRequest.Date)
	}
	if keyword := strings.TrimSpace(c.Query("keyword")); keyword != "" {
		pageRequest.Keyword = keyword
	} else {
		pageRequest.Keyword = strings.TrimSpace(pageRequest.Keyword)
	}
	if tag := strings.TrimSpace(c.Query("tag")); tag != "" {
		pageRequest.Tag = strings.TrimPrefix(tag, "#")
	} else {
		pageRequest.Tag = strings.TrimPrefix(strings.TrimSpace(pageRequest.Tag), "#")
	}
	if pinScope := strings.TrimSpace(c.Query("pinScope")); pinScope != "" {
		pageRequest.PinScope = pinScope
	} else {
		pageRequest.PinScope = strings.TrimSpace(pageRequest.PinScope)
	}
	if strings.EqualFold(pageRequest.PinScope, services.MessagePinScopePersonal) && (currentUserID == nil || authorID == nil || *currentUserID != *authorID) {
		c.JSON(http.StatusForbidden, dto.Fail[string]("个人作用域仅允许查询当前用户的笔记"))
		return
	}

	pageQueryResult, err := services.GetMessagesByPage(page, pageSize, currentUserID, isAdmin, authorID, username, &pageRequest.Date, &pageRequest.Keyword, &pageRequest.Tag, pageRequest.PinScope, pageRequest.ExcludeID)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](err.Error()))
		return
	}

	c.JSON(http.StatusOK, dto.OK(pageQueryResult, models.GetMessagesByPageSuccess))
}
func GetStatus(c *gin.Context) {
	currentUserID, _ := currentMessageViewer(c)
	viewerID := uint(0)
	if currentUserID != nil {
		viewerID = *currentUserID
	}
	status, err := services.GetStatus(viewerID)
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
	currentUserID, isAdmin := currentMessageViewer(c)
	q := services.ApplyMessageVisibilityScope(database.DB.Model(&models.Message{}), currentUserID, isAdmin).
		Where("user_id = ?", user.ID).
		Where(services.GuestbookSQLPredicate("messages.is_guestbook")).
		Where("content NOT LIKE ? AND content NOT LIKE ? AND content NOT LIKE ? AND content NOT LIKE ?",
			"%#友链%", "%友情链接%",
			"%#关于%", "%关于本站%")
	if err := q.Count(&total).Error; err != nil {
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

	actorID, ok := commentAuthUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, dto.Fail[string]("未授权访问"))
		return
	}
	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 0, "msg": err.Error()})
		return
	}
	var message models.Message
	if err := db.First(&message, uint(messageID)).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 0, "msg": "消息不存在"})
		return
	}
	authorizer := authorization.New(db)
	isOwner := message.UserID == actorID
	if err := services.TrashMessage(db, actorID, uint(messageID), "author request"); err != nil {
		if !isOwner {
			writeMessageMutationDeniedAuditForError(c, authorizer, actorID, authorization.CapabilityNotesTrash, "trash", &message, err)
		}
		switch {
		case errors.Is(err, services.ErrMessageNotVisible), errors.Is(err, services.ErrMessageProtected), errors.Is(err, services.ErrMessageNotFound):
			c.JSON(http.StatusNotFound, gin.H{"code": 0, "msg": "消息不存在"})
		case errors.Is(err, services.ErrMessageNotAuthorized):
			c.JSON(http.StatusForbidden, gin.H{"code": 0, "msg": "无权删除此消息"})
		case errors.Is(err, services.ErrMessageAlreadyTrashed):
			c.JSON(http.StatusConflict, gin.H{"code": 0, "msg": "消息状态不允许执行此操作"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"code": 0, "msg": "删除失败"})
		}
		return
	}
	if !isOwner {
		if err := writeMessageMutationSuccessAudit(c, authorizer, actorID, authorization.CapabilityNotesTrash, "trash", &message); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 0, "msg": "写入管理员审计失败"})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "删除成功"})
}

func GenerateRSS(c *gin.Context) {
	rss, err := services.GenerateRSS(c)
	if err != nil {
		if err == services.ErrRSSDisabled {
			c.JSON(http.StatusNotFound, gin.H{"code": 0, "msg": "RSS 已禁用"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"code": 0, "msg": err.Error()})
		return
	}

	c.Header("Content-Type", "application/rss+xml; charset=utf-8")
	c.String(http.StatusOK, rss)
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

	safeUser := *user
	safeUser.Password = ""
	c.JSON(http.StatusOK, dto.OK(safeUser, models.UpdateUserSuccessMessage))
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
	id, ok := commentUint(userID)
	if !ok || id == 0 {
		return 0, fmt.Errorf("未授权访问")
	}
	db, err := database.GetDB()
	if err != nil {
		return 0, err
	}
	var user models.User
	if err := db.Select("id,is_admin").First(&user, id).Error; err != nil {
		return 0, err
	}
	if !user.IsAdmin {
		return 0, fmt.Errorf("需要管理员权限")
	}
	return id, nil
}

func requirePrimaryAdmin(c *gin.Context) (uint, error) {
	userID, err := checkAdmin(c)
	if err != nil {
		return 0, err
	}
	if userID != models.PrimaryAdminUserID {
		return 0, fmt.Errorf("仅 1 号管理员可管理 VoceChat 配置")
	}
	return userID, nil
}

func UpdateUserAdmin(c *gin.Context) {
	_, err := requirePrimaryAdmin(c)
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

type registrationApplicationReviewRequest struct {
	Note string `json:"note"`
}

func ListRegistrationApplications(c *gin.Context) {
	viewerUserID, err := checkAdmin(c)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](err.Error()))
		return
	}

	status := strings.TrimSpace(c.Query("status"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	result, err := services.ListRegistrationApplicationsForViewer(viewerUserID, status, limit, offset)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[string]("获取注册申请失败: "+err.Error()))
		return
	}
	c.JSON(http.StatusOK, dto.OK(result, models.QuerySuccessMessage))
}

func ApproveRegistrationApplication(c *gin.Context) {
	reviewerUserID, err := checkAdmin(c)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](err.Error()))
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusOK, dto.Fail[string](models.InvalidIDMessage))
		return
	}
	var req registrationApplicationReviewRequest
	_ = c.ShouldBindJSON(&req)
	user, err := services.ApproveRegistrationApplication(uint(id), reviewerUserID, req.Note)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](err.Error()))
		return
	}
	user.Password = ""
	c.JSON(http.StatusOK, dto.OK(user, models.UpdateUserSuccessMessage))
}

func RejectRegistrationApplication(c *gin.Context) {
	reviewerUserID, err := checkAdmin(c)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](err.Error()))
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusOK, dto.Fail[string](models.InvalidIDMessage))
		return
	}
	var req registrationApplicationReviewRequest
	_ = c.ShouldBindJSON(&req)
	if err := services.RejectRegistrationApplication(uint(id), reviewerUserID, req.Note); err != nil {
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

func hasAdminOnlySettingFields(setting dto.SettingDto) bool {
	return setting.AllowRegistration != nil ||
		setting.AutoApproveRegistration != nil ||
		setting.SmtpEnabled != nil ||
		setting.SmtpDriver != nil ||
		setting.SmtpHost != nil ||
		setting.SmtpPort != nil ||
		setting.SmtpUser != nil ||
		setting.SmtpPass != nil ||
		setting.ClearSmtpUser != nil ||
		setting.ClearSmtpPass != nil ||
		setting.SmtpFrom != nil ||
		setting.SmtpEncryption != nil ||
		setting.SmtpTLS != nil ||
		setting.StorageEnabled != nil ||
		setting.StorageConfig != nil ||
		setting.AttachmentStorageEnabled != nil ||
		setting.AttachmentStorageConfig != nil ||
		setting.RecycleBinRetentionDays != nil ||
		setting.VoceChatConfig != nil
}

func hasRSSManagementSettings(frontendSettings map[string]interface{}) bool {
	for _, field := range []string{
		"rssEnabled",
		"rssMemberIDs",
		"rssTitle",
		"rssDescription",
		"rssAuthorName",
		"rssFaviconURL",
	} {
		if _, exists := frontendSettings[field]; exists {
			return true
		}
	}
	return false
}

func hasLoginExpirySettings(frontendSettings map[string]interface{}) bool {
	for _, field := range []string{
		"loginExpireDays", "loginExpireHours",
		"delegatedAdminLoginExpireDays", "delegatedAdminLoginExpireHours",
	} {
		if _, exists := frontendSettings[field]; exists {
			return true
		}
	}
	return false
}

type widgetPreferencesRequest struct {
	FrontendSettings map[string]interface{} `json:"frontendSettings"`
}

func GetWidgetPreferences(c *gin.Context) {
	user, err := checkUser(c)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[any](err.Error()))
		return
	}
	config, err := services.GetFrontendConfig(user.ID)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[any]("获取小组件设置失败"))
		return
	}
	c.JSON(http.StatusOK, dto.OK(config["frontendSettings"], "获取小组件设置成功"))
}

func UpdateWidgetPreferences(c *gin.Context) {
	user, err := checkUser(c)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[any](err.Error()))
		return
	}
	var request widgetPreferencesRequest
	if err := c.ShouldBindJSON(&request); err != nil || !services.IsUserFrontendSettingsOnly(request.FrontendSettings) {
		c.JSON(http.StatusOK, dto.Fail[any]("小组件设置格式无效"))
		return
	}
	if err := services.UpdateUserWidgetPreferences(user.ID, request.FrontendSettings); err != nil {
		c.JSON(http.StatusOK, dto.Fail[any]("保存我的小组件失败: "+err.Error()))
		return
	}
	c.JSON(http.StatusOK, dto.OK[any](nil, "我的小组件已保存"))
}

func GetGuestWidgetPreferences(c *gin.Context) {
	if _, err := requirePrimaryAdmin(c); err != nil {
		c.JSON(http.StatusOK, dto.Fail[any](err.Error()))
		return
	}
	config, err := services.GetFrontendConfig(0)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[any]("获取访客默认失败"))
		return
	}
	c.JSON(http.StatusOK, dto.OK(config["frontendSettings"], "获取访客默认成功"))
}

func UpdateGuestWidgetPreferences(c *gin.Context) {
	if _, err := requirePrimaryAdmin(c); err != nil {
		c.JSON(http.StatusOK, dto.Fail[any](err.Error()))
		return
	}
	var request widgetPreferencesRequest
	if err := c.ShouldBindJSON(&request); err != nil || !services.IsGuestWidgetSettingsOnly(request.FrontendSettings) {
		c.JSON(http.StatusOK, dto.Fail[any]("访客默认小组件设置格式无效"))
		return
	}
	if err := services.UpdateGuestWidgetPreferences(request.FrontendSettings); err != nil {
		c.JSON(http.StatusOK, dto.Fail[any]("保存访客默认失败: "+err.Error()))
		return
	}
	c.JSON(http.StatusOK, dto.OK[any](nil, "访客默认已保存"))
}

func UpdateSetting(c *gin.Context) {
	user, err := checkUser(c)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](err.Error()))
		return
	}

	var setting dto.SettingDto
	if err := c.ShouldBindJSON(&setting); err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](models.InvalidRequestBodyMessage))
		return
	}

	frontendSettings := setting.FrontendSettings
	if hasRSSManagementSettings(frontendSettings) && user.ID != models.PrimaryAdminUserID {
		c.JSON(http.StatusOK, dto.Fail[string]("仅 1 号管理员可管理 RSS"))
		return
	}
	if hasLoginExpirySettings(frontendSettings) && user.ID != models.PrimaryAdminUserID {
		c.JSON(http.StatusOK, dto.Fail[string]("仅 1 号管理员可管理登录过期时间"))
		return
	}
	if !user.IsAdmin {
		if hasAdminOnlySettingFields(setting) || frontendSettings == nil || !services.IsUserFrontendSettingsOnly(frontendSettings) {
			c.JSON(http.StatusOK, dto.Fail[string]("需要管理员权限"))
			return
		}
		if err := services.UpdateUserLifeCountdownConfig(user.ID, frontendSettings); err != nil {
			c.JSON(http.StatusOK, dto.Fail[string]("保存人生倒计时配置失败: "+err.Error()))
			return
		}
		if err := services.UpdateUserFrontendPreferenceConfig(user.ID, frontendSettings); err != nil {
			c.JSON(http.StatusOK, dto.Fail[string]("保存个人界面配置失败: "+err.Error()))
			return
		}
		c.JSON(http.StatusOK, dto.OK[any](nil, models.UpdateSettingSuccessMessage))
		return
	}

	db, _ := database.GetDB()
	var oldSetting models.Setting
	if err := db.Table("settings").First(&oldSetting).Error; err != nil {
		c.JSON(http.StatusOK, dto.Fail[string]("读取原有配置失败"))
		return
	}
	var oldSiteConfig models.SiteConfig
	_ = db.Table("site_configs").First(&oldSiteConfig).Error
	oldRetention := oldSiteConfig.RecycleBinRetentionDays

	if setting.AllowRegistration != nil {
		oldSetting.AllowRegistration = *setting.AllowRegistration
	}
	if setting.AutoApproveRegistration != nil {
		oldSetting.AutoApproveRegistration = *setting.AutoApproveRegistration
	}

	settingMap := map[string]interface{}{}
	hasSiteConfigUpdate := false
	if setting.RecycleBinRetentionDays != nil {
		if user.ID != models.PrimaryAdminUserID {
			c.JSON(http.StatusOK, dto.Fail[string]("仅 1 号管理员可管理回收站自动清理"))
			return
		}
		allowed := map[int]bool{0: true, 7: true, 30: true, 90: true, 180: true, 365: true}
		if !allowed[*setting.RecycleBinRetentionDays] {
			c.JSON(http.StatusOK, dto.Fail[string]("回收站保留期限无效"))
			return
		}
		settingMap["recycleBinRetentionDays"] = *setting.RecycleBinRetentionDays
		hasSiteConfigUpdate = true
	}
	if frontendSettings != nil {
		if services.HasLifeCountdownSettings(frontendSettings) {
			if err := services.UpdateUserLifeCountdownConfig(user.ID, frontendSettings); err != nil {
				c.JSON(http.StatusOK, dto.Fail[string]("保存人生倒计时配置失败: "+err.Error()))
				return
			}
			frontendSettings = services.StripLifeCountdownSettings(frontendSettings)
		}
		if len(frontendSettings) > 0 {
			settingMap["frontendSettings"] = frontendSettings
			hasSiteConfigUpdate = true
		}
	}
	if setting.AllowRegistration != nil {
		settingMap["allowRegistration"] = *setting.AllowRegistration
	}
	if setting.AutoApproveRegistration != nil {
		settingMap["autoApproveRegistration"] = *setting.AutoApproveRegistration
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
	if setting.ClearSmtpUser != nil {
		settingMap["clearSmtpUser"] = *setting.ClearSmtpUser
		hasSiteConfigUpdate = true
	}
	if setting.ClearSmtpPass != nil {
		settingMap["clearSmtpPass"] = *setting.ClearSmtpPass
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
	if setting.VoceChatConfig != nil {
		if user.ID != models.PrimaryAdminUserID {
			c.JSON(http.StatusOK, dto.Fail[string]("仅 1 号管理员可管理 VoceChat 配置"))
			return
		}
		settingMap["voceChatConfig"] = setting.VoceChatConfig
		hasSiteConfigUpdate = true
	}

	if hasSiteConfigUpdate {
		if err := services.UpdateFrontendSetting(0, settingMap); err != nil {
			c.JSON(http.StatusOK, dto.Fail[string]("保存前端配置失败: "+err.Error()))
			return
		}
		if setting.RecycleBinRetentionDays != nil {
			changes, _ := json.Marshal(map[string]int{"from": oldRetention, "to": *setting.RecycleBinRetentionDays})
			authorization.New(db).WriteAuditBestEffort(models.AdminAuditLog{
				ActorUserID: user.ID,
				Capability:  string(authorization.CapabilitySiteSettingsManage),
				Module:      "notes",
				Action:      "update_recycle_retention",
				TargetType:  "recycle_bin_policy",
				TargetID:    "retention_days",
				Result:      "success",
				Summary:     "updated recycle-bin retention policy",
				ChangesJSON: string(changes),
			})
		}
	}

	if err := db.Table("settings").Save(&oldSetting).Error; err != nil {
		c.JSON(http.StatusOK, dto.Fail[string]("保存配置失败"))
		return
	}

	if setting.AttachmentStorageEnabled != nil || setting.AttachmentStorageConfig != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
			defer cancel()
			if err := services.MigrateLegacyCloudAttachments(ctx); err != nil {
				log.Printf("历史云附件安全迁移暂未完成，将自动重试: %v", err)
			}
		}()
	}
	c.JSON(http.StatusOK, dto.OK[any](nil, models.UpdateSettingSuccessMessage))
}

func GetFrontendConfig(c *gin.Context) {
	viewerUserID := uint(0)
	if user, ok := currentReadUser(c); ok {
		viewerUserID = user.ID
	}

	config, err := services.GetFrontendConfig(viewerUserID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "获取配置失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 1, "data": config})
}

func CheckVoceChatHealth(c *gin.Context) {
	if _, err := requirePrimaryAdmin(c); err != nil {
		c.JSON(http.StatusOK, dto.Fail[map[string]interface{}](err.Error()))
		return
	}

	config, err := services.CheckVoceChatHealth(c.Request.Context())
	if err != nil {
		if config != nil {
			c.JSON(http.StatusOK, gin.H{"code": 0, "msg": err.Error(), "data": config})
			return
		}
		c.JSON(http.StatusOK, dto.Fail[map[string]interface{}](err.Error()))
		return
	}
	c.JSON(http.StatusOK, dto.OK(config, "VoceChat 健康检查完成"))
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

// 评论账号鉴权：兼容当前登录逻辑写入的 user_id session，保留中间件注入上下文的可能性。
func commentAuthUserID(c *gin.Context) (uint, bool) {
	if user, ok := currentReadUser(c); ok {
		return user.ID, true
	}
	return 0, false
}

func commentAuthIsAdmin(c *gin.Context) bool {
	user, ok := currentReadUser(c)
	return ok && user.IsAdmin
}

func normalizeCommentVisibility(value string) (string, bool) {
	return services.NormalizeCommentVisibility(value)
}

func normalizedCommentVisibilityOrPublic(value string) string {
	return services.NormalizedCommentVisibilityOrPublic(value)
}

func commentVisibilityRank(value string) int {
	return services.CommentVisibilityRank(value)
}

func defaultCommentVisibilityForMessage(messageVisibility string) string {
	return services.DefaultCommentVisibilityForMessage(messageVisibility)
}

func normalizeRequestedCommentVisibility(value string, messageVisibility string) (string, bool) {
	if strings.TrimSpace(value) == "" {
		return defaultCommentVisibilityForMessage(messageVisibility), true
	}
	return normalizeCommentVisibility(value)
}

func commentVisibilityAllowedForMessage(visibility string, messageVisibility string) bool {
	return services.CommentVisibilityAllowedForMessage(visibility, messageVisibility)
}

func effectiveCommentVisibilityForMessage(visibility string, messageVisibility string) string {
	return services.EffectiveCommentVisibilityForMessage(visibility, messageVisibility)
}

func isGuestbookMessage(message models.Message) bool {
	return services.IsGuestbookMessage(message)
}

func canViewComment(message models.Message, comment models.Comment, commentMap map[uint]models.Comment, viewerID uint, hasViewer bool, isAdmin bool) bool {
	return services.CanViewCommentInThread(message, comment, commentMap, viewerID, hasViewer, isAdmin)
}

func authorizeCommentMutation(c *gin.Context, message models.Message, comment models.Comment, commentMap map[uint]models.Comment, capability authorization.Capability) authorization.Decision {
	userID, ok := commentAuthUserID(c)
	if !ok {
		return authorization.Decision{Reason: authorization.DenialNotAdministrator}
	}
	if comment.UserID != nil && *comment.UserID == userID {
		return authorization.Decision{Allowed: true}
	}
	db, err := database.GetDB()
	if err != nil {
		return authorization.Decision{Reason: authorization.DenialContentNotReadable}
	}
	decision := services.AuthorizeCommentMutation(db, userID, message, comment, commentMap, capability)
	if !decision.Allowed && decision.Reason != authorization.DenialNotAdministrator {
		authorization.New(db).WriteDeniedBestEffort(commentMutationAuditRecord(c, userID, capability, "mutation", "denied", "comment mutation denied", string(decision.Reason), &comment))
	}
	return decision
}

func commentUint(v any) (uint, bool) {
	switch val := v.(type) {
	case uint:
		return val, true
	case int:
		if val >= 0 {
			return uint(val), true
		}
	case int64:
		if val >= 0 {
			return uint(val), true
		}
	case float64:
		if val >= 0 {
			return uint(val), true
		}
	case string:
		s := strings.TrimSpace(val)
		if s == "" {
			return 0, false
		}
		n, err := strconv.ParseUint(s, 10, 64)
		if err == nil {
			return uint(n), true
		}
	}
	return 0, false
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
	var message models.Message
	if err := db.First(&message, msgID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 0, "msg": "消息不存在"})
		return
	}
	var comments []models.Comment
	if err := db.Where("message_id = ?", msgID).Order("created_at ASC").Find(&comments).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 0, "msg": "获取评论失败"})
		return
	}
	var actorID *uint
	if user, ok := currentReadUser(c); ok {
		id := user.ID
		actorID = &id
	}
	scope, scopeErr := services.ResolveContentReadScope(db, actorID)
	if scopeErr != nil || !scope.CanReadMessage(message) {
		c.JSON(http.StatusNotFound, gin.H{"code": 0, "msg": "消息不存在"})
		return
	}
	commentMap := services.CommentMap(comments)
	visibleComments := make([]models.Comment, 0, len(comments))
	for _, comment := range comments {
		if scope.CanReadComment(message, comment, commentMap) {
			comment.CanInteract = scope.CanInteractWithComment(message, comment, commentMap)
			visibleComments = append(visibleComments, comment)
		}
	}
	userIDs := make([]uint, 0)
	seenUserIDs := map[uint]bool{}
	for _, comment := range visibleComments {
		if comment.UserID != nil && *comment.UserID > 0 {
			if !seenUserIDs[*comment.UserID] {
				seenUserIDs[*comment.UserID] = true
				userIDs = append(userIDs, *comment.UserID)
			}
		}
	}
	if len(userIDs) > 0 {
		var users []models.User
		if err := db.Select("id, username, avatar_url").Where("id IN ?", userIDs).Find(&users).Error; err == nil {
			userInfo := map[uint]models.CommentUserInfo{}
			for _, user := range users {
				userInfo[user.ID] = models.CommentUserInfo{
					ID:        user.ID,
					Username:  strings.TrimSpace(user.Username),
					AvatarURL: strings.TrimSpace(user.AvatarURL),
				}
			}
			for i := range visibleComments {
				if visibleComments[i].UserID == nil {
					continue
				}
				if info, ok := userInfo[*visibleComments[i].UserID]; ok {
					visibleComments[i].User = &info
				}
			}
		}
	}
	c.JSON(http.StatusOK, gin.H{"code": 1, "data": visibleComments})
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
	var messages []models.Message
	if err := db.Where("id IN ?", req.IDs).Find(&messages).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 0, "msg": "获取评论数量失败"})
		return
	}
	messageMap := make(map[uint]models.Message, len(messages))
	for _, message := range messages {
		messageMap[message.ID] = message
	}
	var comments []models.Comment
	if err := db.Where("message_id IN ?", req.IDs).Order("created_at ASC").Find(&comments).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 0, "msg": "获取评论数量失败"})
		return
	}
	var actorID *uint
	if user, ok := currentReadUser(c); ok {
		id := user.ID
		actorID = &id
	}
	scope, scopeErr := services.ResolveContentReadScope(db, actorID)
	if scopeErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 0, "msg": "获取评论数量失败"})
		return
	}
	counts := make(map[uint]int64)
	commentMap := services.CommentMap(comments)
	for _, comment := range comments {
		if comment.ParentID != nil {
			continue
		}
		message, ok := messageMap[comment.MessageID]
		if !ok {
			continue
		}
		if scope.CanReadComment(message, comment, commentMap) {
			counts[comment.MessageID]++
		}
	}
	result := make([]gin.H, 0, len(counts))
	for _, id := range req.IDs {
		if cnt, ok := counts[id]; ok {
			result = append(result, gin.H{"id": id, "count": cnt})
		}
	}
	c.JSON(http.StatusOK, gin.H{"code": 1, "data": result})
}

// GetGuestbookMessageID 获取或创建用于留言板的独立消息ID
func GetGuestbookMessageID(c *gin.Context) {
	db, _ := database.GetDB()
	descriptor, err := services.EnsureGuestbook(db)
	if err != nil || descriptor.MessageID == 0 {
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "初始化留言板失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 1, "data": gin.H{"id": descriptor.MessageID}})
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
		Content    string `json:"content"`
		Visibility string `json:"visibility"`
		ParentID   *uint  `json:"parent_id"`
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
	// 读取站点配置；评论/留言/回复统一强制绑定当前登录账号，不再信任前端提交的昵称/邮箱/网址。
	var cfg models.SiteConfig
	_ = db.Table("site_configs").First(&cfg).Error
	userID, ok := commentAuthUserID(c)
	if !ok {
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "请登录后评论"})
		return
	}
	var currentUser models.User
	if err := db.Select("id, username, avatar_url, email").First(&currentUser, userID).Error; err != nil || currentUser.ID == 0 {
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "账号不存在或已失效"})
		return
	}
	viewerID := currentUser.ID
	scope, scopeErr := services.ResolveContentReadScope(db, &viewerID)
	if scopeErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 0, "msg": "授权服务不可用"})
		return
	}
	if !scope.CanReadMessage(message) {
		c.JSON(http.StatusNotFound, gin.H{"code": 0, "msg": "消息不存在"})
		return
	}
	if !scope.CanInteractWithMessage(message) {
		c.JSON(http.StatusForbidden, gin.H{"code": 0, "msg": "无权限评论该内容"})
		return
	}
	messageVisibility := services.StoredMessageVisibility(message)
	visibility, ok := normalizeRequestedCommentVisibility(req.Visibility, messageVisibility)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"code": 0, "msg": "无效的可见范围"})
		return
	}
	if !commentVisibilityAllowedForMessage(visibility, messageVisibility) {
		c.JSON(http.StatusBadRequest, gin.H{"code": 0, "msg": "评论可见范围不能宽于当前笔记"})
		return
	}
	var notificationParent *models.Comment
	if req.ParentID != nil {
		var parent models.Comment
		if err := db.First(&parent, *req.ParentID).Error; err != nil || parent.MessageID != msgID {
			c.JSON(http.StatusBadRequest, gin.H{"code": 0, "msg": "回复目标不存在"})
			return
		}
		notificationParent = &parent
		commentMap, err := services.LoadCommentMapForMessage(msgID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 0, "msg": "获取评论失败"})
			return
		}
		commentMap[parent.ID] = parent
		if !scope.CanInteractWithComment(message, parent, commentMap) {
			c.JSON(http.StatusForbidden, gin.H{"code": 0, "msg": "无权限回复该内容"})
			return
		}
		parentVisibility := services.EffectiveCommentVisibilityInThread(parent, messageVisibility, commentMap)
		if commentVisibilityRank(visibility) > commentVisibilityRank(parentVisibility) {
			if strings.TrimSpace(req.Visibility) == "" {
				visibility = parentVisibility
			} else {
				c.JSON(http.StatusBadRequest, gin.H{"code": 0, "msg": "回复可见范围不能宽于被回复内容"})
				return
			}
		}
		if commentAuthIsAdmin(c) && currentUser.ID != message.UserID && parentVisibility != "public" {
			c.JSON(http.StatusForbidden, gin.H{"code": 0, "msg": "管理员不可回复非公开评论"})
			return
		}
	}
	commentUserID := currentUser.ID
	commentUsername := strings.TrimSpace(currentUser.Username)
	if commentUsername == "" {
		commentUsername = fmt.Sprintf("用户%d", currentUser.ID)
	}
	comment := models.Comment{
		MessageID:  msgID,
		UserID:     &commentUserID,
		Content:    req.Content,
		Visibility: visibility,
		ParentID:   req.ParentID,
	}
	if err := db.Create(&comment).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 0, "msg": "保存评论失败"})
		return
	}
	comment.User = &models.CommentUserInfo{ID: currentUser.ID, Username: commentUsername, AvatarURL: strings.TrimSpace(currentUser.AvatarURL)}
	if err := services.CreateNotificationsForComment(message, comment, notificationParent); err != nil {
		log.Printf("创建站内评论通知失败: %v", err)
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
			textBody := fmt.Sprintf("站点：%s\n用户：%s\n内容：\n%s\n\n查看：%s/m/%d", cfg.SiteTitle, commentUsername, comment.Content, siteURL, message.ID)
			if strings.TrimSpace(cfg.CommentEmailAdminTemplate) != "" {
				tpl := cfg.CommentEmailAdminTemplate
				tpl = strings.ReplaceAll(tpl, "{site}", cfg.SiteTitle)
				tpl = strings.ReplaceAll(tpl, "{user}", commentUsername)
				tpl = strings.ReplaceAll(tpl, "{content}", comment.Content)
				tpl = strings.ReplaceAll(tpl, "{url}", fmt.Sprintf("%s/m/%d", siteURL, message.ID))
				textBody = tpl
			}
			htmlTpl := strings.TrimSpace(cfg.CommentEmailAdminTemplateHTML)
			if htmlTpl != "" {
				htmlTpl = strings.ReplaceAll(htmlTpl, "{site}", cfg.SiteTitle)
				htmlTpl = strings.ReplaceAll(htmlTpl, "{user}", commentUsername)
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
			var parentUser models.User
			parentEmail := ""
			if err := db.First(&parent, *comment.ParentID).Error; err == nil && parent.UserID != nil {
				if err := db.Select("id, email, email_verified").First(&parentUser, *parent.UserID).Error; err == nil && parentUser.EmailVerified {
					parentEmail = strings.TrimSpace(parentUser.Email)
				}
			}
			if parentEmail != "" && parent.UserID != nil && *parent.UserID != currentUser.ID && services.CanUserViewCommentInThread(message, comment, *parent.UserID) {
				prefixReply := strings.TrimSpace(cfg.CommentEmailReplyPrefix)
				if prefixReply != "" {
					prefixReply = prefixReply + " "
				}
				replySubject := fmt.Sprintf("%s你的评论有新回复 - %s", prefixReply, cfg.SiteTitle)
				textTpl := fmt.Sprintf("用户 %s 回复了你的评论：\n\n原评论：%s\n回复内容：%s\n\n查看：%s/m/%d", commentUsername, parent.Content, comment.Content, siteURL, message.ID)
				if strings.TrimSpace(cfg.CommentEmailReplyTemplate) != "" {
					tpl := cfg.CommentEmailReplyTemplate
					tpl = strings.ReplaceAll(tpl, "{site}", cfg.SiteTitle)
					tpl = strings.ReplaceAll(tpl, "{user}", commentUsername)
					tpl = strings.ReplaceAll(tpl, "{content}", comment.Content)
					tpl = strings.ReplaceAll(tpl, "{url}", fmt.Sprintf("%s/m/%d", siteURL, message.ID))
					textTpl = tpl
				}
				htmlTpl := strings.TrimSpace(cfg.CommentEmailReplyTemplateHTML)
				if htmlTpl != "" {
					htmlTpl = strings.ReplaceAll(htmlTpl, "{site}", cfg.SiteTitle)
					htmlTpl = strings.ReplaceAll(htmlTpl, "{user}", commentUsername)
					htmlTpl = strings.ReplaceAll(htmlTpl, "{content}", comment.Content)
					htmlTpl = strings.ReplaceAll(htmlTpl, "{url}", fmt.Sprintf("%s/m/%d", siteURL, message.ID))
					_ = models.SendEmailHTMLWithFrom(parentEmail, replySubject, htmlTpl, strings.TrimSpace(cfg.CommentEmailReplyName))
				} else {
					_ = models.SendEmailWithFrom(parentEmail, replySubject, textTpl, strings.TrimSpace(cfg.CommentEmailReplyName))
				}
			}
		}
	}
	c.JSON(http.StatusOK, gin.H{"code": 1, "data": comment, "msg": "评论已发布"})
}

// 更新评论：管理员可更新任意评论，普通用户仅可更新自己发布的评论/留言/回复。
func UpdateComment(c *gin.Context) {
	msgIDStr := c.Param("id")
	cidStr := c.Param("cid")
	msgID, err1 := strconv.ParseUint(msgIDStr, 10, 64)
	cid, err2 := strconv.ParseUint(cidStr, 10, 64)
	if err1 != nil || err2 != nil || msgID == 0 || cid == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 0, "msg": "无效的ID"})
		return
	}
	var req struct {
		Content    string `json:"content"`
		Visibility string `json:"visibility"`
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
	var cm models.Comment
	if err := db.First(&cm, cid).Error; err != nil || cm.MessageID != uint(msgID) {
		c.JSON(http.StatusNotFound, gin.H{"code": 0, "msg": "评论不存在"})
		return
	}
	var message models.Message
	if err := db.First(&message, uint(msgID)).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 0, "msg": "消息不存在"})
		return
	}
	commentMap, err := services.LoadCommentMapForMessage(uint(msgID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 0, "msg": "获取评论失败"})
		return
	}
	decision := authorizeCommentMutation(c, message, cm, commentMap, authorization.CapabilityCommentsEdit)
	if !decision.Allowed {
		if decision.Reason == authorization.DenialContentNotReadable {
			c.JSON(http.StatusNotFound, gin.H{"code": 0, "msg": "评论不存在"})
		} else {
			c.JSON(http.StatusForbidden, gin.H{"code": 0, "msg": "无权限"})
		}
		return
	}
	messageVisibility := services.StoredMessageVisibility(message)
	visibility := cm.Visibility
	if strings.TrimSpace(req.Visibility) != "" {
		var ok bool
		visibility, ok = normalizeCommentVisibility(req.Visibility)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"code": 0, "msg": "无效的可见范围"})
			return
		}
	} else if strings.TrimSpace(visibility) == "" {
		visibility = defaultCommentVisibilityForMessage(messageVisibility)
	} else {
		var ok bool
		visibility, ok = normalizeCommentVisibility(visibility)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"code": 0, "msg": "无效的可见范围"})
			return
		}
	}
	if !commentVisibilityAllowedForMessage(visibility, messageVisibility) {
		c.JSON(http.StatusBadRequest, gin.H{"code": 0, "msg": "评论可见范围不能宽于当前笔记"})
		return
	}
	if cm.ParentID != nil {
		var parent models.Comment
		if err := db.First(&parent, *cm.ParentID).Error; err == nil {
			commentMap[parent.ID] = parent
			parentVisibility := services.EffectiveCommentVisibilityInThread(parent, messageVisibility, commentMap)
			if commentVisibilityRank(visibility) > commentVisibilityRank(parentVisibility) {
				if strings.TrimSpace(req.Visibility) == "" {
					visibility = parentVisibility
				} else {
					c.JSON(http.StatusBadRequest, gin.H{"code": 0, "msg": "回复可见范围不能宽于被回复内容"})
					return
				}
			}
		}
	}
	cm.Content = req.Content
	cm.Visibility = visibility
	if err := persistCommentMutation(c, db, &cm, authorization.CapabilityCommentsEdit, "edit", false); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 0, "msg": "更新失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 1, "data": cm, "msg": "已更新"})
}

// 删除评论：管理员可删除任意评论，普通用户仅可删除自己发布的评论/留言/回复。
func DeleteComment(c *gin.Context) {
	msgIDStr := c.Param("id")
	cidStr := c.Param("cid")
	msgID, err1 := strconv.ParseUint(msgIDStr, 10, 64)
	cid, err2 := strconv.ParseUint(cidStr, 10, 64)
	if err1 != nil || err2 != nil || msgID == 0 || cid == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 0, "msg": "无效的ID"})
		return
	}
	db, _ := database.GetDB()
	// 确认评论属于该消息
	var cm models.Comment
	if err := db.First(&cm, cid).Error; err != nil || cm.MessageID != uint(msgID) {
		c.JSON(http.StatusNotFound, gin.H{"code": 0, "msg": "评论不存在"})
		return
	}
	var message models.Message
	if err := db.First(&message, uint(msgID)).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 0, "msg": "消息不存在"})
		return
	}
	commentMap, err := services.LoadCommentMapForMessage(uint(msgID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 0, "msg": "获取评论失败"})
		return
	}
	decision := authorizeCommentMutation(c, message, cm, commentMap, authorization.CapabilityCommentsDelete)
	if !decision.Allowed {
		if decision.Reason == authorization.DenialContentNotReadable {
			c.JSON(http.StatusNotFound, gin.H{"code": 0, "msg": "评论不存在"})
		} else {
			c.JSON(http.StatusForbidden, gin.H{"code": 0, "msg": "无权限"})
		}
		return
	}
	if err := persistCommentMutation(c, db, &cm, authorization.CapabilityCommentsDelete, "delete", true); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 0, "msg": "删除失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "已删除"})
}

// 列出所有内置评论（管理员，支持搜索与分页）
func ListComments(c *gin.Context) {
	actor, err := checkAdmin(c)
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
	actorID := actor
	scope, err := services.ResolveContentReadScope(db, &actorID)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[string]("查询失败"))
		return
	}
	tx := db.Model(&models.Comment{}).Joins("LEFT JOIN users ON users.id = comments.user_id")
	if q != "" {
		like := "%" + q + "%"
		tx = tx.Where("comments.content LIKE ? OR users.username LIKE ?", like, like)
	}
	var candidates []models.Comment
	if err := tx.Select("comments.*").Order("comments.created_at DESC").Find(&candidates).Error; err != nil {
		c.JSON(http.StatusOK, dto.Fail[string]("查询失败"))
		return
	}
	messageIDs := make([]uint, 0)
	seenMessageIDs := make(map[uint]struct{})
	for _, comment := range candidates {
		if _, exists := seenMessageIDs[comment.MessageID]; exists {
			continue
		}
		seenMessageIDs[comment.MessageID] = struct{}{}
		messageIDs = append(messageIDs, comment.MessageID)
	}
	var messages []models.Message
	if len(messageIDs) > 0 {
		if err := db.Where("id IN ?", messageIDs).Find(&messages).Error; err != nil {
			c.JSON(http.StatusOK, dto.Fail[string]("查询失败"))
			return
		}
	}
	messageByID := make(map[uint]models.Message, len(messages))
	commentMaps := make(map[uint]map[uint]models.Comment, len(messages))
	for _, message := range messages {
		messageByID[message.ID] = message
	}
	if len(messageIDs) > 0 {
		var threadComments []models.Comment
		if err := db.Where("message_id IN ?", messageIDs).Find(&threadComments).Error; err != nil {
			c.JSON(http.StatusOK, dto.Fail[string]("查询失败"))
			return
		}
		commentsByMessage := make(map[uint][]models.Comment)
		for _, comment := range threadComments {
			commentsByMessage[comment.MessageID] = append(commentsByMessage[comment.MessageID], comment)
		}
		for messageID, comments := range commentsByMessage {
			commentMaps[messageID] = services.CommentMap(comments)
		}
	}
	visible := make([]models.Comment, 0, len(candidates))
	for _, comment := range candidates {
		message, exists := messageByID[comment.MessageID]
		if exists && scope.CanReadComment(message, comment, commentMaps[comment.MessageID]) {
			comment.CanInteract = scope.CanInteractWithComment(message, comment, commentMaps[comment.MessageID])
			visible = append(visible, comment)
		}
	}
	total := int64(len(visible))
	start := (page - 1) * pageSize
	if start > len(visible) {
		start = len(visible)
	}
	end := start + pageSize
	if end > len(visible) {
		end = len(visible)
	}
	list := visible[start:end]
	userIDs := make([]uint, 0)
	for _, item := range list {
		if item.UserID != nil {
			userIDs = append(userIDs, *item.UserID)
		}
	}
	if len(userIDs) > 0 {
		var users []models.User
		if err := db.Select("id, username, avatar_url").Where("id IN ?", userIDs).Find(&users).Error; err == nil {
			userInfo := map[uint]models.CommentUserInfo{}
			for _, user := range users {
				userInfo[user.ID] = models.CommentUserInfo{ID: user.ID, Username: strings.TrimSpace(user.Username), AvatarURL: strings.TrimSpace(user.AvatarURL)}
			}
			for i := range list {
				if list[i].UserID == nil {
					continue
				}
				if info, ok := userInfo[*list[i].UserID]; ok {
					list[i].User = &info
				}
			}
		}
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
	title := "个人站点"
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
	if title == "个人站点" {
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
		Content    *string `json:"content"`
		Private    *bool   `json:"private"`
		Visibility *string `json:"visibility"`
		CreatedAt  *string `json:"created_at"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 0, "msg": "请求参数错误"})
		return
	}

	createdAt, err := parseMessageCreatedAt(req.CreatedAt)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 0, "msg": err.Error()})
		return
	}

	// 检查消息是否存在并且属于当前用户
	messageID, err := strconv.ParseUint(id, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 0, "msg": "无效的消息ID"})
		return
	}

	// 检查消息所有权或管理员权限
	message, err := services.GetMessageByID(uint(messageID), true)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 0, "msg": "消息不存在"})
		return
	}

	actorID, ok := commentUint(userID)
	if !ok || actorID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 0, "msg": "未授权访问"})
		return
	}
	db, err := database.GetDB()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 0, "msg": "授权服务不可用"})
		return
	}
	authorizer := authorization.New(db)
	requireMutation := func(capability authorization.Capability, messageText string) bool {
		if decision := services.AuthorizeMessageMutation(db, actorID, *message, capability); !decision.Allowed {
			writeMessageMutationDeniedAuditForDecision(c, authorizer, actorID, capability, "update", message, decision)
			if decision.Reason == authorization.DenialContentNotReadable {
				c.JSON(http.StatusNotFound, gin.H{"code": 0, "msg": "消息不存在"})
			} else {
				c.JSON(http.StatusForbidden, gin.H{"code": 0, "msg": messageText})
			}
			return false
		}
		return true
	}
	if createdAt != nil {
		if !requireMutation(authorization.CapabilityNotesPublishTime, "无权限调整发布时间") {
			return
		}
	}
	if message.UserID != actorID {
		if req.Content != nil && !requireMutation(authorization.CapabilityNotesEdit, "无权限修改此消息") {
			return
		}
		if (req.Private != nil || req.Visibility != nil) && !requireMutation(authorization.CapabilityNotesVisibility, "无权限调整可见范围") {
			return
		}
	}

	updated, err := services.UpdateMessage(uint(messageID), req.Content, req.Private, req.Visibility, createdAt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 0, "msg": err.Error()})
		return
	}
	if message.UserID != actorID {
		capability := authorization.CapabilityNotesEdit
		if req.Content == nil && (req.Private != nil || req.Visibility != nil) {
			capability = authorization.CapabilityNotesVisibility
		} else if req.Content == nil && createdAt != nil {
			capability = authorization.CapabilityNotesPublishTime
		}
		if err := writeMessageMutationSuccessAudit(c, authorizer, actorID, capability, "update", message); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 0, "msg": "写入管理员审计失败"})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "更新成功", "data": updated})

	// 即时模式触发云同步（防抖）
	syncmanager.Trigger()
}

// 更新消息置顶状态
func UpdateMessagePinned(c *gin.Context) {
	// Compatibility alias: the legacy route is still a global-pin operation,
	// so it must pass the same administrator authorization and audit path.
	UpdateMessageGlobalPin(c)
}

// 点赞接口：POST /api/messages/:id/like
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
	userID, ok := commentAuthUserID(c)
	if !ok || userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 0, "msg": "请先登录后再点赞"})
		return
	}
	created, count, err := services.IncrementLikeCount(uint(messageID), userID, commentAuthIsAdmin(c))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": err.Error()})
		return
	}
	if created {
		if err := services.CreateNotificationForLike(uint(messageID), userID); err != nil {
			log.Printf("创建点赞通知失败: %v", err)
		}
	}
	c.JSON(http.StatusOK, gin.H{"code": 1, "data": map[string]interface{}{"liked": true, "like_count": count}})
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

	userID, ok := commentAuthUserID(c)
	if !ok || userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 0, "msg": "请先登录后再点赞"})
		return
	}
	uid := userID
	liked, count, err := services.ToggleLike(uint(messageID), &uid, "", commentAuthIsAdmin(c))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": err.Error()})
		return
	}
	if liked {
		if err := services.CreateNotificationForLike(uint(messageID), userID); err != nil {
			log.Printf("创建点赞通知失败: %v", err)
		}
	}
	c.JSON(http.StatusOK, gin.H{"code": 1, "data": map[string]interface{}{"liked": liked, "like_count": count}})
}
func GetMessagesCalendar(c *gin.Context) {
	currentUserID, isAdmin := currentMessageViewer(c)

	var authorID *uint
	if aid := strings.TrimSpace(c.Query("authorId")); aid != "" {
		if v, err := strconv.ParseUint(aid, 10, 64); err == nil && v > 0 {
			vv := uint(v)
			authorID = &vv
		}
	}

	calendarData, err := services.GetMessagesGroupByDate(currentUserID, isAdmin, authorID)
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

	currentUserID, isAdmin := currentMessageViewer(c)

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

	result, err := services.SearchMessages(keyword, page, pageSize, currentUserID, isAdmin, authorID, username)
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
		c.JSON(http.StatusOK, gin.H{"code": 1, "data": gin.H{"hasUpdate": false, "lastUpdateTime": time.Now().Format(time.RFC3339), "currentTag": publicVersionLabel()}})
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
	c.JSON(http.StatusOK, gin.H{"code": 1, "data": gin.H{"hasUpdate": hasUpdate, "lastUpdateTime": latest.LastUpdated, "currentTag": publicVersionLabel()}})
}

func publicVersionLabel() string {
	return "installed"
}

// 获取公开运行版本。真实镜像标签仅供服务端升级逻辑使用，避免向匿名请求暴露构建提交。
func GetVersion(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"code": 1,
		"data": gin.H{
			"version": publicVersionLabel(),
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

func parseMessageCreatedAt(raw *string) (*time.Time, error) {
	if raw == nil {
		return nil, nil
	}
	value := strings.TrimSpace(*raw)
	if value == "" {
		return nil, fmt.Errorf("发布时间不能为空")
	}

	if t, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return &t, nil
	}

	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("Asia/Shanghai", 8*60*60)
	}
	for _, layout := range []string{
		"2006-01-02T15:04:05",
		"2006-01-02T15:04",
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
	} {
		if t, err := time.ParseInLocation(layout, value, loc); err == nil {
			return &t, nil
		}
	}

	return nil, fmt.Errorf("发布时间格式错误")
}

// 保留这个新版本的 PostMessage 函数
func PostMessage(c *gin.Context) {
	// 解析请求数据
	var request struct {
		Content    string  `json:"content"`
		Private    bool    `json:"private"`
		Visibility string  `json:"visibility"`
		ImageURL   string  `json:"image_url"`
		VideoURL   string  `json:"video_url"` // 新增视频字段
		Notify     *bool   `json:"notify"`
		CreatedAt  *string `json:"created_at"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusOK, dto.Fail[string]("内容不能为空"))
		return
	}

	createdAt, err := parseMessageCreatedAt(request.CreatedAt)
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](err.Error()))
		return
	}

	// 验证用户身份
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusOK, dto.Fail[string]("未授权访问"))
		return
	}
	user, err := services.GetUserByID(userID.(uint))
	if err != nil {
		c.JSON(http.StatusOK, dto.Fail[string]("获取用户信息失败"))
		return
	}
	var siteCfg models.SiteConfig
	_ = database.DB.Table("site_configs").First(&siteCfg).Error
	viaStr := c.GetString("auth_via")
	shouldNotify := shouldNotifyPublishedMessage(siteCfg.NotifyEnabled, user.IsAdmin, viaStr, request.Notify)
	if shouldNotify {
		actorID, ok := commentUint(userID)
		db, dbErr := database.GetDB()
		if !ok || dbErr != nil || !authorization.New(db).Authorize(actorID, authorization.CapabilityNotificationsManage, nil).Allowed {
			c.JSON(http.StatusForbidden, dto.Fail[string]("无权限发送发布通知"))
			return
		}
	}
	if createdAt != nil {
		actorID, ok := commentUint(userID)
		db, dbErr := database.GetDB()
		if !ok || dbErr != nil || !authorization.New(db).Authorize(actorID, authorization.CapabilityNotesPublishTime, nil).Allowed {
			c.JSON(http.StatusOK, dto.Fail[string]("仅管理员可以指定发布时间"))
			return
		}
	}

	// 创建消息
	message := &models.Message{
		Content:    request.Content,
		Private:    request.Private,
		Visibility: request.Visibility,
		ImageURL:   request.ImageURL,
		UserID:     userID.(uint),
	}
	if createdAt != nil {
		message.CreatedAt = *createdAt
	}

	if err := services.CreateMessage(message); err != nil {
		c.JSON(http.StatusOK, dto.Fail[string](err.Error()))
		return
	}

	// 推送策略仅对管理员生效：会话发布需显式 notify=true，管理员 token 发布跟随总开关自动推送。
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

func shouldNotifyPublishedMessage(siteEnabled bool, isAdmin bool, authVia string, requested *bool) bool {
	if !siteEnabled || !isAdmin {
		return false
	}
	if authVia == "token" {
		return true
	}
	return requested != nil && *requested
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

// 上传音频
func UploadAudio(c *gin.Context) {
	db, _ := database.GetDB()
	var siteConfig models.SiteConfig
	if err := db.First(&siteConfig).Error; err != nil {
		siteConfig = models.SiteConfig{}
	}

	allowedMimeTypes := []string{"audio/webm", "audio/ogg", "audio/mpeg", "audio/mp4", "audio/wav", "audio/x-wav", "audio/flac", "audio/x-flac"}

	audioURL, err := pkg.UploadAudio(c, allowedMimeTypes, &siteConfig)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 1,
		"msg":  "上传成功",
		"data": audioURL,
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
	user, err := services.GetUserByUsername(account)
	if err != nil || user == nil {
		c.JSON(http.StatusOK, dto.Fail[string]("用户不存在"))
		return
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
		if target.ID == models.PrimaryAdminUserID {
			c.JSON(http.StatusForbidden, dto.Fail[string]("不能删除 1 号管理员"))
			return
		}
		if currentID != models.PrimaryAdminUserID {
			c.JSON(http.StatusForbidden, dto.Fail[string]("仅 1 号管理员可以删除管理员账号"))
			return
		}
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
	currentID, err := checkAdmin(c)
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
	if user.IsAdmin && currentID != models.PrimaryAdminUserID {
		c.JSON(http.StatusForbidden, dto.Fail[string]("仅 1 号管理员可以重置管理员账号密码"))
		return
	}
	if user.ID == models.PrimaryAdminUserID {
		c.JSON(http.StatusForbidden, dto.Fail[string]("不能通过管理员功能重置 1 号管理员密码"))
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
