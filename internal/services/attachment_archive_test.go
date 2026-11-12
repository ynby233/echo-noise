package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/rcy1314/echo-noise/internal/backup"
	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
)

func attachmentArchiveTestDB(t *testing.T, p string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(p), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.Message{}, &models.Comment{}, &models.SiteConfig{}, &models.AttachmentBlob{}, &models.AttachmentReference{}, &models.CloudAttachmentObject{}, &models.LocalAttachmentGrant{}); err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

func attachmentArchiveFixture(t *testing.T) (string, attachmentArchiveDocument, []byte) {
	t.Helper()
	stage := t.TempDir()
	content := []byte("complete archived content, not remote placeholder")
	hash := sha256.Sum256(content)
	digest := hex.EncodeToString(hash[:])
	entry := AttachmentArchiveEntry{BlobID: 17, Backend: "cloud", Key: "old-bucket/original-key", Hash: digest, Size: int64(len(content))}
	entry.File = attachmentEntryFile(entry)
	doc := attachmentArchiveDocument{Version: 1, Scope: "all", Entries: []AttachmentArchiveEntry{entry}, Blobs: []archiveBlob{{ID: 17, Backend: "cloud", Key: entry.Key, Hash: digest, Size: entry.Size, ContentType: "text/plain"}}, References: []archiveReference{{ID: 1, BlobID: 17, OwnerUserID: 1, PublicID: "public-reference", Kind: "image", OriginalName: "original.png"}}}
	if err := os.MkdirAll(filepath.Dir(filepath.Join(stage, entry.File)), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stage, entry.File), content, 0600); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(doc)
	if err := os.WriteFile(filepath.Join(stage, AttachmentInventoryName), data, 0600); err != nil {
		t.Fatal(err)
	}
	return stage, doc, content
}

func TestAttachmentOriginalRestoreUploadsArchiveBytesAndMapsTargetKeys(t *testing.T) {
	stage, doc, content := attachmentArchiveFixture(t)
	var mu sync.Mutex
	objects := map[string][]byte{}
	puts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch r.Method {
		case http.MethodPut:
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Error(err)
				w.WriteHeader(500)
				return
			}
			objects[r.URL.Path] = body
			puts++
			w.Header().Set("ETag", `"test-etag"`)
		case http.MethodGet:
			body, ok := objects[r.URL.Path]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte("<Error><Code>NoSuchKey</Code></Error>"))
				return
			}
			_, _ = w.Write(body)
		default:
			t.Errorf("unexpected method %s", r.Method)
			w.WriteHeader(405)
		}
	}))
	defer server.Close()
	dbPath := filepath.Join(t.TempDir(), "target.db")
	db := attachmentArchiveTestDB(t, dbPath)
	site := models.SiteConfig{AttachmentStorageEnabled: true, AttachmentStorageProvider: "s3", AttachmentStorageEndpoint: server.URL, AttachmentStorageRegion: "us-east-1", AttachmentStorageBucket: "target-bucket", AttachmentStorageAccessKey: "test-access", AttachmentStorageSecretKey: "test-secret", AttachmentStorageUsePathStyle: true}
	if err := db.Create(&site).Error; err != nil {
		t.Fatal(err)
	}
	disabledSite := site
	disabledSite.AttachmentStorageEnabled = false
	if err := PrepareAttachmentRestoreWithSite(context.Background(), stage, "original", disabledSite); !errors.Is(err, ErrAttachmentFallbackRequired) {
		t.Fatalf("disabled cloud did not require local confirmation: %v", err)
	}
	if err := PrepareAttachmentRestoreWithSite(context.Background(), stage, "original", site); err != nil {
		t.Fatal(err)
	}
	if puts != 0 {
		t.Fatal("request preflight wrote remote content")
	}
	sqlDB, _ := db.DB()
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}
	candidate, err := PrepareAttachmentDatabase(context.Background(), stage, dbPath, "original")
	if err != nil {
		t.Fatal(err)
	}
	mapped := "/target-bucket/" + restoredCloudKey(doc.Entries[0].Hash)
	if got := objects[mapped]; string(got) != string(content) || puts != 1 {
		t.Fatalf("PUT %q count=%d", got, puts)
	}
	ready, err := backup.OpenOffline(candidate, false)
	if err != nil {
		t.Fatal(err)
	}
	readySQL, _ := ready.DB()
	defer readySQL.Close()
	var blob models.AttachmentBlob
	if err := ready.First(&blob).Error; err != nil {
		t.Fatal(err)
	}
	if blob.StorageKey != restoredCloudKey(doc.Entries[0].Hash) {
		t.Fatal(blob.StorageKey)
	}
	var liveCount int64
	live, err := backup.OpenOffline(dbPath, false)
	if err != nil {
		t.Fatal(err)
	}
	liveSQL, _ := live.DB()
	defer liveSQL.Close()
	if err := live.Model(&models.AttachmentReference{}).Count(&liveCount).Error; err != nil || liveCount != 0 {
		t.Fatal("candidate changed current database")
	}
}

