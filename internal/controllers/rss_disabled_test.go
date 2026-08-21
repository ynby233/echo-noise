package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRSSHandlersReturnNotFoundWhenDisabled(t *testing.T) {
	_, r, _, _ := setupCommentAccountTest(t)

	t.Run("rss feed", func(t *testing.T) {
		r.GET("/rss", GenerateRSS)

		req := httptest.NewRequest(http.MethodGet, "/rss", nil)
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
