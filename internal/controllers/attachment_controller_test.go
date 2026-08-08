package controllers

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	attachmentregistry "github.com/rcy1314/echo-noise/internal/attachments"
	"github.com/rcy1314/echo-noise/internal/models"
)

func TestIsAudioExt(t *testing.T) {
	accepted := []string{"recording.webm", "voice.ogg", "clip.mp3", "memo.m4a", "capture.wav"}
	for _, name := range accepted {
		if !isAudioExt(name) {
			t.Fatalf("expected %q to be treated as audio", name)
		}
	}

	rejected := []string{"photo.png", "movie.mov", "movie.mp4", "archive.zip", "audio"}
	for _, name := range rejected {
		if isAudioExt(name) {
			t.Fatalf("expected %q to be rejected as audio", name)
		}
	}
}

func TestListOtherAttachmentsShowsSameOriginalNameAsSeparateLogicalReferences(t *testing.T) {
	db, r, owner, firstMessage := setupCommentAccountTest(t)
	if err := db.AutoMigrate(&models.AttachmentBlob{}, &models.AttachmentReference{}); err != nil {
		t.Fatalf("migrate attachment registry: %v", err)
	}
	content := []byte(`{"same":"auth"}`)
	sum := sha256.Sum256(content)
	input := attachmentregistry.CreateInput{
		Kind:         "file",
		OwnerUserID:  owner.ID,
		OriginalName: "auth.json",
		ContentType:  "application/json",
		ContentHash:  hex.EncodeToString(sum[:]),
		Size:         int64(len(content)),
	}
	store := attachmentregistry.NewLocalStore(t.TempDir())
	registry := attachmentregistry.NewRegistry(db)
	first, err := registry.Create(context.Background(), store, input, bytes.NewReader(content))
	if err != nil {
		t.Fatalf("create first reference: %v", err)
	}
	second, err := registry.Create(context.Background(), store, input, bytes.NewReader(content))
	if err != nil {
		t.Fatalf("create second reference: %v", err)
	}
	firstMessage.Content = "[first](" + attachmentregistry.ReferenceURL(first, "local") + ")"
	if err := db.Save(&firstMessage).Error; err != nil {
		t.Fatalf("save first message: %v", err)
	}
	secondMessage := models.Message{Content: "[second](" + attachmentregistry.ReferenceURL(second, "local") + ")", UserID: owner.ID, Visibility: "private", Private: true}
	if err := db.Create(&secondMessage).Error; err != nil {
		t.Fatalf("save second message: %v", err)
	}

	r.GET("/api/attachments/other", ListOtherAttachments)
	req := httptest.NewRequest(http.MethodGet, "/api/attachments/other", nil)
	response := httptest.NewRecorder()
	r.ServeHTTP(response, req)
	var payload struct {
		Code int              `json:"code"`
		Data []AttachmentInfo `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if payload.Code != 1 || len(payload.Data) != 2 {
		t.Fatalf("list response code=%d items=%#v", payload.Code, payload.Data)
	}
	seen := map[string]uint{}
	for _, item := range payload.Data {
		if item.Name != "auth.json" || item.LogicalID == "" || len(item.Belongs) != 1 {
			t.Fatalf("invalid logical attachment item: %#v", item)
		}
		seen[item.LogicalID] = item.Belongs[0].ID
	}
	if seen[first.PublicID] != firstMessage.ID || seen[second.PublicID] != secondMessage.ID {
		t.Fatalf("logical reference associations = %#v", seen)
	}
}

func TestListImageAttachmentsShowsSiteAndAvatarAssociations(t *testing.T) {
	db, r, owner, _ := setupCommentAccountTest(t)
	if err := db.AutoMigrate(&models.AttachmentBlob{}, &models.AttachmentReference{}); err != nil {
		t.Fatalf("migrate attachment registry: %v", err)
	}
	content := []byte("shared site image")
	sum := sha256.Sum256(content)
	store := attachmentregistry.NewLocalStore(t.TempDir())
	reference, err := attachmentregistry.NewRegistry(db).Create(context.Background(), store, attachmentregistry.CreateInput{
		Kind:         "image",
		OwnerUserID:  owner.ID,
		OriginalName: "site-image.png",
		ContentType:  "image/png",
		ContentHash:  hex.EncodeToString(sum[:]),
		Size:         int64(len(content)),
	}, bytes.NewReader(content))
	if err != nil {
		t.Fatalf("create image reference: %v", err)
	}
	imageURL := attachmentregistry.ReferenceURL(reference, "local")
	owner.AvatarURL = imageURL
	if err := db.Save(&owner).Error; err != nil {
		t.Fatalf("save user avatar: %v", err)
	}
	backgrounds, _ := json.Marshal([]map[string]string{{"url": imageURL}})
	ads, _ := json.Marshal([]map[string]string{{"imageURL": imageURL}})
	if err := db.Create(&models.SiteConfig{
		AvatarURL:        imageURL,
		WelcomeAvatarURL: imageURL,
		Backgrounds:      string(backgrounds),
		LeftAds:          string(ads),
	}).Error; err != nil {
		t.Fatalf("create site config: %v", err)
	}

	r.GET("/api/attachments/images", ListImageAttachments)
	req := httptest.NewRequest(http.MethodGet, "/api/attachments/images", nil)
	response := httptest.NewRecorder()
	r.ServeHTTP(response, req)
	var payload struct {
		Code int `json:"code"`
		Data []struct {
			LogicalID string `json:"logical_id"`
			Belongs   []struct {
				Kind  string `json:"kind"`
				Label string `json:"label"`
			} `json:"belongs"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode image list response: %v", err)
	}
	var kinds map[string]string
	for _, item := range payload.Data {
		if item.LogicalID != reference.PublicID {
			continue
		}
		kinds = make(map[string]string, len(item.Belongs))
		for _, belong := range item.Belongs {
			kinds[belong.Kind] = belong.Label
		}
	}
	for _, kind := range []string{"user_avatar", "site_avatar", "welcome_avatar", "header_background", "advertisement"} {
		if kinds[kind] == "" {
			t.Fatalf("association %q missing from %#v", kind, kinds)
		}
	}
}

