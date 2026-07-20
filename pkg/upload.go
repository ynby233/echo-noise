package pkg

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/nfnt/resize"
	"github.com/rcy1314/echo-noise/config"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
)

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

func normalizeUploadExt(ext, fallback string) string {
	ext = strings.ToLower(strings.TrimSpace(ext))
	if ext == "" {
		return fallback
	}
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	for _, r := range ext[1:] {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			continue
		}
		return fallback
	}
	return ext
}

func audioUploadExt(filename, contentType string) string {
	ext := normalizeUploadExt(filepath.Ext(filename), ".webm")
	switch ext {
	case ".webm", ".ogg", ".mp3", ".m4a", ".wav", ".flac":
		return ext
	}

	baseType := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	switch baseType {
	case "audio/ogg":
		return ".ogg"
	case "audio/mpeg":
		return ".mp3"
	case "audio/mp4":
		return ".m4a"
	case "audio/wav", "audio/x-wav":
		return ".wav"
	case "audio/flac", "audio/x-flac":
		return ".flac"
	default:
		return ".webm"
	}
}

const attachmentSHA256MetadataKey = "sha256"

func attachmentContentHashFromBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func attachmentContentHashFromReader(content io.Reader) (string, error) {
	h := sha256.New()
	if _, err := io.Copy(h, content); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func attachmentContentHashFromReadSeeker(content io.ReadSeeker) (string, error) {
	if _, err := content.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	hash, err := attachmentContentHashFromReader(content)
	if err != nil {
		return "", err
	}
	if _, err := content.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	return hash, nil
}

func attachmentContentHashFromPath(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	return attachmentContentHashFromReader(f)
}

func safeAttachmentFileName(originalName, ext, fallbackStem string) string {
	clean := strings.TrimSpace(originalName)
	clean = strings.ReplaceAll(clean, "\\", "/")
	clean = filepath.Base(clean)
	clean = strings.Map(func(r rune) rune {
		if r == 0 || r < 32 || r == 127 || r == '/' || r == '\\' {
			return -1
		}
		return r
	}, clean)
	clean = strings.TrimSpace(clean)

	if strings.TrimSpace(fallbackStem) == "" {
		fallbackStem = "attachment"
	}
	normalizedExt := normalizeUploadExt(ext, ".bin")
	if clean == "" || clean == "." {
		return fallbackStem + normalizedExt
	}

	currentExt := filepath.Ext(clean)
	stem := strings.TrimSpace(strings.TrimSuffix(clean, currentExt))
	if stem == "" || stem == "." {
		stem = fallbackStem
	}
	return stem + normalizedExt
}

func sequencedAttachmentFileName(name string, index int) string {
	if index <= 0 {
		return name
	}
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	return fmt.Sprintf("%s(%d)%s", stem, index, ext)
}

func localFileMatchesContentHash(path, contentHash string) bool {
	if contentHash == "" {
		return false
	}
	existingHash, err := attachmentContentHashFromPath(path)
	return err == nil && existingHash == contentHash
}

func localAttachmentFileNameForContent(dir, preferredName, contentHash string) (string, bool, error) {
	if contentHash != "" {
		entries, err := os.ReadDir(dir)
		if err != nil && !os.IsNotExist(err) {
			return "", false, err
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			if localFileMatchesContentHash(filepath.Join(dir, entry.Name()), contentHash) {
				return entry.Name(), true, nil
			}
		}
	}

	for i := 0; ; i++ {
		candidate := sequencedAttachmentFileName(preferredName, i)
		p := filepath.Join(dir, candidate)
		info, err := os.Stat(p)
		if os.IsNotExist(err) {
			return candidate, false, nil
		}
		if err != nil {
			return "", false, err
		}
		if info.IsDir() {
			continue
		}
		if localFileMatchesContentHash(p, contentHash) {
			return candidate, true, nil
		}
	}
}

func isolateRestrictedLocalAttachmentReuse(kind, preferredName, existingName string, existed bool) (string, bool, error) {
	if !existed {
		return existingName, false, nil
	}
	db, err := database.GetDB()
	if err != nil {
		return "", false, err
	}
	var grantCount int64
	if err := db.Model(&models.LocalAttachmentGrant{}).
		Where("kind = ? AND name = ?", kind, existingName).
		Count(&grantCount).Error; err != nil {
		return "", false, err
	}
	if grantCount == 0 {
		return existingName, true, nil
	}
	var publicGrantCount int64
	if err := db.Model(&models.LocalAttachmentGrant{}).
		Where("kind = ? AND name = ? AND visibility = ?", kind, existingName, "public").
		Count(&publicGrantCount).Error; err != nil {
		return "", false, err
	}
	if publicGrantCount > 0 {
		return existingName, true, nil
	}
	extension := normalizeUploadExt(filepath.Ext(preferredName), filepath.Ext(existingName))
	return uuid.NewString() + extension, false, nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// UploadImage 上传图片并返回图片的URL
func UploadImage(c *gin.Context, allowedExtensions []string, siteConfig *models.SiteConfig) (string, error) {
	// 获取上传的文件
	file, err := c.FormFile("image")
	if err != nil {
		return "", errors.New(models.NotUploadImageErrorMessage)
	}

	// 检查图片类型是否合法
	contentType := file.Header.Get("Content-Type")
	if !isAllowedImageType(contentType, allowedExtensions) {
		return "", errors.New(models.NotSupportedImageTypeMessage)
	}

	// 检查文件大小
	if file.Size > 5*1024*1024*1024 {
		return "", errors.New(models.ImageSizeLimitErrorMessage + strconv.Itoa(5*1024) + "MB")
	}

	// 打开文件进行处理
	srcFile, err := file.Open()
	if err != nil {
		return "", err
	}
	defer srcFile.Close()

	// 图片处理（自动缩放）
	var fileData []byte
	var isResized bool

	// 尝试解码图片
	img, format, err := image.Decode(srcFile)
	if err == nil {
		// 如果宽度超过 1920，进行缩放
		bounds := img.Bounds()
		if bounds.Dx() > 1920 {
			// 使用 Lanczos3 算法进行高质量缩放
			m := resize.Resize(1920, 0, img, resize.Lanczos3)
			buf := new(bytes.Buffer)
			if format == "png" {
				err = png.Encode(buf, m)
			} else {
				// 默认使用 jpeg
				err = jpeg.Encode(buf, m, nil)
			}
			if err == nil {
				fileData = buf.Bytes()
				isResized = true
			}
		}
	}

	// 如果没有缩放或缩放失败，读取原始内容
	if !isResized {
		srcFile.Seek(0, 0)
		buf := new(bytes.Buffer)
		buf.ReadFrom(srcFile)
		fileData = buf.Bytes()
	}

	// 获取原始文件名和扩展名
	ext := normalizeUploadExt(filepath.Ext(file.Filename), ".img")

	// 检查是否启用了压缩
	if siteConfig != nil && siteConfig.EnableCompression {
		if compressedData, err := CompressImageWithFFmpeg(fileData, ext); err == nil {
			fileData = compressedData
		} else {
			// 压缩失败不影响主流程，回退原图上传
			_ = err
		}
	}

	contentHash := attachmentContentHashFromBytes(fileData)
	preferredFileName := safeAttachmentFileName(file.Filename, ext, "image")

	// 检查是否启用了附件云存储
	if siteConfig != nil && siteConfig.AttachmentStorageEnabled {
		return UploadAttachmentToCloud(siteConfig, preferredFileName, bytes.NewReader(fileData), contentType, contentHash)
	}

	// 本地存储逻辑
	// 创建存储图片的目录（如果没有的话）
	if err := createImageDirIfNotExist(config.Config.Upload.SavePath); err != nil {
		return "", err
	}

	newFileName, existed, err := localAttachmentFileNameForContent(config.Config.Upload.SavePath, preferredFileName, contentHash)
	if err != nil {
		return "", err
	}
	newFileName, existed, err = isolateRestrictedLocalAttachmentReuse("image", preferredFileName, newFileName, existed)
	if err != nil {
		return "", err
	}

	// 保存文件到指定目录；相同内容已经存在时直接复用。
	savePath := filepath.Join(config.Config.Upload.SavePath, newFileName)
	if !existed && !fileExists(savePath) {
		if err := os.WriteFile(savePath, fileData, 0644); err != nil {
			return "", errors.New(models.ImageUploadErrorMessage)
		}
	}

	// 返回图片的 URL
	imageURL := fmt.Sprintf("/api/images/%s", url.PathEscape(newFileName))
	return imageURL, nil
}

// UploadAttachmentToCloud 上传附件到云存储
func UploadAttachmentToCloud(cfg *models.SiteConfig, preferredFileName string, content io.ReadSeeker, contentType string, contentHash string) (string, error) {
	if cfg.AttachmentStorageBucket == "" || cfg.AttachmentStorageAccessKey == "" || cfg.AttachmentStorageSecretKey == "" || cfg.AttachmentStorageEndpoint == "" {
		return "", errors.New("附件云存储配置不完整")
	}
	if contentHash == "" {
		var err error
		contentHash, err = attachmentContentHashFromReadSeeker(content)
		if err != nil {
			return "", err
		}
	}

	// 配置 AWS SDK
	r2Resolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
		return aws.Endpoint{
			URL: cfg.AttachmentStorageEndpoint,
		}, nil
	})

	creds := credentials.NewStaticCredentialsProvider(cfg.AttachmentStorageAccessKey, cfg.AttachmentStorageSecretKey, "")

	// 加载配置
	awsConfig, err := awscfg.LoadDefaultConfig(context.TODO(),
		awscfg.WithCredentialsProvider(creds),
		awscfg.WithEndpointResolverWithOptions(r2Resolver),
		awscfg.WithRegion(cfg.AttachmentStorageRegion),
	)
	if err != nil {
		return "", fmt.Errorf("加载云存储配置失败: %v", err)
	}

	client := s3.NewFromConfig(awsConfig, func(o *s3.Options) {
		o.UsePathStyle = cfg.AttachmentStorageUsePathStyle
	})

	db, dbErr := database.GetDB()
	if dbErr != nil {
		return "", dbErr
	}
	if contentHash != "" {
		var existing models.CloudAttachmentObject
		if err := db.Where("content_hash = ?", contentHash).Order("id ASC").First(&existing).Error; err == nil {
			exists, sameContent, verifyErr := cloudObjectMatchesContentHash(context.TODO(), client, cfg.AttachmentStorageBucket, existing.ObjectKey, contentHash, false)
			if verifyErr != nil {
				return "", verifyErr
			}
			var grantCount int64
			var publicGrantCount int64
			if err := db.Model(&models.LocalAttachmentGrant{}).Where("kind = ? AND name = ?", "cloud", existing.PublicID).Count(&grantCount).Error; err != nil {
				return "", err
			}
			if grantCount > 0 {
				if err := db.Model(&models.LocalAttachmentGrant{}).
					Where("kind = ? AND name = ? AND visibility = ?", "cloud", existing.PublicID, "public").
					Count(&publicGrantCount).Error; err != nil {
					return "", err
				}
			}
			if exists && sameContent && (grantCount == 0 || publicGrantCount > 0) {
				return fmt.Sprintf("/api/cloud-attachments/%s/%s", existing.PublicID, url.PathEscape(existing.OriginalName)), nil
			}
			if !exists {
				if err := db.Transaction(func(tx *gorm.DB) error {
					if err := tx.Where("kind = ? AND name = ?", "cloud", existing.PublicID).Delete(&models.LocalAttachmentGrant{}).Error; err != nil {
						return err
					}
					return tx.Delete(&existing).Error
				}); err != nil {
					return "", err
				}
			}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return "", err
		}
	}

	var contentLength int64
	if _, err := content.Seek(0, io.SeekStart); err == nil {
		if n, err := content.Seek(0, io.SeekEnd); err == nil {
			contentLength = n
		}
		content.Seek(0, io.SeekStart)
	}

	_, prefix := splitPublicBaseURL(cfg.AttachmentStoragePublicBaseURL)
	storageID := uuid.NewString()
	publicID := uuid.NewString()
	key := storageID + "/" + strings.TrimLeft(preferredFileName, "/")
	if prefix != "" {
		key = prefix + "/" + key
	}

	if _, err := content.Seek(0, io.SeekStart); err != nil {
		return "", err
	}

	// 上传文件
	_, err = client.PutObject(context.TODO(), &s3.PutObjectInput{
		Bucket:      aws.String(cfg.AttachmentStorageBucket),
		Key:         aws.String(key),
		Body:        content,
		ContentType: aws.String(contentType),
		Metadata: map[string]string{
			attachmentSHA256MetadataKey: contentHash,
		},
		ContentLength: func() *int64 {
			if contentLength > 0 {
				return aws.Int64(contentLength)
			}
			return nil
		}(),
	})
	if err != nil {
		return "", fmt.Errorf("上传到云存储失败: %v", err)
	}

	record := models.CloudAttachmentObject{
		PublicID:     publicID,
		ObjectKey:    key,
		OriginalName: preferredFileName,
		ContentType:  contentType,
		ContentHash:  contentHash,
	}
	if dbErr := db.Create(&record).Error; dbErr != nil {
		_, _ = client.DeleteObject(context.TODO(), &s3.DeleteObjectInput{Bucket: aws.String(cfg.AttachmentStorageBucket), Key: aws.String(key)})
		return "", dbErr
	}
	return fmt.Sprintf("/api/cloud-attachments/%s/%s", publicID, url.PathEscape(preferredFileName)), nil
}

