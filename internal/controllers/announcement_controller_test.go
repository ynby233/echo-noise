package controllers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/repository"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupAnnouncementControllerTest(t *testing.T) (*gorm.DB, *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.Announcement{}, &models.AnnouncementRead{}, &models.AnnouncementPushDelivery{}, &models.SiteConfig{}); err != nil {
		t.Fatalf("migrate announcement test db: %v", err)
	}
	database.DB = db
	models.SetDB(db)
	repository.ClearUserCache()
	t.Cleanup(func() {
		repository.ClearUserCache()
		database.DB = nil
		models.SetDB(nil)
	})
	r := gin.New()
	r.Use(sessions.Sessions("test", cookie.NewStore([]byte("announcement-controller-test"))))
	return db, r
}

func announcementCookieFromResponse(t *testing.T, response *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, item := range response.Result().Cookies() {
		if item.Name == announcementDeviceCookieName && item.Value != "" {
			return item
		}
	}
	t.Fatalf("response did not issue %s", announcementDeviceCookieName)
	return nil
}

func TestUnreadAnnouncementsReturnsPublishedNewestFirstAndIssuesDeviceCookie(t *testing.T) {
	db, r := setupAnnouncementControllerTest(t)
	older := time.Date(2026, 7, 1, 8, 0, 0, 0, time.UTC)
	newer := older.Add(time.Hour)
	records := []models.Announcement{
		{Title: "草稿", Content: "不可见", Status: models.AnnouncementStatusDraft},
		{Title: "较早公告", Content: "第一条", Status: models.AnnouncementStatusPublished, Revision: 1, PublishedAt: &older},
		{Title: "最新公告", Content: "第二条", Status: models.AnnouncementStatusPublished, Revision: 1, PublishedAt: &newer},
	}
	if err := db.Create(&records).Error; err != nil {
		t.Fatalf("seed announcements: %v", err)
	}

	r.GET("/announcements/unread", GetUnreadAnnouncements)
	req := httptest.NewRequest(http.MethodGet, "/announcements/unread", nil)
	response := httptest.NewRecorder()
	r.ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}

	var payload struct {
		Code int `json:"code"`
		Data struct {
			UnreadCount int `json:"unread_count"`
			Items       []struct {
				ID    uint   `json:"id"`
				Title string `json:"title"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Code != 1 || payload.Data.UnreadCount != 2 || len(payload.Data.Items) != 2 {
		t.Fatalf("unexpected payload: %s", response.Body.String())
	}
	if payload.Data.Items[0].Title != "最新公告" || payload.Data.Items[1].Title != "较早公告" {
		t.Fatalf("announcement order = %#v", payload.Data.Items)
	}
	cookies := response.Result().Cookies()
	foundDeviceCookie := false
	for _, item := range cookies {
		if item.Name == announcementDeviceCookieName && item.Value != "" && item.HttpOnly {
			foundDeviceCookie = true
			break
		}
	}
	if !foundDeviceCookie {
		t.Fatalf("expected an HttpOnly %s cookie", announcementDeviceCookieName)
	}
}

func TestMarkAnnouncementReadRemovesItFromSameDeviceUnreadList(t *testing.T) {
	db, r := setupAnnouncementControllerTest(t)
	publishedAt := time.Date(2026, 7, 1, 8, 0, 0, 0, time.UTC)
	announcement := models.Announcement{
		Title: "需要阅读", Content: "正文", Status: models.AnnouncementStatusPublished, Revision: 1, PublishedAt: &publishedAt,
	}
	if err := db.Create(&announcement).Error; err != nil {
		t.Fatalf("seed announcement: %v", err)
	}
	r.GET("/announcements/unread", GetUnreadAnnouncements)
	r.PUT("/announcements/:id/read", MarkAnnouncementRead)

	first := httptest.NewRecorder()
	r.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/announcements/unread", nil))
	deviceCookie := announcementCookieFromResponse(t, first)

	readRequest := httptest.NewRequest(http.MethodPut, "/announcements/"+strconvFormatUint(announcement.ID)+"/read", bytes.NewReader([]byte("{}")))
	readRequest.Header.Set("Content-Type", "application/json")
	readRequest.AddCookie(deviceCookie)
	readResponse := httptest.NewRecorder()
	r.ServeHTTP(readResponse, readRequest)
	if readResponse.Code != http.StatusOK {
		t.Fatalf("mark read status = %d, body = %s", readResponse.Code, readResponse.Body.String())
	}

	secondRequest := httptest.NewRequest(http.MethodGet, "/announcements/unread", nil)
	secondRequest.AddCookie(deviceCookie)
	second := httptest.NewRecorder()
	r.ServeHTTP(second, secondRequest)
	var payload struct {
		Code int `json:"code"`
		Data struct {
			UnreadCount int `json:"unread_count"`
		} `json:"data"`
	}
	if err := json.Unmarshal(second.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode unread response: %v", err)
	}
	if payload.Code != 1 || payload.Data.UnreadCount != 0 {
		t.Fatalf("announcement remained unread: %s", second.Body.String())
	}
}

func TestMarkAnnouncementReadDoesNotTrapSnapshotAfterConcurrentWithdrawal(t *testing.T) {
	db, r := setupAnnouncementControllerTest(t)
	announcement := models.Announcement{
		Title: "已下架公告", Content: "正文", Status: models.AnnouncementStatusWithdrawn, Revision: 1,
	}
	if err := db.Create(&announcement).Error; err != nil {
		t.Fatalf("seed withdrawn announcement: %v", err)
	}
	r.PUT("/announcements/:id/read", MarkAnnouncementRead)
	request := httptest.NewRequest(http.MethodPut, "/announcements/"+strconvFormatUint(announcement.ID)+"/read", bytes.NewReader([]byte("{}")))
	response := httptest.NewRecorder()
	r.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var payload struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Code != 1 {
		t.Fatalf("withdrawn snapshot could not be dismissed: %s", response.Body.String())
	}
	var readCount int64
	if err := db.Model(&models.AnnouncementRead{}).Where("announcement_id = ?", announcement.ID).Count(&readCount).Error; err != nil {
		t.Fatalf("count reads: %v", err)
	}
	if readCount != 0 {
		t.Fatalf("withdrawn announcement created %d read rows", readCount)
	}
}

func TestAnonymousDeviceReadsMergeIntoUserAndThenOtherDevices(t *testing.T) {
	db, r := setupAnnouncementControllerTest(t)
	user := models.User{Username: "alice-announcements", Password: "hashed"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	publishedAt := time.Date(2026, 7, 1, 8, 0, 0, 0, time.UTC)
	announcement := models.Announcement{
		Title: "跨设备公告", Content: "正文", Status: models.AnnouncementStatusPublished, Revision: 1, PublishedAt: &publishedAt,
	}
	if err := db.Create(&announcement).Error; err != nil {
		t.Fatalf("seed announcement: %v", err)
	}
	r.Use(func(c *gin.Context) {
		if c.GetHeader("X-Test-Authenticated") == "yes" {
			c.Set("user_id", user.ID)
			c.Set("is_admin", false)
		}
		c.Next()
	})
	r.GET("/announcements/unread", GetUnreadAnnouncements)
	r.PUT("/announcements/:id/read", MarkAnnouncementRead)

	first := httptest.NewRecorder()
	r.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/announcements/unread", nil))
	deviceACookie := announcementCookieFromResponse(t, first)
	readRequest := httptest.NewRequest(http.MethodPut, "/announcements/"+strconvFormatUint(announcement.ID)+"/read", bytes.NewReader([]byte("{}")))
	readRequest.AddCookie(deviceACookie)
	readResponse := httptest.NewRecorder()
	r.ServeHTTP(readResponse, readRequest)
	if readResponse.Code != http.StatusOK {
		t.Fatalf("anonymous mark read status = %d: %s", readResponse.Code, readResponse.Body.String())
	}

	loginRefresh := httptest.NewRequest(http.MethodGet, "/announcements/unread", nil)
	loginRefresh.AddCookie(deviceACookie)
	loginRefresh.Header.Set("X-Test-Authenticated", "yes")
	loginResponse := httptest.NewRecorder()
	r.ServeHTTP(loginResponse, loginRefresh)
	if unreadCountFromResponse(t, loginResponse) != 0 {
		t.Fatalf("device A became unread after login: %s", loginResponse.Body.String())
	}

	deviceBRequest := httptest.NewRequest(http.MethodGet, "/announcements/unread", nil)
	deviceBRequest.Header.Set("X-Test-Authenticated", "yes")
	deviceBResponse := httptest.NewRecorder()
	r.ServeHTTP(deviceBResponse, deviceBRequest)
	if unreadCountFromResponse(t, deviceBResponse) != 0 {
		t.Fatalf("device B did not inherit user read state: %s", deviceBResponse.Body.String())
	}
}

func unreadCountFromResponse(t *testing.T, response *httptest.ResponseRecorder) int {
	t.Helper()
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var payload struct {
		Code int `json:"code"`
		Data struct {
			UnreadCount int `json:"unread_count"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Code != 1 {
		t.Fatalf("request failed: %s", response.Body.String())
	}
	return payload.Data.UnreadCount
}

func TestMarkAllAnnouncementsReadClearsCurrentUnreadSet(t *testing.T) {
	db, r := setupAnnouncementControllerTest(t)
	base := time.Date(2026, 7, 1, 8, 0, 0, 0, time.UTC)
	announcements := []models.Announcement{
		{Title: "一", Content: "一", Status: models.AnnouncementStatusPublished, Revision: 1, PublishedAt: &base},
		{Title: "二", Content: "二", Status: models.AnnouncementStatusPublished, Revision: 1, PublishedAt: timePointer(base.Add(time.Hour))},
		{Title: "三", Content: "三", Status: models.AnnouncementStatusPublished, Revision: 1, PublishedAt: timePointer(base.Add(2 * time.Hour))},
	}
	if err := db.Create(&announcements).Error; err != nil {
		t.Fatalf("seed announcements: %v", err)
	}
	r.GET("/announcements/unread", GetUnreadAnnouncements)
	r.PUT("/announcements/read-all", MarkAllAnnouncementsRead)

	first := httptest.NewRecorder()
	r.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/announcements/unread", nil))
	if unreadCountFromResponse(t, first) != 3 {
		t.Fatalf("initial unread payload: %s", first.Body.String())
	}
	deviceCookie := announcementCookieFromResponse(t, first)
	markAllRequest := httptest.NewRequest(http.MethodPut, "/announcements/read-all", bytes.NewReader([]byte("{}")))
	markAllRequest.AddCookie(deviceCookie)
	markAllResponse := httptest.NewRecorder()
	r.ServeHTTP(markAllResponse, markAllRequest)
	if markAllResponse.Code != http.StatusOK {
		t.Fatalf("mark all status = %d: %s", markAllResponse.Code, markAllResponse.Body.String())
	}

	afterRequest := httptest.NewRequest(http.MethodGet, "/announcements/unread", nil)
	afterRequest.AddCookie(deviceCookie)
	after := httptest.NewRecorder()
	r.ServeHTTP(after, afterRequest)
	if unreadCountFromResponse(t, after) != 0 {
		t.Fatalf("unread announcements remained: %s", after.Body.String())
	}
}

func timePointer(value time.Time) *time.Time {
	return &value
}

func TestListAnnouncementsReturnsPublishedHistoryWithPerReaderState(t *testing.T) {
	db, r := setupAnnouncementControllerTest(t)
	base := time.Date(2026, 7, 1, 8, 0, 0, 0, time.UTC)
	announcements := []models.Announcement{
		{Title: "旧公告", Content: "旧正文", Status: models.AnnouncementStatusPublished, Revision: 1, PublishedAt: &base},
		{Title: "新公告", Content: "新正文", Status: models.AnnouncementStatusPublished, Revision: 1, PublishedAt: timePointer(base.Add(time.Hour))},
	}
	if err := db.Create(&announcements).Error; err != nil {
		t.Fatalf("seed announcements: %v", err)
	}
	r.GET("/announcements/unread", GetUnreadAnnouncements)
	r.GET("/announcements", ListAnnouncements)
	r.PUT("/announcements/:id/read", MarkAnnouncementRead)

	initial := httptest.NewRecorder()
	r.ServeHTTP(initial, httptest.NewRequest(http.MethodGet, "/announcements/unread", nil))
	deviceCookie := announcementCookieFromResponse(t, initial)
	markRequest := httptest.NewRequest(http.MethodPut, "/announcements/"+strconvFormatUint(announcements[1].ID)+"/read", bytes.NewReader([]byte("{}")))
	markRequest.AddCookie(deviceCookie)
	markResponse := httptest.NewRecorder()
	r.ServeHTTP(markResponse, markRequest)

	listRequest := httptest.NewRequest(http.MethodGet, "/announcements?page=1&pageSize=10", nil)
	listRequest.AddCookie(deviceCookie)
	listResponse := httptest.NewRecorder()
	r.ServeHTTP(listResponse, listRequest)
	if listResponse.Code != http.StatusOK {
		t.Fatalf("list status = %d: %s", listResponse.Code, listResponse.Body.String())
	}
	var payload struct {
		Code int `json:"code"`
		Data struct {
			Total       int `json:"total"`
			UnreadCount int `json:"unread_count"`
			Items       []struct {
				Title string `json:"title"`
				Read  bool   `json:"read"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(listResponse.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if payload.Code != 1 || payload.Data.Total != 2 || payload.Data.UnreadCount != 1 || len(payload.Data.Items) != 2 {
		t.Fatalf("unexpected list payload: %s", listResponse.Body.String())
	}
	if payload.Data.Items[0].Title != "新公告" || !payload.Data.Items[0].Read || payload.Data.Items[1].Title != "旧公告" || payload.Data.Items[1].Read {
		t.Fatalf("unexpected reader state: %#v", payload.Data.Items)
	}
}

func TestAdminCreatesAnnouncementAsDraft(t *testing.T) {
	db, r := setupAnnouncementControllerTest(t)
	admin := models.User{Username: "announcement-admin", Password: "hashed", IsAdmin: true}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatalf("seed admin: %v", err)
	}
	r.Use(func(c *gin.Context) {
		c.Set("user_id", admin.ID)
		c.Set("is_admin", true)
		c.Next()
	})
	r.POST("/admin/announcements", CreateAnnouncement)

	body, _ := json.Marshal(map[string]any{"title": "维护公告", "content": "今晚维护"})
	request := httptest.NewRequest(http.MethodPost, "/admin/announcements", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	r.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("create status = %d: %s", response.Code, response.Body.String())
	}
	var payload struct {
		Code int `json:"code"`
		Data struct {
			ID           uint   `json:"id"`
			Title        string `json:"title"`
			Status       string `json:"status"`
			AuthorUserID uint   `json:"author_user_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if payload.Code != 1 || payload.Data.ID == 0 || payload.Data.Title != "维护公告" || payload.Data.Status != models.AnnouncementStatusDraft || payload.Data.AuthorUserID != admin.ID {
		t.Fatalf("unexpected draft: %s", response.Body.String())
	}

	public := httptest.NewRecorder()
	publicRouter := gin.New()
	publicRouter.Use(sessions.Sessions("public-test", cookie.NewStore([]byte("public-announcement-test"))))
	publicRouter.GET("/announcements/unread", GetUnreadAnnouncements)
	publicRouter.ServeHTTP(public, httptest.NewRequest(http.MethodGet, "/announcements/unread", nil))
	if unreadCountFromResponse(t, public) != 0 {
		t.Fatalf("draft leaked to public unread list: %s", public.Body.String())
	}
}

func TestAdminPublishesDraftAndItBecomesUnread(t *testing.T) {
	db, r := setupAnnouncementControllerTest(t)
	admin := models.User{Username: "publisher", Password: "hashed", IsAdmin: true}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatalf("seed admin: %v", err)
	}
	draft := models.Announcement{Title: "发布测试", Content: "正文", Status: models.AnnouncementStatusDraft, Revision: 1, AuthorUserID: admin.ID}
	if err := db.Create(&draft).Error; err != nil {
		t.Fatalf("seed draft: %v", err)
	}
	r.Use(func(c *gin.Context) {
		if c.Request.URL.Path[:6] == "/admin" {
			c.Set("user_id", admin.ID)
			c.Set("is_admin", true)
		}
		c.Next()
	})
	r.POST("/admin/announcements/:id/publish", PublishAnnouncement)
	r.GET("/announcements/unread", GetUnreadAnnouncements)

	body, _ := json.Marshal(map[string]any{"push_enabled": false})
	publishRequest := httptest.NewRequest(http.MethodPost, "/admin/announcements/"+strconvFormatUint(draft.ID)+"/publish", bytes.NewReader(body))
	publishRequest.Header.Set("Content-Type", "application/json")
	publishResponse := httptest.NewRecorder()
	r.ServeHTTP(publishResponse, publishRequest)
	if publishResponse.Code != http.StatusOK {
		t.Fatalf("publish status = %d: %s", publishResponse.Code, publishResponse.Body.String())
	}

	publicResponse := httptest.NewRecorder()
	r.ServeHTTP(publicResponse, httptest.NewRequest(http.MethodGet, "/announcements/unread", nil))
	if unreadCountFromResponse(t, publicResponse) != 1 {
		t.Fatalf("published draft not unread: %s", publicResponse.Body.String())
	}
}

func TestEditingPublishedAnnouncementOnlyBecomesUnreadWhenRenotifyIsEnabled(t *testing.T) {
	db, r := setupAnnouncementControllerTest(t)
	admin := models.User{Username: "editor", Password: "hashed", IsAdmin: true}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatalf("seed admin: %v", err)
	}
	publishedAt := time.Date(2026, 7, 1, 8, 0, 0, 0, time.UTC)
	announcement := models.Announcement{Title: "原标题", Content: "原正文", Status: models.AnnouncementStatusPublished, Revision: 1, AuthorUserID: admin.ID, PublishedAt: &publishedAt}
	if err := db.Create(&announcement).Error; err != nil {
		t.Fatalf("seed announcement: %v", err)
	}
	r.Use(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/admin/") {
			c.Set("user_id", admin.ID)
			c.Set("is_admin", true)
		}
		c.Next()
	})
	r.GET("/announcements/unread", GetUnreadAnnouncements)
	r.PUT("/announcements/:id/read", MarkAnnouncementRead)
	r.PUT("/admin/announcements/:id", UpdateAnnouncement)

	initial := httptest.NewRecorder()
	r.ServeHTTP(initial, httptest.NewRequest(http.MethodGet, "/announcements/unread", nil))
	deviceCookie := announcementCookieFromResponse(t, initial)
	markRequest := httptest.NewRequest(http.MethodPut, "/announcements/"+strconvFormatUint(announcement.ID)+"/read", bytes.NewReader([]byte("{}")))
	markRequest.AddCookie(deviceCookie)
	r.ServeHTTP(httptest.NewRecorder(), markRequest)

	update := func(title string, renotify bool) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]any{"title": title, "content": "更新后的正文", "renotify": renotify})
		request := httptest.NewRequest(http.MethodPut, "/admin/announcements/"+strconvFormatUint(announcement.ID), bytes.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		r.ServeHTTP(response, request)
		return response
	}
	if response := update("普通修订", false); response.Code != http.StatusOK {
		t.Fatalf("ordinary edit failed: %d %s", response.Code, response.Body.String())
	}
	ordinaryCheck := httptest.NewRequest(http.MethodGet, "/announcements/unread", nil)
	ordinaryCheck.AddCookie(deviceCookie)
	ordinaryResponse := httptest.NewRecorder()
	r.ServeHTTP(ordinaryResponse, ordinaryCheck)
	if unreadCountFromResponse(t, ordinaryResponse) != 0 {
		t.Fatalf("ordinary edit reset read state: %s", ordinaryResponse.Body.String())
	}

	if response := update("需要重新阅读", true); response.Code != http.StatusOK {
		t.Fatalf("renotify edit failed: %d %s", response.Code, response.Body.String())
	}
	renotifyCheck := httptest.NewRequest(http.MethodGet, "/announcements/unread", nil)
	renotifyCheck.AddCookie(deviceCookie)
	renotifyResponse := httptest.NewRecorder()
	r.ServeHTTP(renotifyResponse, renotifyCheck)
	if unreadCountFromResponse(t, renotifyResponse) != 1 {
		t.Fatalf("renotify edit did not reset read state: %s", renotifyResponse.Body.String())
	}
}

