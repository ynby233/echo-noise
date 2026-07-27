package controllers

import (
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const attachmentContentSecurityPolicy = "default-src 'none'; sandbox"

// 允许浏览器尝试直接打开、不强制弹下载框的安全类型。
// 这不是浏览器能力表：inline 只表示服务端允许内联，最终显示内置阅读器、交给系统应用还是
// 回退下载，仍由浏览器决定。白名单只回答“即使内联解释，是否仍在可接受的安全边界内”。
//
// html/svg/xml 以及任何未列入 safeAttachmentContentType 的类型都会被压成
// application/octet-stream；配合 X-Content-Type-Options: nosniff 和 sandbox CSP，浏览器不会
// 把响应体提升为同源主动内容。PDF 保留 application/pdf，并继续使用同一套 nosniff + sandbox
// 安全头；支持内置阅读器的浏览器可直接查看，不支持的浏览器自行选择下载或外部应用。
// 图片与音视频仍由页面里的 <img>/<video>/<audio> 内联渲染，不需要改变直接导航时的下载语义。
var inlineViewableAttachmentContentTypes = map[string]struct{}{
	"text/plain":       {},
	"text/csv":         {},
	"application/json": {},
	"application/pdf":  {},
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
