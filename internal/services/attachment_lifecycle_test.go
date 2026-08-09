package services

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
)

func TestRemoveUnreferencedMessageAttachmentReferencesPreservesSharedBlobAndSurvivingUsage(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.Message{}, &models.Comment{}, &models.AttachmentBlob{}, &models.AttachmentReference{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	blob := models.AttachmentBlob{StorageBackend: "local", StorageKey: "shared", ContentHash: "hash", Size: 1}
	if err := db.Create(&blob).Error; err != nil {
		t.Fatalf("create blob: %v", err)
	}
	removed := models.AttachmentReference{PublicID: "removed-ref", BlobID: blob.ID, OwnerUserID: 2, Kind: "file", OriginalName: "removed.txt"}
	shared := models.AttachmentReference{PublicID: "shared-ref", BlobID: blob.ID, OwnerUserID: 2, Kind: "file", OriginalName: "shared.txt"}
	if err := db.Create(&[]models.AttachmentReference{removed, shared}).Error; err != nil {
		t.Fatalf("create refs: %v", err)
	}
	target := models.Message{UserID: 2, Content: "[a](/api/files/refs/removed-ref/a.txt) [b](/api/files/refs/shared-ref/b.txt)"}
	survivor := models.Message{UserID: 3, Content: "[b](/api/files/refs/shared-ref/b.txt)"}
	if err := db.Create(&target).Error; err != nil {
		t.Fatalf("create target: %v", err)
	}
	if err := db.Create(&survivor).Error; err != nil {
		t.Fatalf("create survivor: %v", err)
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		return RemoveUnreferencedMessageAttachmentReferences(tx, target.ID, target, nil)
	}); err != nil {
		t.Fatalf("remove refs: %v", err)
	}
	var refs []models.AttachmentReference
	if err := db.Order("public_id").Find(&refs).Error; err != nil {
		t.Fatalf("load refs: %v", err)
	}
	if len(refs) != 1 || refs[0].PublicID != "shared-ref" {
		t.Fatalf("unexpected refs: %#v", refs)
	}
	var blobs int64
	if err := db.Model(&models.AttachmentBlob{}).Count(&blobs).Error; err != nil || blobs != 1 {
		t.Fatalf("shared physical blob must remain, count=%d err=%v", blobs, err)
	}
}

func TestRemoveUnreferencedMessageAttachmentReferencesIncludesCommentsAndReplies(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.Message{}, &models.Comment{}, &models.AttachmentBlob{}, &models.AttachmentReference{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	blob := models.AttachmentBlob{StorageBackend: "local", StorageKey: "comment", ContentHash: "comment-hash", Size: 1}
	if err := db.Create(&blob).Error; err != nil {
		t.Fatalf("create blob: %v", err)
	}
	ref := models.AttachmentReference{PublicID: "comment-ref", BlobID: blob.ID, OwnerUserID: 2, Kind: "file", OriginalName: "comment.txt"}
	if err := db.Create(&ref).Error; err != nil {
		t.Fatalf("create ref: %v", err)
	}
	message := models.Message{UserID: 2, Content: "note"}
	if err := db.Create(&message).Error; err != nil {
		t.Fatalf("create message: %v", err)
	}
	owner := uint(2)
	comment := models.Comment{MessageID: message.ID, UserID: &owner, Content: "[file](/api/files/refs/comment-ref/comment.txt)"}
	if err := db.Create(&comment).Error; err != nil {
		t.Fatalf("create comment: %v", err)
	}
	if err := RemoveUnreferencedMessageAttachmentReferences(db, message.ID, message, []models.Comment{comment}); err != nil {
		t.Fatalf("remove refs: %v", err)
	}
	var count int64
	if err := db.Model(&models.AttachmentReference{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("comment ref remains: count=%d err=%v", count, err)
	}
}
