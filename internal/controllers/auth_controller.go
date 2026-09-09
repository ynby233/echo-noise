package controllers

import (
	"crypto/rand"
	"errors"
	"fmt"
	"log"
	"net/http"
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
	"github.com/rcy1314/echo-noise/internal/services"
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
				_ = pkg.ClearUserSession(c)
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

	// 隐藏敏感字段。登录服务可能返回仓库缓存中的对象，不能原地改写，
	// 否则会把缓存密码清空并导致同一账号后续登录失败。
	safeUser := *user
	safeUser.Password = ""

	if err := establishLoginSession(c, user); err != nil {
		c.JSON(http.StatusOK, dto.Fail[any]("Session 保存失败"))
		return
	}
	_ = recordUserLoginAudit(c, user, loginAuditActionLogin)

	c.JSON(http.StatusOK, dto.OK(&safeUser, "登录成功"))
}

// 添加登出功能
func Logout(c *gin.Context) {
	session := sessions.Default(c)
	recordSessionLogoutAudit(c, session)
	if userID, ok := commentUint(session.Get("user_id")); ok && userID > 0 {
		if sessionID := strings.TrimSpace(toString(session.Get(webPushSessionKey))); sessionID != "" {
			if err := services.DisableWebPushSubscriptionsForSession(database.DB, userID, sessionID); err != nil {
				log.Printf("Web Push 登出清理失败: user_id=%d error_type=%T", userID, err)
			}
		}
	}
	if err := pkg.ClearUserSession(c); err != nil {
		c.JSON(http.StatusInternalServerError, dto.Fail[any]("退出登录失败"))
		return
	}
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
