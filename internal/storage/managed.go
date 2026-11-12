package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"github.com/rcy1314/echo-noise/internal/models"
)

// ErrMissingContent identifies absent content without treating permission errors as absence.
var ErrMissingContent = errors.New("attachment content is missing")

type managedS3API interface {
	GetObject(context.Context, *s3.GetObjectInput, ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	PutObject(context.Context, *s3.PutObjectInput, ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	DeleteObject(context.Context, *s3.DeleteObjectInput, ...func(*s3.Options)) (*s3.DeleteObjectOutput, error)
	ListObjectsV2(context.Context, *s3.ListObjectsV2Input, ...func(*s3.Options)) (*s3.ListObjectsV2Output, error)
}

// ManagedStore restricts history operations to direct ZIP children of one application prefix.
type ManagedStore struct {
	client managedS3API
	bucket string
	prefix string
}

type ManagedObject struct {
	Name         string
	Size         int64
	LastModified time.Time
}

func NewManagedStore(site models.SiteConfig, kind string) (*ManagedStore, error) {
	if !site.StorageEnabled || (kind != "data" && kind != "attachments" && kind != "server-config") || strings.TrimSpace(site.StorageBucket) == "" || strings.TrimSpace(site.StorageAccessKey) == "" || strings.TrimSpace(site.StorageSecretKey) == "" {
		return nil, errors.New("managed backup storage configuration or kind is invalid")
	}
	client, err := getS3Client(site)
	if err != nil {
		return nil, err
	}
	return &ManagedStore{client: client, bucket: site.StorageBucket, prefix: ResolveObjectKey(site, "noise-managed-backups/"+kind+"/")}, nil
}

func validManagedSegment(value string) bool {
	if value == "" || value == "." || value == ".." {
		return false
	}
	for _, ch := range value {
		if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '-' || ch == '_' || ch == '.') {
			return false
		}
	}
	return true
}

func (s *ManagedStore) key(name string) (string, error) {
	if s == nil || s.client == nil || s.bucket == "" || s.prefix == "" || !strings.HasSuffix(s.prefix, "/") {
		return "", errors.New("managed storage is not initialized")
	}
	if !validManagedSegment(name) || path.Base(name) != name || !strings.HasSuffix(name, ".zip") {
		return "", errors.New("managed backup name must be a direct ZIP filename")
	}
	return s.prefix + name, nil
}

func (s *ManagedStore) Upload(ctx context.Context, name string, body io.ReadSeeker) error {
	key, err := s.key(name)
	if err != nil {
		return err
	}
	if body == nil {
		return errors.New("managed backup body is nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	_, err = s.client.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key), Body: body, ContentType: aws.String("application/zip")})
	if err != nil {
		return fmt.Errorf("upload managed backup %q: %w", name, err)
	}
	return ctx.Err()
}

func (s *ManagedStore) Download(ctx context.Context, name string, dst io.Writer) (int64, error) {
	key, err := s.key(name)
	if err != nil {
		return 0, err
	}
	return downloadObject(ctx, s.client, s.bucket, key, dst)
}

// Open performs GetObject synchronously, so missing/forbidden objects fail before
// an HTTP controller sends download headers. The reader checks complete content.
func (s *ManagedStore) Open(ctx context.Context, name string) (io.ReadCloser, error) {
	key, err := s.key(name)
	if err != nil {
		return nil, err
	}
	return openObject(ctx, s.client, s.bucket, key)
}

func (s *ManagedStore) Delete(ctx context.Context, name string) error {
	key, err := s.key(name)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	_, err = s.client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)})
	if err != nil {
		return fmt.Errorf("delete managed backup %q: %w", name, err)
	}
	return ctx.Err()
}

func (s *ManagedStore) List(ctx context.Context) ([]ManagedObject, error) {
	if _, err := s.key("probe.zip"); err != nil {
		return nil, err
	}
	var result []ManagedObject
	var token *string
	seen := map[string]bool{}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		page, err := s.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{Bucket: aws.String(s.bucket), Prefix: aws.String(s.prefix), ContinuationToken: token})
		if err != nil {
			return nil, fmt.Errorf("list managed backups: %w", err)
		}
		if page == nil {
			return nil, errors.New("list managed backups returned no page")
		}
		for _, object := range page.Contents {
			key := aws.ToString(object.Key)
			if !strings.HasPrefix(key, s.prefix) {
				continue
			}
			name := strings.TrimPrefix(key, s.prefix)
			if _, err := s.key(name); err != nil {
				continue
			}
			result = append(result, ManagedObject{Name: name, Size: aws.ToInt64(object.Size), LastModified: aws.ToTime(object.LastModified)})
		}
		if !aws.ToBool(page.IsTruncated) {
			return result, ctx.Err()
		}
		next := aws.ToString(page.NextContinuationToken)
		if next == "" || seen[next] {
			return nil, errors.New("managed backup listing has an invalid continuation token")
		}
		seen[next] = true
		token = aws.String(next)
	}
}

