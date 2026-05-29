package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"

	"github.com/rcy1314/echo-noise/internal/models"
)

func TestExtractMessageTagsStopsAtPunctuation(t *testing.T) {
	content := "今天 #生活，记录 #旅行。英文 #daily, #go! 连写 #one#two 括号 #主题）结束 音乐 #song?id=123"
	got := extractMessageTags(content)
	want := []string{"生活", "旅行", "daily", "go", "one", "two", "主题"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %#v, got %#v", want, got)
	}
}

func TestGetMessagesByTagTreatsPunctuationAsBoundary(t *testing.T) {
	db, r, _, _ := setupCommentAccountTest(t)
	r.GET("/messages/tags/:tag", GetMessagesByTag)

	messages := []models.Message{
		{Content: "今天 #生活，记录", Username: "alice", Private: false},
		{Content: "这是 #生活方式，不应匹配生活", Username: "alice", Private: false},
		{Content: "私密 #生活，隐藏", Username: "alice", Private: true},
	}
	for _, msg := range messages {
		if err := db.Create(&msg).Error; err != nil {
			t.Fatalf("create message: %v", err)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/messages/tags/"+url.PathEscape("生活"), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Code int              `json:"code"`
		Data []models.Message `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Code != 1 {
		t.Fatalf("expected success response, got %#v", resp)
	}
	if len(resp.Data) != 1 || resp.Data[0].Content != "今天 #生活，记录" {
		t.Fatalf("expected only the punctuation-delimited public tag match, got %#v", resp.Data)
	}
}
