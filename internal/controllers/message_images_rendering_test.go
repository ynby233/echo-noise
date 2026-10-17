package controllers

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rcy1314/echo-noise/config"
	"github.com/rcy1314/echo-noise/internal/models"
)

func TestExtractMessageImagesMatchesRenderedContent(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    []string
	}{
		{
			name:    "uploaded image attachment links including table cells",
			content: "[图片附件：photo.png](/api/images/refs/photo-id/photo.png)\n\n| 图片 |\n| --- |\n| [图片附件：table.png](/api/images/refs/table-id/table.png) |",
			want:    []string{"/api/images/refs/photo-id/photo.png", "/api/images/refs/table-id/table.png"},
		},
		{
			name:    "ordinary links and other media are not images",
			content: "[photo](/photo.png) [文件附件：file.png](/file.png) [视频附件：clip.mp4](/video.mp4) [音频附件：recording.mp3](/audio.mp3)\n\n<video poster='/poster.png'></video>",
			want:    []string{},
		},
		{
			name:    "other attachment cards discard nested preview images",
			content: "[文件附件：file.pdf ![cover](/file-cover.png)](/file.pdf)\n\n[视频附件：clip ![cover](/video-cover.png)](/clip.mp4)\n\n[音频附件：recording ![cover](/audio-cover.png)](/recording.mp3)",
			want:    []string{},
		},
		{
			name:    "inert templates do not contribute images or attachment links",
			content: "<div><template><img src='/hidden.png'><a href='/hidden-link.png'>图片附件：hidden.png</a></template><img src='/visible.png'></div>",
			want:    []string{"/visible.png"},
		},
		{
			name:    "multiline attachment labels follow frontend label matching",
			content: "<a href='/linebreak.png'>图片附件：甲&#10;乙</a><a href='/carriage.png'>图片附件：甲&#13;乙</a><a href='/separator.png'>图片附件：甲&#8232;乙</a>",
			want:    []string{},
		},
		{
			name:    "attachment reference links preserve occurrences and mixed ordering",
			content: "[图片附件：first][photo]\n\n![markdown](/middle.png)\n\n[图片附件：second][photo]\n\n[photo]: /reference.png",
			want:    []string{"/reference.png", "/middle.png", "/reference.png"},
		},
		{
			name:    "attachment marker in code stays text",
			content: "`[图片附件：inline](/inline.png)`\n\n```markdown\n[图片附件：fenced](/fenced.png)\n```",
			want:    []string{},
		},
		{
			name:    "markdown and resized html in document order",
			content: "![first](/first.png)\n\n<img src=\"/resized.png\" width=\"320\" style=\"height:auto\" />\n\n![last](/last.png)",
			want:    []string{"/first.png", "/resized.png", "/last.png"},
		},
		{
			name:    "html table and case insensitive attributes",
			content: "<table><tr><td><IMG WIDTH='120' SRC='/table.png'></td></tr></table>",
			want:    []string{"/table.png"},
		},
		{
			name:    "markdown title and balanced destination parentheses",
			content: "![photo](/photo(1).png \"caption\")",
			want:    []string{"/photo(1).png"},
		},
		{
			name:    "reference markdown preserves repeated image occurrences",
			content: "![first][photo]\n\n![second][photo]\n\n[photo]: /reference.png \"caption\"",
			want:    []string{"/reference.png", "/reference.png"},
		},
		{
			name:    "html entities decoded and blank sources skipped",
			content: "<img src='/photo.png?a=1&amp;b=2'><img src=' '><img alt='no source'>",
			want:    []string{"/photo.png?a=1&b=2"},
		},
		{
			name:    "examples and escaped markup are not rendered images",
			content: "`![example](/inline.png)`\n\n```html\n<img src='/code.png'>\n![example](/fenced.png)\n```\n\n&lt;img src='/escaped.png'&gt;\n\n<!-- <img src='/comment.png'> -->",
			want:    []string{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := extractMessageImages(tc.content); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("expected rendered images %#v, got %#v", tc.want, got)
			}
		})
	}
}

