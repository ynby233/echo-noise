package controllers

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	attachmentregistry "github.com/rcy1314/echo-noise/internal/attachments"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/services"
)

func TestServeAttachmentReferenceAllowsAnonymousPublicSiteConfigUsage(t *testing.T) {
	db, r, owner, _ := setupCommentAccountTest(t)
	if err := db.AutoMigrate(&models.AttachmentBlob{}, &models.AttachmentReference{}); err != nil {
		t.Fatalf("migrate attachment registry: %v", err)
	}
	content := []byte("public advertisement image")
	sum := sha256.Sum256(content)
	store := attachmentregistry.NewLocalStore(t.TempDir())
	reference, err := attachmentregistry.NewRegistry(db).Create(context.Background(), store, attachmentregistry.CreateInput{
		Kind:         "image",
		OwnerUserID:  owner.ID,
		OriginalName: "advertisement.webp",
		ContentType:  "image/webp",
		ContentHash:  hex.EncodeToString(sum[:]),
		Size:         int64(len(content)),
	}, bytes.NewReader(content))
	if err != nil {
		t.Fatalf("create image reference: %v", err)
	}
	path := attachmentregistry.ReferenceURL(reference, "local")
	ads, _ := json.Marshal([]map[string]string{{"imageURL": path}})
	if err := db.Create(&models.SiteConfig{LeftAds: string(ads)}).Error; err != nil {
		t.Fatalf("create site config: %v", err)
	}

	r.GET("/api/images/*name", serveLocalAttachment("image", t.TempDir(), store.Root()))
	request := httptest.NewRequest(http.MethodGet, path, nil)
	response := httptest.NewRecorder()
	r.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Body.String() != string(content) {
		t.Fatalf("anonymous site-config attachment response = %d %q", response.Code, response.Body.String())
	}
}

func TestServeAttachmentReferenceUsesOwningMessagesInsteadOfSharedBlobIdentity(t *testing.T) {
	db, r, owner, publicMessage := setupCommentAccountTest(t)
	if err := db.AutoMigrate(&models.AttachmentBlob{}, &models.AttachmentReference{}); err != nil {
		t.Fatalf("migrate attachment registry: %v", err)
	}
	owner.Token = "reference-owner-token"
	if err := db.Model(&owner).Update("token", owner.Token).Error; err != nil {
		t.Fatalf("update owner token: %v", err)
	}
	attacker := models.User{Username: "reference-attacker", Password: "", Token: "reference-attacker-token"}
	if err := db.Create(&attacker).Error; err != nil {
		t.Fatalf("create attacker: %v", err)
	}

	content := []byte("private bytes shared by hash")
	sum := sha256.Sum256(content)
	store := attachmentregistry.NewLocalStore(t.TempDir())
	registry := attachmentregistry.NewRegistry(db)
	reference, err := registry.Create(context.Background(), store, attachmentregistry.CreateInput{
		Kind:         "file",
		OwnerUserID:  owner.ID,
		OriginalName: "auth.json",
		ContentType:  "application/json",
		ContentHash:  hex.EncodeToString(sum[:]),
		Size:         int64(len(content)),
	}, bytes.NewReader(content))
	if err != nil {
		t.Fatalf("create attachment reference: %v", err)
	}
	path := "/api/files/refs/" + reference.PublicID + "/auth.json"

	handler := serveLocalAttachment("file", t.TempDir(), store.Root())
	r.GET("/api/files/*name", handler)
	r.HEAD("/api/files/*name", handler)
	request := func(token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		response := httptest.NewRecorder()
		r.ServeHTTP(response, req)
		return response
	}

	if got := request(""); got.Code != http.StatusNotFound {
		t.Fatalf("pending attachment anonymous status = %d, want 404", got.Code)
	}
	if got := request(owner.Token); got.Code != http.StatusOK || got.Body.String() != string(content) {
		t.Fatalf("pending attachment owner response = %d %q", got.Code, got.Body.String())
	}

	attackerMessage := models.Message{
		Content:    "[copied](" + path + ")",
		UserID:     attacker.ID,
		Visibility: "public",
	}
	if err := services.CreateMessage(&attackerMessage); err != nil {
		t.Fatalf("create attacker message: %v", err)
	}
	if got := request(""); got.Code != http.StatusNotFound {
		t.Fatalf("different user's public note exposed attachment: %d", got.Code)
	}

	publicMessage.Content = "[owned](" + path + ")"
	publicMessage.Visibility = "public"
	if err := db.Save(&publicMessage).Error; err != nil {
		t.Fatalf("publish owner message: %v", err)
	}
	if got := request(""); got.Code != http.StatusOK || got.Body.String() != string(content) {
		t.Fatalf("owner public attachment response = %d %q", got.Code, got.Body.String())
	}
}