func TestRenotifyEditDoesNotCreateAnotherVoceChatDelivery(t *testing.T) {
	db, r := setupAnnouncementControllerTest(t)
	admin := models.User{Username: "renotify-admin", Password: "hashed", IsAdmin: true}
	recipient := models.User{Username: "renotify-recipient", Password: "hashed", VoceChatNotificationEnabled: true, VoceChatUserID: "88"}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatalf("seed admin: %v", err)
	}
	if err := db.Create(&recipient).Error; err != nil {
		t.Fatalf("seed recipient: %v", err)
	}
	publishedAt := time.Now()
	announcement := models.Announcement{Title: "原公告", Content: "原正文", Status: models.AnnouncementStatusPublished, Revision: 1, PushEnabled: true, PublishedAt: &publishedAt, AuthorUserID: admin.ID}
	if err := db.Create(&announcement).Error; err != nil {
		t.Fatalf("seed announcement: %v", err)
	}
	sentAt := time.Now()
	delivery := models.AnnouncementPushDelivery{AnnouncementID: announcement.ID, RecipientUserID: recipient.ID, RecipientVoceChatUserID: recipient.VoceChatUserID, Status: models.AnnouncementPushSent, AttemptCount: 1, SentAt: &sentAt}
	if err := db.Create(&delivery).Error; err != nil {
		t.Fatalf("seed delivery: %v", err)
	}
	r.Use(func(c *gin.Context) { c.Set("user_id", admin.ID); c.Set("is_admin", true); c.Next() })
	r.PUT("/admin/announcements/:id", UpdateAnnouncement)
	r.GET("/admin/announcements/:id/push-summary", GetAnnouncementPushSummary)

	body, _ := json.Marshal(map[string]any{"title": "大幅修订", "content": "修订正文", "renotify": true})
	request := httptest.NewRequest(http.MethodPut, "/admin/announcements/"+strconvFormatUint(announcement.ID), bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	r.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("edit failed: %d %s", response.Code, response.Body.String())
	}
	summaryResponse := httptest.NewRecorder()
	r.ServeHTTP(summaryResponse, httptest.NewRequest(http.MethodGet, "/admin/announcements/"+strconvFormatUint(announcement.ID)+"/push-summary", nil))
	var payload struct {
		Code int `json:"code"`
		Data struct {
			Total   int64 `json:"total"`
			Sent    int64 `json:"sent"`
			Pending int64 `json:"pending"`
		} `json:"data"`
	}
	if err := json.Unmarshal(summaryResponse.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode summary: %v", err)
	}
	if payload.Code != 1 || payload.Data.Total != 1 || payload.Data.Sent != 1 || payload.Data.Pending != 0 {
		t.Fatalf("renotify changed VoceChat deliveries: %s", summaryResponse.Body.String())
	}
}

