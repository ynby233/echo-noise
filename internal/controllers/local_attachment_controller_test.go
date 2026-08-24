package controllers

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rcy1314/echo-noise/internal/authorization"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/services"
)

func TestServeLocalAttachmentEnforcesMessageVisibilityAndPreservesHTTPMediaSemantics(t *testing.T) {
	db, r, owner, publicMessage := setupCommentAccountTest(t)
	owner.Token = "attachment-owner-token"
	if err := db.Model(&owner).Update("token", owner.Token).Error; err != nil {
		t.Fatalf("update owner token: %v", err)
	}
	admin := models.User{Username: "attachment-admin", Password: "", Token: "attachment-admin-token", IsAdmin: true}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatalf("create admin: %v", err)
	}
	if err := db.Create(&models.AdminCapabilityGrant{UserID: admin.ID, Capability: string(authorization.CapabilityContentViewHidden), GrantedByUserID: models.PrimaryAdminUserID}).Error; err != nil {
		t.Fatalf("grant hidden content read: %v", err)
	}

	dir := t.TempDir()
	for name, content := range map[string]string{
		"public.txt":  "public attachment",
		"private.txt": "private attachment",
		"preview.txt": "unreferenced preview",
		"prefix.txt":  "prefix boundary",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	publicMessage.Content = "[public](/api/files/public.txt)"
	publicMessage.Visibility = "public"
	if err := db.Save(&publicMessage).Error; err != nil {
		t.Fatalf("save public message: %v", err)
	}
	privateMessage := models.Message{
		Content:    "[private](/api/files/private.txt)",
		UserID:     owner.ID,
		Visibility: "private",
		Private:    true,
	}
	if err := services.CreateMessage(&privateMessage); err != nil {
		t.Fatalf("create private message: %v", err)
	}
	decoyMessage := models.Message{
		Content:    "[different file](/api/files/prefix.txt.bak)",
		UserID:     owner.ID,
		Visibility: "private",
		Private:    true,
	}
	if err := services.CreateMessage(&decoyMessage); err != nil {
		t.Fatalf("create boundary decoy message: %v", err)
	}

	handler := ServeLocalAttachment("file", dir)
	r.GET("/api/files/*name", handler)
	r.HEAD("/api/files/*name", handler)
	request := func(method, path, token, byteRange string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		if byteRange != "" {
			req.Header.Set("Range", byteRange)
		}
		response := httptest.NewRecorder()
		r.ServeHTTP(response, req)
		return response
	}

	if got := request(http.MethodGet, "/api/files/public.txt", "", ""); got.Code != http.StatusOK || got.Body.String() != "public attachment" {
		t.Fatalf("public attachment response = %d %q", got.Code, got.Body.String())
	}
	if got := request(http.MethodGet, "/api/files/preview.txt", "", ""); got.Code != http.StatusOK {
		t.Fatalf("unreferenced preview status = %d", got.Code)
	}
	if got := request(http.MethodGet, "/api/files/prefix.txt", "", ""); got.Code != http.StatusOK {
		t.Fatalf("filename prefix false-positive status = %d", got.Code)
	}
	if got := request(http.MethodGet, "/api/files/private.txt", "", ""); got.Code != http.StatusNotFound {
		t.Fatalf("anonymous private attachment status = %d, want 404", got.Code)
	}
	if got := request(http.MethodGet, "/api/files/private.txt", "invalid", ""); got.Code != http.StatusNotFound {
		t.Fatalf("invalid token private attachment status = %d, want 404", got.Code)
	}
	for _, token := range []string{owner.Token} {
		got := request(http.MethodGet, "/api/files/private.txt", token, "")
		if got.Code != http.StatusOK || got.Body.String() != "private attachment" {
			t.Fatalf("authorized private attachment for %s = %d %q", token, got.Code, got.Body.String())
		}
		if got.Header().Get("Cache-Control") != "private, no-store" || !strings.Contains(got.Header().Get("Vary"), "Authorization") {
			t.Fatalf("private cache headers = Cache-Control %q Vary %q", got.Header().Get("Cache-Control"), got.Header().Get("Vary"))
		}
	}
	if got := request(http.MethodGet, "/api/files/private.txt", admin.Token, ""); got.Code != http.StatusNotFound {
		t.Fatalf("delegated admin must not read primary-owned private attachment, got %d", got.Code)
	}
	if got := request(http.MethodHead, "/api/files/private.txt", owner.Token, ""); got.Code != http.StatusOK || got.Body.Len() != 0 {
		t.Fatalf("private HEAD response = %d body bytes %d", got.Code, got.Body.Len())
	}
	if got := request(http.MethodGet, "/api/files/public.txt", "", "bytes=0-3"); got.Code != http.StatusPartialContent || got.Body.String() != "publ" {
		t.Fatalf("public range response = %d %q", got.Code, got.Body.String())
	}
	if got := request(http.MethodGet, "/api/files/%2e%2e%2fprivate.txt", owner.Token, ""); got.Code != http.StatusNotFound {
		t.Fatalf("path traversal status = %d, want 404", got.Code)
	}

	removedContent := "private message after removing its attachment reference"
	if _, err := services.UpdateMessage(privateMessage.ID, &removedContent, nil, nil, nil); err != nil {
		t.Fatalf("remove private attachment reference: %v", err)
	}
	if got := request(http.MethodGet, "/api/files/private.txt", "", ""); got.Code != http.StatusNotFound {
		t.Fatalf("removed private attachment reference became public: %d", got.Code)
	}
	if got := request(http.MethodGet, "/api/files/private.txt", owner.Token, ""); got.Code != http.StatusOK {
		t.Fatalf("owner lost removed private attachment snapshot: %d", got.Code)
	}
	if err := services.DeleteMessage(privateMessage.ID, owner.ID); err != nil {
		t.Fatalf("delete private attachment message: %v", err)
	}
	if got := request(http.MethodGet, "/api/files/private.txt", "", ""); got.Code != http.StatusNotFound {
		t.Fatalf("deleted private message attachment became public: %d", got.Code)
	}
	if got := request(http.MethodGet, "/api/files/private.txt", admin.Token, ""); got.Code != http.StatusNotFound {
		t.Fatalf("delegated admin must not read primary-owned deleted attachment snapshot: %d", got.Code)
	}
}

