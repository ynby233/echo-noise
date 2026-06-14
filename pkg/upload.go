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
	"github.com/rcy1314/echo-noise/config"
	"github.com/rcy1314/echo-noise/internal/models"
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

func hashedAttachmentFileNameFromBytes(data []byte, ext string) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]) + normalizeUploadExt(ext, ".bin")
}

func hashedAttachmentFileNameFromReadSeeker(content io.ReadSeeker, ext string) (string, error) {
	if _, err := content.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	h := sha256.New()
	if _, err := io.Copy(h, content); err != nil {
		return "", err
	}
	if _, err := content.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)) + normalizeUploadExt(ext, ".bin"), nil
}

func hashedAttachmentFileNameFromPath(path, ext string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)) + normalizeUploadExt(ext, ".bin"), nil
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
	if file.Size > int64(config.Config.Upload.MaxSize) {
		return "", errors.New(models.ImageSizeLimitErrorMessage + strconv.Itoa(config.Config.Upload.MaxSize/1024/1024) + "MB")
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

	// 使用最终内容的 SHA-256 生成稳定文件名，相同文件只保存一份。
	newFileName := hashedAttachmentFileNameFromBytes(fileData, ext)

	// 检查是否启用了附件云存储
	if siteConfig != nil && siteConfig.AttachmentStorageEnabled {
		return UploadAttachmentToCloud(siteConfig, newFileName, bytes.NewReader(fileData), contentType)
	}

	// 本地存储逻辑
	// 创建存储图片的目录（如果没有的话）
	if err := createImageDirIfNotExist(config.Config.Upload.SavePath); err != nil {
		return "", err
	}

	// 保存文件到指定目录；相同内容已经存在时直接复用。
	savePath := filepath.Join(config.Config.Upload.SavePath, newFileName)
	if !fileExists(savePath) {
		if err := os.WriteFile(savePath, fileData, 0644); err != nil {
			return "", errors.New(models.ImageUploadErrorMessage)
		}
	}

	// 返回图片的 URL
	imageURL := fmt.Sprintf("/api/images/%s", newFileName)
	return imageURL, nil
}

// UploadAttachmentToCloud 上传附件到云存储
func UploadAttachmentToCloud(cfg *models.SiteConfig, fileName string, content io.ReadSeeker, contentType string) (string, error) {
	if cfg.AttachmentStorageBucket == "" || cfg.AttachmentStorageAccessKey == "" || cfg.AttachmentStorageSecretKey == "" || cfg.AttachmentStorageEndpoint == "" {
		return "", errors.New("附件云存储配置不完整")
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

	var contentLength int64
	if _, err := content.Seek(0, io.SeekStart); err == nil {
		if n, err := content.Seek(0, io.SeekEnd); err == nil {
			contentLength = n
		}
		content.Seek(0, io.SeekStart)
	}

	origin, prefix := splitPublicBaseURL(cfg.AttachmentStoragePublicBaseURL)
	key := strings.TrimLeft(fileName, "/")
	if prefix != "" {
		key = prefix + "/" + key
	}

	// 构建返回 URL
	buildPublicURL := func() (string, error) {
		if strings.TrimSpace(cfg.AttachmentStoragePublicBaseURL) != "" {
			if origin == "" {
				return "", errors.New("请在设置中配置云存储公共访问域名(PublicBaseURL)")
			}
			return fmt.Sprintf("%s/%s", origin, escapeObjectKeyForURL(key)), nil
		}

		return "", errors.New("请在设置中配置云存储公共访问域名(PublicBaseURL)")
	}

	if _, err := client.HeadObject(context.TODO(), &s3.HeadObjectInput{
		Bucket: aws.String(cfg.AttachmentStorageBucket),
		Key:    aws.String(key),
	}); err == nil {
		return buildPublicURL()
	}

	// 上传文件
	_, err = client.PutObject(context.TODO(), &s3.PutObjectInput{
		Bucket:      aws.String(cfg.AttachmentStorageBucket),
		Key:         aws.String(key),
		Body:        content,
		ContentType: aws.String(contentType),
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

	return buildPublicURL()
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
	if file.Size > 1024*1024*1024 {
		return "", errors.New("视频大小不能超过1024MB")
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

	var newFileName string
	if uploadPath != "" {
		newFileName, err = hashedAttachmentFileNameFromPath(uploadPath, ext)
	} else if seeker, ok := srcFile.(io.ReadSeeker); ok {
		newFileName, err = hashedAttachmentFileNameFromReadSeeker(seeker, ext)
	} else {
		return "", errors.New("无法读取上传文件")
	}
	if err != nil {
		return "", err
	}

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
			return UploadAttachmentToCloud(siteConfig, newFileName, f, contentType)
		}
		if seeker, ok := srcFile.(io.ReadSeeker); ok {
			if _, err := seeker.Seek(0, io.SeekStart); err != nil {
				return "", err
			}
			return UploadAttachmentToCloud(siteConfig, newFileName, seeker, contentType)
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

	savePath := filepath.Join(videoPath, newFileName)
	if !fileExists(savePath) {
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

	videoURL := fmt.Sprintf("/api/video/%s", newFileName)
	return videoURL, nil
}