func TestWithdrawAndBatchDeleteOnlyRemoveNonPublishedAnnouncements(t *testing.T) {
	db, r := setupAnnouncementControllerTest(t)
	admin := models.User{Username: "lifecycle-admin", Password: "hashed", IsAdmin: true}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatalf("seed admin: %v", err)
	}
	publishedAt := time.Date(2026, 7, 1, 8, 0, 0, 0, time.UTC)
	records := []models.Announcement{
		{Title: "待撤回", Content: "正文", Status: models.AnnouncementStatusPublished, Revision: 1, AuthorUserID: admin.ID, PublishedAt: &publishedAt},
		{Title: "保留发布", Content: "正文", Status: models.AnnouncementStatusPublished, Revision: 1, AuthorUserID: admin.ID, PublishedAt: &publishedAt},
		{Title: "删除草稿", Content: "正文", Status: models.AnnouncementStatusDraft, Revision: 1, AuthorUserID: admin.ID},
	}
	if err := db.Create(&records).Error; err != nil {
		t.Fatalf("seed announcements: %v", err)
	}
	r.Use(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/admin/") {
			c.Set("user_id", admin.ID)
			c.Set("is_admin", true)
		}
		c.Next()
	})
	r.POST("/admin/announcements/:id/withdraw", WithdrawAnnouncement)
	r.POST("/admin/announcements/batch-delete", BatchDeleteAnnouncements)
	r.GET("/announcements/unread", GetUnreadAnnouncements)

	withdrawRequest := httptest.NewRequest(http.MethodPost, "/admin/announcements/"+strconvFormatUint(records[0].ID)+"/withdraw", bytes.NewReader([]byte("{}")))
	withdrawResponse := httptest.NewRecorder()
	r.ServeHTTP(withdrawResponse, withdrawRequest)
	if withdrawResponse.Code != http.StatusOK {
		t.Fatalf("withdraw failed: %d %s", withdrawResponse.Code, withdrawResponse.Body.String())
	}

	batchBody, _ := json.Marshal(map[string]any{"ids": []uint{records[0].ID, records[1].ID, records[2].ID}})
	batchRequest := httptest.NewRequest(http.MethodPost, "/admin/announcements/batch-delete", bytes.NewReader(batchBody))
	batchRequest.Header.Set("Content-Type", "application/json")
	batchResponse := httptest.NewRecorder()
	r.ServeHTTP(batchResponse, batchRequest)
	if batchResponse.Code != http.StatusOK {
		t.Fatalf("batch delete failed: %d %s", batchResponse.Code, batchResponse.Body.String())
	}
	var batchPayload struct {
		Code int `json:"code"`
		Data struct {
			DeletedCount int    `json:"deleted_count"`
			SkippedIDs   []uint `json:"skipped_ids"`
		} `json:"data"`
	}
	if err := json.Unmarshal(batchResponse.Body.Bytes(), &batchPayload); err != nil {
		t.Fatalf("decode batch response: %v", err)
	}
	if batchPayload.Code != 1 || batchPayload.Data.DeletedCount != 2 || len(batchPayload.Data.SkippedIDs) != 1 || batchPayload.Data.SkippedIDs[0] != records[1].ID {
		t.Fatalf("unexpected batch result: %s", batchResponse.Body.String())
	}
	publicResponse := httptest.NewRecorder()
	r.ServeHTTP(publicResponse, httptest.NewRequest(http.MethodGet, "/announcements/unread", nil))
	if unreadCountFromResponse(t, publicResponse) != 1 {
		t.Fatalf("published announcement was deleted or withdrawn announcement remained public: %s", publicResponse.Body.String())
	}
}