func findCloudAttachmentByContentHash(ctx context.Context, client *s3.Client, bucket, contentHash string) (string, bool, error) {
	if contentHash == "" {
		return "", false, nil
	}
	var token *string
	for {
		resp, err := client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket:            aws.String(bucket),
			ContinuationToken: token,
			MaxKeys:           aws.Int32(1000),
		})
		if err != nil {
			return "", false, err
		}
		for _, obj := range resp.Contents {
			key := strings.TrimLeft(aws.ToString(obj.Key), "/")
			if key == "" {
				continue
			}
			_, sameContent, err := cloudObjectMatchesContentHash(ctx, client, bucket, key, contentHash, false)
			if err != nil {
				return "", false, err
			}
			if sameContent {
				return key, true, nil
			}
		}
		if aws.ToBool(resp.IsTruncated) && resp.NextContinuationToken != nil && aws.ToString(resp.NextContinuationToken) != "" {
			token = resp.NextContinuationToken
			continue
		}
		break
	}
	return "", false, nil
}

func cloudObjectMatchesContentHash(ctx context.Context, client *s3.Client, bucket, key, contentHash string, allowBodyHash bool) (bool, bool, error) {
	head, err := client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		if isS3NotFound(err) {
			return false, false, nil
		}
		return false, false, err
	}
	if contentHash != "" {
		legacyName := filepath.Base(strings.TrimLeft(key, "/"))
		if strings.TrimSuffix(legacyName, filepath.Ext(legacyName)) == contentHash {
			return true, true, nil
		}
		for k, v := range head.Metadata {
			if strings.EqualFold(k, attachmentSHA256MetadataKey) && strings.EqualFold(strings.TrimSpace(v), contentHash) {
				return true, true, nil
			}
		}
	}
	if allowBodyHash && contentHash != "" {
		obj, err := client.GetObject(ctx, &s3.GetObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(key),
		})
		if err != nil {
			if isS3NotFound(err) {
				return false, false, nil
			}
			return true, false, err
		}
		defer obj.Body.Close()
		existingHash, err := attachmentContentHashFromReader(obj.Body)
		if err != nil {
			return true, false, err
		}
		return true, existingHash == contentHash, nil
	}
	return true, false, nil
}

