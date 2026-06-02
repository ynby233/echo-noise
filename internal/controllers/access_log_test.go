package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/internal/middleware"
	"github.com/rcy1314/echo-noise/internal/models"
)

func TestAccessLogMiddlewareDisabledByDefault(t *testing.T) {
	db, r, _, _ := setupCommentAccountTest(t)
	if err := db.AutoMigrate(&models.SecurityConfig{}, &models.SecurityAccessLog{}); err != nil {
		t.Fatalf("migrate access logs: %v", err)
	}

	r.Use(middleware.AccessLogMiddleware())
	r.GET("/api/messages/page", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	performAccessLogRequest(r, http.MethodGet, "/api/messages/page", "198.51.100.7:1234", "access-agent")
	var count int64
	if err := db.Model(&models.SecurityAccessLog{}).Count(&count).Error; err != nil {
		t.Fatalf("count access logs: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected access logs to stay disabled by default, got %d", count)
	}

	if err := db.Create(&models.SecurityConfig{AccessLogEnabled: true}).Error; err != nil {
		t.Fatalf("enable access logs: %v", err)
	}
	performAccessLogRequest(r, http.MethodGet, "/api/messages/page", "198.51.100.7:1234", "access-agent")
	if err := db.Model(&models.SecurityAccessLog{}).Count(&count).Error; err != nil {
		t.Fatalf("count access logs after enabling: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one access log after enabling, got %d", count)
	}
}

func TestAccessLogMiddlewareRecordsDynamicRequestWithUser(t *testing.T) {
	db, r, user, _ := setupCommentAccountTest(t)
	if err := db.AutoMigrate(&models.SecurityConfig{}, &models.SecurityAccessLog{}); err != nil {
		t.Fatalf("migrate access logs: %v", err)
	}
	if err := db.Create(&models.SecurityConfig{AccessLogEnabled: true}).Error; err != nil {
		t.Fatalf("enable access logs: %v", err)
	}

	r.Use(middleware.AccessLogMiddleware())
	r.GET("/api/messages/page", func(c *gin.Context) {
		c.Set("user_id", user.ID)
		c.Set("username", user.Username)
		c.Set("is_admin", false)
		c.JSON(http.StatusCreated, gin.H{"ok": true})
	})
	r.GET("/_nuxt/app.js", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	performAccessLogRequest(r, http.MethodGet, "/api/messages/page", "198.51.100.7:1234", "access-agent")
	performAccessLogRequest(r, http.MethodGet, "/_nuxt/app.js", "198.51.100.8:1234", "static-agent")

	var logs []models.SecurityAccessLog
	if err := db.Find(&logs).Error; err != nil {
		t.Fatalf("list access logs: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("expected one dynamic access log, got %d", len(logs))
	}
	log := logs[0]
	if log.IP != "198.51.100.7" || log.Method != "GET" || log.Path != "/api/messages/page" || log.Status != http.StatusCreated {
		t.Fatalf("unexpected access log request fields: %#v", log)
	}
	if log.UserID != user.ID || log.Username != user.Username || log.IsAdmin {
		t.Fatalf("unexpected access log user fields: %#v", log)
	}
	if log.UserAgent != "access-agent" {
		t.Fatalf("expected user agent to be recorded, got %q", log.UserAgent)
	}
}

func TestGetAccessLogsFiltersAndClears(t *testing.T) {
	db, r, user, _ := setupCommentAccountTest(t)
	if err := db.AutoMigrate(&models.SecurityAccessLog{}); err != nil {
		t.Fatalf("migrate access logs: %v", err)
	}
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatalf("load timezone: %v", err)
	}

	rows := []models.SecurityAccessLog{
		{IP: "198.51.100.7", Method: "GET", Path: "/api/messages/page", Status: 200, UserID: user.ID, Username: user.Username, UserAgent: "first"},
		{IP: "203.0.113.8", Method: "POST", Path: "/api/messages", Status: 201, UserID: 0, Username: "", UserAgent: "second"},
	}
	rows[0].CreatedAt = time.Date(2026, 1, 2, 9, 0, 0, 0, loc)
	rows[1].CreatedAt = time.Date(2026, 1, 3, 9, 0, 0, 0, loc)
	for _, row := range rows {
		if err := db.Create(&row).Error; err != nil {
			t.Fatalf("create access log row: %v", err)
		}
	}

	r.GET("/security/access-logs", GetAccessLogs)
	r.DELETE("/security/access-logs", ClearAccessLogs)

	filtered := decodeAccessLogsResponse(t, performAccessLogRequest(r, http.MethodGet, "/security/access-logs?ip=198.51.100.7&username=ali&method=GET&path=messages&status=200&startDate=2026-01-02&endDate=2026-01-02", "127.0.0.1:1", "test"))
	if len(filtered) != 1 || filtered[0].Username != user.Username || filtered[0].IP != "198.51.100.7" {
		t.Fatalf("expected filtered access log, got %#v", filtered)
	}

	visitor := decodeAccessLogsResponse(t, performAccessLogRequest(r, http.MethodGet, "/security/access-logs?username=%E8%AE%BF%E5%AE%A2", "127.0.0.1:1", "test"))
	if len(visitor) != 1 || visitor[0].UserID != 0 || visitor[0].IP != "203.0.113.8" {
		t.Fatalf("expected visitor access log, got %#v", visitor)
	}

	selectedUsers := decodeAccessLogsResponse(t, performAccessLogRequest(r, http.MethodGet, "/security/access-logs?user_ids=0,"+strconv.Itoa(int(user.ID)), "127.0.0.1:1", "test"))
	if len(selectedUsers) != 2 {
		t.Fatalf("expected selected visitor and user logs, got %#v", selectedUsers)
	}

	clear := performAccessLogRequest(r, http.MethodDelete, "/security/access-logs", "127.0.0.1:1", "test")
	if clear.Code != http.StatusOK {
		t.Fatalf("expected clear 200, got %d: %s", clear.Code, clear.Body.String())
	}
	var count int64
	if err := db.Model(&models.SecurityAccessLog{}).Count(&count).Error; err != nil {
		t.Fatalf("count access logs: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected access logs to be cleared, got %d", count)
	}
}

func performAccessLogRequest(r http.Handler, method string, path string, remoteAddr string, userAgent string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	req.RemoteAddr = remoteAddr
	req.Header.Set("User-Agent", userAgent)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func decodeAccessLogsResponse(t *testing.T, w *httptest.ResponseRecorder) []models.SecurityAccessLog {
	t.Helper()
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Code int                        `json:"code"`
		Data []models.SecurityAccessLog `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Code != 1 {
		t.Fatalf("expected success response, got %#v", resp)
	}
	return resp.Data
}
