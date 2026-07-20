package controllers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/services"
)

func TestServeCloudAttachmentUsesOpaqueMappingAndMessageVisibility(t *testing.T) {
	db, r, owner, _ := setupCommentAccountTest(t)
	if err := db.AutoMigrate(&models.CloudAttachmentObject{}); err != nil {
		t.Fatalf("migrate cloud mapping: %v", err)
	}
	owner.Token = "cloud-owner-token"
	if err := db.Model(&owner).Update("token", owner.Token).Error; err != nil {
		t.Fatalf("update owner token: %v", err)
	}

	const body = "cloud attachment"
	storageCalls := 0
	storage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		storageCalls++
		if !strings.Contains(req.URL.Path, "/bucket/secret-object-key") {
			t.Fatalf("unexpected storage path %q", req.URL.Path)
		}
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Accept-Ranges", "bytes")
		if req.Method == http.MethodHead {
			w.Header().Set("Content-Length", "16")
			w.WriteHeader(http.StatusOK)
			return
		}
		if req.Header.Get("Range") == "bytes=0-4" {
			w.Header().Set("Content-Range", "bytes 0-4/16")
			w.Header().Set("Content-Length", "5")
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write([]byte("cloud"))
			return
		}
		w.Header().Set("Content-Length", "16")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	defer storage.Close()
	if err := db.Create(&models.SiteConfig{
		AttachmentStorageEnabled:      true,
		AttachmentStorageProvider:     "r2",
		AttachmentStorageEndpoint:     storage.URL,
		AttachmentStorageRegion:       "auto",
		AttachmentStorageBucket:       "bucket",
		AttachmentStorageAccessKey:    "access",
		AttachmentStorageSecretKey:    "secret",
		AttachmentStorageUsePathStyle: true,
	}).Error; err != nil {
		t.Fatalf("create attachment storage config: %v", err)
	}
	object := models.CloudAttachmentObject{PublicID: "opaque-public-id", ObjectKey: "secret-object-key", OriginalName: "secret.txt", ContentType: "text/plain"}
	if err := db.Create(&object).Error; err != nil {
		t.Fatalf("create cloud mapping: %v", err)
	}
	message := models.Message{
		Content:    "[secret](/api/cloud-attachments/opaque-public-id/secret.txt)",
		UserID:     owner.ID,
		Visibility: "private",
		Private:    true,
	}
	if err := services.CreateMessage(&message); err != nil {
		t.Fatalf("create private message: %v", err)
	}
	r.GET("/api/cloud-attachments/:id/*name", ServeCloudAttachment)
	r.HEAD("/api/cloud-attachments/:id/*name", ServeCloudAttachment)

	request := func(method, token, byteRange string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "/api/cloud-attachments/opaque-public-id/secret.txt", nil)
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

	if got := request(http.MethodGet, "", ""); got.Code != http.StatusNotFound || storageCalls != 0 {
		t.Fatalf("anonymous private cloud response = %d, storage calls %d", got.Code, storageCalls)
	}
	if got := request(http.MethodGet, owner.Token, ""); got.Code != http.StatusOK || got.Body.String() != body {
		t.Fatalf("owner cloud response = %d %q", got.Code, got.Body.String())
	}
	if got := request(http.MethodHead, owner.Token, ""); got.Code != http.StatusOK || got.Body.Len() != 0 || got.Header().Get("Content-Length") != "16" {
		t.Fatalf("owner cloud HEAD = %d body %d length %q", got.Code, got.Body.Len(), got.Header().Get("Content-Length"))
	}
	if got := request(http.MethodGet, owner.Token, "bytes=0-4"); got.Code != http.StatusPartialContent || got.Body.String() != "cloud" || got.Header().Get("Content-Range") != "bytes 0-4/16" {
		t.Fatalf("owner cloud range = %d %q range %q", got.Code, got.Body.String(), got.Header().Get("Content-Range"))
	}

	publicVisibility := "public"
	if _, err := services.UpdateMessage(message.ID, nil, nil, &publicVisibility, nil); err != nil {
		t.Fatalf("make message public: %v", err)
	}
	if got := request(http.MethodGet, "", ""); got.Code != http.StatusOK || got.Body.String() != body {
		t.Fatalf("public cloud response = %d %q", got.Code, got.Body.String())
	}

	privateVisibility := "private"
	if _, err := services.UpdateMessage(message.ID, nil, nil, &privateVisibility, nil); err != nil {
		t.Fatalf("make message private again: %v", err)
	}
	removedContent := "private message after cloud attachment removal"
	if _, err := services.UpdateMessage(message.ID, &removedContent, nil, nil, nil); err != nil {
		t.Fatalf("remove cloud attachment reference: %v", err)
	}
	callsBeforeDeniedRequest := storageCalls
	if got := request(http.MethodGet, "", ""); got.Code != http.StatusNotFound || storageCalls != callsBeforeDeniedRequest {
		t.Fatalf("removed private cloud attachment became public: status %d storage calls %d -> %d", got.Code, callsBeforeDeniedRequest, storageCalls)
	}
	if got := request(http.MethodGet, owner.Token, ""); got.Code != http.StatusOK {
		t.Fatalf("owner lost removed private cloud attachment snapshot: %d", got.Code)
	}
	if err := services.DeleteMessage(message.ID, owner.ID); err != nil {
		t.Fatalf("delete private cloud attachment message: %v", err)
	}
	if got := request(http.MethodGet, "", ""); got.Code != http.StatusNotFound {
		t.Fatalf("deleted private message cloud attachment became public: %d", got.Code)
	}
	if err := db.Model(&models.SiteConfig{}).Where("1 = 1").Update("attachment_storage_enabled", false).Error; err != nil {
		t.Fatalf("disable new cloud uploads: %v", err)
	}
	if got := request(http.MethodGet, owner.Token, ""); got.Code != http.StatusOK {
		t.Fatalf("disabling new cloud uploads broke an existing controlled attachment: %d", got.Code)
	}
}