func isS3NotFound(err error) bool {
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		code := strings.ToLower(apiErr.ErrorCode())
		return code == "notfound" || code == "nosuchkey" || code == "404"
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "notfound") || strings.Contains(msg, "not found") || strings.Contains(msg, "nosuchkey") || strings.Contains(msg, "status code: 404")
}

// 检查文件类型是否合法
func isAllowedType(contentType string, allowedTypes []string) bool {
	contentType = strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	if contentType == "" {
		return false
	}
	for _, allowed := range allowedTypes {
		allowed = strings.ToLower(strings.TrimSpace(allowed))
		if allowed == contentType {
			return true
		}
		if strings.HasSuffix(allowed, "/*") && strings.HasPrefix(contentType, strings.TrimSuffix(allowed, "*")) {
			return true
		}
	}
	return false
}

func isAllowedImageType(contentType string, allowedTypes []string) bool {
	contentType = strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	return isAllowedType(contentType, allowedTypes) || strings.HasPrefix(contentType, "image/")
}

// 创建存储图片的目录
func createImageDirIfNotExist(imagePath string) error {
	if _, err := os.Stat(imagePath); os.IsNotExist(err) {
		if err := os.MkdirAll(imagePath, os.ModePerm); err != nil {
			return err
		}
	}
	return nil
}