func TestLocalAttachmentMigrationSurvivesLaterDataRestore(t *testing.T) {
	stage, doc, _ := attachmentArchiveFixture(t)
	root := t.TempDir()
	dbPath := filepath.Join(root, "target.db")
	db := attachmentArchiveTestDB(t, dbPath)
	user := models.User{ID: 1, Username: "owner", Password: "hash"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	oldURL := "https://old.example/api/cloud-attachments/public-reference/original.png"
	message := models.Message{UserID: 1, Content: "![image](" + oldURL + ")", ImageURL: oldURL, Visibility: "private"}
	if err := db.Create(&message).Error; err != nil {
		t.Fatal(err)
	}
	comment := models.Comment{UserID: &user.ID, Content: oldURL}
	if err := db.Create(&comment).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.LocalAttachmentGrant{Kind: "cloud", Name: "public-reference", MessageID: message.ID, OwnerUserID: 1, Visibility: "private"}).Error; err != nil {
		t.Fatal(err)
	}
	sourceSite := models.SiteConfig{SiteTitle: "source title", StorageBucket: "source-backups", AttachmentStorageBucket: "source-attachments"}
	if err := db.Create(&sourceSite).Error; err != nil {
		t.Fatal(err)
	}
	dataStage := t.TempDir()
	if err := db.Exec("VACUUM INTO ?", filepath.Join(dataStage, "database.db")).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&sourceSite).UpdateColumns(map[string]interface{}{
		"site_title": "target title", "storage_bucket": "target-backups", "attachment_storage_bucket": "target-attachments",
	}).Error; err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}
	candidate, err := PrepareAttachmentDatabase(context.Background(), stage, dbPath, "local")
	if err != nil {
		t.Fatal(err)
	}
	if err := PrepareDataAttachmentRestore(context.Background(), dataStage, candidate); err != nil {
		t.Fatal(err)
	}
	restored, err := backup.OpenOffline(filepath.Join(dataStage, "database.db"), false)
	if err != nil {
		t.Fatal(err)
	}
	restoredSQL, _ := restored.DB()
	defer restoredSQL.Close()
	var restoredSite models.SiteConfig
	if err := restored.First(&restoredSite).Error; err != nil {
		t.Fatal(err)
	}
	if restoredSite.SiteTitle != "source title" || restoredSite.StorageBucket != "target-backups" || restoredSite.AttachmentStorageBucket != "target-attachments" {
		t.Fatal("migration overlay must preserve target channels and source business settings")
	}
	var ref models.AttachmentReference
	if err := restored.Where("public_id = ?", "public-reference").First(&ref).Error; err != nil {
		t.Fatal(err)
	}
	var blob models.AttachmentBlob
	if err := restored.First(&blob, ref.BlobID).Error; err != nil {
		t.Fatal(err)
	}
	if blob.StorageBackend != "local" || blob.StorageKey != doc.Entries[0].Hash[:2]+"/"+doc.Entries[0].Hash {
		t.Fatalf("mapping %+v", blob)
	}
	if err := restored.First(&message, message.ID).Error; err != nil {
		t.Fatal(err)
	}
	wantURL := "/api/images/refs/public-reference/original.png"
	if message.Content != "![image]("+wantURL+")" || message.ImageURL != wantURL || strings.Contains(message.Content+message.ImageURL, "old.example") {
		t.Fatalf("stale message URLs: %s %s", message.Content, message.ImageURL)
	}
	if err := restored.First(&comment, comment.ID).Error; err != nil {
		t.Fatal(err)
	}
	if comment.Content != wantURL || strings.Contains(comment.Content, "old.example") {
		t.Fatalf("stale comment URL: %s", comment.Content)
	}
	var grant models.LocalAttachmentGrant
	if err := restored.Where("kind = ? AND name = ? AND message_id = ?", "cloud", "public-reference", message.ID).First(&grant).Error; err != nil {
		t.Fatal(err)
	}
	if grant.OwnerUserID != user.ID || grant.Visibility != "private" {
		t.Fatalf("private grant changed: %+v", grant)
	}
}

func TestAttachmentArchiveRejectsContentAndMetadataTampering(t *testing.T) {
	stage, doc, _ := attachmentArchiveFixture(t)
	if err := os.WriteFile(filepath.Join(stage, doc.Entries[0].File), []byte("truncated"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := PrepareAttachmentRestoreWithSite(context.Background(), stage, "local", models.SiteConfig{}); err == nil {
		t.Fatal("corrupt content accepted")
	}
	_, doc, _ = attachmentArchiveFixture(t)
	doc.References[0].Kind = "image|file"
	if err := validateAttachmentRegistry(doc); err == nil {
		t.Fatal("forged reference kind accepted")
	}
	doc.References[0].Kind = "file"
	doc.References[0].PublicID = "../escape"
	if err := validateAttachmentRegistry(doc); err == nil {
		t.Fatal("path public ID accepted")
	}
}