func TestServeLocalContactAttachmentUsesEffectiveRuntimeVisibilityForHeadGetAndRange(t *testing.T) {
	db, r, _, _ := setupCommentAccountTest(t)
	config := models.SiteConfig{
		RuntimeMode: models.RuntimeModeVoceChat, RuntimeModeMigrationVersion: models.RuntimeModeMigrationVersionCurrent,
		VoceChatEnabled: true, VoceChatBaseURL: "https://vc.example.test", VoceChatAdminToken: "configured-token",
		VoceChatContactsEnabled: true, VoceChatLastHealthStatus: "ok",
	}
	if err := db.Create(&config).Error; err != nil {
		t.Fatalf("create runtime config: %v", err)
	}
	author := models.User{
		Username: "contact-attachment-author", Password: "hashed", Token: "contact-attachment-author-token",
		VoceChatEmail: "author@vc.example", VoceChatUserID: "901", VoceChatSyncStatus: models.VoceChatSyncStatusLinked,
	}
	viewer := models.User{
		Username: "contact-attachment-viewer", Password: "hashed", Token: "contact-attachment-viewer-token",
		VoceChatEmail: "viewer@vc.example", VoceChatUserID: "902", VoceChatSyncStatus: models.VoceChatSyncStatusLinked,
	}
	if err := db.Create(&author).Error; err != nil {
		t.Fatalf("create contact author: %v", err)
	}
	if err := db.Create(&viewer).Error; err != nil {
		t.Fatalf("create contact viewer: %v", err)
	}
	now := time.Now().UTC()
	cache := []models.VoceChatContactCache{
		{UserID: author.ID, ContactUserID: 0, VoceChatUserID: author.VoceChatUserID, Source: "vocechat", SyncedAt: now, ExpiresAt: now.Add(time.Hour), LastSyncStatus: models.VoceChatContactSyncStatusOK},
		{UserID: author.ID, ContactUserID: viewer.ID, VoceChatUserID: author.VoceChatUserID, ContactVoceChatID: viewer.VoceChatUserID, Source: "vocechat", SyncedAt: now, ExpiresAt: now.Add(time.Hour), LastSyncStatus: models.VoceChatContactSyncStatusOK},
	}
	if err := db.Create(&cache).Error; err != nil {
		t.Fatalf("create contact cache: %v", err)
	}
	message := models.Message{Content: "[contact](/api/files/contact-runtime.txt)", ImageURL: "https://example.test/contact-runtime.jpg", UserID: author.ID, Username: author.Username, Visibility: services.MessageVisibilityContacts}
	if err := services.ApplyMessageVisibilityForSave(&message); err != nil {
		t.Fatalf("normalize contact message: %v", err)
	}
	if err := db.Create(&message).Error; err != nil {
		t.Fatalf("create contact message: %v", err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "contact-runtime.txt"), []byte("contact attachment"), 0o600); err != nil {
		t.Fatalf("write contact attachment: %v", err)
	}
	handler := ServeLocalAttachment("file", dir)
	r.GET("/api/files/*name", handler)
	r.HEAD("/api/files/*name", handler)
	r.GET("/api/images", GetAllImages)
	request := func(method string, byteRange string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "/api/files/contact-runtime.txt", nil)
		req.Header.Set("Authorization", "Bearer "+viewer.Token)
		if byteRange != "" {
			req.Header.Set("Range", byteRange)
		}
		response := httptest.NewRecorder()
		r.ServeHTTP(response, req)
		return response
	}
	assertVisible := func() {
		t.Helper()
		if got := request(http.MethodGet, ""); got.Code != http.StatusOK || got.Body.String() != "contact attachment" {
			t.Fatalf("contact GET = %d %q", got.Code, got.Body.String())
		}
		if got := request(http.MethodHead, ""); got.Code != http.StatusOK || got.Body.Len() != 0 {
			t.Fatalf("contact HEAD = %d body=%d", got.Code, got.Body.Len())
		}
		if got := request(http.MethodGet, "bytes=0-6"); got.Code != http.StatusPartialContent || got.Body.String() != "contact" {
			t.Fatalf("contact Range = %d %q", got.Code, got.Body.String())
		}
		galleryRequest := httptest.NewRequest(http.MethodGet, "/api/images", nil)
		galleryRequest.Header.Set("Authorization", "Bearer "+viewer.Token)
		gallery := httptest.NewRecorder()
		r.ServeHTTP(gallery, galleryRequest)
		if gallery.Code != http.StatusOK || !strings.Contains(gallery.Body.String(), message.ImageURL) {
			t.Fatalf("contact gallery = %d %q", gallery.Code, gallery.Body.String())
		}
	}
	assertPrivate := func() {
		t.Helper()
		for _, item := range []struct{ method, byteRange string }{{http.MethodGet, ""}, {http.MethodHead, ""}, {http.MethodGet, "bytes=0-6"}} {
			if got := request(item.method, item.byteRange); got.Code != http.StatusNotFound {
				t.Fatalf("private contact %s range=%q status=%d", item.method, item.byteRange, got.Code)
			}
		}
		galleryRequest := httptest.NewRequest(http.MethodGet, "/api/images", nil)
		galleryRequest.Header.Set("Authorization", "Bearer "+viewer.Token)
		gallery := httptest.NewRecorder()
		r.ServeHTTP(gallery, galleryRequest)
		if gallery.Code != http.StatusOK || strings.Contains(gallery.Body.String(), message.ImageURL) {
			t.Fatalf("private contact leaked through gallery: %d %q", gallery.Code, gallery.Body.String())
		}
	}

	assertVisible()
	if err := db.Model(&models.SiteConfig{}).Where("id = ?", config.ID).Updates(map[string]interface{}{"runtime_mode": models.RuntimeModeLocal, "voce_chat_enabled": false}).Error; err != nil {
		t.Fatalf("switch local: %v", err)
	}
	assertPrivate()
	if err := db.Model(&models.SiteConfig{}).Where("id = ?", config.ID).Updates(map[string]interface{}{"runtime_mode": models.RuntimeModeVoceChat, "voce_chat_enabled": true, "voce_chat_last_health_status": "failed"}).Error; err != nil {
		t.Fatalf("switch degraded: %v", err)
	}
	assertPrivate()
	if err := db.Model(&models.SiteConfig{}).Where("id = ?", config.ID).Update("voce_chat_last_health_status", "ok").Error; err != nil {
		t.Fatalf("recover VoceChat: %v", err)
	}
	assertVisible()
	var stored models.Message
	if err := db.First(&stored, message.ID).Error; err != nil || stored.Visibility != services.MessageVisibilityContacts {
		t.Fatalf("runtime visibility mutated stored message: message=%#v err=%v", stored, err)
	}
}

