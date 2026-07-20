package controllers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/internal/models"
)

func TestBearerTokenIdentifiesOrdinaryViewerAcrossMessageReads(t *testing.T) {
	db, r, owner, _ := setupCommentAccountTest(t)
	owner.Token = "ordinary-read-token"
	if err := db.Model(&owner).Update("token", owner.Token).Error; err != nil {
		t.Fatalf("update owner token: %v", err)
	}
	messages := []models.Message{
		{Content: "owner users", UserID: owner.ID, Visibility: "users", Private: true},
		{Content: "owner contacts", UserID: owner.ID, Visibility: "contacts", Private: true},
		{Content: "owner private", UserID: owner.ID, Visibility: "private", Private: true},
	}
	for i := range messages {
		if err := db.Create(&messages[i]).Error; err != nil {
			t.Fatalf("create %s message: %v", messages[i].Visibility, err)
		}
	}
	r.GET("/messages/:id", GetMessage)
	r.POST("/messages/page", GetMessagesByPage)

	for _, message := range messages {
		req := httptest.NewRequest(http.MethodGet, "/messages/"+strconvFormatUint(message.ID), nil)
		req.Header.Set("Authorization", "Bearer "+owner.Token)
		response := httptest.NewRecorder()
		r.ServeHTTP(response, req)
		var payload struct {
			Code int             `json:"code"`
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode %s detail response: %v", message.Visibility, err)
		}
		var returned models.Message
		if payload.Code == 1 {
			_ = json.Unmarshal(payload.Data, &returned)
		}
		if payload.Code != 1 || returned.ID != message.ID {
			t.Fatalf("ordinary token could not read own %s message: %s", message.Visibility, response.Body.String())
		}
	}

	body := bytes.NewBufferString(`{"page":1,"pageSize":100}`)
	req := httptest.NewRequest(http.MethodPost, "/messages/page", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+owner.Token)
	response := httptest.NewRecorder()
	r.ServeHTTP(response, req)
	var pagePayload struct {
		Code int `json:"code"`
		Data struct {
			Items []models.Message `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &pagePayload); err != nil {
		t.Fatalf("decode page response: %v", err)
	}
	seen := map[string]bool{}
	for _, message := range pagePayload.Data.Items {
		seen[message.Visibility] = true
	}
	for _, visibility := range []string{"users", "contacts", "private"} {
		if !seen[visibility] {
			t.Fatalf("ordinary token page omitted own %s message: %s", visibility, response.Body.String())
		}
	}
}

func TestCurrentReadUserRejectsExpiredSessionAndFallsBackToBearer(t *testing.T) {
	db, r, owner, _ := setupCommentAccountTest(t)
	owner.Token = "expired-session-bearer"
	if err := db.Model(&owner).Update("token", owner.Token).Error; err != nil {
		t.Fatalf("update owner token: %v", err)
	}

	r.GET("/seed-expired", func(c *gin.Context) {
		session := sessions.Default(c)
		session.Set("user_id", owner.ID)
		session.Set("username", owner.Username)
		session.Set("is_admin", false)
		session.Set("login_expire_at", time.Now().Add(-time.Hour).Unix())
		if err := session.Save(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.Status(http.StatusNoContent)
	})
	r.GET("/read-identity", func(c *gin.Context) {
		user, ok := currentReadUser(c)
		c.JSON(http.StatusOK, gin.H{"authenticated": ok, "user_id": user.ID})
	})

	seed := httptest.NewRecorder()
	r.ServeHTTP(seed, httptest.NewRequest(http.MethodGet, "/seed-expired", nil))
	if seed.Code != http.StatusNoContent {
		t.Fatalf("seed expired session: %d %s", seed.Code, seed.Body.String())
	}
	cookies := seed.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("seed expired session did not return a cookie")
	}

	request := func(token string) map[string]any {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/read-identity", nil)
		for _, cookie := range cookies {
			req.AddCookie(cookie)
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		response := httptest.NewRecorder()
		r.ServeHTTP(response, req)
		var payload map[string]any
		if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode read identity response: %v", err)
		}
		return payload
	}

	if payload := request(""); payload["authenticated"] != false || payload["user_id"] != float64(0) {
		t.Fatalf("expired session remained authenticated: %#v", payload)
	}
	if payload := request(owner.Token); payload["authenticated"] != true || payload["user_id"] != float64(owner.ID) {
		t.Fatalf("bearer fallback did not identify viewer after session expiry: %#v", payload)
	}
}

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
