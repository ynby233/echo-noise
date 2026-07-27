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

func TestPurgeBlobRemovesEveryReferenceAndThePhysicalObject(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.AutoMigrate(&models.AttachmentBlob{}, &models.AttachmentReference{}); err != nil {
		t.Fatalf("migrate attachment registry: %v", err)
	}

	content := []byte("purge every logical reference")
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

	created := make([]models.AttachmentReference, 0, 3)
	for i := 0; i < 3; i++ {
		reference, err := registry.Create(context.Background(), store, input, bytes.NewReader(content))
		if err != nil {
			t.Fatalf("create reference %d: %v", i, err)
		}
		created = append(created, reference)
	}

	var blob models.AttachmentBlob
	if err := db.First(&blob, created[0].BlobID).Error; err != nil {
		t.Fatalf("load shared blob: %v", err)
	}

	removed, err := registry.PurgeBlob(context.Background(), store, created[1].PublicID)
	if err != nil {
		t.Fatalf("purge shared blob: %v", err)
	}
	if removed != 3 {
		t.Fatalf("purged reference count = %d, want 3", removed)
	}

	var referenceCount int64
	if err := db.Model(&models.AttachmentReference{}).Count(&referenceCount).Error; err != nil || referenceCount != 0 {
		t.Fatalf("reference count = %d, err = %v", referenceCount, err)
	}
	if err := db.First(&models.AttachmentBlob{}, blob.ID).Error; !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("blob record still exists after purge: %v", err)
	}
	if exists, err := store.Exists(context.Background(), blob.StorageKey); err != nil || exists {
		t.Fatalf("physical blob exists=%v err=%v", exists, err)
	}
}

func TestPurgeBlobLeavesUnrelatedBlobsUntouched(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.AutoMigrate(&models.AttachmentBlob{}, &models.AttachmentReference{}); err != nil {
		t.Fatalf("migrate attachment registry: %v", err)
	}

	store := NewLocalStore(t.TempDir())
	registry := NewRegistry(db)
	newInput := func(body string, name string) (CreateInput, []byte) {
		payload := []byte(body)
		sum := sha256.Sum256(payload)
		return CreateInput{
			Kind:         "file",
			OwnerUserID:  7,
			OriginalName: name,
			ContentType:  "text/plain",
			ContentHash:  hex.EncodeToString(sum[:]),
			Size:         int64(len(payload)),
		}, payload
	}

	targetInput, targetBody := newInput("target content", "target.txt")
	keepInput, keepBody := newInput("unrelated content", "keep.txt")

	target, err := registry.Create(context.Background(), store, targetInput, bytes.NewReader(targetBody))
	if err != nil {
		t.Fatalf("create target reference: %v", err)
	}
	keep, err := registry.Create(context.Background(), store, keepInput, bytes.NewReader(keepBody))
	if err != nil {
		t.Fatalf("create unrelated reference: %v", err)
	}

	if _, err := registry.PurgeBlob(context.Background(), store, target.PublicID); err != nil {
		t.Fatalf("purge target blob: %v", err)
	}

	if err := db.First(&models.AttachmentReference{}, "public_id = ?", keep.PublicID).Error; err != nil {
		t.Fatalf("unrelated reference was removed: %v", err)
	}
	var keepBlob models.AttachmentBlob
	if err := db.First(&keepBlob, keep.BlobID).Error; err != nil {
		t.Fatalf("unrelated blob was removed: %v", err)
	}
	if exists, err := store.Exists(context.Background(), keepBlob.StorageKey); err != nil || !exists {
		t.Fatalf("unrelated physical blob exists=%v err=%v", exists, err)
	}
}
