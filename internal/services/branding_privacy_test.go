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
	message := models.Message{Content: legacyWelcomeMessage, Username: "admin", UserID: 1}
	if err := db.Create(&message).Error; err != nil {
		t.Fatalf("create legacy message: %v", err)
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
	} {
		if strings.Contains(strings.ToLower(value), strings.ToLower(token)) {
			t.Fatalf("public value contains legacy token %q: %s", token, value)
		}
	}
}
