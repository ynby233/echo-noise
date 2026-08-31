package controllers

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/middleware"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/pkg"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func validControllerWebPushKeys(t *testing.T) (string, string) {
	t.Helper()
	_, publicKey, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		t.Fatalf("generate controller test key: %v", err)
	}
	return publicKey, base64.RawURLEncoding.EncodeToString(make([]byte, 16))
}

func setupWebPushControllerTest(t *testing.T) (*gin.Engine, models.User) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	t.Setenv("SESSION_SECRET", "web-push-controller-test-session-secret")
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.AutoMigrate(
		&models.User{}, &models.WebPushSubscription{}, &models.WebPushPreference{}, &models.WebPushDelivery{},
		&models.UserNotification{}, &models.SiteConfig{},
	); err != nil {
		t.Fatalf("migrate database: %v", err)
	}
	database.DB = db
	models.SetDB(db)
	user := models.User{Username: "push-user", Password: "hashed"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	router := gin.New()
	pkg.InitSession(router)
	router.POST("/test/login", func(context *gin.Context) {
		if err := establishLoginSession(context, &user); err != nil {
			context.Status(http.StatusInternalServerError)
			return
		}
		context.Status(http.StatusNoContent)
	})
	auth := router.Group("/api")
	auth.Use(middleware.SessionAuthMiddleware())
	auth.POST("/web-push/subscriptions", RegisterWebPushSubscription)
	return router, user
}

func authenticatedWebPushRequest(t *testing.T, router *gin.Engine, method string, path string, body any, origin string) *httptest.ResponseRecorder {
	t.Helper()
	loginRequest := httptest.NewRequest(http.MethodPost, "/test/login", nil)
	loginRecorder := httptest.NewRecorder()
	router.ServeHTTP(loginRecorder, loginRequest)
	if loginRecorder.Code != http.StatusNoContent {
		t.Fatalf("test login status = %d", loginRecorder.Code)
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("encode request: %v", err)
	}
	request := httptest.NewRequest(method, path, bytes.NewReader(encoded))
	request.Host = "site.example"
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", origin)
	for _, cookie := range loginRecorder.Result().Cookies() {
		request.AddCookie(cookie)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestRegisterWebPushSubscriptionAcceptsSameOriginAndRejectsCrossSiteCookieUse(t *testing.T) {
	router, user := setupWebPushControllerTest(t)
	p256dh, auth := validControllerWebPushKeys(t)
	body := map[string]any{
		"endpoint": "https://push.example/controller",
		"keys":     map[string]string{"p256dh": p256dh, "auth": auth},
		"platform": "ios",
	}
	sameOrigin := authenticatedWebPushRequest(t, router, http.MethodPost, "/api/web-push/subscriptions", body, "https://site.example")
	if sameOrigin.Code != http.StatusOK {
		t.Fatalf("same-origin status = %d body=%s", sameOrigin.Code, sameOrigin.Body.String())
	}
	var count int64
	if err := database.DB.Model(&models.WebPushSubscription{}).Where("user_id = ?", user.ID).Count(&count).Error; err != nil {
		t.Fatalf("count subscriptions: %v", err)
	}
	if count != 1 {
		t.Fatalf("same-origin subscription count = %d, want 1", count)
	}

	crossSiteBody := map[string]any{
		"endpoint": "https://push.example/cross-site",
		"keys":     map[string]string{"p256dh": p256dh, "auth": auth},
		"platform": "ios",
	}
	crossSite := authenticatedWebPushRequest(t, router, http.MethodPost, "/api/web-push/subscriptions", crossSiteBody, "https://evil.example")
	if crossSite.Code != http.StatusForbidden {
		t.Fatalf("cross-site status = %d body=%s", crossSite.Code, crossSite.Body.String())
	}
	if err := database.DB.Model(&models.WebPushSubscription{}).Count(&count).Error; err != nil {
		t.Fatalf("count subscriptions after cross-site request: %v", err)
	}
	if count != 1 {
		t.Fatalf("cross-site request changed subscriptions: count=%d", count)
	}
}
