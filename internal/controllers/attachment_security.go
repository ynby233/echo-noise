package controllers

import (
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const attachmentContentSecurityPolicy = "default-src 'none'; sandbox"

func applyAttachmentSecurityHeaders(c *gin.Context, originalName, contentType string) {
	c.Header("Content-Type", safeAttachmentContentType(contentType))
	c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": originalName}))
	c.Header("Content-Security-Policy", attachmentContentSecurityPolicy)
	c.Header("X-Content-Type-Options", "nosniff")
}

func safeAttachmentContentType(contentType string) string {
	contentType = strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	switch contentType {
	case "image/jpeg", "image/jpg", "image/png", "image/gif", "image/webp", "image/avif", "image/bmp", "image/x-icon", "image/vnd.microsoft.icon",
		"video/mp4", "video/webm", "video/ogg", "video/mpeg", "video/quicktime", "video/x-msvideo",
		"audio/mpeg", "audio/mp4", "audio/ogg", "audio/wav", "audio/x-wav", "audio/webm", "audio/aac", "audio/flac",
		"text/plain", "text/csv", "application/pdf", "application/json", "application/zip", "application/gzip", "application/x-7z-compressed", "application/x-rar-compressed",
		"application/msword", "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		"application/vnd.ms-excel", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		"application/vnd.ms-powerpoint", "application/vnd.openxmlformats-officedocument.presentationml.presentation",
		"application/octet-stream":
		return contentType
	default:
		return "application/octet-stream"
	}
}

func sniffAttachmentContentType(content io.ReadSeeker) string {
	if content == nil {
		return "application/octet-stream"
	}
	_, _ = content.Seek(0, io.SeekStart)
	buffer := make([]byte, 512)
	n, _ := content.Read(buffer)
	_, _ = content.Seek(0, io.SeekStart)
	return http.DetectContentType(buffer[:n])
}
