package services

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/glebarez/sqlite"
	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type legacyCloudMigrationS3Mock struct {
	bucket         string
	objects        map[string]bool
	copyCalls      int
	deleteFailures map[string]int
}

func (mock *legacyCloudMigrationS3Mock) CopyObject(_ context.Context, input *s3.CopyObjectInput, _ ...func(*s3.Options)) (*s3.CopyObjectOutput, error) {
	source, err := url.PathUnescape(strings.TrimLeft(aws.ToString(input.CopySource), "/"))
	if err != nil {
		return nil, err
	}
	source = strings.TrimPrefix(source, mock.bucket+"/")
	if !mock.objects[source] {
		return nil, errors.New("source object not found")
	}
	mock.objects[aws.ToString(input.Key)] = true
	mock.copyCalls++
	return &s3.CopyObjectOutput{}, nil
}

func (mock *legacyCloudMigrationS3Mock) DeleteObject(_ context.Context, input *s3.DeleteObjectInput, _ ...func(*s3.Options)) (*s3.DeleteObjectOutput, error) {
	key := aws.ToString(input.Key)
	if mock.deleteFailures[key] > 0 {
		mock.deleteFailures[key]--
		return nil, errors.New("temporary delete failure")
	}
	delete(mock.objects, key)
	return &s3.DeleteObjectOutput{}, nil
}

func TestLegacyCloudAttachmentMigrationRewritesURLsAndRetriesPublicObjectCleanup(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	if err := db.AutoMigrate(&models.Message{}, &models.CloudAttachmentObject{}, &models.LocalAttachmentGrant{}); err != nil {
		t.Fatalf("migrate test database: %v", err)
	}
	cfg := models.SiteConfig{
		AttachmentStorageEnabled:       false,
		AttachmentStorageBucket:        "bucket",
		AttachmentStoragePublicBaseURL: "https://public.example.test/note",
	}
	oldKey := "note/legacy/report.txt"
	oldURL := "https://public.example.test/note/legacy/report.txt"
	message := models.Message{
		Content:    "[legacy report](" + oldURL + ")",
		ImageURL:   oldURL,
		UserID:     7,
		Visibility: "private",
		Private:    true,
	}
	if err := db.Create(&message).Error; err != nil {
		t.Fatalf("create legacy message: %v", err)
	}
	client := &legacyCloudMigrationS3Mock{
		bucket:         cfg.AttachmentStorageBucket,
		objects:        map[string]bool{oldKey: true},
		deleteFailures: map[string]int{oldKey: 1},
	}

	if err := migrateLegacyCloudAttachmentsWithClient(context.Background(), db, cfg, client); err == nil {
		t.Fatal("first migration should report the simulated legacy-object cleanup failure")
	}
	var object models.CloudAttachmentObject
	if err := db.Where("legacy_object_key = ?", oldKey).First(&object).Error; err != nil {
		t.Fatalf("load migrated object mapping: %v", err)
	}
	if object.PublicID == "" || object.ObjectKey == "" || object.ObjectKey == oldKey || !object.LegacyCleanupPending {
		t.Fatalf("migrated object mapping = %#v", object)
	}
	if !client.objects[oldKey] || !client.objects[object.ObjectKey] {
		t.Fatalf("objects after failed cleanup = %#v", client.objects)
	}
	if err := db.First(&message, message.ID).Error; err != nil {
		t.Fatalf("reload migrated message: %v", err)
	}
	controlledURL := "/api/cloud-attachments/" + object.PublicID + "/report.txt"
	if strings.Contains(message.Content, oldURL) || strings.Contains(message.ImageURL, oldURL) ||
		!strings.Contains(message.Content, controlledURL) || message.ImageURL != controlledURL {
		t.Fatalf("message URLs were not rewritten: content=%q image=%q", message.Content, message.ImageURL)
	}
	var grant models.LocalAttachmentGrant
	if err := db.Where("kind = ? AND name = ? AND message_id = ?", "cloud", object.PublicID, message.ID).First(&grant).Error; err != nil {
		t.Fatalf("load migrated cloud visibility grant: %v", err)
	}
	if grant.OwnerUserID != message.UserID || grant.Visibility != "private" {
		t.Fatalf("migrated cloud visibility grant = %#v", grant)
	}

	if err := migrateLegacyCloudAttachmentsWithClient(context.Background(), db, cfg, client); err != nil {
		t.Fatalf("retry pending cleanup: %v", err)
	}
	if client.objects[oldKey] {
		t.Fatalf("legacy public object still exists after retry: %#v", client.objects)
	}
	if client.copyCalls != 1 {
		t.Fatalf("legacy object copied %d times, want once", client.copyCalls)
	}
	if err := db.First(&object, object.ID).Error; err != nil {
		t.Fatalf("reload migrated object: %v", err)
	}
	if object.LegacyCleanupPending {
		t.Fatalf("legacy cleanup remained pending: %#v", object)
	}
}
