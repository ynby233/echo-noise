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

	user := models.User{Username: "alice", Password: "", AvatarURL: "https://example.com/avatar.png"}
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

func performCommentJSONRequest(r http.Handler, method string, messageID uint, commentID uint, body map[string]any) *httptest.ResponseRecorder {
	payload, _ := json.Marshal(body)
	path := "/messages/" + strconvFormatUint(messageID) + "/comments/" + strconvFormatUint(commentID)
	req := httptest.NewRequest(method, path, bytes.NewReader(payload))
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

func TestOwnerCanUpdateOwnCommentContentAndVisibility(t *testing.T) {
	db, r, user, msg := setupCommentAccountTest(t)
	r.Use(func(c *gin.Context) {
		c.Set("user_id", user.ID)
		c.Set("is_admin", false)
		c.Next()
	})
	r.PUT("/messages/:id/comments/:cid", UpdateComment)

	comment := models.Comment{MessageID: msg.ID, UserID: &user.ID, Nick: user.Username, Content: "old", Visibility: "public"}
	if err := db.Create(&comment).Error; err != nil {
		t.Fatalf("create comment: %v", err)
	}

	w := performCommentJSONRequest(r, http.MethodPut, msg.ID, comment.ID, map[string]any{
		"content":    "updated content",
		"visibility": "private",
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
	if resp.Code != 1 || resp.Data.Content != "updated content" || resp.Data.Visibility != "private" {
		t.Fatalf("expected updated content and visibility, got %#v", resp)
	}
	var saved models.Comment
	if err := db.First(&saved, comment.ID).Error; err != nil {
		t.Fatalf("load saved comment: %v", err)
	}
	if saved.Content != "updated content" || saved.Visibility != "private" {
		t.Fatalf("saved comment mismatch: %#v", saved)
	}
}

func TestUserCannotUpdateOthersComment(t *testing.T) {
	db, r, user, msg := setupCommentAccountTest(t)
	other := models.User{Username: "bob", Password: ""}
	if err := db.Create(&other).Error; err != nil {
		t.Fatalf("create other user: %v", err)
	}
	r.Use(func(c *gin.Context) {
		c.Set("user_id", user.ID)
		c.Set("is_admin", false)
		c.Next()
	})
	r.PUT("/messages/:id/comments/:cid", UpdateComment)

	comment := models.Comment{MessageID: msg.ID, UserID: &other.ID, Nick: other.Username, Content: "other", Visibility: "public"}
	if err := db.Create(&comment).Error; err != nil {
		t.Fatalf("create comment: %v", err)
	}

	w := performCommentJSONRequest(r, http.MethodPut, msg.ID, comment.ID, map[string]any{
		"content":    "hacked",
		"visibility": "private",
	})
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
	var saved models.Comment
	if err := db.First(&saved, comment.ID).Error; err != nil {
		t.Fatalf("load saved comment: %v", err)
	}
	if saved.Content != "other" || saved.Visibility != "public" {
		t.Fatalf("expected other user's comment unchanged, got %#v", saved)
	}
}

func TestOwnerCanDeleteOwnCommentButNotOthers(t *testing.T) {
	db, r, user, msg := setupCommentAccountTest(t)
	other := models.User{Username: "bob", Password: ""}
	if err := db.Create(&other).Error; err != nil {
		t.Fatalf("create other user: %v", err)
	}
	r.Use(func(c *gin.Context) {
		c.Set("user_id", user.ID)
		c.Set("is_admin", false)
		c.Next()
	})
	r.DELETE("/messages/:id/comments/:cid", DeleteComment)

	ownComment := models.Comment{MessageID: msg.ID, UserID: &user.ID, Nick: user.Username, Content: "mine", Visibility: "public"}
	otherComment := models.Comment{MessageID: msg.ID, UserID: &other.ID, Nick: other.Username, Content: "other", Visibility: "public"}
	if err := db.Create(&ownComment).Error; err != nil {
		t.Fatalf("create own comment: %v", err)
	}
	if err := db.Create(&otherComment).Error; err != nil {
		t.Fatalf("create other comment: %v", err)
	}

	w := performCommentJSONRequest(r, http.MethodDelete, msg.ID, ownComment.ID, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected owner delete 200, got %d: %s", w.Code, w.Body.String())
	}
	var count int64
	if err := db.Model(&models.Comment{}).Where("id = ?", ownComment.ID).Count(&count).Error; err != nil {
		t.Fatalf("count own comment: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected own comment deleted, got count %d", count)
	}

	w = performCommentJSONRequest(r, http.MethodDelete, msg.ID, otherComment.ID, nil)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected other delete 403, got %d: %s", w.Code, w.Body.String())
	}
	if err := db.Model(&models.Comment{}).Where("id = ?", otherComment.ID).Count(&count).Error; err != nil {
		t.Fatalf("count other comment: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected other comment retained, got count %d", count)
	}
}

func TestGetCommentsFiltersVisibility(t *testing.T) {
	db, r, owner, msg := setupCommentAccountTest(t)
	other := models.User{Username: "bob", Password: ""}
	if err := db.Create(&other).Error; err != nil {
		t.Fatalf("create other user: %v", err)
	}
	r.Use(func(c *gin.Context) {
		if raw := c.GetHeader("X-Test-User-ID"); raw != "" {
			id, _ := strconv.ParseUint(raw, 10, 64)
			c.Set("user_id", uint(id))
			c.Set("is_admin", c.GetHeader("X-Test-Is-Admin") == "true")
		}
		c.Next()
	})
	r.GET("/messages/:id/comments", GetComments)

	comments := []models.Comment{
		{MessageID: msg.ID, UserID: &owner.ID, Nick: owner.Username, Content: "public", Visibility: "public"},
		{MessageID: msg.ID, UserID: &owner.ID, Nick: owner.Username, Content: "users", Visibility: "users"},
		{MessageID: msg.ID, UserID: &owner.ID, Nick: owner.Username, Content: "private", Visibility: "private"},
		{MessageID: msg.ID, UserID: &owner.ID, Nick: owner.Username, Content: "contacts", Visibility: "contacts"},
	}
	for i := range comments {
		if err := db.Create(&comments[i]).Error; err != nil {
			t.Fatalf("create comment %d: %v", i, err)
		}
	}

	decode := func(w *httptest.ResponseRecorder) []models.Comment {
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
		if resp.Code != 1 {
			t.Fatalf("expected success response, got %#v", resp)
		}
		return resp.Data
	}
	request := func(userID uint, isAdmin bool) []models.Comment {
		req := httptest.NewRequest(http.MethodGet, "/messages/"+strconvFormatUint(msg.ID)+"/comments", nil)
		if userID > 0 {
			req.Header.Set("X-Test-User-ID", strconvFormatUint(userID))
		}
		if isAdmin {
			req.Header.Set("X-Test-Is-Admin", "true")
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return decode(w)
	}
	contents := func(list []models.Comment) []string {
		out := make([]string, 0, len(list))
		for _, c := range list {
			out = append(out, c.Content)
		}
		return out
	}

	if got := contents(request(0, false)); len(got) != 1 || got[0] != "public" {
		t.Fatalf("anonymous should only see public comments, got %#v", got)
	}
	if got := contents(request(other.ID, false)); len(got) != 2 || got[0] != "public" || got[1] != "users" {
		t.Fatalf("logged-in non-owner should see public/users comments, got %#v", got)
	}
	if got := contents(request(owner.ID, false)); len(got) != 4 {
		t.Fatalf("owner should see all own comments, got %#v", got)
	}
	if got := contents(request(other.ID, true)); len(got) != 4 {
		t.Fatalf("admin should see all comments, got %#v", got)
	}
}
