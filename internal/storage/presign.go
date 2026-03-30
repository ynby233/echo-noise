package storage

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
	"github.com/rcy1314/echo-noise/internal/models"
)

func splitPublicBaseURL(raw string) (string, string) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", ""
	}
	s = strings.TrimRight(s, "/")
	if strings.HasPrefix(s, "//") {
		s = "https:" + s
	}
	parseStr := s
	if !strings.Contains(parseStr, "://") {
		parseStr = "https://" + strings.TrimLeft(parseStr, "/")
	}
	u, err := url.Parse(parseStr)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return s, ""
	}
	origin := strings.TrimRight(u.Scheme+"://"+u.Host, "/")
	prefix := strings.Trim(u.Path, "/")
	return origin, prefix
}

func prefixedKey(site models.SiteConfig, key string) string {
	_, prefix := splitPublicBaseURL(site.StoragePublicBaseURL)
	clean := strings.TrimLeft(strings.TrimSpace(key), "/")
	if clean == "" {
		return ""
	}
	if prefix == "" {
		return clean
	}
	if strings.HasPrefix(clean, prefix+"/") {
		return clean
	}
	return prefix + "/" + clean
}

func ResolveObjectKey(site models.SiteConfig, key string) string {
	return prefixedKey(site, key)
}

func getAWSConfigFromSite(cfg models.SiteConfig) (aws.Config, error) {
	cr := aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider(cfg.StorageAccessKey, cfg.StorageSecretKey, ""))
	region := cfg.StorageRegion
	if cfg.StorageProvider == "r2" {
		region = "auto"
	}
	if strings.TrimSpace(region) == "" {
		region = "auto"
	}
	endpoint := strings.TrimSpace(cfg.StorageEndpoint)
	if endpoint != "" {
		if u, err := url.Parse(endpoint); err == nil {
			base := u.Scheme + "://" + u.Host
			endpoint = strings.TrimRight(base, "/")
		}
	}
	base := awscfg.WithRegion(region)
	base2 := awscfg.WithCredentialsProvider(cr)
	return awscfg.LoadDefaultConfig(context.Background(), base, base2, awscfg.WithEndpointResolverWithOptions(
		aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
			if endpoint != "" {
				return aws.Endpoint{
					URL:               endpoint,
					SigningRegion:     region,
					HostnameImmutable: true,
				}, nil
			}
			return aws.Endpoint{}, &aws.EndpointNotFoundError{}
		}),
	))
}

func getS3Client(site models.SiteConfig) (*s3.Client, error) {
	awsCfg, err := getAWSConfigFromSite(site)
	if err != nil {
		return nil, err
	}
	cli := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if site.StorageUsePathStyle {
			o.UsePathStyle = true
		}
	})
	return cli, nil
}

func PresignUpload(site models.SiteConfig, bucket, key string, expires time.Duration, contentType string) (string, error) {
	cli, err := getS3Client(site)
	if err != nil {
		return "", err
	}
	key = prefixedKey(site, key)
	presigner := s3.NewPresignClient(cli, func(po *s3.PresignOptions) {
		po.Expires = expires
	})
	input := &s3.PutObjectInput{
		Bucket:      &bucket,
		Key:         &key,
		ContentType: &contentType,
	}
	req, err := presigner.PresignPutObject(context.Background(), input)
	if err != nil {
		return "", err
	}
	return req.URL, nil
}

type ObjectMeta struct {
	ETag         string
	LastModified *time.Time
}

func HeadObject(site models.SiteConfig, bucket, key string) (*ObjectMeta, error) {
	cli, err := getS3Client(site)
	if err != nil {
		return nil, err
	}
	key = prefixedKey(site, key)
	out, err := cli.HeadObject(context.Background(), &s3.HeadObjectInput{Bucket: &bucket, Key: &key})
	if err != nil {
		var apiErr smithy.APIError
		if ok := errors.As(err, &apiErr); ok {
			code := strings.ToLower(strings.TrimSpace(apiErr.ErrorCode()))
			// S3/R2 对不存在对象的错误码可能不同，统一当作“无对象”处理
			if code == "notfound" || code == "nosuchkey" || code == "notfoundexception" {
				return nil, nil
			}
		}
		return nil, err
	}
	meta := &ObjectMeta{}
	if out.ETag != nil {
		meta.ETag = strings.Trim(*out.ETag, "\"")
	}
	if out.LastModified != nil {
		t := *out.LastModified
		meta.LastModified = &t
	}
	return meta, nil
}

func PresignDownload(site models.SiteConfig, bucket, key string, expires time.Duration) (string, error) {
	cli, err := getS3Client(site)
	if err != nil {
		return "", err
	}
	key = prefixedKey(site, key)
	presigner := s3.NewPresignClient(cli, func(po *s3.PresignOptions) {
		po.Expires = expires
	})
	input := &s3.GetObjectInput{
		Bucket: &bucket,
		Key:    &key,
	}
	req, err := presigner.PresignGetObject(context.Background(), input)
	if err != nil {
		return "", err
	}
	return req.URL, nil
}
