package controllers

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/dto"
	"github.com/rcy1314/echo-noise/internal/services"
)

const webPushSessionKey = "web_push_session_id"

type webPushSubscriptionRequest struct {
	Endpoint       string `json:"endpoint"`
	ExpirationTime *int64 `json:"expirationTime"`
	Platform       string `json:"platform"`
	Keys           struct {
		P256dh string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
}

type webPushUnsubscribeRequest struct {
	Endpoint string `json:"endpoint"`
}

type webPushPreferenceRequest struct {
	Enabled                bool `json:"enabled"`
	CommentEnabled         bool `json:"comment_enabled"`
	ReplyEnabled           bool `json:"reply_enabled"`
	GuestbookEnabled       bool `json:"guestbook_enabled"`
	LikeEnabled            bool `json:"like_enabled"`
	AnnouncementEnabled    bool `json:"announcement_enabled"`
	AccountSecurityEnabled bool `json:"account_security_enabled"`
	ShowPreview            bool `json:"show_preview"`
}

func newWebPushSessionID() (string, error) {
	value := make([]byte, 24)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

func ensureWebPushSession(session sessions.Session) (string, error) {
	if session == nil {
		return "", errors.New("登录会话无效")
	}
	if current := strings.TrimSpace(toString(session.Get(webPushSessionKey))); current != "" {
		return current, nil
	}
	created, err := newWebPushSessionID()
	if err != nil {
		return "", err
	}
	session.Set(webPushSessionKey, created)
	if err := session.Save(); err != nil {
		return "", err
	}
	return created, nil
}

func toString(value interface{}) string {
	text, _ := value.(string)
	return text
}

func webPushSessionExpiry(session sessions.Session) *time.Time {
	if session == nil {
		return nil
	}
	expireAt := parseReadSessionExpireAt(session.Get("login_expire_at"))
	if expireAt <= 0 {
		return nil
	}
	value := time.Unix(expireAt, 0)
	return &value
}

func webPushSessionIssuedAt(session sessions.Session) *time.Time {
	if session == nil {
		return nil
	}
	issuedAt := parseReadSessionExpireAt(session.Get("login_issued_at"))
	if issuedAt <= 0 {
		return nil
	}
	value := time.Unix(issuedAt, 0)
	return &value
}

func requestSourceHost(context *gin.Context) string {
	origin := strings.TrimSpace(context.GetHeader("Origin"))
	if origin == "" {
		origin = strings.TrimSpace(context.GetHeader("Referer"))
	}
	parsed, err := url.Parse(origin)
	if err != nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(parsed.Host))
}

func requireWebPushSameOrigin(context *gin.Context) bool {
	requestHost := strings.ToLower(strings.TrimSpace(context.Request.Host))
	if requestHost == "" || requestSourceHost(context) != requestHost {
		context.JSON(http.StatusForbidden, dto.Fail[any]("请求来源无效"))
		return false
	}
	return true
}

func currentWebPushSession(context *gin.Context) (uint, sessions.Session, string, error) {
	if context.GetString("auth_via") != "session" {
		return 0, nil, "", errors.New("Web Push 仅支持浏览器登录会话")
	}
	userID, ok := commentUint(context.GetUint("user_id"))
	if !ok || userID == 0 {
		return 0, nil, "", errors.New("未登录")
	}
	session := sessions.Default(context)
	sessionID, err := ensureWebPushSession(session)
	return userID, session, sessionID, err
}

func RegisterWebPushSubscription(context *gin.Context) {
	if !requireWebPushSameOrigin(context) {
		return
	}
	userID, session, sessionID, err := currentWebPushSession(context)
	if err != nil {
		context.JSON(http.StatusUnauthorized, dto.Fail[any](err.Error()))
		return
	}
	var request webPushSubscriptionRequest
	if err := context.ShouldBindJSON(&request); err != nil {
		context.JSON(http.StatusBadRequest, dto.Fail[any]("推送订阅参数错误"))
		return
	}
	var expiresAt *time.Time
	if request.ExpirationTime != nil && *request.ExpirationTime > 0 {
		value := time.UnixMilli(*request.ExpirationTime)
		expiresAt = &value
	}
	_, err = services.UpsertWebPushSubscription(database.DB, userID, sessionID, webPushSessionIssuedAt(session), webPushSessionExpiry(session), services.WebPushSubscriptionInput{
		Endpoint: request.Endpoint, P256dh: request.Keys.P256dh, Auth: request.Keys.Auth,
		Platform: request.Platform, ExpiresAt: expiresAt,
	}, context.GetHeader("User-Agent"))
	if err != nil {
		context.JSON(http.StatusBadRequest, dto.Fail[any](err.Error()))
		return
	}
	context.JSON(http.StatusOK, dto.OK(gin.H{"subscribed": true}, "当前设备已开启推送"))
}

func UnregisterWebPushSubscription(context *gin.Context) {
	if !requireWebPushSameOrigin(context) {
		return
	}
	userID, _, sessionID, err := currentWebPushSession(context)
	if err != nil {
		context.JSON(http.StatusUnauthorized, dto.Fail[any](err.Error()))
		return
	}
	var request webPushUnsubscribeRequest
	if err := context.ShouldBindJSON(&request); err != nil || strings.TrimSpace(request.Endpoint) == "" {
		context.JSON(http.StatusBadRequest, dto.Fail[any]("退订参数错误"))
		return
	}
	if err := services.UnregisterWebPushSubscription(database.DB, userID, sessionID, request.Endpoint); err != nil {
		context.JSON(http.StatusInternalServerError, dto.Fail[any]("退订失败"))
		return
	}
	context.JSON(http.StatusOK, dto.OK(gin.H{"subscribed": false}, "当前设备已关闭推送"))
}

func GetWebPushConfig(context *gin.Context) {
	userID, _, sessionID, err := currentWebPushSession(context)
	if err != nil {
		context.JSON(http.StatusUnauthorized, dto.Fail[any](err.Error()))
		return
	}
	preference, err := services.GetWebPushPreference(database.DB, userID)
	if err != nil {
		context.JSON(http.StatusInternalServerError, dto.Fail[any]("读取推送设置失败"))
		return
	}
	configured := services.LoadWebPushRuntimeConfig()
	subscribed, err := services.HasActiveWebPushSubscriptionForSession(database.DB, userID, sessionID, time.Now())
	if err != nil {
		context.JSON(http.StatusInternalServerError, dto.Fail[any]("读取订阅状态失败"))
		return
	}
	publicKey := ""
	if configured.Ready() {
		publicKey = configured.PublicKey
	}
	context.JSON(http.StatusOK, dto.OK(gin.H{
		"configured": configured.Ready(), "public_key": publicKey,
		"session_subscribed": subscribed, "preferences": preference,
	}, "获取推送设置成功"))
}

func UpdateWebPushPreferences(context *gin.Context) {
	if !requireWebPushSameOrigin(context) {
		return
	}
	userID, _, _, err := currentWebPushSession(context)
	if err != nil {
		context.JSON(http.StatusUnauthorized, dto.Fail[any](err.Error()))
		return
	}
	var request webPushPreferenceRequest
	if err := context.ShouldBindJSON(&request); err != nil {
		context.JSON(http.StatusBadRequest, dto.Fail[any]("推送设置参数错误"))
		return
	}
	preference, err := services.UpdateWebPushPreference(database.DB, userID, services.WebPushPreferenceInput(request))
	if err != nil {
		context.JSON(http.StatusInternalServerError, dto.Fail[any]("保存推送设置失败"))
		return
	}
	context.JSON(http.StatusOK, dto.OK(preference, "推送设置已保存"))
}

func SendWebPushTest(context *gin.Context) {
	if !requireWebPushSameOrigin(context) {
		return
	}
	userID, _, _, err := currentWebPushSession(context)
	if err != nil {
		context.JSON(http.StatusUnauthorized, dto.Fail[any](err.Error()))
		return
	}
	if err := services.QueueWebPushTest(database.DB, userID, time.Now()); err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, services.ErrWebPushTestRateLimited) {
			status = http.StatusTooManyRequests
		}
		context.JSON(status, dto.Fail[any](err.Error()))
		return
	}
	context.JSON(http.StatusOK, dto.OK[any](nil, "测试通知已进入发送队列"))
}
