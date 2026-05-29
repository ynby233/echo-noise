package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRSSHandlersReturnNotFoundWhenDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name    string
		method  string
		path    string
		handler gin.HandlerFunc
	}{
		{name: "rss feed", method: http.MethodGet, path: "/rss", handler: GenerateRSS},
		{name: "rss refresh", method: http.MethodPost, path: "/api/rss/refresh", handler: RefreshRSS},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := gin.New()
			r.Handle(tt.method, tt.path, tt.handler)

			req := httptest.NewRequest(tt.method, tt.path, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

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
		})
	}
}