func TestRestoringWithdrawnAnnouncementPreservesReadStateAndPushDeliveries(t *testing.T) {
	db, r := setupAnnouncementControllerTest(t)
	admin := models.User{Username: "restore-admin", Password: "hashed", IsAdmin: true}
	recipient := models.User{Username: "restore-recipient", Password: "hashed", VoceChatNotificationEnabled: true, VoceChatUserID: "77"}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatalf("seed admin: %v", err)
	}
	if err := db.Create(&recipient).Error; err != nil {
		t.Fatalf("seed recipient: %v", err)
	}
	publishedAt := time.Now()
	announcement := models.Announcement{
		Title: "恢复发布", Content: "正文", Status: models.AnnouncementStatusPublished, Revision: 1,
		PushEnabled: true, AuthorUserID: admin.ID, PublishedAt: &publishedAt,
	}
	if err := db.Create(&announcement).Error; err != nil {
		t.Fatalf("seed announcement: %v", err)
	}
	sentAt := time.Now()
	delivery := models.AnnouncementPushDelivery{
		AnnouncementID: announcement.ID, RecipientUserID: recipient.ID, RecipientVoceChatUserID: recipient.VoceChatUserID,
		Status: models.AnnouncementPushSent, AttemptCount: 1, SentAt: &sentAt,
	}
	if err := db.Create(&delivery).Error; err != nil {
		t.Fatalf("seed delivery: %v", err)
	}
	r.Use(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/admin/") {
			c.Set("user_id", admin.ID)
			c.Set("is_admin", true)
		}
		c.Next()
	})
	r.GET("/announcements/unread", GetUnreadAnnouncements)
	r.PUT("/announcements/:id/read", MarkAnnouncementRead)
	r.POST("/admin/announcements/:id/withdraw", WithdrawAnnouncement)
	r.POST("/admin/announcements/:id/publish", PublishAnnouncement)

	initial := httptest.NewRecorder()
	r.ServeHTTP(initial, httptest.NewRequest(http.MethodGet, "/announcements/unread", nil))
	deviceCookie := announcementCookieFromResponse(t, initial)
	markRequest := httptest.NewRequest(http.MethodPut, "/announcements/"+strconvFormatUint(announcement.ID)+"/read", bytes.NewReader([]byte("{}")))
	markRequest.AddCookie(deviceCookie)
	r.ServeHTTP(httptest.NewRecorder(), markRequest)

	withdrawResponse := httptest.NewRecorder()
	r.ServeHTTP(withdrawResponse, httptest.NewRequest(http.MethodPost, "/admin/announcements/"+strconvFormatUint(announcement.ID)+"/withdraw", bytes.NewReader([]byte("{}"))))
	if withdrawResponse.Code != http.StatusOK {
		t.Fatalf("withdraw failed: %d %s", withdrawResponse.Code, withdrawResponse.Body.String())
	}
	restoreBody, _ := json.Marshal(map[string]any{"push_enabled": true})
	restoreRequest := httptest.NewRequest(http.MethodPost, "/admin/announcements/"+strconvFormatUint(announcement.ID)+"/publish", bytes.NewReader(restoreBody))
	restoreRequest.Header.Set("Content-Type", "application/json")
	restoreResponse := httptest.NewRecorder()
	r.ServeHTTP(restoreResponse, restoreRequest)
	if restoreResponse.Code != http.StatusOK {
		t.Fatalf("restore failed: %d %s", restoreResponse.Code, restoreResponse.Body.String())
	}

	refreshRequest := httptest.NewRequest(http.MethodGet, "/announcements/unread", nil)
	refreshRequest.AddCookie(deviceCookie)
	refreshResponse := httptest.NewRecorder()
	r.ServeHTTP(refreshResponse, refreshRequest)
	if unreadCountFromResponse(t, refreshResponse) != 0 {
		t.Fatalf("restore reset read state: %s", refreshResponse.Body.String())
	}
	var deliveryCount int64
	if err := db.Model(&models.AnnouncementPushDelivery{}).Where("announcement_id = ?", announcement.ID).Count(&deliveryCount).Error; err != nil {
		t.Fatalf("count deliveries: %v", err)
	}
	if deliveryCount != 1 {
		t.Fatalf("delivery count after restore = %d, want 1", deliveryCount)
	}
	if err := db.First(&announcement, announcement.ID).Error; err != nil {
		t.Fatalf("reload announcement: %v", err)
	}
	if announcement.Status != models.AnnouncementStatusPublished || announcement.Revision != 1 {
		t.Fatalf("restored announcement = %#v", announcement)
	}
}