type objectGetter interface {
	GetObject(context.Context, *s3.GetObjectInput, ...func(*s3.Options)) (*s3.GetObjectOutput, error)
}

// DownloadAttachmentObject reads a persisted key verbatim. It never adds a public URL prefix.
func DownloadAttachmentObject(ctx context.Context, site models.SiteConfig, key string, dst io.Writer) (int64, error) {
	if strings.TrimSpace(key) == "" {
		return 0, errors.New("attachment object key is empty")
	}
	if strings.TrimSpace(site.AttachmentStorageBucket) == "" || strings.TrimSpace(site.AttachmentStorageAccessKey) == "" || strings.TrimSpace(site.AttachmentStorageSecretKey) == "" {
		return 0, errors.New("attachment cloud storage configuration is incomplete")
	}
	cloud := models.SiteConfig{StorageProvider: site.AttachmentStorageProvider, StorageEndpoint: site.AttachmentStorageEndpoint, StorageRegion: site.AttachmentStorageRegion, StorageAccessKey: site.AttachmentStorageAccessKey, StorageSecretKey: site.AttachmentStorageSecretKey, StorageUsePathStyle: site.AttachmentStorageUsePathStyle}
	client, err := getS3Client(cloud)
	if err != nil {
		return 0, err
	}
	return downloadObject(ctx, client, site.AttachmentStorageBucket, key, dst)
}

func downloadObject(ctx context.Context, client objectGetter, bucket, key string, dst io.Writer) (int64, error) {
	if dst == nil {
		return 0, errors.New("download destination is nil")
	}
	body, err := openObject(ctx, client, bucket, key)
	if err != nil {
		return 0, err
	}
	defer body.Close()
	n, err := CopyContext(ctx, dst, body)
	if err != nil {
		return n, fmt.Errorf("read complete cloud object %q: %w", key, err)
	}
	return n, nil
}

func openObject(ctx context.Context, client objectGetter, bucket, key string) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	object, err := client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})
	if err != nil {
		var api smithy.APIError
		var response *smithyhttp.ResponseError
		if errors.As(err, &api) && (api.ErrorCode() == "NoSuchKey" || api.ErrorCode() == "NotFound") || errors.As(err, &response) && response.HTTPStatusCode() == 404 {
			return nil, fmt.Errorf("cloud object %q: %w", key, ErrMissingContent)
		}
		return nil, fmt.Errorf("download cloud object %q: %w", key, err)
	}
	if object == nil || object.Body == nil {
		return nil, fmt.Errorf("cloud object %q returned no body: %w", key, ErrMissingContent)
	}
	if aws.ToString(object.ContentRange) != "" {
		_ = object.Body.Close()
		return nil, errors.New("server returned partial object content")
	}
	reader := &completeObjectReader{body: object.Body, ctx: ctx, expected: object.ContentLength}
	reader.stop = context.AfterFunc(ctx, func() { _ = object.Body.Close() })
	return reader, nil
}

type completeObjectReader struct {
	body     io.ReadCloser
	ctx      context.Context
	expected *int64
	read     int64
	stop     func() bool
}

func (r *completeObjectReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := r.body.Read(p)
	r.read += int64(n)
	if canceled := r.ctx.Err(); canceled != nil {
		return n, canceled
	}
	if r.expected != nil && (r.read > *r.expected || err == io.EOF && r.read != *r.expected) {
		return n, io.ErrUnexpectedEOF
	}
	return n, err
}

func (r *completeObjectReader) Close() error {
	r.stop()
	return r.body.Close()
}

// CopyContext checks cancellation between reads and writes. Network callers must also close on cancel.
func CopyContext(ctx context.Context, dst io.Writer, src io.Reader) (int64, error) {
	buffer := make([]byte, 64*1024)
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		n, readErr := src.Read(buffer)
		if err := ctx.Err(); err != nil {
			return total, err
		}
		if n > 0 {
			written, err := dst.Write(buffer[:n])
			total += int64(written)
			if err != nil {
				return total, err
			}
			if written != n {
				return total, io.ErrShortWrite
			}
		}
		if readErr == io.EOF {
			return total, ctx.Err()
		}
		if readErr != nil {
			return total, readErr
		}
	}
}
