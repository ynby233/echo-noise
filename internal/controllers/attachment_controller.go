package controllers

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/gin-gonic/gin"
	"github.com/lin-snow/ech0/config"
	"github.com/lin-snow/ech0/internal/database"
	"github.com/lin-snow/ech0/internal/models"
)

type AttachmentInfo struct {
	Key        string       `json:"key"`
	Name       string       `json:"name"`
	URL        string       `json:"url"`
	Size       int64        `json:"size"`
	ModifiedAt time.Time    `json:"modified_at"`
	Belongs    []BelongItem `json:"belongs"`
}

type BelongItem struct {
	ID        uint      `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	Snippet   string    `json:"snippet"`
}

func escapeObjectKeyForURL(key string) string {
	s := strings.TrimLeft(key, "/")
	if s == "" {
		return ""
	}
	return strings.ReplaceAll(url.PathEscape(s), "%2F", "/")
}

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

func ListImageAttachments(c *gin.Context) {
	var siteCfg models.SiteConfig
	_ = database.DB.Table("site_configs").First(&siteCfg).Error
	if siteCfg.AttachmentStorageEnabled {
		list, err := listCloudAttachments(siteCfg, func(name string) bool {
			return isImageExt(name)
		})
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "data": []AttachmentInfo{}})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 1, "data": list})
		return
	}

	wd, _ := os.Getwd()
	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)
	sp := strings.TrimRight(config.Config.Upload.SavePath, "/")
	dir := pickDir([]string{
		sp,
		"./" + sp,
		filepath.Join(wd, sp),
		filepath.Join(exeDir, sp),
		"./data/images",
		filepath.Join(wd, "data/images"),
		filepath.Join(exeDir, "data/images"),
		"/data/images",
		"/app/data/images",
	}, "./data/images")
	entries, err := os.ReadDir(dir)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 1, "data": []AttachmentInfo{}})
		return
	}

	var messages []models.Message
	database.DB.Select("id", "content", "image_url", "created_at").Order("created_at DESC").Find(&messages)

	var list []AttachmentInfo
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		p := filepath.Join(dir, name)
		fi, err := os.Stat(p)
		if err != nil {
			continue
		}
		urlPath := "/api/images/" + url.PathEscape(name)
		belongs := findBelongs(messages, name, "/images/", "/api/images/")
		list = append(list, AttachmentInfo{Key: name, Name: name, URL: urlPath, Size: fi.Size(), ModifiedAt: fi.ModTime(), Belongs: belongs})
	}

	c.JSON(http.StatusOK, gin.H{"code": 1, "data": list})
}

func ListVideoAttachments(c *gin.Context) {
	var siteCfg models.SiteConfig
	_ = database.DB.Table("site_configs").First(&siteCfg).Error
	if siteCfg.AttachmentStorageEnabled {
		list, err := listCloudAttachments(siteCfg, func(name string) bool {
			return isVideoExt(name)
		})
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "data": []AttachmentInfo{}})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 1, "data": list})
		return
	}

	wd, _ := os.Getwd()
	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)
	dir := pickDir([]string{
		"./data/video",
		filepath.Join(wd, "data/video"),
		filepath.Join(exeDir, "data/video"),
		"/data/video",
		"/app/data/video",
	}, "./data/video")
	entries, err := os.ReadDir(dir)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 1, "data": []AttachmentInfo{}})
		return
	}

	var messages []models.Message
	database.DB.Select("id", "content", "image_url", "created_at").Order("created_at DESC").Find(&messages)

	var list []AttachmentInfo
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		p := filepath.Join(dir, name)
		fi, err := os.Stat(p)
		if err != nil {
			continue
		}
		urlPath := "/video/" + url.PathEscape(name)
		belongs := findBelongs(messages, name, "/video/", "/api/video/")
		list = append(list, AttachmentInfo{Key: name, Name: name, URL: urlPath, Size: fi.Size(), ModifiedAt: fi.ModTime(), Belongs: belongs})
	}

	c.JSON(http.StatusOK, gin.H{"code": 1, "data": list})
}

func findBelongs(messages []models.Message, name, p1, p2 string) []BelongItem {
	var out []BelongItem
	// 原始文件名匹配
	needle1 := p1 + name
	needle2 := p2 + name
	// URL 编码后的文件名匹配
	encodedName := url.PathEscape(name)
	needle3 := p1 + encodedName
	needle4 := p2 + encodedName

	for _, m := range messages {
		has := false
		if strings.Contains(m.Content, needle1) || strings.Contains(m.Content, needle2) {
			has = true
		}
		if !has {
			if strings.Contains(m.Content, needle3) || strings.Contains(m.Content, needle4) {
				has = true
			}
		}
		if !has {
			if strings.Contains(m.ImageURL, needle1) || strings.Contains(m.ImageURL, needle2) {
				has = true
			}
		}
		if !has {
			if strings.Contains(m.ImageURL, needle3) || strings.Contains(m.ImageURL, needle4) {
				has = true
			}
		}
		if has {
			snip := m.Content
			if len(snip) > 80 {
				snip = snip[:80]
			}
			out = append(out, BelongItem{ID: m.ID, CreatedAt: m.CreatedAt, Snippet: snip})
		}
	}
	return out
}

func DeleteImageAttachment(c *gin.Context) {
	name := c.Param("name")
	base := filepath.Base(name)

	var siteCfg models.SiteConfig
	_ = database.DB.Table("site_configs").First(&siteCfg).Error
	if siteCfg.AttachmentStorageEnabled {
		decoded, err := url.PathUnescape(name)
		if err != nil {
			decoded = name
		}
		if err := deleteCloudAttachment(siteCfg, decoded); err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "删除失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 1, "data": true})
		return
	}

	wd, _ := os.Getwd()
	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)
	sp := strings.TrimRight(config.Config.Upload.SavePath, "/")
	imgDir := pickDir([]string{
		sp,
		"./" + sp,
		filepath.Join(wd, sp),
		filepath.Join(exeDir, sp),
		"./data/images",
		filepath.Join(wd, "data/images"),
		filepath.Join(exeDir, "data/images"),
		"/data/images",
		"/app/data/images",
	}, "./data/images")
	p := filepath.Join(imgDir, base)
	if _, err := os.Stat(p); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "文件不存在"})
		return
	}
	if err := os.Remove(p); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "删除失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 1, "data": true})
}

func DeleteVideoAttachment(c *gin.Context) {
	name := c.Param("name")
	base := filepath.Base(name)

	var siteCfg models.SiteConfig
	_ = database.DB.Table("site_configs").First(&siteCfg).Error
	if siteCfg.AttachmentStorageEnabled {
		decoded, err := url.PathUnescape(name)
		if err != nil {
			decoded = name
		}
		if err := deleteCloudAttachment(siteCfg, decoded); err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "删除失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 1, "data": true})
		return
	}

	wd, _ := os.Getwd()
	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)
	vidDir := pickDir([]string{
		"./data/video",
		filepath.Join(wd, "data/video"),
		filepath.Join(exeDir, "data/video"),
		"/data/video",
		"/app/data/video",
	}, "./data/video")
	p := filepath.Join(vidDir, base)
	if _, err := os.Stat(p); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "文件不存在"})
		return
	}
	if err := os.Remove(p); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "删除失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 1, "data": true})
}

func pickDir(candidates []string, fallback string) string {
	for _, d := range candidates {
		if d == "" {
			continue
		}
		info, err := os.Stat(d)
		if err == nil && info.IsDir() {
			return d
		}
	}
	return fallback
}

func normalizePublicBaseURL(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
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
		return s
	}
	path := strings.TrimRight(u.Path, "/")
	if path == "/" {
		path = ""
	}
	return strings.TrimRight(u.Scheme+"://"+u.Host+path, "/")
}

func newAttachmentS3Client(cfg models.SiteConfig) (*s3.Client, string, string, error) {
	if strings.TrimSpace(cfg.AttachmentStorageBucket) == "" ||
		strings.TrimSpace(cfg.AttachmentStorageAccessKey) == "" ||
		strings.TrimSpace(cfg.AttachmentStorageSecretKey) == "" {
		return nil, "", "", errors.New("附件云存储配置不完整")
	}

	region := strings.TrimSpace(cfg.AttachmentStorageRegion)
	if cfg.AttachmentStorageProvider == "r2" {
		region = "auto"
	}
	if region == "" {
		region = "auto"
	}

	endpoint := strings.TrimSpace(cfg.AttachmentStorageEndpoint)
	if endpoint != "" {
		if u, err := url.Parse(endpoint); err == nil {
			base := u.Scheme + "://" + u.Host
			endpoint = strings.TrimRight(base, "/")
		}
	}

	r2Resolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
		if endpoint == "" {
			return aws.Endpoint{}, &aws.EndpointNotFoundError{}
		}
		return aws.Endpoint{
			URL:               endpoint,
			SigningRegion:     region,
			HostnameImmutable: true,
		}, nil
	})

	creds := credentials.NewStaticCredentialsProvider(cfg.AttachmentStorageAccessKey, cfg.AttachmentStorageSecretKey, "")
	awsConfig, err := awscfg.LoadDefaultConfig(context.Background(),
		awscfg.WithCredentialsProvider(creds),
		awscfg.WithEndpointResolverWithOptions(r2Resolver),
		awscfg.WithRegion(region),
	)
	if err != nil {
		return nil, "", "", err
	}

	client := s3.NewFromConfig(awsConfig, func(o *s3.Options) {
		o.UsePathStyle = cfg.AttachmentStorageUsePathStyle
	})
	return client, cfg.AttachmentStorageBucket, normalizePublicBaseURL(cfg.AttachmentStoragePublicBaseURL), nil
}

func isImageExt(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp":
		return true
	default:
		return false
	}
}

func isVideoExt(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".mp4", ".webm", ".mov", ".avi":
		return true
	default:
		return false
	}
}

func listCloudAttachments(siteCfg models.SiteConfig, keep func(name string) bool) ([]AttachmentInfo, error) {
	cli, bucket, publicBaseURL, err := newAttachmentS3Client(siteCfg)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(publicBaseURL) == "" {
		return []AttachmentInfo{}, nil
	}
	origin, prefix := splitPublicBaseURL(publicBaseURL)
	if strings.TrimSpace(origin) == "" {
		return []AttachmentInfo{}, nil
	}

	var messages []models.Message
	database.DB.Select("id", "content", "image_url", "created_at").Order("created_at DESC").Find(&messages)

	var out []AttachmentInfo
	var token *string
	for {
		resp, err := cli.ListObjectsV2(context.Background(), &s3.ListObjectsV2Input{
			Bucket:            aws.String(bucket),
			ContinuationToken: token,
			MaxKeys:           aws.Int32(1000),
		})
		if err != nil {
			return nil, err
		}
		for _, obj := range resp.Contents {
			key := aws.ToString(obj.Key)
			if key == "" {
				continue
			}
			cleanKey := strings.TrimLeft(key, "/")
			name := filepath.Base(cleanKey)
			if !keep(name) {
				continue
			}
			// 兼容历史对象：PublicBaseURL 可能带有 path 前缀（如 /note），但对象 key 未必包含该前缀。
			// 如果 key 不带 prefix，则在生成展示 URL 时补齐 prefix；但 Key 字段仍返回真实对象 key（用于删除）。
			keyForURL := cleanKey
			if prefix != "" && !strings.HasPrefix(cleanKey, prefix+"/") {
				keyForURL = prefix + "/" + cleanKey
			}
			urlPath := origin + "/" + escapeObjectKeyForURL(keyForURL)
			modAt := time.Time{}
			if obj.LastModified != nil {
				modAt = *obj.LastModified
			}
			belongs := findBelongsCloud(messages, cleanKey, origin, prefix)
			out = append(out, AttachmentInfo{
				Key:        cleanKey,
				Name:       name,
				URL:        urlPath,
				Size:       aws.ToInt64(obj.Size),
				ModifiedAt: modAt,
				Belongs:    belongs,
			})
		}
		if aws.ToBool(resp.IsTruncated) && resp.NextContinuationToken != nil && aws.ToString(resp.NextContinuationToken) != "" {
			token = resp.NextContinuationToken
			continue
		}
		break
	}
	return out, nil
}

func findBelongsCloud(messages []models.Message, key string, origin string, prefix string) []BelongItem {
	var out []BelongItem
	cleanKey := strings.TrimLeft(key, "/")
	url1 := origin + "/" + escapeObjectKeyForURL(cleanKey)
	url2 := ""
	if prefix != "" && !strings.HasPrefix(cleanKey, prefix+"/") {
		url2 = origin + "/" + escapeObjectKeyForURL(prefix+"/"+cleanKey)
	}
	needle3 := "/" + cleanKey
	needle4 := "/" + url.PathEscape(cleanKey)

	for _, m := range messages {
		has := false
		if strings.Contains(m.Content, url1) || (url2 != "" && strings.Contains(m.Content, url2)) ||
			strings.Contains(m.Content, needle3) || strings.Contains(m.Content, needle4) {
			has = true
		}
		if !has {
			if strings.Contains(m.ImageURL, url1) || (url2 != "" && strings.Contains(m.ImageURL, url2)) ||
				strings.Contains(m.ImageURL, needle3) || strings.Contains(m.ImageURL, needle4) {
				has = true
			}
		}
		if has {
			snip := m.Content
			if len(snip) > 80 {
				snip = snip[:80]
			}
			out = append(out, BelongItem{ID: m.ID, CreatedAt: m.CreatedAt, Snippet: snip})
		}
	}
	return out
}

func deleteCloudAttachment(siteCfg models.SiteConfig, key string) error {
	cli, bucket, _, err := newAttachmentS3Client(siteCfg)
	if err != nil {
		return err
	}
	cleanKey := strings.TrimLeft(key, "/")
	_, err = cli.DeleteObject(context.Background(), &s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(cleanKey),
	})
	return err
}