func TestServeLocalAttachmentDoesNotExposeHiddenCommentOnlyReference(t *testing.T) {
	db, r, owner, message := setupCommentAccountTest(t)
	owner.Token = "comment-attachment-owner-token"
	if err := db.Model(&owner).Update("token", owner.Token).Error; err != nil {
		t.Fatalf("update owner token: %v", err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "comment-only.txt"), []byte("comment attachment"), 0o600); err != nil {
		t.Fatal(err)
	}
	comment := models.Comment{MessageID: message.ID, UserID: &owner.ID, Content: "hidden /api/files/comment-only.txt", Visibility: "private"}
	if err := db.Create(&comment).Error; err != nil {
		t.Fatalf("create comment: %v", err)
	}
	handler := ServeLocalAttachment("file", dir)
	r.GET("/api/files/*name", handler)
	request := func(token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/api/files/comment-only.txt", nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		response := httptest.NewRecorder()
		r.ServeHTTP(response, req)
		return response
	}
	if got := request(""); got.Code != http.StatusNotFound {
		t.Fatalf("anonymous hidden comment attachment status = %d, want 404", got.Code)
	}
	if got := request(owner.Token); got.Code != http.StatusOK || got.Body.String() != "comment attachment" {
		t.Fatalf("owner hidden comment attachment = %d %q", got.Code, got.Body.String())
	}
	publicComment := models.Comment{MessageID: message.ID, UserID: &owner.ID, Content: "public /api/files/public-comment-only.txt", Visibility: "public"}
	if err := db.Create(&publicComment).Error; err != nil {
		t.Fatalf("create public comment: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "public-comment-only.txt"), []byte("public comment attachment"), 0o600); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/files/public-comment-only.txt", nil)
	response := httptest.NewRecorder()
	r.ServeHTTP(response, req)
	if response.Code != http.StatusOK || response.Body.String() != "public comment attachment" {
		t.Fatalf("anonymous public comment attachment = %d %q", response.Code, response.Body.String())
	}
}
