package controllers

import "testing"

// 附件的 Content-Disposition 必须跟着归一后的类型走：
// 纯文本族可以直接在浏览器里打开，其余（含被压成 octet-stream 的 html/svg）一律强制下载。
func TestAttachmentContentDispositionOnlyInlinesPlainTextFamily(t *testing.T) {
	inline := []string{"text/plain", "text/csv", "application/json"}
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
		"application/pdf",
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
}
