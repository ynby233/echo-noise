package attachments

import (
	"context"
	"errors"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	smithyhttp "github.com/aws/smithy-go/transport/http"
)

type S3Store struct {
	client *s3.Client
	bucket string
	prefix string
}

func NewS3Store(client *s3.Client, bucket, prefix string) *S3Store {
	return &S3Store{client: client, bucket: strings.TrimSpace(bucket), prefix: strings.Trim(strings.TrimSpace(prefix), "/")}
}

func (s *S3Store) ID() string { return "cloud" }

func (s *S3Store) Key(contentHash string) string {
	key := "attachment-blobs/sha256/" + contentHash[:2] + "/" + contentHash
	if s.prefix != "" {
		key = s.prefix + "/" + key
	}
	return key
}

func (s *S3Store) Exists(ctx context.Context, key string) (bool, error) {
	_, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)})
	if err == nil {
		return true, nil
	}
	var responseErr *smithyhttp.ResponseError
	if errors.As(err, &responseErr) && responseErr.HTTPStatusCode() == 404 {
		return false, nil
	}
	return false, err
}

func (s *S3Store) Put(ctx context.Context, key string, content io.ReadSeeker, contentType, contentHash string, size int64) error {
	input := &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
		Body:   content,
		Metadata: map[string]string{
			"sha256": contentHash,
		},
	}
	if contentType != "" {
		input.ContentType = aws.String(contentType)
	}
	if size > 0 {
		input.ContentLength = aws.Int64(size)
	}
	_, err := s.client.PutObject(ctx, input)
	return err
}

func (s *S3Store) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)})
	return err
}