// UploadVideo 上传视频并返回视频的URL
func UploadVideo(c *gin.Context, allowedExtensions []string, siteConfig *models.SiteConfig) (string, error) {
	// 获取上传的文件
	file, err := c.FormFile("video")
	if err != nil {
		return "", errors.New("未上传视频文件")
	}

	// 检查视频类型是否合法
	contentType := file.Header.Get("Content-Type")
	if !isAllowedType(contentType, allowedExtensions) {
		return "", errors.New("不支持的视频类型")
	}

	// 检查文件大小（200MB）
	// 允许更大的视频上传（默认 1GiB）。如需进一步调整，可在后续引入配置项。
	if file.Size > 5*1024*1024*1024 {
		return "", errors.New("视频大小不能超过5GB")
	}

	// 读取文件内容
	srcFile, err := file.Open()
	if err != nil {
		return "", err
	}
	defer srcFile.Close()

	ext := normalizeUploadExt(filepath.Ext(file.Filename), ".mp4")

	var uploadPath string
	var cleanupPaths []string
	if siteConfig != nil && siteConfig.EnableCompression {
		tmpInput, err := os.CreateTemp("", "upload-vid-*"+ext)
		if err != nil {
			return "", err
		}
		tmpInputPath := tmpInput.Name()
		cleanupPaths = append(cleanupPaths, tmpInputPath)
		if _, err := io.Copy(tmpInput, srcFile); err != nil {
			tmpInput.Close()
			for _, p := range cleanupPaths {
				_ = os.Remove(p)
			}
			return "", err
		}
		if err := tmpInput.Close(); err != nil {
			for _, p := range cleanupPaths {
				_ = os.Remove(p)
			}
			return "", err
		}

		uploadPath = tmpInputPath
		compressedPath, err := CompressVideoWithFFmpeg(tmpInputPath)
		if err == nil && compressedPath != "" {
			uploadPath = compressedPath
			cleanupPaths = append(cleanupPaths, compressedPath)
			ext = filepath.Ext(compressedPath)
			contentType = "video/mp4"
		} else {
			if err != nil {
				// 压缩失败不影响主流程，回退原视频上传
				_ = err
			}
		}
	}
	for _, p := range cleanupPaths {
		defer os.Remove(p)
	}

	var contentHash string
	if uploadPath != "" {
		contentHash, err = attachmentContentHashFromPath(uploadPath)
	} else if seeker, ok := srcFile.(io.ReadSeeker); ok {
		contentHash, err = attachmentContentHashFromReadSeeker(seeker)
	} else {
		return "", errors.New("无法读取上传文件")
	}
	if err != nil {
		return "", err
	}
	preferredFileName := safeAttachmentFileName(file.Filename, ext, "video")

	// 检查是否启用了附件云存储
	if siteConfig != nil && siteConfig.AttachmentStorageEnabled {
		if uploadPath != "" {
			f, err := os.Open(uploadPath)
			if err != nil {
				return "", err
			}
			defer f.Close()
			if _, err := f.Seek(0, io.SeekStart); err != nil {
				return "", err
			}
			return UploadAttachmentToCloud(siteConfig, preferredFileName, f, contentType, contentHash)
		}
		if seeker, ok := srcFile.(io.ReadSeeker); ok {
			if _, err := seeker.Seek(0, io.SeekStart); err != nil {
				return "", err
			}
			return UploadAttachmentToCloud(siteConfig, preferredFileName, seeker, contentType, contentHash)
		}
		return "", errors.New("无法读取上传文件")
	}

	// 本地存储逻辑
	// 创建存储视频的目录
	// 本地存储逻辑
	// 确定视频存储路径，优先级：/data/video > /app/data/video > ./data/video
	videoPath := "./data/video"
	if _, err := os.Stat("/data"); err == nil {
		videoPath = "/data/video"
	} else if _, err := os.Stat("/app/data"); err == nil {
		videoPath = "/app/data/video"
	}

	if err := createImageDirIfNotExist(videoPath); err != nil {
		return "", err
	}

	newFileName, existed, err := localAttachmentFileNameForContent(videoPath, preferredFileName, contentHash)
	if err != nil {
		return "", err
	}
	newFileName, existed, err = isolateRestrictedLocalAttachmentReuse("video", preferredFileName, newFileName, existed)
	if err != nil {
		return "", err
	}

	savePath := filepath.Join(videoPath, newFileName)
	if !existed && !fileExists(savePath) {
		if uploadPath != "" {
			in, err := os.Open(uploadPath)
			if err != nil {
				return "", err
			}
			defer in.Close()
			out, err := os.Create(savePath)
			if err != nil {
				return "", err
			}
			if _, err := io.Copy(out, in); err != nil {
				out.Close()
				_ = os.Remove(savePath)
				return "", errors.New("视频上传失败")
			}
			if err := out.Close(); err != nil {
				_ = os.Remove(savePath)
				return "", errors.New("视频上传失败")
			}
		} else {
			out, err := os.Create(savePath)
			if err != nil {
				return "", errors.New("视频上传失败")
			}
			if _, err := io.Copy(out, srcFile); err != nil {
				out.Close()
				_ = os.Remove(savePath)
				return "", errors.New("视频上传失败")
			}
			if err := out.Close(); err != nil {
				_ = os.Remove(savePath)
				return "", errors.New("视频上传失败")
			}
		}
	}

	videoURL := fmt.Sprintf("/api/video/%s", url.PathEscape(newFileName))
	return videoURL, nil
}

