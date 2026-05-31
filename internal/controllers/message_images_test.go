package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rcy1314/echo-noise/internal/models"
)

func TestGetAllImagesScopesPrivateMessagesByViewer(t *testing.T) {
	db, r, owner, _ := setupCommentAccountTest(t)
	r.GET("/messages/images", GetAllImages)

	viewer := models.User{Username: "bob", Token: "bob-token"}
	admin := models.User{Username: "admin", Token: "admin-token", IsAdmin: true}
	owner.Token = "owner-token"
	if err := db.Model(&owner).Update("token", owner.Token).Error; err != nil {
		t.Fatalf("update owner token: %v", err)
	}
	if err := db.Create(&viewer).Error; err != nil {
		t.Fatalf("create viewer: %v", err)
	}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatalf("create admin: %v", err)
	}

	messages := []models.Message{
		{Content: "public markdown ![public](/public-md.png)", ImageURL: "/public-field.png", UserID: owner.ID, Username: owner.Username, Private: false},
		{Content: "owner private ![owner](/owner-private-md.png)", ImageURL: "/owner-private-field.png", UserID: owner.ID, Username: owner.Username, Private: true},
		{Content: "viewer private ![viewer](/viewer-private-md.png)", ImageURL: "/viewer-private-field.png", UserID: viewer.ID, Username: viewer.Username, Private: true},
	}
	for _, msg := range messages {
		if err := db.Create(&msg).Error; err != nil {
			t.Fatalf("create message %q: %v", msg.Content, err)
		}
	}

	assertImages(t, performImagesRequest(r, ""), []string{"/public-field.png", "/public-md.png"})
	assertImages(t, performImagesRequest(r, "owner-token"), []string{"/public-field.png", "/public-md.png", "/owner-private-field.png", "/owner-private-md.png"})
	assertImages(t, performImagesRequest(r, "bob-token"), []string{"/public-field.png", "/public-md.png", "/viewer-private-field.png", "/viewer-private-md.png"})
	assertImages(t, performImagesRequest(r, "admin-token"), []string{"/public-field.png", "/public-md.png", "/owner-private-field.png", "/owner-private-md.png", "/viewer-private-field.png", "/viewer-private-md.png"})
}

func performImagesRequest(r http.Handler, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/messages/images", nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func assertImages(t *testing.T, w *httptest.ResponseRecorder, want []string) {
	t.Helper()
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Code int `json:"code"`
		Data []struct {
			ImageURL string `json:"image_url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Code != 1 {
		t.Fatalf("expected success response, got %#v", resp)
	}
	if len(resp.Data) != len(want) {
		t.Fatalf("expected %d images, got %d: %#v", len(want), len(resp.Data), resp.Data)
	}

	got := map[string]int{}
	for _, image := range resp.Data {
		got[image.ImageURL]++
	}
	for _, imageURL := range want {
		if got[imageURL] != 1 {
			t.Fatalf("expected image %q exactly once, got counts %#v", imageURL, got)
		}
	}
}
