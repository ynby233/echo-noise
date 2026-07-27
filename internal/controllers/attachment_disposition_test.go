package controllers

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// 附件的 Content-Disposition 必须跟着归一后的类型走：
// 安全的文本族与 PDF 允许浏览器尝试内联，其余（含被压成 octet-stream 的 html/svg）强制下载。
func TestAttachmentContentDispositionOnlyInlinesSafeBrowserViewableTypes(t *testing.T) {
	inline := []string{"text/plain", "text/csv", "application/json", "application/pdf"}
	for _, contentType := range inline {
		if got := attachmentContentDisposition(safeAttachmentContentType(contentType)); got != "inline" {
			t.Fatalf("disposition for %q = %q, want inline", contentType, got)
		}
	}

	forced := []string{
		"text/html",
		"image/svg+xml",
		"application/xml",
		"text/xml",
		"image/png",
		"video/mp4",
		"audio/mpeg",
		"application/zip",
		"application/octet-stream",
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		"",
	}
	for _, contentType := range forced {
		if got := attachmentContentDisposition(safeAttachmentContentType(contentType)); got != "attachment" {
			t.Fatalf("disposition for %q = %q, want attachment", contentType, got)
		}
	}
}

// 带参数的 Content-Type 也要能命中 inline，否则上传来的 "text/plain; charset=utf-8" 会退回下载。
func TestAttachmentContentDispositionIgnoresContentTypeParameters(t *testing.T) {
	if got := attachmentContentDisposition(safeAttachmentContentType("text/plain; charset=utf-8")); got != "inline" {
		t.Fatalf("disposition for parameterized text/plain = %q, want inline", got)
	}
	if got := attachmentContentDisposition(safeAttachmentContentType("TEXT/CSV")); got != "inline" {
		t.Fatalf("disposition for uppercase text/csv = %q, want inline", got)
	}
	if got := attachmentContentDisposition(safeAttachmentContentType("APPLICATION/PDF; version=1.7")); got != "inline" {
		t.Fatalf("disposition for parameterized application/pdf = %q, want inline", got)
	}
}

func TestApplyAttachmentSecurityHeadersAllowsSandboxedPdfInline(t *testing.T) {
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)

	applyAttachmentSecurityHeaders(context, "浏览器预览.pdf", "application/pdf; version=1.7")

	if got := response.Header().Get("Content-Type"); got != "application/pdf" {
		t.Fatalf("pdf content type = %q, want application/pdf", got)
	}
	if got := response.Header().Get("Content-Disposition"); !strings.HasPrefix(got, "inline;") || !strings.Contains(got, "filename") {
		t.Fatalf("pdf content disposition = %q, want inline with filename", got)
	}
	if got := response.Header().Get("Content-Security-Policy"); got != attachmentContentSecurityPolicy {
		t.Fatalf("pdf CSP = %q, want %q", got, attachmentContentSecurityPolicy)
	}
	if got := response.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("pdf nosniff = %q, want nosniff", got)
	}
}
