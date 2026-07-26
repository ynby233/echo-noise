package controllers

import (
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const attachmentContentSecurityPolicy = "default-src 'none'; sandbox"

// 允许浏览器直接打开查看、不强制弹下载框的类型。
// 收敛条件有两条，缺一不可：
//  1. 经 safeAttachmentContentType 归一后仍是纯文本族。html/svg/xml 以及任何未列入白名单的类型
//     都会被压成 application/octet-stream，配合 X-Content-Type-Options: nosniff，
//     浏览器不会把响应体当 HTML 解析，同源存储型 XSS 因此不成立。
//  2. 浏览器原生就能渲染，不需要放宽 CSP 的 sandbox。
//
// 其余类型（图片、音视频、PDF、归档、Office 文档等）继续走 attachment：
// 图片与音视频在页面里由 <img>/<video>/<audio> 内联渲染，不受 disposition 影响；
// PDF 依赖浏览器内置阅读器，而阅读器与 CSP sandbox 的相容性无法在当前环境验证，先不改。
var inlineViewableAttachmentContentTypes = map[string]struct{}{
	"text/plain":       {},
	"text/csv":         {},
	"application/json": {},
}

func attachmentContentDisposition(contentType string) string {
	if _, ok := inlineViewableAttachmentContentTypes[contentType]; ok {
		return "inline"
	}
	return "attachment"
}

func applyAttachmentSecurityHeaders(c *gin.Context, originalName, contentType string) {
	safeType := safeAttachmentContentType(contentType)
	c.Header("Content-Type", safeType)
	c.Header("Content-Disposition", mime.FormatMediaType(attachmentContentDisposition(safeType), map[string]string{"filename": originalName}))
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
