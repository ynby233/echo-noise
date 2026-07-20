package services

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path"
	"strings"
	"sync"
	"unicode"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
)

type legacyCloudAttachmentS3API interface {
	CopyObject(context.Context, *s3.CopyObjectInput, ...func(*s3.Options)) (*s3.CopyObjectOutput, error)
	DeleteObject(context.Context, *s3.DeleteObjectInput, ...func(*s3.Options)) (*s3.DeleteObjectOutput, error)
}

type legacyCloudAttachmentReference struct {
	ObjectKey    string
	OriginalName string
	OldURLs      []string
}

var legacyCloudAttachmentMigrationMu sync.Mutex

// MigrateLegacyCloudAttachments replaces historical public-bucket URLs with
// opaque, viewer-authorized application URLs. It is safe to retry: database
// rewrites and mappings are transactional, while failed legacy-object deletes
// remain marked for cleanup on the next run.
func MigrateLegacyCloudAttachments(ctx context.Context) error {
	legacyCloudAttachmentMigrationMu.Lock()
	defer legacyCloudAttachmentMigrationMu.Unlock()

	db, err := database.GetDB()
	if err != nil {
		return err
	}
	var cfg models.SiteConfig
	if err := db.Table("site_configs").First(&cfg).Error; err != nil {
		return err
	}
	if strings.TrimSpace(cfg.AttachmentStorageEndpoint) == "" ||
		strings.TrimSpace(cfg.AttachmentStorageBucket) == "" ||
		strings.TrimSpace(cfg.AttachmentStorageAccessKey) == "" ||
		strings.TrimSpace(cfg.AttachmentStorageSecretKey) == "" {
		return nil
	}
	client, err := newLegacyCloudAttachmentS3Client(ctx, cfg)
	if err != nil {
		return err
	}
	return migrateLegacyCloudAttachmentsWithClient(ctx, db, cfg, client)
}

func newLegacyCloudAttachmentS3Client(ctx context.Context, cfg models.SiteConfig) (*s3.Client, error) {
	if strings.TrimSpace(cfg.AttachmentStorageEndpoint) == "" ||
		strings.TrimSpace(cfg.AttachmentStorageBucket) == "" ||
		strings.TrimSpace(cfg.AttachmentStorageAccessKey) == "" ||
		strings.TrimSpace(cfg.AttachmentStorageSecretKey) == "" {
		return nil, errors.New("attachment storage configuration is incomplete")
	}
	resolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
		return aws.Endpoint{URL: strings.TrimSpace(cfg.AttachmentStorageEndpoint)}, nil
	})
	region := strings.TrimSpace(cfg.AttachmentStorageRegion)
	if region == "" {
		region = "auto"
	}
	awsConfig, err := awscfg.LoadDefaultConfig(
		ctx,
		awscfg.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			cfg.AttachmentStorageAccessKey,
			cfg.AttachmentStorageSecretKey,
			"",
		)),
		awscfg.WithEndpointResolverWithOptions(resolver),
		awscfg.WithRegion(region),
	)
	if err != nil {
		return nil, err
	}
	return s3.NewFromConfig(awsConfig, func(options *s3.Options) {
		options.UsePathStyle = cfg.AttachmentStorageUsePathStyle
	}), nil
}

func migrateLegacyCloudAttachmentsWithClient(ctx context.Context, db *gorm.DB, cfg models.SiteConfig, client legacyCloudAttachmentS3API) error {
	if db == nil || client == nil {
		return errors.New("legacy cloud attachment migration is not initialized")
	}
	bucket := strings.TrimSpace(cfg.AttachmentStorageBucket)
	if bucket == "" {
		return errors.New("attachment storage bucket is empty")
	}

	var migrationErrors []error
	if err := retryLegacyCloudAttachmentCleanup(ctx, db, bucket, client); err != nil {
		migrationErrors = append(migrationErrors, err)
	}

	var messages []models.Message
	if err := db.Find(&messages).Error; err != nil {
		return errors.Join(append(migrationErrors, err)...)
	}
	references, prefix, err := collectLegacyCloudAttachmentReferences(messages, cfg.AttachmentStoragePublicBaseURL)
	if err != nil {
		return errors.Join(append(migrationErrors, err)...)
	}
	for _, reference := range references {
		if err := migrateLegacyCloudAttachmentReference(ctx, db, client, bucket, prefix, reference, messages); err != nil {
			migrationErrors = append(migrationErrors, err)
		}
	}
	return errors.Join(migrationErrors...)
}

func retryLegacyCloudAttachmentCleanup(ctx context.Context, db *gorm.DB, bucket string, client legacyCloudAttachmentS3API) error {
	var pending []models.CloudAttachmentObject
	if err := db.Where("legacy_cleanup_pending = ? AND legacy_object_key <> ''", true).Find(&pending).Error; err != nil {
		return err
	}
	var cleanupErrors []error
	for _, object := range pending {
		if _, err := client.DeleteObject(ctx, &s3.DeleteObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(object.LegacyObjectKey),
		}); err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("delete legacy cloud object %q: %w", object.LegacyObjectKey, err))
			continue
		}
		if err := db.Model(&models.CloudAttachmentObject{}).
			Where("id = ?", object.ID).
			Update("legacy_cleanup_pending", false).Error; err != nil {
			cleanupErrors = append(cleanupErrors, err)
		}
	}
	return errors.Join(cleanupErrors...)
}