func TestPublishingWithPushPersistsOneDeliveryPerOptedInUser(t *testing.T) {
	db, r := setupAnnouncementControllerTest(t)
	if err := db.Create(&models.SiteConfig{
		VoceChatEnabled: true, VoceChatNotificationEnabled: true,
		VoceChatBaseURL: "https://vc.example.test", VoceChatBotAPIKey: "bot-key",
	}).Error; err != nil {
		t.Fatalf("seed normal VoceChat config: %v", err)
	}
	admin := models.User{Username: "push-admin", Password: "hashed", IsAdmin: true}
	optedIn := models.User{Username: "push-enabled", Password: "hashed", VoceChatNotificationEnabled: true, VoceChatUserID: "42"}
	optedOut := models.User{Username: "push-disabled", Password: "hashed", VoceChatNotificationEnabled: false, VoceChatUserID: "43"}
	users := []models.User{admin, optedIn, optedOut}
	if err := db.Create(&users).Error; err != nil {
		t.Fatalf("seed users: %v", err)
	}
	admin = users[0]
	draft := models.Announcement{Title: "推送公告", Content: "推送正文", Status: models.AnnouncementStatusDraft, Revision: 1, AuthorUserID: admin.ID}
	if err := db.Create(&draft).Error; err != nil {
		t.Fatalf("seed draft: %v", err)
	}
	r.Use(func(c *gin.Context) {
		c.Set("user_id", admin.ID)
		c.Set("is_admin", true)
		c.Next()
	})
	r.POST("/admin/announcements/:id/publish", PublishAnnouncement)
	r.GET("/admin/announcements/:id/push-summary", GetAnnouncementPushSummary)

	body, _ := json.Marshal(map[string]any{"push_enabled": true})
	publishRequest := httptest.NewRequest(http.MethodPost, "/admin/announcements/"+strconvFormatUint(draft.ID)+"/publish", bytes.NewReader(body))
	publishRequest.Header.Set("Content-Type", "application/json")
	publishResponse := httptest.NewRecorder()
	r.ServeHTTP(publishResponse, publishRequest)
	if publishResponse.Code != http.StatusOK {
		t.Fatalf("publish failed: %d %s", publishResponse.Code, publishResponse.Body.String())
	}

	summaryRequest := httptest.NewRequest(http.MethodGet, "/admin/announcements/"+strconvFormatUint(draft.ID)+"/push-summary", nil)
	summaryResponse := httptest.NewRecorder()
	r.ServeHTTP(summaryResponse, summaryRequest)
	if summaryResponse.Code != http.StatusOK {
		t.Fatalf("summary failed: %d %s", summaryResponse.Code, summaryResponse.Body.String())
	}
	var payload struct {
		Code int `json:"code"`
		Data struct {
			Total   int64 `json:"total"`
			Pending int64 `json:"pending"`
			Sent    int64 `json:"sent"`
			Failed  int64 `json:"failed"`
			Skipped int64 `json:"skipped"`
		} `json:"data"`
	}
	if err := json.Unmarshal(summaryResponse.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode summary: %v", err)
	}
	if payload.Code != 1 || payload.Data.Total != 1 || payload.Data.Pending != 1 || payload.Data.Sent != 0 || payload.Data.Failed != 0 || payload.Data.Skipped != 0 {
		t.Fatalf("unexpected push summary: %s", summaryResponse.Body.String())
	}
}