// UploadAudio 上传音频并返回音频的URL
func UploadAudio(c *gin.Context, allowedMimeTypes []string, siteConfig *models.SiteConfig) (string, error) {
	file, err := c.FormFile("audio")
	if err != nil {
		return "", errors.New("未上传音频文件")
	}

	contentType := file.Header.Get("Content-Type")
	if !isAllowedType(contentType, allowedMimeTypes) {
		return "", errors.New("不支持的音频类型")
	}

	if file.Size > 5*1024*1024*1024 {
		return "", errors.New("音频大小不能超过5GB")
	}

	srcFile, err := file.Open()
	if err != nil {
		return "", err
	}
	defer srcFile.Close()

	ext := audioUploadExt(file.Filename, contentType)
	contentHash, err := attachmentContentHashFromReadSeeker(srcFile)
	if err != nil {
		return "", err
	}
	preferredFileName := safeAttachmentFileName(file.Filename, ext, "audio")

	if siteConfig != nil && siteConfig.AttachmentStorageEnabled {
		if _, err := srcFile.Seek(0, io.SeekStart); err != nil {
			return "", err
		}
		return UploadAttachmentToCloud(siteConfig, preferredFileName, srcFile, contentType, contentHash)
	}

	audioPath := "./data/audio"
	if _, err := os.Stat("/data"); err == nil {
		audioPath = "/data/audio"
	} else if _, err := os.Stat("/app/data"); err == nil {
		audioPath = "/app/data/audio"
	}

	if err := createImageDirIfNotExist(audioPath); err != nil {
		return "", err
	}

	newFileName, existed, err := localAttachmentFileNameForContent(audioPath, preferredFileName, contentHash)
	if err != nil {
		return "", err
	}
	newFileName, existed, err = isolateRestrictedLocalAttachmentReuse("audio", preferredFileName, newFileName, existed)
	if err != nil {
		return "", err
	}

	savePath := filepath.Join(audioPath, newFileName)
	if !existed && !fileExists(savePath) {
		if _, err := srcFile.Seek(0, io.SeekStart); err != nil {
			return "", err
		}
		out, err := os.Create(savePath)
		if err != nil {
			return "", errors.New("音频上传失败")
		}
		if _, err := io.Copy(out, srcFile); err != nil {
			out.Close()
			_ = os.Remove(savePath)
			return "", errors.New("音频上传失败")
		}
		if err := out.Close(); err != nil {
			_ = os.Remove(savePath)
			return "", errors.New("音频上传失败")
		}
	}

	audioURL := fmt.Sprintf("/api/audio/%s", url.PathEscape(newFileName))
	return audioURL, nil
}

