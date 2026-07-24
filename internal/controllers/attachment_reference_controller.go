package controllers

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/gin-gonic/gin"
	attachmentregistry "github.com/rcy1314/echo-noise/internal/attachments"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/services"
	"gorm.io/gorm"
)

func serveLocalAttachmentReference(c *gin.Context, kind, blobRoot, rawPath string) {
	parts := strings.SplitN(strings.TrimPrefix(rawPath, "/"), "/", 3)
	if len(parts) != 3 || parts[0] != "refs" || parts[1] == "" || parts[2] == "" {
		c.Status(http.StatusNotFound)
		return
	}
	originalName, err := url.PathUnescape(parts[2])
	if err != nil || originalName == "" || strings.ContainsAny(originalName, `/\`) {
		c.Status(http.StatusNotFound)
		return
	}
	db, err := database.GetDB()
	if err != nil {
		c.Status(http.StatusServiceUnavailable)
		return
	}
	resolved, err := attachmentregistry.NewRegistry(db).Resolve(parts[1])
	if err != nil || resolved.Reference.Kind != kind || resolved.Reference.OriginalName != originalName || resolved.Blob.StorageBackend != "local" {
		c.Status(http.StatusNotFound)
		return
	}
	allowed, publiclyReferenced, err := canReadAttachmentReference(c, resolved.Reference, "local")
	if err != nil {
		c.Status(http.StatusServiceUnavailable)
		return
	}
	if !allowed {
		c.Header("Cache-Control", "private, no-store")
		c.Header("Vary", "Cookie, Authorization")
		c.Status(http.StatusNotFound)
		return
	}
	if publiclyReferenced {
		c.Header("Cache-Control", "public, max-age=300")
	} else {
		c.Header("Cache-Control", "private, no-store")
		c.Header("Vary", "Cookie, Authorization")
	}
	file, info, err := attachmentregistry.NewLocalStore(blobRoot).Open(resolved.Blob.StorageKey)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	defer file.Close()
	if resolved.Blob.ContentType != "" {
		c.Header("Content-Type", resolved.Blob.ContentType)
	}
	c.Header("X-Content-Type-Options", "nosniff")
	http.ServeContent(c.Writer, c.Request, resolved.Reference.OriginalName, info.ModTime(), file)
}

func canReadAttachmentReference(c *gin.Context, reference models.AttachmentReference, backend string) (bool, bool, error) {
	messages, err := messagesReferencingAttachmentReference(reference, backend)
	if err != nil {
		return false, false, err
	}
	if reference.Kind == "image" && isPublicSiteImageReference(reference, backend) {
		return true, true, nil
	}
	viewerID, isAdmin := currentMessageViewer(c)
	if isAdmin {
		public := false
		for _, message := range messages {
			if services.StoredMessageVisibility(message) == services.MessageVisibilityPublic {
				public = true
				break
			}
		}
		return true, public, nil
	}
	if len(messages) == 0 {
		return viewerID != nil && *viewerID == reference.OwnerUserID, false, nil
	}
	publiclyReferenced := false
	allowed := false
	for _, message := range messages {
		if services.StoredMessageVisibility(message) == services.MessageVisibilityPublic {
			publiclyReferenced = true
		}
		if services.CanViewMessage(message, viewerID, false) {
			allowed = true
		}
	}
	return allowed, publiclyReferenced, nil
}

func isPublicSiteImageReference(reference models.AttachmentReference, backend string) bool {
	needle := attachmentReferenceURLPrefix(reference.Kind, backend, reference.PublicID)
	for _, usage := range loadPublicSiteImageAttachmentUsages() {
		if strings.Contains(usage.URL, needle) {
			return true
		}
	}
	return false
}

func messagesReferencingAttachmentReference(reference models.AttachmentReference, backend string) ([]models.Message, error) {
	db, err := database.GetDB()
	if err != nil {
		return nil, err
	}
	var candidates []models.Message
	if err := db.Select("id", "content", "image_url", "user_id", "private", "visibility", "created_at").
		Where("user_id = ? AND (content LIKE ? OR image_url LIKE ?)", reference.OwnerUserID, "%"+reference.PublicID+"%", "%"+reference.PublicID+"%").
		Find(&candidates).Error; err != nil {
		return nil, err
	}
	needle := attachmentReferenceURLPrefix(reference.Kind, backend, reference.PublicID)
	matched := make([]models.Message, 0, len(candidates))
	for _, message := range candidates {
		if strings.Contains(message.Content, needle) || strings.Contains(message.ImageURL, needle) {
			matched = append(matched, message)
		}
	}
	return matched, nil
}

func attachmentReferenceURLPrefix(kind, backend, publicID string) string {
	if backend == "cloud" {
		return "/api/cloud-attachments/" + publicID + "/"
	}
	prefix := "/api/files/"
	switch kind {
	case "image":
		prefix = "/api/images/"
	case "video":
		prefix = "/api/video/"
	case "audio":
		prefix = "/api/audio/"
	}
	return prefix + "refs/" + publicID + "/"
}

func tryServeCloudAttachmentReference(c *gin.Context, publicID string) bool {
	db, err := database.GetDB()
	if err != nil {
		c.Status(http.StatusServiceUnavailable)
		return true
	}
	resolved, err := attachmentregistry.NewRegistry(db).Resolve(publicID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false
	}
	if err != nil {
		c.Status(http.StatusServiceUnavailable)
		return true
	}
	if resolved.Blob.StorageBackend != "cloud" {
		c.Status(http.StatusNotFound)
		return true
	}
	rawName := strings.TrimPrefix(c.Param("name"), "/")
	originalName, err := url.PathUnescape(rawName)
	if err != nil || originalName != resolved.Reference.OriginalName {
		c.Status(http.StatusNotFound)
		return true
	}
	allowed, publiclyReferenced, err := canReadAttachmentReference(c, resolved.Reference, "cloud")
	if err != nil {
		c.Status(http.StatusServiceUnavailable)
		return true
	}
	if !allowed {
		c.Header("Cache-Control", "private, no-store")
		c.Header("Vary", "Cookie, Authorization")
		c.Status(http.StatusNotFound)
		return true
	}
	if publiclyReferenced {
		c.Header("Cache-Control", "public, max-age=300")
	} else {
		c.Header("Cache-Control", "private, no-store")
		c.Header("Vary", "Cookie, Authorization")
	}
	c.Header("X-Content-Type-Options", "nosniff")

	var cfg models.SiteConfig
	if err := db.Table("site_configs").First(&cfg).Error; err != nil {
		c.Status(http.StatusNotFound)
		return true
	}
	client, bucket, _, err := newAttachmentS3Client(cfg)
	if err != nil {
		c.Status(http.StatusServiceUnavailable)
		return true
	}
	object := models.CloudAttachmentObject{OriginalName: resolved.Reference.OriginalName, ContentType: resolved.Blob.ContentType}
	ctx := c.Request.Context()
	if c.Request.Method == http.MethodHead {
		head, err := client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(bucket), Key: aws.String(resolved.Blob.StorageKey)})
		if err != nil {
			c.Status(http.StatusNotFound)
			return true
		}
		applyCloudAttachmentHeaders(c, object, aws.ToString(head.ContentType), aws.ToInt64(head.ContentLength), aws.ToString(head.ETag), aws.ToTime(head.LastModified), "")
		c.Status(http.StatusOK)
		return true
	}
	input := &s3.GetObjectInput{Bucket: aws.String(bucket), Key: aws.String(resolved.Blob.StorageKey)}
	if byteRange := strings.TrimSpace(c.GetHeader("Range")); byteRange != "" {
		input.Range = aws.String(byteRange)
	}
	result, err := client.GetObject(ctx, input)
	if err != nil {
		c.Status(http.StatusNotFound)
		return true
	}
	defer result.Body.Close()
	contentRange := aws.ToString(result.ContentRange)
	applyCloudAttachmentHeaders(c, object, aws.ToString(result.ContentType), aws.ToInt64(result.ContentLength), aws.ToString(result.ETag), aws.ToTime(result.LastModified), contentRange)
	status := http.StatusOK
	if contentRange != "" {
		status = http.StatusPartialContent
	}
	c.Status(status)
	_, _ = io.Copy(c.Writer, result.Body)
	return true
}
