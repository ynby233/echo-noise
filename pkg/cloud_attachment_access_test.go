package pkg

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
)

func TestUploadAttachmentToCloudCreatesSeparateReferencesForOneObject(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.AutoMigrate(&models.CloudAttachmentObject{}, &models.AttachmentBlob{}, &models.AttachmentReference{}, &models.LocalAttachmentGrant{}); err != nil {
		t.Fatalf("migrate cloud attachment mapping: %v", err)
	}
	database.DB = db
	t.Cleanup(func() { database.DB = nil })

	var uploadedPath string
	var uploadedHash string
	putCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			putCount++
			uploadedPath = r.URL.Path
			uploadedHash = r.Header.Get("X-Amz-Meta-Sha256")
			w.WriteHeader(http.StatusOK)
		case http.MethodHead:
			if r.URL.Path != uploadedPath {
				t.Fatalf("storage HEAD path = %q, want %q", r.URL.Path, uploadedPath)
			}
			w.Header().Set("X-Amz-Meta-Sha256", uploadedHash)
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("storage request method = %s, want PUT or HEAD", r.Method)
		}
	}))
	defer server.Close()

	cfg := &models.SiteConfig{
		AttachmentStorageEnabled:       true,
		AttachmentStorageProvider:      "r2",
		AttachmentStorageEndpoint:      server.URL,
		AttachmentStorageRegion:        "auto",
		AttachmentStorageBucket:        "attachments",
		AttachmentStorageAccessKey:     "access",
		AttachmentStorageSecretKey:     "secret",
		AttachmentStorageUsePathStyle:  true,
		AttachmentStoragePublicBaseURL: "https://public.example.test/note",
	}
	content := bytes.NewReader([]byte("opaque cloud attachment"))
	url, err := UploadAttachmentToCloud(cfg, "file", 7, "report.txt", content, "text/plain", "")
	if err != nil {
		t.Fatalf("upload cloud attachment: %v", err)
	}
	if !strings.HasPrefix(url, "/api/cloud-attachments/") || strings.Contains(url, server.URL) || strings.Contains(url, "public.example.test") {
		t.Fatalf("upload returned uncontrolled URL %q", url)
	}

	var firstReference models.AttachmentReference
	if err := db.Order("id ASC").First(&firstReference).Error; err != nil {
		t.Fatalf("load cloud attachment reference: %v", err)
	}
	var object models.AttachmentBlob
	if err := db.First(&object, firstReference.BlobID).Error; err != nil {
		t.Fatalf("load cloud attachment blob: %v", err)
	}
	if firstReference.PublicID == "" || firstReference.OriginalName != "report.txt" || object.StorageKey == "" {
		t.Fatalf("invalid reference/blob: %#v %#v", firstReference, object)
	}
	if !strings.Contains(url, firstReference.PublicID) || strings.Contains(url, object.StorageKey) {
		t.Fatalf("controlled URL %q exposes or omits reference/blob %#v %#v", url, firstReference, object)
	}
	if !strings.Contains(uploadedPath, object.StorageKey) || strings.Contains(uploadedPath, firstReference.PublicID) {
		t.Fatalf("storage path %q should contain only blob key, reference/blob %#v %#v", uploadedPath, firstReference, object)
	}

	duplicateURL, err := UploadAttachmentToCloud(cfg, "file", 7, "renamed-report.txt", bytes.NewReader([]byte("opaque cloud attachment")), "text/plain", "")
	if err != nil {
		t.Fatalf("deduplicate controlled cloud attachment: %v", err)
	}
	if duplicateURL == url || !strings.HasSuffix(duplicateURL, "/renamed-report.txt") {
		t.Fatalf("duplicate upload URL = %q, first URL %q", duplicateURL, url)
	}
	if putCount != 1 {
		t.Fatalf("duplicate controlled upload issued %d PUT requests, want 1", putCount)
	}
	var count int64
	if err := db.Model(&models.AttachmentBlob{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("cloud blob count = %d, err = %v", count, err)
	}
	if err := db.Model(&models.AttachmentReference{}).Count(&count).Error; err != nil || count != 2 {
		t.Fatalf("cloud reference count = %d, err = %v", count, err)
	}
}