func TestRenderedImagesAppearInGalleryAndPersonalStats(t *testing.T) {
	db, router, owner, _ := setupCommentAccountTest(t)
	if err := db.Where("1 = 1").Delete(&models.Message{}).Error; err != nil {
		t.Fatalf("clear seed messages: %v", err)
	}
	if err := db.AutoMigrate(&models.AttachmentReference{}, &models.AttachmentBlob{}, &models.CloudAttachmentObject{}); err != nil {
		t.Fatalf("migrate attachment tables: %v", err)
	}
	if err := db.Model(&owner).Updates(map[string]any{"token": "rendered-images-owner", "is_admin": true}).Error; err != nil {
		t.Fatalf("update owner: %v", err)
	}

	imageDir := t.TempDir()
	for _, name := range []string{"field.png", "markdown.png", "private.png", "deleted-note.png", "code.png"} {
		if err := os.WriteFile(filepath.Join(imageDir, name), []byte("png"), 0o600); err != nil {
			t.Fatalf("write image %s: %v", name, err)
		}
	}
	originalSavePath := config.Config.Upload.SavePath
	config.Config.Upload.SavePath = imageDir
	t.Cleanup(func() { config.Config.Upload.SavePath = originalSavePath })
	blob := models.AttachmentBlob{StorageBackend: "local", StorageKey: "ab/rendered", ContentHash: "rendered", Size: 3}
	if err := db.Create(&blob).Error; err != nil {
		t.Fatalf("create blob: %v", err)
	}
	reference := models.AttachmentReference{PublicID: "rendered-ref", BlobID: blob.ID, OwnerUserID: owner.ID, Kind: "image", OriginalName: "resized.png"}
	if err := db.Create(&reference).Error; err != nil {
		t.Fatalf("create reference: %v", err)
	}

	deletedAt := time.Now().UTC()
	messages := []models.Message{
		{
			Content: "#public\n\n![markdown](/api/images/markdown.png)\n\n" +
				"| 图片 |\n| --- |\n| [图片附件：resized.png](/api/images/refs/rendered-ref/resized.png) |\n\n" +
				"<img src=\"/api/images/refs/missing-ref/missing.png\" width=\"320\" />\n\n" +
				"```html\n<img src=\"/api/images/code.png\">\n```",
			ImageURL: "/api/images/field.png", UserID: owner.ID, Username: owner.Username,
		},
		{Content: "#private\n\n[图片附件：private.png](/api/images/private.png)", UserID: owner.ID, Username: owner.Username, Visibility: "private", Private: true},
		{Content: "<img src='/api/images/deleted-note.png'>", UserID: owner.ID, Username: owner.Username, DeletedAt: &deletedAt},
	}
	for i := range messages {
		if err := db.Create(&messages[i]).Error; err != nil {
			t.Fatalf("create message: %v", err)
		}
	}
	router.GET("/messages/images", GetAllImages)
	router.GET("/users/me/stats", func(c *gin.Context) {
		c.Set("user_id", owner.ID)
		GetCurrentUserHomeStats(c)
	})

	publicImages := []string{"/api/images/field.png", "/api/images/markdown.png", "/api/images/refs/rendered-ref/resized.png"}
	assertImages(t, performImagesRequest(router, ""), publicImages)
	assertImages(t, performImagesRequest(router, "rendered-images-owner"), append(publicImages, "/api/images/private.png"))
	stats := decodeHomeStatsResponse(t, performHomeStatsRequest(router, "owner"), 200)
	if stats.TotalMessages != 2 || stats.TotalTags != 2 || stats.TotalImages != 4 {
		t.Fatalf("expected personal stats 2/2/4 including uploaded image attachments, got %#v", stats)
	}
}
