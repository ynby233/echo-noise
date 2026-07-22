package services

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/rcy1314/echo-noise/internal/models"
)

func TestDefaultFrontendConfigDoesNotExposeLegacyBranding(t *testing.T) {
	raw, err := json.Marshal(getDefaultConfig())
	if err != nil {
		t.Fatalf("marshal default config: %v", err)
	}
	assertNoLegacyPublicBranding(t, string(raw))
}

func TestVoceChatNotificationFallbackDoesNotExposeLegacyBranding(t *testing.T) {
	markdown := buildVoceChatNotificationMarkdown(models.SiteConfig{}, models.UserNotification{
		Type: "comment",
	})
	assertNoLegacyPublicBranding(t, markdown)
	if !strings.Contains(markdown, neutralSiteTitle) {
		t.Fatalf("notification fallback title = %q, want it to contain %q", markdown, neutralSiteTitle)
	}
}

func TestScrubTraceableLocalAPIURLsPreservesFunctionAndUnrelatedLinks(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		want        string
		wantChanged bool
	}{
		{
			name:        "branded attachment URL becomes relative",
			input:       `[文件](https://echo-noise.example.com:20714/api/files/a%20b.txt)`,
			want:        `[文件](/api/files/a%20b.txt)`,
			wantChanged: true,
		},
		{
			name:        "query and fragment are preserved",
			input:       `https://ech0-noise.example.com/api/images/a.png?size=large#preview`,
			want:        `/api/images/a.png?size=large#preview`,
			wantChanged: true,
		},
		{
			name:        "unrelated API host is preserved",
			input:       `https://files.example.com/api/files/a.txt`,
			want:        `https://files.example.com/api/files/a.txt`,
			wantChanged: false,
		},
		{
			name:        "non API page on branded host is preserved",
			input:       `https://echo-noise.example.com/about`,
			want:        `https://echo-noise.example.com/about`,
			wantChanged: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, changed := scrubTraceableLocalAPIURLs(tt.input)
			if got != tt.want || changed != tt.wantChanged {
				t.Fatalf("scrubTraceableLocalAPIURLs(%q) = (%q, %v), want (%q, %v)", tt.input, got, changed, tt.want, tt.wantChanged)
			}
		})
	}
}

func TestScrubLegacySiteConfigValuesPreservesCustomSettings(t *testing.T) {
	config := models.SiteConfig{
		SiteTitle:          "我的站点",
		Username:           "Noise",
		Description:        "执迷不悟",
		PageFooterHTML:     `<a href="https://github.com/rcy1314/echo-noise">legacy</a>`,
		RSSTitle:           "说说笔记",
		RSSDescription:     "一个说说笔记~",
		RSSAuthorName:      "Noise",
		PwaTitle:           "说说笔记",
		PwaDescription:     "一个丰富的个人说说笔记",
		AnnouncementText:   "欢迎访问我的说说笔记！",
		WelcomeName:        "Noise",
		WelcomeDescription: "执迷不悟",
		AvatarURL:          legacyDefaultAvatarURL,
		WelcomeAvatarURL:   legacyDefaultAvatarURL,
		Backgrounds:        `["https://s2.loli.net/2025/03/26/d7iyuPYA8cRqD1K.jpg","https://example.com/custom.jpg"]`,
		SocialLinks:        `[{"name":"主页","url":"https://www.noisework.cn/","icon":"home"},{"name":"自定义","url":"https://example.com","icon":"link"}]`,
		LeftAds:            `[{"imageURL":"https://picsum.photos/seed/ad/640/640","linkURL":"https://note.noisework.cn","description":"示例"}]`,
		FeedSources:        `[{"type":"说说笔记","name":"说说笔记","url":"/api/messages/page","enabled":true,"visible":true}]`,
	}

	if !scrubLegacySiteConfigValues(&config) {
		t.Fatal("expected legacy defaults to be scrubbed")
	}
	if config.SiteTitle != "我的站点" {
		t.Fatalf("custom site title changed to %q", config.SiteTitle)
	}
	if !strings.Contains(config.SocialLinks, "https://example.com") {
		t.Fatalf("custom social link removed: %s", config.SocialLinks)
	}
	if !strings.Contains(config.Backgrounds, "https://example.com/custom.jpg") {
		t.Fatalf("custom background removed: %s", config.Backgrounds)
	}
	assertNoLegacyPublicBranding(t, strings.Join([]string{
		config.Username,
		config.Description,
		config.PageFooterHTML,
		config.RSSTitle,
		config.RSSDescription,
		config.RSSAuthorName,
		config.PwaTitle,
		config.PwaDescription,
		config.AnnouncementText,
		config.WelcomeName,
		config.WelcomeDescription,
		config.AvatarURL,
		config.WelcomeAvatarURL,
		config.Backgrounds,
		config.SocialLinks,
		config.LeftAds,
		config.FeedSources,
	}, "\n"))
}

