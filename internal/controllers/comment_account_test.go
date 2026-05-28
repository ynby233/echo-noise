package controllers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
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

func createTestComment(t *testing.T, db *gorm.DB, messageID uint, author *models.User, content string, visibility string, parentID *uint) models.Comment {
	t.Helper()
	comment := models.Comment{MessageID: messageID, Content: content, Visibility: visibility, ParentID: parentID}
	if author != nil {
		comment.UserID = &author.ID
		comment.Nick = author.Username
	} else {
		comment.Nick = "legacy"
	}
	if err := db.Create(&comment).Error; err != nil {
		t.Fatalf("create comment %s: %v", content, err)
	}
	return comment
}

func decodeCommentListResponse(t *testing.T, w *httptest.ResponseRecorder) []models.Comment {
	t.Helper()
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

func decodeCommentCountResponse(t *testing.T, w *httptest.ResponseRecorder) map[uint]int64 {
	t.Helper()
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Code float64 `json:"code"`
		Data []struct {
			ID    uint  `json:"id"`
			Count int64 `json:"count"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode count response: %v", err)
	}
	if resp.Code != 1 {
		t.Fatalf("expected success count response, got %#v", resp)
	}
	result := make(map[uint]int64, len(resp.Data))
	for _, item := range resp.Data {
		result[item.ID] = item.Count
	}
	return result
}

func contentsOfComments(list []models.Comment) []string {
	out := make([]string, 0, len(list))
	for _, c := range list {
		out = append(out, c.Content)
	}
	sort.Strings(out)
	return out
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
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Code != 1 {
		t.Fatalf("expected success response, got %#v", resp)
	}
	if resp.Data.UserID == nil || *resp.Data.UserID != user.ID {
		t.Fatalf("expected response user_id %d, got %#v", user.ID, resp.Data.UserID)
	}
	if resp.Data.Nick != user.Username {
		t.Fatalf("expected response nick %q, got %q", user.Username, resp.Data.Nick)
	}
	if resp.Data.Mail != "" || resp.Data.Link != "" {
		t.Fatalf("expected response contact fields cleared, got mail=%q link=%q", resp.Data.Mail, resp.Data.Link)
	}
	if resp.Data.User == nil || resp.Data.User.ID != user.ID || resp.Data.User.Username != user.Username || resp.Data.User.AvatarURL != user.AvatarURL {
		t.Fatalf("expected response user info populated, got %#v", resp.Data.User)
	}

	var saved models.Comment
	if err := db.First(&saved, resp.Data.ID).Error; err != nil {
		t.Fatalf("load saved comment: %v", err)
	}
	if saved.UserID == nil || *saved.UserID != user.ID {
		t.Fatalf("expected saved user_id %d, got %#v", user.ID, saved.UserID)
	}
	if saved.Nick != user.Username {
		t.Fatalf("expected saved nick %q, got %q", user.Username, saved.Nick)
	}
	if saved.Mail != "" || saved.Link != "" {
		t.Fatalf("expected saved contact fields cleared, got mail=%q link=%q", saved.Mail, saved.Link)
	}
}

func TestGetCommentsReturnsAccountInfoAndKeepsLegacyComments(t *testing.T) {
	db, r, user, msg := setupCommentAccountTest(t)
	r.GET("/messages/:id/comments", GetComments)

	accountComment := models.Comment{
		MessageID: msg.ID,
		UserID:    &user.ID,
		Nick:      "outdated-nick",
		Mail:      "secret@example.com",
		Link:      "https://hidden.example.com",
		Content:   "account-backed",
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

	comments := decodeCommentListResponse(t, w)
	if len(comments) != 2 {
		t.Fatalf("expected 2 comments, got %d", len(comments))
	}

	var gotAccount *models.Comment
	var gotLegacy *models.Comment
	for i := range comments {
		comment := comments[i]
		switch comment.Content {
		case "account-backed":
			gotAccount = &comment
		case "legacy comment":
			gotLegacy = &comment
		}
	}
	if gotAccount == nil || gotLegacy == nil {
		t.Fatalf("unexpected comments payload: %#v", comments)
	}
	if gotAccount.User == nil {
		t.Fatalf("expected account comment user info, got nil")
	}
	if gotAccount.User.ID != user.ID || gotAccount.User.Username != user.Username || gotAccount.User.AvatarURL != user.AvatarURL {
		t.Fatalf("unexpected account user info: %#v", gotAccount.User)
	}
	if gotAccount.Nick != user.Username {
		t.Fatalf("expected account nick overwritten with username %q, got %q", user.Username, gotAccount.Nick)
	}
	if gotAccount.Mail != "" || gotAccount.Link != "" {
		t.Fatalf("expected account contact fields cleared, got mail=%q link=%q", gotAccount.Mail, gotAccount.Link)
	}
	if gotLegacy.User != nil {
		t.Fatalf("expected legacy comment user info nil, got %#v", gotLegacy.User)
	}
	if gotLegacy.Nick != legacyComment.Nick || gotLegacy.Mail != legacyComment.Mail || gotLegacy.Link != legacyComment.Link {
		t.Fatalf("expected legacy fields kept, got %#v", gotLegacy)
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
		"content":    "updated",
		"visibility": "private",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var saved models.Comment
	if err := db.First(&saved, comment.ID).Error; err != nil {
		t.Fatalf("load saved comment: %v", err)
	}
	if saved.Content != "updated" || saved.Visibility != "private" {
		t.Fatalf("expected updated/private, got %#v", saved)
	}
}

func TestUserCannotUpdateOthersComment(t *testing.T) {
	db, r, user, msg := setupCommentAccountTest(t)
	other := models.User{Username: "bob", Password: ""}
	if err := db.Create(&other).Error; err != nil {
		t.Fatalf("create other user: %v", err)
	}
	_ = user
	r.Use(func(c *gin.Context) {
		c.Set("user_id", other.ID)
		c.Set("is_admin", false)
		c.Next()
	})
	r.PUT("/messages/:id/comments/:cid", UpdateComment)

	comment := models.Comment{MessageID: msg.ID, UserID: &user.ID, Nick: user.Username, Content: "mine", Visibility: "public"}
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
	if saved.Content != "mine" || saved.Visibility != "public" {
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
		return decodeCommentListResponse(t, w)
	}

	if got := contentsOfComments(request(0, false)); len(got) != 1 || got[0] != "public" {
		t.Fatalf("anonymous should only see public comments, got %#v", got)
	}
	if got := contentsOfComments(request(other.ID, false)); len(got) != 2 || got[0] != "public" || got[1] != "users" {
		t.Fatalf("logged-in non-owner should see public/users comments, got %#v", got)
	}
	if got := contentsOfComments(request(owner.ID, false)); len(got) != 4 {
		t.Fatalf("owner should see all own comments, got %#v", got)
	}
	if got := contentsOfComments(request(other.ID, true)); len(got) != 4 {
		t.Fatalf("admin should see all comments, got %#v", got)
	}
}

func TestPrivateCommentVisibilityMatchesNewRules(t *testing.T) {
	db, r, postAuthor, msg := setupCommentAccountTest(t)
	commenter := models.User{Username: "bob", Password: ""}
	outsider := models.User{Username: "charlie", Password: ""}
	admin := models.User{Username: "admin", Password: "", IsAdmin: true}
	for _, u := range []*models.User{&commenter, &outsider, &admin} {
		if err := db.Create(u).Error; err != nil {
			t.Fatalf("create user %s: %v", u.Username, err)
		}
	}
	privateComment := createTestComment(t, db, msg.ID, &commenter, "private-comment", "private", nil)
	_ = privateComment

	r.Use(func(c *gin.Context) {
		if raw := c.GetHeader("X-Test-User-ID"); raw != "" {
			id, _ := strconv.ParseUint(raw, 10, 64)
			c.Set("user_id", uint(id))
		}
		if c.GetHeader("X-Test-Is-Admin") == "true" {
			c.Set("is_admin", true)
		}
		c.Next()
	})
	r.GET("/messages/:id/comments", GetComments)

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
		return decodeCommentListResponse(t, w)
	}

	if got := contentsOfComments(request(0, false)); len(got) != 0 {
		t.Fatalf("anonymous should not see private comment, got %#v", got)
	}
	if got := contentsOfComments(request(outsider.ID, false)); len(got) != 0 {
		t.Fatalf("outsider should not see private comment, got %#v", got)
	}
	if got := contentsOfComments(request(commenter.ID, false)); len(got) != 1 || got[0] != "private-comment" {
		t.Fatalf("comment author should see own private comment, got %#v", got)
	}
	if got := contentsOfComments(request(postAuthor.ID, false)); len(got) != 1 || got[0] != "private-comment" {
		t.Fatalf("post author should see private comment, got %#v", got)
	}
	if got := contentsOfComments(request(admin.ID, true)); len(got) != 1 || got[0] != "private-comment" {
		t.Fatalf("admin should see private comment, got %#v", got)
	}
}

func TestReplyVisibilityMatchesNewRules(t *testing.T) {
	db, r, postAuthor, msg := setupCommentAccountTest(t)
	parentAuthor := models.User{Username: "bob", Password: ""}
	replyAuthor := models.User{Username: "charlie", Password: ""}
	outsider := models.User{Username: "dave", Password: ""}
	admin := models.User{Username: "admin", Password: "", IsAdmin: true}
	for _, u := range []*models.User{&parentAuthor, &replyAuthor, &outsider, &admin} {
		if err := db.Create(u).Error; err != nil {
			t.Fatalf("create user %s: %v", u.Username, err)
		}
	}
	parent := createTestComment(t, db, msg.ID, &parentAuthor, "parent-private", "private", nil)
	createTestComment(t, db, msg.ID, &replyAuthor, "reply-private", "private", &parent.ID)
	_ = postAuthor

	r.Use(func(c *gin.Context) {
		if raw := c.GetHeader("X-Test-User-ID"); raw != "" {
			id, _ := strconv.ParseUint(raw, 10, 64)
			c.Set("user_id", uint(id))
		}
		if c.GetHeader("X-Test-Is-Admin") == "true" {
			c.Set("is_admin", true)
		}
		c.Next()
	})
	r.GET("/messages/:id/comments", GetComments)

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
		return decodeCommentListResponse(t, w)
	}

	if got := contentsOfComments(request(outsider.ID, false)); len(got) != 0 {
		t.Fatalf("outsider should not see private reply chain, got %#v", got)
	}
	if got := contentsOfComments(request(parentAuthor.ID, false)); len(got) != 2 {
		t.Fatalf("parent author should see parent and reply, got %#v", got)
	}
	if got := contentsOfComments(request(postAuthor.ID, false)); len(got) != 1 || got[0] != "parent-private" {
		t.Fatalf("post author should not see private reply, got %#v", got)
	}
	if got := contentsOfComments(request(replyAuthor.ID, false)); len(got) != 1 || got[0] != "reply-private" {
		t.Fatalf("reply author should only see own reply, got %#v", got)
	}
	if got := contentsOfComments(request(admin.ID, true)); len(got) != 2 {
		t.Fatalf("admin should see private reply chain, got %#v", got)
	}
}

func TestCommentCountsFollowVisibilityRules(t *testing.T) {
	db, r, postAuthor, msg := setupCommentAccountTest(t)
	commenter := models.User{Username: "bob", Password: ""}
	outsider := models.User{Username: "charlie", Password: ""}
	admin := models.User{Username: "admin", Password: "", IsAdmin: true}
	for _, u := range []*models.User{&commenter, &outsider, &admin} {
		if err := db.Create(u).Error; err != nil {
			t.Fatalf("create user %s: %v", u.Username, err)
		}
	}
	createTestComment(t, db, msg.ID, &postAuthor, "public", "public", nil)
	createTestComment(t, db, msg.ID, &commenter, "private", "private", nil)
	createTestComment(t, db, msg.ID, &commenter, "users", "users", nil)

	r.Use(func(c *gin.Context) {
		if raw := c.GetHeader("X-Test-User-ID"); raw != "" {
			id, _ := strconv.ParseUint(raw, 10, 64)
			c.Set("user_id", uint(id))
		}
		if c.GetHeader("X-Test-Is-Admin") == "true" {
			c.Set("is_admin", true)
		}
		c.Next()
	})
	r.POST("/messages/comments/counts", GetCommentCounts)

	request := func(userID uint, isAdmin bool) map[uint]int64 {
		body, _ := json.Marshal(map[string]any{"ids": []uint{msg.ID}})
		req := httptest.NewRequest(http.MethodPost, "/messages/comments/counts", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if userID > 0 {
			req.Header.Set("X-Test-User-ID", strconvFormatUint(userID))
		}
		if isAdmin {
			req.Header.Set("X-Test-Is-Admin", "true")
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return decodeCommentCountResponse(t, w)
	}

	if got := request(0, false)[msg.ID]; got != 1 {
		t.Fatalf("anonymous should count only public comments, got %d", got)
	}
	if got := request(outsider.ID, false)[msg.ID]; got != 2 {
		t.Fatalf("outsider should count public + users comments only, got %d", got)
	}
	if got := request(commenter.ID, false)[msg.ID]; got != 3 {
		t.Fatalf("comment author should count all visible comments, got %d", got)
	}
	if got := request(postAuthor.ID, false)[msg.ID]; got != 3 {
		t.Fatalf("post author should count all visible comments on own post, got %d", got)
	}
	if got := request(admin.ID, true)[msg.ID]; got != 3 {
		t.Fatalf("admin should count all comments, got %d", got)
	}
}

func TestAdminCannotReplyPrivateComment(t *testing.T) {
	db, r, postAuthor, msg := setupCommentAccountTest(t)
	commenter := models.User{Username: "bob", Password: ""}
	admin := models.User{Username: "admin", Password: "", IsAdmin: true}
	for _, u := range []*models.User{&commenter, &admin} {
		if err := db.Create(u).Error; err != nil {
			t.Fatalf("create user %s: %v", u.Username, err)
		}
	}
	privateComment := createTestComment(t, db, msg.ID, &commenter, "private-comment", "private", nil)
	_ = postAuthor

	r.Use(func(c *gin.Context) {
		c.Set("user_id", admin.ID)
		c.Set("is_admin", true)
		c.Next()
	})
	r.POST("/messages/:id/comments", PostComment)

	w := performCommentRequest(r, msg.ID, map[string]any{
		"content":    "admin-reply",
		"visibility": "private",
		"parent_id":  privateComment.ID,
	})
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 when admin replies private comment, got %d: %s", w.Code, w.Body.String())
	}
	var count int64
	if err := db.Model(&models.Comment{}).Where("content = ?", "admin-reply").Count(&count).Error; err != nil {
		t.Fatalf("count replies: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected no admin private reply created, got %d", count)
	}
}

func TestReplyCannotBroadenParentVisibility(t *testing.T) {
	db, r, _, msg := setupCommentAccountTest(t)
	parentAuthor := models.User{Username: "bob", Password: ""}
	replyAuthor := models.User{Username: "charlie", Password: ""}
	for _, u := range []*models.User{&parentAuthor, &replyAuthor} {
		if err := db.Create(u).Error; err != nil {
			t.Fatalf("create user %s: %v", u.Username, err)
		}
	}
	parent := createTestComment(t, db, msg.ID, &parentAuthor, "parent-users", "users", nil)

	r.Use(func(c *gin.Context) {
		c.Set("user_id", replyAuthor.ID)
		c.Set("is_admin", false)
		c.Next()
	})
	r.POST("/messages/:id/comments", PostComment)

	w := performCommentRequest(r, msg.ID, map[string]any{
		"content":    "reply-public",
		"visibility": "public",
		"parent_id":  parent.ID,
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when reply broadens parent visibility, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGuestbookVisibilityAndReplyRestrictions(t *testing.T) {
	db, r, _, _ := setupCommentAccountTest(t)
	admin := models.User{Username: "admin", Password: "", IsAdmin: true}
	visitor := models.User{Username: "bob", Password: ""}
	outsider := models.User{Username: "charlie", Password: ""}
	for _, u := range []*models.User{&admin, &visitor, &outsider} {
		if err := db.Create(u).Error; err != nil {
			t.Fatalf("create user %s: %v", u.Username, err)
		}
	}
	guestbook := models.Message{Content: "留言板\n\n#留言 #guestbook", UserID: admin.ID}
	if err := db.Create(&guestbook).Error; err != nil {
		t.Fatalf("create guestbook: %v", err)
	}
	entry := createTestComment(t, db, guestbook.ID, &visitor, "guestbook-entry", "public", nil)
	createTestComment(t, db, guestbook.ID, &admin, "admin-reply", "private", &entry.ID)

	r.Use(func(c *gin.Context) {
		if raw := c.GetHeader("X-Test-User-ID"); raw != "" {
			id, _ := strconv.ParseUint(raw, 10, 64)
			c.Set("user_id", uint(id))
		}
		if c.GetHeader("X-Test-Is-Admin") == "true" {
			c.Set("is_admin", true)
		}
		c.Next()
	})
	r.GET("/messages/:id/comments", GetComments)
	r.POST("/messages/:id/comments", PostComment)

	request := func(userID uint, isAdmin bool) []models.Comment {
		req := httptest.NewRequest(http.MethodGet, "/messages/"+strconvFormatUint(guestbook.ID)+"/comments", nil)
		if userID > 0 {
			req.Header.Set("X-Test-User-ID", strconvFormatUint(userID))
		}
		if isAdmin {
			req.Header.Set("X-Test-Is-Admin", "true")
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return decodeCommentListResponse(t, w)
	}

	if got := contentsOfComments(request(0, false)); len(got) != 0 {
		t.Fatalf("anonymous should not see guestbook thread, got %#v", got)
	}
	if got := contentsOfComments(request(outsider.ID, false)); len(got) != 0 {
		t.Fatalf("outsider should not see guestbook thread, got %#v", got)
	}
	if got := contentsOfComments(request(visitor.ID, false)); len(got) != 2 {
		t.Fatalf("guestbook author should see entry and admin reply, got %#v", got)
	}
	if got := contentsOfComments(request(admin.ID, true)); len(got) != 2 {
		t.Fatalf("admin should see guestbook thread, got %#v", got)
	}

	payload, _ := json.Marshal(map[string]any{
		"content":    "visitor-reply",
		"visibility": "private",
		"parent_id":  entry.ID,
	})
	req := httptest.NewRequest(http.MethodPost, "/messages/"+strconvFormatUint(guestbook.ID)+"/comments", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Test-User-ID", strconvFormatUint(visitor.ID))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 when non-admin replies guestbook entry, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAdminCannotCommentPrivateMessage(t *testing.T) {
	db, r, postAuthor, _ := setupCommentAccountTest(t)
	admin := models.User{Username: "admin", Password: "", IsAdmin: true}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatalf("create admin: %v", err)
	}
	privateMsg := models.Message{Content: "secret", UserID: postAuthor.ID, Private: true}
	if err := db.Create(&privateMsg).Error; err != nil {
		t.Fatalf("create private message: %v", err)
	}

	r.Use(func(c *gin.Context) {
		c.Set("user_id", admin.ID)
		c.Set("is_admin", true)
		c.Next()
	})
	r.POST("/messages/:id/comments", PostComment)

	w := performCommentRequest(r, privateMsg.ID, map[string]any{
		"content":    "admin-comment",
		"visibility": "private",
	})
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 when admin comments private message, got %d: %s", w.Code, w.Body.String())
	}
}
