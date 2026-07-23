package attachments

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
)

func TestCreateKeepsLogicalReferencesSeparateWhileSharingOneBlob(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.AutoMigrate(&models.AttachmentBlob{}, &models.AttachmentReference{}); err != nil {
		t.Fatalf("migrate attachment registry: %v", err)
	}

	content := []byte(`{"same":"content"}`)
	sum := sha256.Sum256(content)
	contentHash := hex.EncodeToString(sum[:])
	store := NewLocalStore(t.TempDir())
	registry := NewRegistry(db)

	first, err := registry.Create(context.Background(), store, CreateInput{
		Kind:         "file",
		OwnerUserID:  7,
		OriginalName: "auth.json",
		ContentType:  "application/json",
		ContentHash:  contentHash,
		Size:         int64(len(content)),
	}, bytes.NewReader(content))
	if err != nil {
		t.Fatalf("create first reference: %v", err)
	}
	second, err := registry.Create(context.Background(), store, CreateInput{
		Kind:         "file",
		OwnerUserID:  7,
		OriginalName: "auth.json",
		ContentType:  "application/json",
		ContentHash:  contentHash,
		Size:         int64(len(content)),
	}, bytes.NewReader(content))
	if err != nil {
		t.Fatalf("create second reference: %v", err)
	}

	if first.PublicID == second.PublicID {
		t.Fatalf("logical references unexpectedly share public id %q", first.PublicID)
	}
	if first.BlobID == 0 || first.BlobID != second.BlobID {
		t.Fatalf("logical references do not share one blob: first=%d second=%d", first.BlobID, second.BlobID)
	}
	if first.OriginalName != "auth.json" || second.OriginalName != "auth.json" {
		t.Fatalf("original names were not preserved: first=%q second=%q", first.OriginalName, second.OriginalName)
	}

	var blobCount int64
	if err := db.Model(&models.AttachmentBlob{}).Count(&blobCount).Error; err != nil || blobCount != 1 {
		t.Fatalf("blob count = %d, err = %v", blobCount, err)
	}
	var referenceCount int64
	if err := db.Model(&models.AttachmentReference{}).Count(&referenceCount).Error; err != nil || referenceCount != 2 {
		t.Fatalf("reference count = %d, err = %v", referenceCount, err)
	}

	files := 0
	err = filepath.Walk(store.Root(), func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !info.IsDir() {
			files++
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk blob store: %v", err)
	}
	if files != 1 {
		t.Fatalf("physical blob count = %d, want 1", files)
	}
}

func TestDeleteReferencePreservesSharedBlobUntilLastReference(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.AutoMigrate(&models.AttachmentBlob{}, &models.AttachmentReference{}); err != nil {
		t.Fatalf("migrate attachment registry: %v", err)
	}

	content := []byte("shared attachment")
	sum := sha256.Sum256(content)
	input := CreateInput{
		Kind:         "file",
		OwnerUserID:  7,
		OriginalName: "shared.txt",
		ContentType:  "text/plain",
		ContentHash:  hex.EncodeToString(sum[:]),
		Size:         int64(len(content)),
	}
	store := NewLocalStore(t.TempDir())
	registry := NewRegistry(db)
	first, err := registry.Create(context.Background(), store, input, bytes.NewReader(content))
	if err != nil {
		t.Fatalf("create first reference: %v", err)
	}
	second, err := registry.Create(context.Background(), store, input, bytes.NewReader(content))
	if err != nil {
		t.Fatalf("create second reference: %v", err)
	}

	if err := registry.DeleteReference(context.Background(), store, first.PublicID); err != nil {
		t.Fatalf("delete first reference: %v", err)
	}
	if err := db.First(&models.AttachmentReference{}, "public_id = ?", second.PublicID).Error; err != nil {
		t.Fatalf("second reference was removed: %v", err)
	}
	var blob models.AttachmentBlob
	if err := db.First(&blob, second.BlobID).Error; err != nil {
		t.Fatalf("shared blob was removed too early: %v", err)
	}
	if exists, err := store.Exists(context.Background(), blob.StorageKey); err != nil || !exists {
		t.Fatalf("shared physical blob exists=%v err=%v", exists, err)
	}

	if err := registry.DeleteReference(context.Background(), store, second.PublicID); err != nil {
		t.Fatalf("delete last reference: %v", err)
	}
	if err := db.First(&models.AttachmentBlob{}, second.BlobID).Error; !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("orphan blob record still exists: %v", err)
	}
	if exists, err := store.Exists(context.Background(), blob.StorageKey); err != nil || exists {
		t.Fatalf("orphan physical blob exists=%v err=%v", exists, err)
	}
}