func TestPublishingWithPushDoesNotQueueInLocalOrDegradedRuntime(t *testing.T) {
	for _, test := range []struct {
		name   string
		mode   string
		health string
	}{
		{name: "local", mode: models.RuntimeModeLocal, health: "ok"},
		{name: "degraded", mode: models.RuntimeModeVoceChat, health: "failed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			db, r := setupAnnouncementControllerTest(t)
			if err := db.Create(&models.SiteConfig{
				RuntimeMode: test.mode, RuntimeModeMigrationVersion: models.RuntimeModeMigrationVersionCurrent,
				VoceChatEnabled: test.mode == models.RuntimeModeVoceChat, VoceChatBaseURL: "https://vc.example.test",
				VoceChatBotAPIKey: "bot-key", VoceChatLastHealthStatus: test.health,
			}).Error; err != nil {
				t.Fatalf("seed runtime config: %v", err)
			}
			users := []models.User{
				{Username: "push-admin-" + test.name, Password: "hashed", IsAdmin: true},
				{Username: "push-recipient-" + test.name, Password: "hashed", VoceChatNotificationEnabled: true, VoceChatUserID: "42"},
			}
			if err := db.Create(&users).Error; err != nil {
				t.Fatalf("seed users: %v", err)
			}
			draft := models.Announcement{Title: "runtime push", Content: "site notification remains", Status: models.AnnouncementStatusDraft, Revision: 1, AuthorUserID: users[0].ID}
			if err := db.Create(&draft).Error; err != nil {
				t.Fatalf("seed draft: %v", err)
			}
			r.Use(func(c *gin.Context) {
				c.Set("user_id", users[0].ID)
				c.Set("is_admin", true)
				c.Next()
			})
			r.POST("/admin/announcements/:id/publish", PublishAnnouncement)
			body, _ := json.Marshal(map[string]any{"push_enabled": true})
			request := httptest.NewRequest(http.MethodPost, "/admin/announcements/"+strconvFormatUint(draft.ID)+"/publish", bytes.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			r.ServeHTTP(response, request)
			if response.Code != http.StatusOK {
				t.Fatalf("publish failed: %d %s", response.Code, response.Body.String())
			}
			var count int64
			if err := db.Model(&models.AnnouncementPushDelivery{}).Where("announcement_id = ?", draft.ID).Count(&count).Error; err != nil {
				t.Fatalf("count deliveries: %v", err)
			}
			if count != 0 {
				t.Fatalf("queued %d historical deliveries in %s runtime", count, test.name)
			}
			if err := db.First(&draft, draft.ID).Error; err != nil || draft.Status != models.AnnouncementStatusPublished || draft.PublishedAt == nil {
				t.Fatalf("site announcement was not published: announcement=%#v err=%v", draft, err)
			}
		})
	}
}

