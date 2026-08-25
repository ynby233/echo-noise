package controllers

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	attachmentregistry "github.com/rcy1314/echo-noise/internal/attachments"
	"github.com/rcy1314/echo-noise/internal/authorization"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/services"
)

func TestServeAttachmentReferenceRequiresDownloadCapabilityForDelegatedAdmins(t *testing.T) {
	db, r, owner, message := setupCommentAccountTest(t)
	if err := db.AutoMigrate(&models.AttachmentBlob{}, &models.AttachmentReference{}); err != nil {
		t.Fatalf("migrate attachment registry: %v", err)
	}
	admin := models.User{Username: "delegated-download-admin", IsAdmin: true}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatalf("create delegated admin: %v", err)
	}
	content := []byte("registered attachment bytes")
	sum := sha256.Sum256(content)
	store := attachmentregistry.NewLocalStore(t.TempDir())
	reference, err := attachmentregistry.NewRegistry(db).Create(context.Background(), store, attachmentregistry.CreateInput{
		Kind: "file", OwnerUserID: owner.ID, OriginalName: "download.txt", ContentType: "text/plain",
		ContentHash: hex.EncodeToString(sum[:]), Size: int64(len(content)),
	}, bytes.NewReader(content))
	if err != nil {
		t.Fatalf("create attachment reference: %v", err)
	}
	message.Content = "[download](/api/files/refs/" + reference.PublicID + "/download.txt)"
	message.Visibility = "public"
	if err := db.Save(&message).Error; err != nil {
		t.Fatalf("save public message: %v", err)
	}
	handler := serveLocalAttachment("file", t.TempDir(), store.Root())
	r.GET("/api/files/*name", func(c *gin.Context) { c.Set("user_id", admin.ID); handler(c) })
	r.HEAD("/api/files/*name", func(c *gin.Context) { c.Set("user_id", admin.ID); handler(c) })
	path := "/api/files/refs/" + reference.PublicID + "/download.txt"
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		req := httptest.NewRequest(method, path, nil)
		if method == http.MethodGet {
			req.Header.Set("Range", "bytes=0-3")
		}
		resp := httptest.NewRecorder()
		r.ServeHTTP(resp, req)
		if resp.Code != http.StatusNotFound {
			t.Fatalf("%s without download capability = %d, want 404", method, resp.Code)
		}
	}
	if err := db.Create(&[]models.AdminCapabilityGrant{
		{UserID: admin.ID, Capability: string(authorization.CapabilityAttachmentsView), GrantedByUserID: models.PrimaryAdminUserID},
		{UserID: admin.ID, Capability: string(authorization.CapabilityAttachmentsDownload), GrantedByUserID: models.PrimaryAdminUserID},
	}).Error; err != nil {
		t.Fatalf("grant download capability: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Range", "bytes=0-3")
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)
	if resp.Code != http.StatusPartialContent || resp.Body.String() != string(content[:4]) {
		t.Fatalf("authorized range response = %d %q", resp.Code, resp.Body.String())
	}
}

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
	if disposition := response.Header().Get("Content-Disposition"); !strings.HasPrefix(disposition, "attachment;") {
		t.Fatalf("site-config attachment disposition = %q, want forced download", disposition)
	}
	if csp := response.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "default-src 'none'") || !strings.Contains(csp, "sandbox") {
		t.Fatalf("site-config attachment CSP = %q", csp)
	}
}

func TestServeAttachmentReferenceNeutralizesStoredActiveContent(t *testing.T) {
	db, r, owner, _ := setupCommentAccountTest(t)
	if err := db.AutoMigrate(&models.AttachmentBlob{}, &models.AttachmentReference{}); err != nil {
		t.Fatalf("migrate attachment registry: %v", err)
	}
	content := []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(document.domain)</script></svg>`)
	sum := sha256.Sum256(content)
	store := attachmentregistry.NewLocalStore(t.TempDir())
	reference, err := attachmentregistry.NewRegistry(db).Create(context.Background(), store, attachmentregistry.CreateInput{
		Kind:         "image",
		OwnerUserID:  owner.ID,
		OriginalName: "payload.svg",
		ContentType:  "image/svg+xml",
		ContentHash:  hex.EncodeToString(sum[:]),
		Size:         int64(len(content)),
	}, bytes.NewReader(content))
	if err != nil {
		t.Fatalf("create active attachment reference: %v", err)
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
		t.Fatalf("active attachment response = %d %q", response.Code, response.Body.String())
	}
	if contentType := response.Header().Get("Content-Type"); contentType != "application/octet-stream" {
		t.Fatalf("active attachment content type = %q", contentType)
	}
	if disposition := response.Header().Get("Content-Disposition"); !strings.HasPrefix(disposition, "attachment;") {
		t.Fatalf("active attachment disposition = %q", disposition)
	}
	if csp := response.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "default-src 'none'") || !strings.Contains(csp, "sandbox") {
		t.Fatalf("active attachment CSP = %q", csp)
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