func TestScrubPersistedLegacyPublicBrandingMigratesOnlyKnownDefaults(t *testing.T) {
	db := setupUserServiceTestDB(t)
	legacy := models.SiteConfig{
		SiteTitle:        "说说笔记",
		Username:         "Noise",
		PageFooterHTML:   `<a href="https://github.com/rcy1314/echo-noise">legacy</a>`,
		SocialLinks:      `[{"name":"legacy","url":"https://www.noisework.cn/"},{"name":"custom","url":"https://example.com"}]`,
		AnnouncementText: "欢迎访问我的说说笔记！",
		AvatarURL:        legacyDefaultAvatarURL,
		WelcomeAvatarURL: legacyDefaultAvatarURL,
		Backgrounds:      `["https://s2.loli.net/2025/03/26/d7iyuPYA8cRqD1K.jpg"]`,
	}
	if err := db.Create(&legacy).Error; err != nil {
		t.Fatalf("create legacy config: %v", err)
	}
	for _, link := range []models.FriendLink{
		{Title: "legacy", Link: "https://www.noisework.cn/"},
		{Title: "custom", Link: "https://example.com"},
	} {
		if err := db.Create(&link).Error; err != nil {
			t.Fatalf("create friend link: %v", err)
		}
	}
	user := models.User{Username: "legacy-admin", AvatarURL: legacyDefaultAvatarURL}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create legacy user: %v", err)
	}
	message := models.Message{Content: legacyWelcomeMessage, Username: user.Username, UserID: user.ID}
	if err := db.Create(&message).Error; err != nil {
		t.Fatalf("create legacy message: %v", err)
	}
	legacyImageMessage := models.Message{
		Content:  "探索未知的世界。 #Travel",
		ImageURL: "https://s2.loli.net/2025/04/05/EnakPbZJjpGxRTw.jpg",
		Username: user.Username,
		UserID:   user.ID,
	}
	if err := db.Create(&legacyImageMessage).Error; err != nil {
		t.Fatalf("create legacy image message: %v", err)
	}
	customImageMessage := models.Message{
		Content:  "自定义内容",
		ImageURL: legacyImageMessage.ImageURL,
		Username: user.Username,
		UserID:   user.ID,
	}
	if err := db.Create(&customImageMessage).Error; err != nil {
		t.Fatalf("create custom image message: %v", err)
	}
	brandedMessage := models.Message{
		Content:  `[文件](https://echo-noise.example.com:20714/api/files/a%20b.txt)`,
		ImageURL: `https://echo-noise.example.com:20714/api/images/a.png?size=large`,
		Username: user.Username,
		UserID:   user.ID,
	}
	if err := db.Create(&brandedMessage).Error; err != nil {
		t.Fatalf("create branded message: %v", err)
	}
	customExternalMessage := models.Message{
		Content:  `https://files.example.com/api/files/a.txt`,
		Username: user.Username,
		UserID:   user.ID,
	}
	if err := db.Create(&customExternalMessage).Error; err != nil {
		t.Fatalf("create custom external message: %v", err)
	}
	brandedComment := models.Comment{
		MessageID: brandedMessage.ID,
		Content:   `查看 https://echo-noise.example.com/api/files/comment.txt`,
	}
	if err := db.Create(&brandedComment).Error; err != nil {
		t.Fatalf("create branded comment: %v", err)
	}

	if err := scrubPersistedLegacyPublicBranding(db); err != nil {
		t.Fatalf("scrub persisted branding: %v", err)
	}

	var gotConfig models.SiteConfig
	if err := db.First(&gotConfig, legacy.ID).Error; err != nil {
		t.Fatalf("reload config: %v", err)
	}
	assertNoLegacyPublicBranding(t, strings.Join([]string{
		gotConfig.SiteTitle,
		gotConfig.Username,
		gotConfig.PageFooterHTML,
		gotConfig.SocialLinks,
		gotConfig.AnnouncementText,
		gotConfig.AvatarURL,
		gotConfig.WelcomeAvatarURL,
		gotConfig.Backgrounds,
	}, "\n"))
	if !strings.Contains(gotConfig.SocialLinks, "https://example.com") {
		t.Fatalf("custom social link removed: %s", gotConfig.SocialLinks)
	}

	var links []models.FriendLink
	if err := db.Order("title").Find(&links).Error; err != nil {
		t.Fatalf("reload friend links: %v", err)
	}
	if len(links) != 1 || links[0].Link != "https://example.com" {
		t.Fatalf("friend links after migration = %#v", links)
	}

	var gotMessage models.Message
	if err := db.First(&gotMessage, message.ID).Error; err != nil {
		t.Fatalf("reload message: %v", err)
	}
	if gotMessage.Content != neutralWelcomeMessage {
		t.Fatalf("legacy welcome message not migrated: %q", gotMessage.Content)
	}

	var gotUser models.User
	if err := db.First(&gotUser, user.ID).Error; err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if gotUser.AvatarURL != neutralAvatarURL {
		t.Fatalf("legacy avatar not migrated: %q", gotUser.AvatarURL)
	}

	var gotLegacyImage models.Message
	if err := db.First(&gotLegacyImage, legacyImageMessage.ID).Error; err != nil {
		t.Fatalf("reload legacy image message: %v", err)
	}
	if gotLegacyImage.ImageURL != "" {
		t.Fatalf("legacy sample image not cleared: %q", gotLegacyImage.ImageURL)
	}
	var gotCustomImage models.Message
	if err := db.First(&gotCustomImage, customImageMessage.ID).Error; err != nil {
		t.Fatalf("reload custom image message: %v", err)
	}
	if gotCustomImage.ImageURL != customImageMessage.ImageURL {
		t.Fatalf("custom image changed: %q", gotCustomImage.ImageURL)
	}

	var gotBrandedMessage models.Message
	if err := db.First(&gotBrandedMessage, brandedMessage.ID).Error; err != nil {
		t.Fatalf("reload branded message: %v", err)
	}
	if gotBrandedMessage.Content != `[文件](/api/files/a%20b.txt)` || gotBrandedMessage.ImageURL != `/api/images/a.png?size=large` {
		t.Fatalf("branded message URLs not made relative: %#v", gotBrandedMessage)
	}
	var gotCustomExternalMessage models.Message
	if err := db.First(&gotCustomExternalMessage, customExternalMessage.ID).Error; err != nil {
		t.Fatalf("reload custom external message: %v", err)
	}
	if gotCustomExternalMessage.Content != customExternalMessage.Content {
		t.Fatalf("unrelated external URL changed: %q", gotCustomExternalMessage.Content)
	}
	var gotBrandedComment models.Comment
	if err := db.First(&gotBrandedComment, brandedComment.ID).Error; err != nil {
		t.Fatalf("reload branded comment: %v", err)
	}
	if gotBrandedComment.Content != `查看 /api/files/comment.txt` {
		t.Fatalf("branded comment URL not made relative: %q", gotBrandedComment.Content)
	}
}

func assertNoLegacyPublicBranding(t *testing.T, value string) {
	t.Helper()
	for _, token := range []string{
		"说说笔记",
		"rcy1314",
		"ynby233",
		"noise233/echo-noise",
		"noisework.cn",
		"noiseblogs.top",
		"liangwenhao3",
		"Ech0-Noise",
		"Echo-Noise",
		"s2.loli.net",
	} {
		if strings.Contains(strings.ToLower(value), strings.ToLower(token)) {
			t.Fatalf("public value contains legacy token %q: %s", token, value)
		}
	}
}