func TestAdminRetriesOnlyFailedAnnouncementPushDeliveries(t *testing.T) {
	db, r := setupAnnouncementControllerTest(t)
	admin := models.User{Username: "retry-admin", Password: "hashed", IsAdmin: true}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatalf("seed admin: %v", err)
	}
	publishedAt := time.Now()
	announcement := models.Announcement{Title: "重试公告", Content: "正文", Status: models.AnnouncementStatusPublished, Revision: 1, PublishedAt: &publishedAt, PushEnabled: true, AuthorUserID: admin.ID}
	if err := db.Create(&announcement).Error; err != nil {
		t.Fatalf("seed announcement: %v", err)
	}
	deliveries := []models.AnnouncementPushDelivery{
		{AnnouncementID: announcement.ID, RecipientUserID: admin.ID, Status: models.AnnouncementPushFailed, AttemptCount: 1, LastError: "temporary failure"},
		{AnnouncementID: announcement.ID, RecipientUserID: admin.ID + 1, Status: models.AnnouncementPushSent, AttemptCount: 1},
	}
	if err := db.Create(&deliveries).Error; err != nil {
		t.Fatalf("seed deliveries: %v", err)
	}
	r.Use(func(c *gin.Context) {
		c.Set("user_id", admin.ID)
		c.Set("is_admin", true)
		c.Next()
	})
	r.POST("/admin/announcements/:id/retry-push", RetryAnnouncementPush)
	r.GET("/admin/announcements/:id/push-summary", GetAnnouncementPushSummary)

	retryRequest := httptest.NewRequest(http.MethodPost, "/admin/announcements/"+strconvFormatUint(announcement.ID)+"/retry-push", bytes.NewReader([]byte("{}")))
	retryResponse := httptest.NewRecorder()
	r.ServeHTTP(retryResponse, retryRequest)
	if retryResponse.Code != http.StatusOK {
		t.Fatalf("retry failed: %d %s", retryResponse.Code, retryResponse.Body.String())
	}
	summaryRequest := httptest.NewRequest(http.MethodGet, "/admin/announcements/"+strconvFormatUint(announcement.ID)+"/push-summary", nil)
	summaryResponse := httptest.NewRecorder()
	r.ServeHTTP(summaryResponse, summaryRequest)
	var payload struct {
		Code int `json:"code"`
		Data struct {
			Pending int64 `json:"pending"`
			Sent    int64 `json:"sent"`
			Failed  int64 `json:"failed"`
		} `json:"data"`
	}
	if err := json.Unmarshal(summaryResponse.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode summary: %v", err)
	}
	if payload.Code != 1 || payload.Data.Pending != 1 || payload.Data.Sent != 1 || payload.Data.Failed != 0 {
		t.Fatalf("unexpected retry result: %s", summaryResponse.Body.String())
	}

	if err := db.Model(&announcement).Update("status", models.AnnouncementStatusWithdrawn).Error; err != nil {
		t.Fatalf("withdraw announcement: %v", err)
	}
	if err := db.Model(&models.AnnouncementPushDelivery{}).
		Where("announcement_id = ? AND status = ?", announcement.ID, models.AnnouncementPushPending).
		Update("status", models.AnnouncementPushFailed).Error; err != nil {
		t.Fatalf("restore failed delivery: %v", err)
	}
	withdrawnRetry := httptest.NewRecorder()
	r.ServeHTTP(withdrawnRetry, httptest.NewRequest(http.MethodPost, "/admin/announcements/"+strconvFormatUint(announcement.ID)+"/retry-push", bytes.NewReader([]byte("{}"))))
	if withdrawnRetry.Code != http.StatusBadRequest {
		t.Fatalf("withdrawn retry status = %d, body = %s", withdrawnRetry.Code, withdrawnRetry.Body.String())
	}
}

