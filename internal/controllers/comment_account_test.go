package controllers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupCommentAccountTest(t *testing.T) (*gorm.DB, *gin.Engine, models.User, models.Message) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.Message{}, &models.Comment{}, &models.SiteConfig{}); err != nil {
		t.Fatalf("migrate test db: %v", err)
	}
	database.DB = db
	models.SetDB(db)
	t.Cleanup(func() {
		database.DB = nil
		models.SetDB(nil)
	})

	user := models.User{Username: "alice", Password: "secret", AvatarURL: "https://example.com/avatar.png"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	msg := models.Message{Content: "hello", UserID: user.ID}
	if err := db.Create(&msg).Error; err != nil {
		t.Fatalf("create message: %v", err)
	}

	r := gin.New()
	r.Use(sessions.Sessions("test", cookie.NewStore([]byte("comment-account-test"))))
	return db, r, user, msg
}

func performCommentRequest(r http.Handler, messageID uint, body map[string]any) *httptest.ResponseRecorder {
	payload, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/messages/"+strconvFormatUint(messageID)+"/comments", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func strconvFormatUint(v uint) string {
	return strconv.FormatUint(uint64(v), 10)
}

func TestPostCommentRequiresAccount(t *testing.T) {
	db, r, _, msg := setupCommentAccountTest(t)
	r.POST("/messages/:id/comments", PostComment)

	w := performCommentRequest(r, msg.ID, map[string]any{
		"nick":    "visitor",
		"mail":    "visitor@example.com",
		"link":    "https://example.com",
		"content": "unauthenticated comment",
	})

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 legacy response, got %d", w.Code)
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["code"] != float64(0) || resp["msg"] != "请登录后评论" {
		t.Fatalf("expected login-required response, got %#v", resp)
	}
	var count int64
	if err := db.Model(&models.Comment{}).Count(&count).Error; err != nil {
		t.Fatalf("count comments: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected no comment created, got %d", count)
	}
}

func TestPostCommentBindsCurrentAccountAndIgnoresContactFields(t *testing.T) {
	db, r, user, msg := setupCommentAccountTest(t)
	r.Use(func(c *gin.Context) {
		c.Set("user_id", user.ID)
		c.Next()
	})
	r.POST("/messages/:id/comments", PostComment)

	w := performCommentRequest(r, msg.ID, map[string]any{
		"nick":    "spoofed",
		"mail":    "spoofed@example.com",
		"link":    "https://spoofed.example.com",
		"content": "account comment",
	})

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Code float64        `json:"code"`
		Data models.Comment `json:"data"`
		Msg  string         `json:"msg"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Code != 1 {
		t.Fatalf("expected success response, got %#v", resp)
	}
	if resp.Data.UserID == nil || *resp.Data.UserID != user.ID {
		t.Fatalf("expected comment bound to user %d, got %#v", user.ID, resp.Data.UserID)
	}
	if resp.Data.Nick != user.Username || resp.Data.Mail != "" || resp.Data.Link != "" {
		t.Fatalf("expected account identity and empty contact fields, got nick=%q mail=%q link=%q", resp.Data.Nick, resp.Data.Mail, resp.Data.Link)
	}
	if resp.Data.User == nil || resp.Data.User.ID != user.ID || resp.Data.User.Username != user.Username || resp.Data.User.AvatarURL != user.AvatarURL {
		t.Fatalf("expected comment user info, got %#v", resp.Data.User)
	}

	var saved models.Comment
	if err := db.First(&saved, resp.Data.ID).Error; err != nil {
		t.Fatalf("load saved comment: %v", err)
	}
	if saved.UserID == nil || *saved.UserID != user.ID || saved.Nick != user.Username || saved.Mail != "" || saved.Link != "" {
		t.Fatalf("saved comment mismatch: %#v", saved)
	}
}

func TestGetCommentsReturnsAccountInfoAndKeepsLegacyComments(t *testing.T) {
	db, r, user, msg := setupCommentAccountTest(t)
	r.GET("/messages/:id/comments", GetComments)

	accountComment := models.Comment{
		MessageID: msg.ID,
		UserID:    &user.ID,
		Nick:      "old-account-name",
		Mail:      "private@example.com",
		Link:      "https://private.example.com",
		Content:   "account comment",
	}
	legacyComment := models.Comment{
		MessageID: msg.ID,
		Nick:      "legacy visitor",
		Mail:      "legacy@example.com",
		Link:      "https://legacy.example.com",
		Content:   "legacy comment",
	}
	if err := db.Create(&accountComment).Error; err != nil {
		t.Fatalf("create account comment: %v", err)
	}
	if err := db.Create(&legacyComment).Error; err != nil {
		t.Fatalf("create legacy comment: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/messages/"+strconvFormatUint(msg.ID)+"/comments", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Code float64          `json:"code"`
		Data []models.Comment `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Code != 1 || len(resp.Data) != 2 {
		t.Fatalf("expected two comments, got %#v", resp)
	}
	if resp.Data[0].User == nil || resp.Data[0].User.ID != user.ID || resp.Data[0].User.AvatarURL != user.AvatarURL {
		t.Fatalf("expected account user info on first comment, got %#v", resp.Data[0])
	}
	if resp.Data[0].Nick != user.Username || resp.Data[0].Mail != "" || resp.Data[0].Link != "" {
		t.Fatalf("expected account comment identity cleanup, got %#v", resp.Data[0])
	}
	if resp.Data[1].User != nil || resp.Data[1].Nick != legacyComment.Nick || resp.Data[1].Mail != legacyComment.Mail || resp.Data[1].Link != legacyComment.Link {
		t.Fatalf("expected legacy comment preserved, got %#v", resp.Data[1])
	}
}
