package controllers

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/rcy1314/echo-noise/config"
	"github.com/rcy1314/echo-noise/internal/models"
)

func TestGetAllImagesSkipsDeletedAttachments(t *testing.T) {
	db, r, owner, _ := setupCommentAccountTest(t)
	if err := db.Where("1 = 1").Delete(&models.Message{}).Error; err != nil {
		t.Fatalf("clear seed messages: %v", err)
	}
	if err := db.AutoMigrate(&models.AttachmentReference{}, &models.AttachmentBlob{}, &models.CloudAttachmentObject{}); err != nil {
		t.Fatalf("migrate attachment tables: %v", err)
	}

	imageDir := t.TempDir()
	for _, name := range []string{"kept.png", "field.png"} {
		if err := os.WriteFile(filepath.Join(imageDir, name), []byte("png"), 0o600); err != nil {
			t.Fatalf("write image %s: %v", name, err)
		}
	}
	originalSavePath := config.Config.Upload.SavePath
	config.Config.Upload.SavePath = imageDir
	t.Cleanup(func() { config.Config.Upload.SavePath = originalSavePath })

	blob := models.AttachmentBlob{StorageBackend: "local", StorageKey: "ab/abc", ContentHash: "abc", Size: 3}
	if err := db.Create(&blob).Error; err != nil {
		t.Fatalf("create blob: %v", err)
	}
	reference := models.AttachmentReference{PublicID: "live-ref", BlobID: blob.ID, OwnerUserID: owner.ID, Kind: "image", OriginalName: "ref.png"}
	if err := db.Create(&reference).Error; err != nil {
		t.Fatalf("create reference: %v", err)
	}
	cloudObject := models.CloudAttachmentObject{PublicID: "live-cloud", ObjectKey: "cloud/live.png", OriginalName: "live.png"}
	if err := db.Create(&cloudObject).Error; err != nil {
		t.Fatalf("create cloud object: %v", err)
	}

	message := models.Message{
		Content: "gallery " +
			"![kept](/api/images/kept.png) " +
			"![gone](/api/images/deleted.png) " +
			"![ref](/api/images/refs/live-ref/ref.png) " +
			"![refgone](/api/images/refs/dead-ref/ref.png) " +
			"![cloud](/api/cloud-attachments/live-cloud/live.png) " +
			"![cloudgone](/api/cloud-attachments/dead-cloud/live.png) " +
			"![external](https://cdn.example.com/remote.png)",
		ImageURL: "/api/images/field.png",
		UserID:   owner.ID,
		Username: owner.Username,
	}
	if err := db.Create(&message).Error; err != nil {
		t.Fatalf("create message: %v", err)
	}

	r.GET("/messages/images", GetAllImages)
	assertImages(t, performImagesRequest(r, ""), []string{
		"/api/images/field.png",
		"/api/images/kept.png",
		"/api/images/refs/live-ref/ref.png",
		"/api/cloud-attachments/live-cloud/live.png",
		"https://cdn.example.com/remote.png",
	})
}

func TestClassifyImageBackingRecognizesStoredURLShapes(t *testing.T) {
	cases := []struct {
		name    string
		rawURL  string
		expect  imageBackingKind
		expects string
	}{
		{name: "absolute local", rawURL: "http://host:27184/api/images/a%20b.png", expect: imageBackingLocalFile, expects: "a b.png"},
		{name: "legacy relative local", rawURL: "/images/legacy.png", expect: imageBackingLocalFile, expects: "legacy.png"},
		{name: "local with query", rawURL: "/api/images/query.png?v=2", expect: imageBackingLocalFile, expects: "query.png"},
		{name: "reference", rawURL: "/api/images/refs/pub-id/name.png", expect: imageBackingReference, expects: "pub-id"},
		{name: "cloud", rawURL: "/api/cloud-attachments/cloud-id/name.png", expect: imageBackingCloudObject, expects: "cloud-id"},
		{name: "external", rawURL: "https://cdn.example.com/x.png", expect: imageBackingUnknown},
		{name: "video", rawURL: "/api/video/clip.mp4", expect: imageBackingUnknown},
		{name: "traversal", rawURL: "/api/images/..%2Fescape.png", expect: imageBackingUnknown},
		{name: "empty", rawURL: "", expect: imageBackingUnknown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyImageBacking(tc.rawURL)
			if got.kind != tc.expect {
				t.Fatalf("expected kind %d, got %d (%#v)", tc.expect, got.kind, got)
			}
			if tc.expects != "" && got.id != tc.expects {
				t.Fatalf("expected id %q, got %q", tc.expects, got.id)
			}
		})
	}
}

func TestGetCurrentUserHomeStatsSkipsDeletedAttachmentImages(t *testing.T) {
	db, r, owner, _ := setupCommentAccountTest(t)
	if err := db.Where("1 = 1").Delete(&models.Message{}).Error; err != nil {
		t.Fatalf("clear seed messages: %v", err)
	}
	if err := db.AutoMigrate(&models.AttachmentReference{}, &models.AttachmentBlob{}, &models.CloudAttachmentObject{}); err != nil {
		t.Fatalf("migrate attachment tables: %v", err)
	}

	imageDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(imageDir, "kept.png"), []byte("png"), 0o600); err != nil {
		t.Fatalf("write kept image: %v", err)
	}
	originalSavePath := config.Config.Upload.SavePath
	config.Config.Upload.SavePath = imageDir
	t.Cleanup(func() { config.Config.Upload.SavePath = originalSavePath })

	message := models.Message{
		Content: "stats #tag " +
			"![kept](/api/images/kept.png) " +
			"![gone](/api/images/deleted.png) " +
			"![external](https://cdn.example.com/remote.png)",
		ImageURL: "/api/images/kept.png",
		UserID:   owner.ID,
		Username: owner.Username,
	}
	if err := db.Create(&message).Error; err != nil {
		t.Fatalf("create message: %v", err)
	}

	r.GET("/users/me/stats", func(c *gin.Context) {
		c.Set("user_id", owner.ID)
		GetCurrentUserHomeStats(c)
	})

	stats := decodeHomeStatsResponse(t, performHomeStatsRequest(r, "owner"), http.StatusOK)
	if stats.TotalMessages != 1 || stats.TotalTags != 1 || stats.TotalImages != 3 {
		t.Fatalf("expected stats 1/1/3 after dropping the deleted attachment, got %#v", stats)
	}
}