func TestAdminListFiltersByStatusAndSingleDeleteRejectsPublishedAnnouncement(t *testing.T) {
	db, r := setupAnnouncementControllerTest(t)
	admin := models.User{Username: "list-admin", Password: "hashed", IsAdmin: true}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatalf("seed admin: %v", err)
	}
	publishedAt := time.Now()
	withdrawnAt := publishedAt.Add(time.Hour)
	records := []models.Announcement{
		{Title: "草稿", Content: "正文", Status: models.AnnouncementStatusDraft, Revision: 1, AuthorUserID: admin.ID},
		{Title: "已撤回", Content: "正文", Status: models.AnnouncementStatusWithdrawn, Revision: 1, AuthorUserID: admin.ID, PublishedAt: &publishedAt, WithdrawnAt: &withdrawnAt},
		{Title: "已发布", Content: "正文", Status: models.AnnouncementStatusPublished, Revision: 1, AuthorUserID: admin.ID, PublishedAt: &publishedAt},
	}
	if err := db.Create(&records).Error; err != nil {
		t.Fatalf("seed announcements: %v", err)
	}
	r.Use(func(c *gin.Context) {
		c.Set("user_id", admin.ID)
		c.Set("is_admin", true)
		c.Next()
	})
	r.GET("/admin/announcements", ListAdminAnnouncements)
	r.DELETE("/admin/announcements/:id", DeleteAnnouncement)

	listResponse := httptest.NewRecorder()
	r.ServeHTTP(listResponse, httptest.NewRequest(http.MethodGet, "/admin/announcements?status=withdrawn&page=1&pageSize=20", nil))
	var listPayload struct {
		Code int `json:"code"`
		Data struct {
			Total int64 `json:"total"`
			Items []struct {
				ID     uint   `json:"id"`
				Status string `json:"status"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(listResponse.Body.Bytes(), &listPayload); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if listPayload.Code != 1 || listPayload.Data.Total != 1 || len(listPayload.Data.Items) != 1 || listPayload.Data.Items[0].ID != records[1].ID {
		t.Fatalf("unexpected admin list: %s", listResponse.Body.String())
	}

	deleteWithdrawn := httptest.NewRecorder()
	r.ServeHTTP(deleteWithdrawn, httptest.NewRequest(http.MethodDelete, "/admin/announcements/"+strconvFormatUint(records[1].ID), nil))
	if deleteWithdrawn.Code != http.StatusOK {
		t.Fatalf("delete withdrawn failed: %d %s", deleteWithdrawn.Code, deleteWithdrawn.Body.String())
	}
	deletePublished := httptest.NewRecorder()
	r.ServeHTTP(deletePublished, httptest.NewRequest(http.MethodDelete, "/admin/announcements/"+strconvFormatUint(records[2].ID), nil))
	if deletePublished.Code != http.StatusBadRequest {
		t.Fatalf("published delete status = %d, body = %s", deletePublished.Code, deletePublished.Body.String())
	}
}
