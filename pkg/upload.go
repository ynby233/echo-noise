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
	"github.com/gin-gonic/gin"
	"github.com/nfnt/resize"
	attachmentregistry "github.com/rcy1314/echo-noise/internal/attachments"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
)

func storeAttachmentReference(ctx context.Context, store attachmentregistry.BlobStore, kind string, ownerUserID uint, originalName, contentType, contentHash string, size int64, content io.ReadSeeker) (string, error) {
	db, err := database.GetDB()
	if err != nil {
		return "", err
	}
	reference, err := attachmentregistry.NewRegistry(db).Create(ctx, store, attachmentregistry.CreateInput{
		Kind:         kind,
		OwnerUserID:  ownerUserID,
		OriginalName: originalName,
		ContentType:  contentType,
		ContentHash:  contentHash,
		Size:         size,
	}, content)
	if err != nil {
		return "", err
	}
	return attachmentregistry.ReferenceURL(reference, store.ID()), nil
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
		return UploadAttachmentToCloud(siteConfig, "image", c.GetUint("user_id"), preferredFileName, bytes.NewReader(fileData), contentType, contentHash)
	}
	ownerUserID := c.GetUint("user_id")
	if ownerUserID == 0 {
		return "", errors.New("未登录或登录已过期")
	}
	return storeAttachmentReference(c.Request.Context(), attachmentregistry.NewLocalStore(attachmentregistry.DefaultLocalRoot()), "image", ownerUserID, preferredFileName, contentType, contentHash, int64(len(fileData)), bytes.NewReader(fileData))
}

// UploadAttachmentToCloud 上传附件到云存储
func UploadAttachmentToCloud(cfg *models.SiteConfig, kind string, ownerUserID uint, preferredFileName string, content io.ReadSeeker, contentType string, contentHash string) (string, error) {
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
	if ownerUserID == 0 {
		return "", errors.New("未登录或登录已过期")
	}
	var referenceSize int64
	if _, err := content.Seek(0, io.SeekStart); err == nil {
		if n, err := content.Seek(0, io.SeekEnd); err == nil {
			referenceSize = n
		}
		_, _ = content.Seek(0, io.SeekStart)
	}
	_, blobPrefix := splitPublicBaseURL(cfg.AttachmentStoragePublicBaseURL)
	store := attachmentregistry.NewS3Store(client, cfg.AttachmentStorageBucket, blobPrefix)
	return storeAttachmentReference(context.Background(), store, kind, ownerUserID, preferredFileName, contentType, contentHash, referenceSize, content)
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
			return UploadAttachmentToCloud(siteConfig, "video", c.GetUint("user_id"), preferredFileName, f, contentType, contentHash)
		}
		if seeker, ok := srcFile.(io.ReadSeeker); ok {
			if _, err := seeker.Seek(0, io.SeekStart); err != nil {
				return "", err
			}
			return UploadAttachmentToCloud(siteConfig, "video", c.GetUint("user_id"), preferredFileName, seeker, contentType, contentHash)
		}
		return "", errors.New("无法读取上传文件")
	}

	// 本地存储逻辑
	// 创建存储视频的目录
	// 本地存储逻辑
	// 确定视频存储路径，优先级：/data/video > /app/data/video > ./data/video
	ownerUserID := c.GetUint("user_id")
	if ownerUserID == 0 {
		return "", errors.New("未登录或登录已过期")
	}
	var localContent io.ReadSeeker = srcFile
	contentSize := file.Size
	if uploadPath != "" {
		compressed, err := os.Open(uploadPath)
		if err != nil {
			return "", err
		}
		defer compressed.Close()
		localContent = compressed
		if info, err := compressed.Stat(); err == nil {
			contentSize = info.Size()
		}
	}
	return storeAttachmentReference(c.Request.Context(), attachmentregistry.NewLocalStore(attachmentregistry.DefaultLocalRoot()), "video", ownerUserID, preferredFileName, contentType, contentHash, contentSize, localContent)
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
		return UploadAttachmentToCloud(siteConfig, "audio", c.GetUint("user_id"), preferredFileName, srcFile, contentType, contentHash)
	}

	ownerUserID := c.GetUint("user_id")
	if ownerUserID == 0 {
		return "", errors.New("未登录或登录已过期")
	}
	return storeAttachmentReference(c.Request.Context(), attachmentregistry.NewLocalStore(attachmentregistry.DefaultLocalRoot()), "audio", ownerUserID, preferredFileName, contentType, contentHash, file.Size, srcFile)
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
		return UploadAttachmentToCloud(siteConfig, "file", c.GetUint("user_id"), preferredFileName, seeker, contentType, contentHash)
	}

	ownerUserID := c.GetUint("user_id")
	if ownerUserID == 0 {
		return "", errors.New("未登录或登录已过期")
	}
	return storeAttachmentReference(c.Request.Context(), attachmentregistry.NewLocalStore(attachmentregistry.DefaultLocalRoot()), "file", ownerUserID, preferredFileName, contentType, contentHash, file.Size, seeker)
}
