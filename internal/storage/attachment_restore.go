package storage

import (
	"context"
	"errors"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/rcy1314/echo-noise/internal/models"
)

// UploadAttachmentArchiveObject writes verified archived bytes to an application
// content-addressed key. Persisted keys are passed verbatim, without URL prefixes.
func UploadAttachmentArchiveObject(ctx context.Context, site models.SiteConfig, key string, body io.ReadSeeker, contentType, hash string, size int64) error {
	if strings.TrimSpace(site.AttachmentStorageBucket) == "" || strings.TrimSpace(site.AttachmentStorageAccessKey) == "" || strings.TrimSpace(site.AttachmentStorageSecretKey) == "" {
		return errors.New("attachment cloud storage configuration is incomplete")
	}
	cloud := models.SiteConfig{StorageProvider: site.AttachmentStorageProvider, StorageEndpoint: site.AttachmentStorageEndpoint, StorageRegion: site.AttachmentStorageRegion, StorageAccessKey: site.AttachmentStorageAccessKey, StorageSecretKey: site.AttachmentStorageSecretKey, StorageUsePathStyle: site.AttachmentStorageUsePathStyle}
	client, err := getS3Client(cloud)
	if err != nil {
		return err
	}
	_, err = client.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(site.AttachmentStorageBucket), Key: aws.String(key), Body: body, ContentLength: aws.Int64(size), ContentType: aws.String(contentType), Metadata: map[string]string{"sha256": hash}})
	return err
}
