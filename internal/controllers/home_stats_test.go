package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/internal/models"
)

func TestGetCurrentUserHomeStatsScopesToCurrentUser(t *testing.T) {
	db, r, alice, _ := setupCommentAccountTest(t)
	if err := db.Where("1 = 1").Delete(&models.Message{}).Error; err != nil {
		t.Fatalf("clear seed messages: %v", err)
	}

	bob := models.User{Username: "bob", IsAdmin: true}
	if err := db.Create(&bob).Error; err != nil {
		t.Fatalf("create bob: %v", err)
	}

	messages := []models.Message{
		{Content: "alice public #life #daily ![md](/alice-md.png)", ImageURL: "/alice-field.png", UserID: alice.ID, Username: alice.Username},
		{Content: "alice private #secret", ImageURL: "/alice-private.png", UserID: alice.ID, Username: alice.Username, Private: true},
		{Content: models.CanonicalGuestbookContent, ImageURL: "/hidden-field.png", UserID: alice.ID, Username: alice.Username, IsGuestbook: true},
		{Content: "bob admin #life #admin", ImageURL: "/bob-field.png", UserID: bob.ID, Username: bob.Username},
	}
	for _, msg := range messages {
		if err := db.Create(&msg).Error; err != nil {
			t.Fatalf("create message %q: %v", msg.Content, err)
		}
	}

	r.GET("/users/me/stats", func(c *gin.Context) {
		switch c.GetHeader("X-Test-User") {
		case "alice":
			c.Set("user_id", alice.ID)
		case "bob":
			c.Set("user_id", bob.ID)
		}
		GetCurrentUserHomeStats(c)
	})

	aliceStats := decodeHomeStatsResponse(t, performHomeStatsRequest(r, "alice"), http.StatusOK)
	if aliceStats.TotalMessages != 2 || aliceStats.TotalTags != 3 || aliceStats.TotalImages != 3 {
		t.Fatalf("expected alice personal stats 2/3/3, got %#v", aliceStats)
	}

	bobStats := decodeHomeStatsResponse(t, performHomeStatsRequest(r, "bob"), http.StatusOK)
	if bobStats.TotalMessages != 1 || bobStats.TotalTags != 2 || bobStats.TotalImages != 1 {
		t.Fatalf("expected bob personal stats 1/2/1, got %#v", bobStats)
	}

	decodeHomeStatsResponse(t, performHomeStatsRequest(r, ""), http.StatusUnauthorized)
}

func performHomeStatsRequest(r http.Handler, user string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/users/me/stats", nil)
	if user != "" {
		req.Header.Set("X-Test-User", user)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func decodeHomeStatsResponse(t *testing.T, w *httptest.ResponseRecorder, wantStatus int) struct {
	TotalMessages int `json:"total_messages"`
	TotalTags     int `json:"total_tags"`
	TotalImages   int `json:"total_images"`
} {
	t.Helper()
	if w.Code != wantStatus {
		t.Fatalf("expected status %d, got %d: %s", wantStatus, w.Code, w.Body.String())
	}

	var resp struct {
		Code int `json:"code"`
		Data struct {
			TotalMessages int `json:"total_messages"`
			TotalTags     int `json:"total_tags"`
			TotalImages   int `json:"total_images"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if wantStatus == http.StatusOK && resp.Code != 1 {
		t.Fatalf("expected success response, got %#v", resp)
	}
	return resp.Data
}