func localAttachmentStorageDir(name string) string {
	dir := filepath.Join(".", "data", name)
	if _, err := os.Stat("/data"); err == nil {
		dir = filepath.Join("/data", name)
	} else if _, err := os.Stat("/app/data"); err == nil {
		dir = filepath.Join("/app/data", name)
	}
	return dir
}

// UploadFileAttachment uploads non-media attachments and returns the public URL.
func UploadFileAttachment(c *gin.Context, siteConfig *models.SiteConfig) (string, error) {
	file, err := c.FormFile("file")
	if err != nil {
		return "", errors.New("未上传附件文件")
	}
	if file.Size > 5*1024*1024*1024 {
		return "", errors.New("附件大小不能超过5GB")
	}

	srcFile, err := file.Open()
	if err != nil {
		return "", err
	}
	defer srcFile.Close()

	seeker, ok := srcFile.(io.ReadSeeker)
	if !ok {
		return "", errors.New("无法读取上传文件")
	}

	contentType := strings.TrimSpace(file.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	ext := normalizeUploadExt(filepath.Ext(file.Filename), ".bin")
	contentHash, err := attachmentContentHashFromReadSeeker(seeker)
	if err != nil {
		return "", err
	}
	preferredFileName := safeAttachmentFileName(file.Filename, ext, "attachment")

	if siteConfig != nil && siteConfig.AttachmentStorageEnabled {
		if _, err := seeker.Seek(0, io.SeekStart); err != nil {
			return "", err
		}
		return UploadAttachmentToCloud(siteConfig, preferredFileName, seeker, contentType, contentHash)
	}

	attachmentPath := localAttachmentStorageDir("attachments")
	if err := createImageDirIfNotExist(attachmentPath); err != nil {
		return "", err
	}

	newFileName, existed, err := localAttachmentFileNameForContent(attachmentPath, preferredFileName, contentHash)
	if err != nil {
		return "", err
	}
	newFileName, existed, err = isolateRestrictedLocalAttachmentReuse("file", preferredFileName, newFileName, existed)
	if err != nil {
		return "", err
	}

	savePath := filepath.Join(attachmentPath, newFileName)
	if !existed && !fileExists(savePath) {
		if _, err := seeker.Seek(0, io.SeekStart); err != nil {
			return "", err
		}
		out, err := os.Create(savePath)
		if err != nil {
			return "", errors.New("附件上传失败")
		}
		if _, err := io.Copy(out, seeker); err != nil {
			out.Close()
			_ = os.Remove(savePath)
			return "", errors.New("附件上传失败")
		}
		if err := out.Close(); err != nil {
			_ = os.Remove(savePath)
			return "", errors.New("附件上传失败")
		}
	}

	return fmt.Sprintf("/api/files/%s", url.PathEscape(newFileName)), nil
}