func TestDeleteAttachmentReferenceDoesNotBreakAnotherLogicalReference(t *testing.T) {
	db, r, owner, _ := setupCommentAccountTest(t)
	if err := db.AutoMigrate(&models.AttachmentBlob{}, &models.AttachmentReference{}); err != nil {
		t.Fatalf("migrate attachment registry: %v", err)
	}
	t.Setenv("ATTACHMENT_BLOB_ROOT", t.TempDir())
	content := []byte("shared delete bytes")
	sum := sha256.Sum256(content)
	input := attachmentregistry.CreateInput{Kind: "file", OwnerUserID: owner.ID, OriginalName: "auth.json", ContentType: "application/json", ContentHash: hex.EncodeToString(sum[:]), Size: int64(len(content))}
	store := attachmentregistry.NewLocalStore(attachmentregistry.DefaultLocalRoot())
	registry := attachmentregistry.NewRegistry(db)
	first, err := registry.Create(context.Background(), store, input, bytes.NewReader(content))
	if err != nil {
		t.Fatalf("create first reference: %v", err)
	}
	second, err := registry.Create(context.Background(), store, input, bytes.NewReader(content))
	if err != nil {
		t.Fatalf("create second reference: %v", err)
	}
	r.DELETE("/api/attachments/references/:id", DeleteAttachmentReference)
	remove := func(id string) {
		req := httptest.NewRequest(http.MethodDelete, "/api/attachments/references/"+id, nil)
		response := httptest.NewRecorder()
		r.ServeHTTP(response, req)
		var payload struct {
			Code int `json:"code"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil || payload.Code != 1 {
			t.Fatalf("delete %s response = %s err=%v", id, response.Body.String(), err)
		}
	}

	remove(first.PublicID)
	resolved, err := registry.Resolve(second.PublicID)
	if err != nil {
		t.Fatalf("second reference was removed: %v", err)
	}
	if exists, err := store.Exists(context.Background(), resolved.Blob.StorageKey); err != nil || !exists {
		t.Fatalf("shared blob exists=%v err=%v", exists, err)
	}
	remove(second.PublicID)
	var count int64
	if err := db.Model(&models.AttachmentBlob{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("blob count after final delete = %d err=%v", count, err)
	}
}

func TestPurgeAttachmentBlobsBatchIsViewerScopedForSharedBlob(t *testing.T) {
	db, r, owner, publicMessage := setupCommentAccountTest(t)
	if err := db.AutoMigrate(&models.AttachmentBlob{}, &models.AttachmentReference{}); err != nil {
		t.Fatalf("migrate attachment registry: %v", err)
	}
	owner.IsAdmin = true
	if err := db.Save(&owner).Error; err != nil {
		t.Fatalf("promote primary user: %v", err)
	}
	delegated := models.User{Username: "delegated", IsAdmin: true, Token: "delegated-purge-token"}
	if err := db.Create(&delegated).Error; err != nil {
		t.Fatalf("create delegated user: %v", err)
	}
	t.Setenv("ATTACHMENT_BLOB_ROOT", t.TempDir())
	content := []byte("shared scoped purge bytes")
	sum := sha256.Sum256(content)
	input := attachmentregistry.CreateInput{Kind: "file", OwnerUserID: owner.ID, OriginalName: "visible.txt", ContentType: "text/plain", ContentHash: hex.EncodeToString(sum[:]), Size: int64(len(content))}
	store := attachmentregistry.NewLocalStore(attachmentregistry.DefaultLocalRoot())
	registry := attachmentregistry.NewRegistry(db)
	visible, err := registry.Create(context.Background(), store, input, bytes.NewReader(content))
	if err != nil {
		t.Fatalf("create visible reference: %v", err)
	}
	input.OwnerUserID = models.PrimaryAdminUserID
	input.OriginalName = "hidden.txt"
	hidden, err := registry.Create(context.Background(), store, input, bytes.NewReader(content))
	if err != nil {
		t.Fatalf("create hidden reference: %v", err)
	}
	publicMessage.Content = attachmentregistry.ReferenceURL(visible, "local")
	publicMessage.Visibility = "public"
	if err := db.Save(&publicMessage).Error; err != nil {
		t.Fatalf("save visible message: %v", err)
	}
	hiddenMessage := models.Message{Content: attachmentregistry.ReferenceURL(hidden, "local"), UserID: models.PrimaryAdminUserID, Visibility: "private", Private: true}
	if err := db.Create(&hiddenMessage).Error; err != nil {
		t.Fatalf("create hidden message: %v", err)
	}
	r.POST("/api/attachments/references/batch-purge", PurgeAttachmentBlobsBatch)
	payload := `{"logical_ids":["` + visible.PublicID + `"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/attachments/references/batch-purge", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+delegated.Token)
	response := httptest.NewRecorder()
	r.ServeHTTP(response, req)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"code":1`) {
		t.Fatalf("scoped purge response = %d %s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), hidden.PublicID) {
		t.Fatalf("scoped purge leaked hidden reference: %s", response.Body.String())
	}
	if _, err := registry.Resolve(visible.PublicID); err == nil {
		t.Fatal("visible reference survived scoped purge")
	}
	resolvedHidden, err := registry.Resolve(hidden.PublicID)
	if err != nil {
		t.Fatalf("hidden reference was removed: %v", err)
	}
	if exists, err := store.Exists(context.Background(), resolvedHidden.Blob.StorageKey); err != nil || !exists {
		t.Fatalf("shared blob exists=%v err=%v", exists, err)
	}
}

func TestDownloadAttachmentZipOmitsHiddenSharedReference(t *testing.T) {
	db, r, owner, publicMessage := setupCommentAccountTest(t)
	if err := db.AutoMigrate(&models.AttachmentBlob{}, &models.AttachmentReference{}); err != nil {
		t.Fatalf("migrate attachment registry: %v", err)
	}
	owner.IsAdmin = true
	if err := db.Save(&owner).Error; err != nil {
		t.Fatalf("promote primary user: %v", err)
	}
	delegated := models.User{Username: "delegated-zip", IsAdmin: true, Token: "delegated-zip-token"}
	if err := db.Create(&delegated).Error; err != nil {
		t.Fatalf("create delegated user: %v", err)
	}
	t.Setenv("ATTACHMENT_BLOB_ROOT", t.TempDir())
	content := []byte("shared zip bytes")
	sum := sha256.Sum256(content)
	input := attachmentregistry.CreateInput{Kind: "file", OwnerUserID: models.PrimaryAdminUserID, OriginalName: "visible.txt", ContentType: "text/plain", ContentHash: hex.EncodeToString(sum[:]), Size: int64(len(content))}
	store := attachmentregistry.NewLocalStore(attachmentregistry.DefaultLocalRoot())
	registry := attachmentregistry.NewRegistry(db)
	visible, err := registry.Create(context.Background(), store, input, bytes.NewReader(content))
	if err != nil {
		t.Fatalf("create visible reference: %v", err)
	}
	input.OriginalName = "hidden.txt"
	hidden, err := registry.Create(context.Background(), store, input, bytes.NewReader(content))
	if err != nil {
		t.Fatalf("create hidden reference: %v", err)
	}
	publicMessage.Content = attachmentregistry.ReferenceURL(visible, "local")
	publicMessage.Visibility = "public"
	if err := db.Save(&publicMessage).Error; err != nil {
		t.Fatalf("save visible message: %v", err)
	}
	hiddenMessage := models.Message{Content: attachmentregistry.ReferenceURL(hidden, "local"), UserID: models.PrimaryAdminUserID, Visibility: "private", Private: true}
	if err := db.Create(&hiddenMessage).Error; err != nil {
		t.Fatalf("create hidden message: %v", err)
	}
	r.POST("/api/attachments/download-zip", DownloadAttachmentZip)
	payload := `{"items":[{"type":"other","logical_id":"` + visible.PublicID + `"},{"type":"other","logical_id":"` + hidden.PublicID + `"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/attachments/download-zip", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+delegated.Token)
	response := httptest.NewRecorder()
	r.ServeHTTP(response, req)
	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "application/zip" {
		t.Fatalf("zip response = %d %s", response.Code, response.Body.String())
	}
	archive, err := zip.NewReader(bytes.NewReader(response.Body.Bytes()), int64(response.Body.Len()))
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	if len(archive.File) != 1 || archive.File[0].Name != "other/visible.txt" {
		t.Fatalf("zip entries = %#v", archive.File)
	}
	if strings.Contains(string(response.Body.Bytes()), "hidden.txt") {
		t.Fatal("zip leaked hidden reference name")
	}

	hiddenOnly := `{"items":[{"type":"other","logical_id":"` + hidden.PublicID + `"}]}`
	hiddenReq := httptest.NewRequest(http.MethodPost, "/api/attachments/download-zip", strings.NewReader(hiddenOnly))
	hiddenReq.Header.Set("Content-Type", "application/json")
	hiddenReq.Header.Set("Authorization", "Bearer "+delegated.Token)
	hiddenResponse := httptest.NewRecorder()
	r.ServeHTTP(hiddenResponse, hiddenReq)
	if hiddenResponse.Code != http.StatusNotFound || strings.Contains(hiddenResponse.Body.String(), hidden.PublicID) {
		t.Fatalf("hidden-only zip response = %d %s", hiddenResponse.Code, hiddenResponse.Body.String())
	}
}
