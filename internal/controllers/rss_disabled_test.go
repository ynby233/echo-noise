package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/internal/database"
)

func TestRSSHandlersReturnNotFoundWhenDisabled(t *testing.T) {
	_, r, user, _ := setupCommentAccountTest(t)

	t.Run("rss feed", func(t *testing.T) {
		r.GET("/rss", GenerateRSS)

		req := httptest.NewRequest(http.MethodGet, "/rss", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assertRSSDisabledResponse(t, w)
	})

	t.Run("rss refresh", func(t *testing.T) {
		user.IsAdmin = true
		if err := database.DB.Save(&user).Error; err != nil {
			t.Fatalf("promote user: %v", err)
		}

		r.POST("/api/rss/refresh", func(c *gin.Context) {
			c.Set("user_id", user.ID)
			RefreshRSS(c)
		})

		req := httptest.NewRequest(http.MethodPost, "/api/rss/refresh", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assertRSSDisabledResponse(t, w)
	})
}

func assertRSSDisabledResponse(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Code != 0 || resp.Msg != "RSS 已禁用" {
		t.Fatalf("unexpected response: %#v", resp)
	}
}
