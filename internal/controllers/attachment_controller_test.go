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
