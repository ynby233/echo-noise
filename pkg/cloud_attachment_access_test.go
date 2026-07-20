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

func TestUploadAttachmentToCloudReturnsOpaqueControlledURL(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.AutoMigrate(&models.CloudAttachmentObject{}, &models.LocalAttachmentGrant{}); err != nil {
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
	url, err := UploadAttachmentToCloud(cfg, "report.txt", content, "text/plain", "")
	if err != nil {
		t.Fatalf("upload cloud attachment: %v", err)
	}
	if !strings.HasPrefix(url, "/api/cloud-attachments/") || strings.Contains(url, server.URL) || strings.Contains(url, "public.example.test") {
		t.Fatalf("upload returned uncontrolled URL %q", url)
	}

	var object models.CloudAttachmentObject
	if err := db.First(&object).Error; err != nil {
		t.Fatalf("load cloud attachment mapping: %v", err)
	}
	if object.PublicID == "" || object.ObjectKey == "" || object.OriginalName != "report.txt" {
		t.Fatalf("invalid mapping: %#v", object)
	}
	if !strings.Contains(url, object.PublicID) || strings.Contains(url, object.ObjectKey) {
		t.Fatalf("controlled URL %q exposes or omits mapping %#v", url, object)
	}
	if !strings.Contains(uploadedPath, object.ObjectKey) || strings.Contains(uploadedPath, object.PublicID) {
		t.Fatalf("storage path %q should contain only opaque object key, mapping %#v", uploadedPath, object)
	}

	duplicateURL, err := UploadAttachmentToCloud(cfg, "renamed-report.txt", bytes.NewReader([]byte("opaque cloud attachment")), "text/plain", "")
	if err != nil {
		t.Fatalf("deduplicate controlled cloud attachment: %v", err)
	}
	if duplicateURL != url {
		t.Fatalf("duplicate upload URL = %q, want existing controlled URL %q", duplicateURL, url)
	}
	if putCount != 1 {
		t.Fatalf("duplicate controlled upload issued %d PUT requests, want 1", putCount)
	}
	var count int64
	if err := db.Model(&models.CloudAttachmentObject{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("cloud mapping count = %d, err = %v", count, err)
	}
}