func migrateLegacyCloudAttachmentReference(
	ctx context.Context,
	db *gorm.DB,
	client legacyCloudAttachmentS3API,
	bucket string,
	prefix string,
	reference legacyCloudAttachmentReference,
	messages []models.Message,
) error {
	var object models.CloudAttachmentObject
	err := db.Where("legacy_object_key = ?", reference.ObjectKey).First(&object).Error
	createdNewObject := false
	if errors.Is(err, gorm.ErrRecordNotFound) {
		publicID := uuid.NewString()
		storageID := uuid.NewString()
		newKey := storageID + "/" + reference.OriginalName
		if prefix != "" {
			newKey = prefix + "/" + newKey
		}
		copySource := url.PathEscape(bucket + "/" + strings.TrimLeft(reference.ObjectKey, "/"))
		if _, err := client.CopyObject(ctx, &s3.CopyObjectInput{
			Bucket:     aws.String(bucket),
			Key:        aws.String(newKey),
			CopySource: aws.String(copySource),
		}); err != nil {
			return fmt.Errorf("copy legacy cloud object %q: %w", reference.ObjectKey, err)
		}
		object = models.CloudAttachmentObject{
			PublicID:             publicID,
			ObjectKey:            newKey,
			OriginalName:         reference.OriginalName,
			LegacyObjectKey:      reference.ObjectKey,
			LegacyCleanupPending: true,
		}
		createdNewObject = true
	} else if err != nil {
		return err
	}

	controlledURL := "/api/cloud-attachments/" + object.PublicID + "/" + url.PathEscape(object.OriginalName)
	if err := db.Transaction(func(tx *gorm.DB) error {
		if object.ID == 0 {
			if err := tx.Create(&object).Error; err != nil {
				return err
			}
		}
		for _, message := range messages {
			var current models.Message
			if err := tx.Select("id", "content", "image_url", "user_id", "private", "visibility").First(&current, message.ID).Error; err != nil {
				return err
			}
			content := replaceLegacyCloudAttachmentURLs(current.Content, reference.OldURLs, controlledURL)
			imageURL := replaceLegacyCloudAttachmentURLs(current.ImageURL, reference.OldURLs, controlledURL)
			if content == current.Content && imageURL == current.ImageURL {
				continue
			}
			if err := tx.Model(&models.Message{}).Where("id = ?", message.ID).
				Updates(map[string]interface{}{"content": content, "image_url": imageURL}).Error; err != nil {
				return err
			}
			current.Content = content
			current.ImageURL = imageURL
			if err := models.SyncLocalAttachmentGrants(tx, &current); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		if createdNewObject {
			_, _ = client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(bucket), Key: aws.String(object.ObjectKey)})
		}
		return err
	}

	if _, err := client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(reference.ObjectKey),
	}); err != nil {
		return fmt.Errorf("delete migrated legacy cloud object %q: %w", reference.ObjectKey, err)
	}
	return db.Model(&models.CloudAttachmentObject{}).
		Where("id = ?", object.ID).
		Update("legacy_cleanup_pending", false).Error
}

func replaceLegacyCloudAttachmentURLs(value string, oldURLs []string, controlledURL string) string {
	for _, oldURL := range oldURLs {
		value = strings.ReplaceAll(value, oldURL, controlledURL)
	}
	return value
}

func collectLegacyCloudAttachmentReferences(messages []models.Message, publicBaseURL string) ([]legacyCloudAttachmentReference, string, error) {
	base, parsedBase, err := normalizeLegacyCloudAttachmentPublicBaseURL(publicBaseURL)
	if err != nil || base == "" {
		return nil, "", err
	}
	prefix, err := url.PathUnescape(strings.Trim(parsedBase.EscapedPath(), "/"))
	if err != nil {
		return nil, "", err
	}
	byKey := make(map[string]*legacyCloudAttachmentReference)
	order := make([]string, 0)
	for _, message := range messages {
		for _, source := range []string{message.Content, message.ImageURL} {
			offset := 0
			needle := base + "/"
			for offset < len(source) {
				relative := strings.Index(source[offset:], needle)
				if relative < 0 {
					break
				}
				start := offset + relative
				end := start + len(needle)
				for end < len(source) && !isLegacyCloudAttachmentURLDelimiter(rune(source[end])) {
					end++
				}
				oldURL := source[start:end]
				parsedURL, parseErr := url.Parse(oldURL)
				if parseErr == nil {
					objectKey, unescapeErr := url.PathUnescape(strings.TrimLeft(parsedURL.EscapedPath(), "/"))
					if unescapeErr == nil && objectKey != "" {
						originalName := path.Base(objectKey)
						if originalName != "." && originalName != "/" && originalName != "" {
							reference, exists := byKey[objectKey]
							if !exists {
								reference = &legacyCloudAttachmentReference{ObjectKey: objectKey, OriginalName: originalName}
								byKey[objectKey] = reference
								order = append(order, objectKey)
							}
							if !containsString(reference.OldURLs, oldURL) {
								reference.OldURLs = append(reference.OldURLs, oldURL)
							}
						}
					}
				}
				offset = end
				if offset <= start {
					offset = start + 1
				}
			}
		}
	}
	references := make([]legacyCloudAttachmentReference, 0, len(order))
	for _, key := range order {
		references = append(references, *byKey[key])
	}
	return references, prefix, nil
}

func normalizeLegacyCloudAttachmentPublicBaseURL(value string) (string, *url.URL, error) {
	value = strings.TrimRight(strings.TrimSpace(value), "/")
	if value == "" {
		return "", nil, nil
	}
	if strings.HasPrefix(value, "//") {
		value = "https:" + value
	}
	if !strings.Contains(value, "://") {
		value = "https://" + strings.TrimLeft(value, "/")
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", nil, fmt.Errorf("invalid attachment public base URL %q", value)
	}
	return strings.TrimRight(parsed.String(), "/"), parsed, nil
}

func isLegacyCloudAttachmentURLDelimiter(value rune) bool {
	return unicode.IsSpace(value) || strings.ContainsRune(`)]}"'<>`+"`", value)
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
