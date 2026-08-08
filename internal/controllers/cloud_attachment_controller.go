package controllers

import (
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/services"
)

func ServeCloudAttachment(c *gin.Context) {
	publicID := strings.TrimSpace(c.Param("id"))
	if publicID == "" {
		c.Status(http.StatusNotFound)
		return
	}
	if tryServeCloudAttachmentReference(c, publicID) {
		return
	}
	db, err := database.GetDB()
	if err != nil {
		c.Status(http.StatusServiceUnavailable)
		return
	}
	var object models.CloudAttachmentObject
	if err := db.Where("public_id = ?", publicID).First(&object).Error; err != nil || object.ID == 0 {
		c.Status(http.StatusNotFound)
		return
	}

	messages, err := messagesReferencingCloudAttachment(object)
	if err != nil {
		c.Status(http.StatusServiceUnavailable)
		return
	}
	grants, err := localAttachmentGrants("cloud", object.PublicID)
	if err != nil {
		c.Status(http.StatusServiceUnavailable)
		return
	}
	viewerID, isAdmin := currentMessageViewer(c)
	allowed := len(messages) == 0 && len(grants) == 0
	publiclyReferenced := false
	liveMessageIDs := make(map[uint]struct{}, len(messages))
	for _, message := range messages {
		liveMessageIDs[message.ID] = struct{}{}
		if services.StoredMessageVisibility(message) == services.MessageVisibilityPublic {
			publiclyReferenced = true
		}
		if services.CanViewMessage(message, viewerID, isAdmin) {
			allowed = true
		}
	}
	for _, grant := range grants {
		if _, isLiveReference := liveMessageIDs[grant.MessageID]; isLiveReference {
			continue
		}
		snapshot := models.Message{
			ID:         grant.MessageID,
			UserID:     grant.OwnerUserID,
			Visibility: grant.Visibility,
			Private:    grant.Visibility != services.MessageVisibilityPublic,
		}
		if services.StoredMessageVisibility(snapshot) == services.MessageVisibilityPublic {
			publiclyReferenced = true
		}
		if services.CanViewMessage(snapshot, viewerID, isAdmin) {
			allowed = true
		}
	}
	legacySources, legacyErr := services.VisibleLegacyAttachmentSources(db, nil, "cloud", object.PublicID)
	if legacyErr == nil && len(legacySources) > 0 {
		allowed = false
	}
	if visibleSources, sourceErr := services.VisibleLegacyAttachmentSourcesForViewer(db, viewerID, "cloud", object.PublicID); sourceErr == nil {
		if len(visibleSources) > 0 {
			allowed = true
		}
		for _, source := range visibleSources {
			if source.SourceType == "message" && services.StoredMessageVisibility(source.Message) == services.MessageVisibilityPublic {
				publiclyReferenced = true
			}
			if source.Comment != nil && source.Visibility == "public" && services.StoredMessageVisibility(source.Message) == services.MessageVisibilityPublic {
				publiclyReferenced = true
			}
		}
	}
	if !allowed {
		c.Header("Cache-Control", "private, no-store")
		c.Header("Vary", "Cookie, Authorization")
		c.Status(http.StatusNotFound)
		return
	}
	if (len(messages) > 0 || len(grants) > 0) && !publiclyReferenced {
		c.Header("Cache-Control", "private, no-store")
		c.Header("Vary", "Cookie, Authorization")
	} else {
		c.Header("Cache-Control", "public, max-age=3600")
	}

	var cfg models.SiteConfig
	if err := db.Table("site_configs").First(&cfg).Error; err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	client, bucket, _, err := newAttachmentS3Client(cfg)
	if err != nil {
		c.Status(http.StatusServiceUnavailable)
		return
	}
	ctx := c.Request.Context()
	if c.Request.Method == http.MethodHead {
		head, err := client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(bucket), Key: aws.String(object.ObjectKey)})
		if err != nil {
			c.Status(http.StatusNotFound)
			return
		}
		applyCloudAttachmentHeaders(c, object, aws.ToString(head.ContentType), aws.ToInt64(head.ContentLength), aws.ToString(head.ETag), aws.ToTime(head.LastModified), "")
		c.Status(http.StatusOK)
		return
	}

	input := &s3.GetObjectInput{Bucket: aws.String(bucket), Key: aws.String(object.ObjectKey)}
	if byteRange := strings.TrimSpace(c.GetHeader("Range")); byteRange != "" {
		input.Range = aws.String(byteRange)
	}
	result, err := client.GetObject(ctx, input)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
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
}

func applyCloudAttachmentHeaders(c *gin.Context, object models.CloudAttachmentObject, contentType string, contentLength int64, etag string, lastModified time.Time, contentRange string) {
	if strings.TrimSpace(contentType) == "" {
		contentType = object.ContentType
	}
	applyAttachmentSecurityHeaders(c, object.OriginalName, contentType)
	if contentLength >= 0 {
		c.Header("Content-Length", strconv.FormatInt(contentLength, 10))
	}
	if etag != "" {
		c.Header("ETag", etag)
	}
	if !lastModified.IsZero() {
		c.Header("Last-Modified", lastModified.Format(http.TimeFormat))
	}
	if contentRange != "" {
		c.Header("Content-Range", contentRange)
	}
	c.Header("Accept-Ranges", "bytes")
}

func messagesReferencingCloudAttachment(object models.CloudAttachmentObject) ([]models.Message, error) {
	db, err := database.GetDB()
	if err != nil {
		return nil, err
	}
	prefix := "/api/cloud-attachments/" + object.PublicID + "/"
	var candidates []models.Message
	if err := db.Select("id", "content", "image_url", "user_id", "private", "visibility").
		Where("content LIKE ? OR image_url LIKE ?", "%"+object.PublicID+"%", "%"+object.PublicID+"%").
		Find(&candidates).Error; err != nil {
		return nil, err
	}
	matched := make([]models.Message, 0, len(candidates))
	for _, message := range candidates {
		if strings.Contains(message.Content, prefix) || strings.Contains(message.ImageURL, prefix) {
			matched = append(matched, message)
		}
	}
	return matched, nil
}
