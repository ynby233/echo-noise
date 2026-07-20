package controllers

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	for _, token := range []string{owner.Token, admin.Token} {
		got := request(http.MethodGet, "/api/files/private.txt", token, "")
		if got.Code != http.StatusOK || got.Body.String() != "private attachment" {
			t.Fatalf("authorized private attachment for %s = %d %q", token, got.Code, got.Body.String())
		}
		if got.Header().Get("Cache-Control") != "private, no-store" || !strings.Contains(got.Header().Get("Vary"), "Authorization") {
			t.Fatalf("private cache headers = Cache-Control %q Vary %q", got.Header().Get("Cache-Control"), got.Header().Get("Vary"))
		}
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
	if got := request(http.MethodGet, "/api/files/private.txt", admin.Token, ""); got.Code != http.StatusOK {
		t.Fatalf("admin lost deleted private attachment snapshot: %d", got.Code)
	}
}
