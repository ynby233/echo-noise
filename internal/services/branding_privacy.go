package services

import (
	"encoding/json"
	"net/url"
	"regexp"
	"strings"

	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
)

const (
	neutralSiteTitle       = "个人站点"
	neutralOwnerName       = "站长"
	neutralDescription     = "欢迎访问"
	neutralRSSTitle        = "个人内容订阅"
	neutralRSSDescription  = "个人内容更新"
	neutralPwaDescription  = "个人内容与记录"
	neutralAnnouncement    = "欢迎访问！"
	neutralAvatarURL       = "/favicon.svg"
	legacyDefaultAvatarURL = "https://s2.loli.net/2025/03/24/HnSXKvibAQlosIW.png"
	legacyFullImageMarker  = "<!-- noise-full-image-attachments -->"
	neutralFullImageMarker = "<!-- full-image-attachments -->"
	legacyWelcomeMessage   = "欢迎来到说说笔记！默认用户名及密码均为admin，记得到后台页修改你的用户名或密码,密码带有加强设置，如需简单密码可在用户管理面板中展开后重置密码"
	neutralWelcomeMessage  = "欢迎来到你的个人站点！默认用户名及密码均为admin，记得到后台页修改你的用户名或密码,密码带有加强设置，如需简单密码可在用户管理面板中展开后重置密码"
)

var legacyPublicBrandingTokens = []string{
	"说说笔记",
	"github.com/rcy1314",
	"github.com/ynby233",
	"ghcr.io/ynby233",
	"noise233/echo-noise",
	"rcy1314",
	"ynby233",
	"noisework.cn",
	"noiseblogs.top",
	"liangwenhao3",
	"ech0-noise",
	"echo-noise",
}

var absolutePublicURLPattern = regexp.MustCompile(`(?i)https?://[^\s<>"')\]]+`)

func containsLegacyPublicBranding(value string) bool {
	lower := strings.ToLower(value)
	for _, token := range legacyPublicBrandingTokens {
		if strings.Contains(lower, strings.ToLower(token)) {
			return true
		}
	}
	return false
}

// scrubTraceableLocalURLs keeps same-site links functional while removing
// branded hostnames that can reveal the project source. Echo-branded hosts are
// always made relative; other legacy hosts are changed only for /api/ paths.
func scrubTraceableLocalURLs(value string) (string, bool) {
	changed := false
	scrubbed := absolutePublicURLPattern.ReplaceAllStringFunc(value, func(raw string) string {
		parsed, err := url.Parse(raw)
		if err != nil {
			return raw
		}
		host := strings.ToLower(parsed.Hostname())
		isEchoHost := strings.Contains(host, "echo-noise") || strings.Contains(host, "ech0-noise")
		isLegacyAPIHost := strings.HasPrefix(parsed.EscapedPath(), "/api/") && containsLegacyPublicBranding(host)
		if !isEchoHost && !isLegacyAPIHost {
			return raw
		}
		replacement := parsed.RequestURI()
		if parsed.Fragment != "" {
			replacement += "#" + parsed.EscapedFragment()
		}
		if replacement == "" {
			return raw
		}
		changed = true
		return replacement
	})
	return scrubbed, changed
}

func scrubTraceableMessageContent(value string) (string, bool) {
	scrubbed, changed := scrubTraceableLocalURLs(value)
	if strings.Contains(scrubbed, legacyFullImageMarker) {
		scrubbed = strings.ReplaceAll(scrubbed, legacyFullImageMarker, neutralFullImageMarker)
		changed = true
	}
	return scrubbed, changed
}

func replaceExact(value *string, replacements map[string]string) bool {
	if replacement, ok := replacements[strings.TrimSpace(*value)]; ok {
		*value = replacement
		return true
	}
	return false
}

func scrubLegacySiteConfigValues(config *models.SiteConfig) bool {
	if config == nil {
		return false
	}

	changed := false
	for field, replacements := range map[*string]map[string]string{
		&config.SiteTitle: {
			"说说笔记":       neutralSiteTitle,
			"Noise的说说笔记": neutralSiteTitle,
		},
		&config.Username: {
			"Noise": neutralOwnerName,
		},
		&config.Description: {
			"执迷不悟": neutralDescription,
		},
		&config.RSSTitle: {
			"说说笔记":       neutralRSSTitle,
			"Noise的说说笔记": neutralRSSTitle,
		},
		&config.RSSDescription: {
			"一个说说笔记~": neutralRSSDescription,
		},
		&config.RSSAuthorName: {
			"Noise": neutralOwnerName,
		},
		&config.PwaTitle: {
			"说说笔记": neutralSiteTitle,
		},
		&config.PwaDescription: {
			"一个丰富的个人说说笔记": neutralPwaDescription,
		},
		&config.AnnouncementText: {
			"欢迎访问我的说说笔记！": neutralAnnouncement,
		},
		&config.WelcomeName: {
			"Noise": neutralOwnerName,
		},
		&config.WelcomeDescription: {
			"执迷不悟": neutralDescription,
		},
		&config.AvatarURL: {
			legacyDefaultAvatarURL: neutralAvatarURL,
		},
		&config.WelcomeAvatarURL: {
			legacyDefaultAvatarURL: neutralAvatarURL,
		},
	} {
		changed = replaceExact(field, replacements) || changed
	}

	if containsLegacyPublicBranding(config.PageFooterHTML) {
		config.PageFooterHTML = ""
		changed = true
	}

	if scrubbed, updated := scrubLegacyLinkList(config.SocialLinks); updated {
		config.SocialLinks = scrubbed
		changed = true
	}
	if scrubbed, updated := scrubLegacyAds(config.LeftAds); updated {
		config.LeftAds = scrubbed
		changed = true
	}
	if scrubbed, updated := scrubLegacyFeedSources(config.FeedSources); updated {
		config.FeedSources = scrubbed
		changed = true
	}
	if scrubbed, updated := scrubLegacyBackgrounds(config.Backgrounds); updated {
		config.Backgrounds = scrubbed
		changed = true
	}

	return changed
}

func scrubLegacyBackgrounds(raw string) (string, bool) {
	if strings.TrimSpace(raw) == "" {
		return raw, false
	}
	var entries []interface{}
	if err := json.Unmarshal([]byte(raw), &entries); err != nil {
		return raw, false
	}
	changed := false
	for index, entry := range entries {
		switch value := entry.(type) {
		case string:
			if _, legacy := legacyDefaultHeaderImageURLs[strings.TrimSpace(value)]; legacy && value != defaultHeaderImageURL {
				entries[index] = defaultHeaderImageURL
				changed = true
			}
		case map[string]interface{}:
			for _, key := range []string{"url", "URL"} {
				url, ok := value[key].(string)
				if !ok {
					continue
				}
				if _, legacy := legacyDefaultHeaderImageURLs[strings.TrimSpace(url)]; legacy && url != defaultHeaderImageURL {
					value[key] = defaultHeaderImageURL
					changed = true
				}
			}
		}
	}
	if !changed {
		return raw, false
	}
	encoded, err := json.Marshal(entries)
	if err != nil {
		return raw, false
	}
	return string(encoded), true
}

func scrubLegacyLinkList(raw string) (string, bool) {
	if strings.TrimSpace(raw) == "" {
		return raw, false
	}
	var entries []map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &entries); err != nil {
		if containsLegacyPublicBranding(raw) {
			return "[]", true
		}
		return raw, false
	}
	filtered := make([]map[string]interface{}, 0, len(entries))
	changed := false
	for _, entry := range entries {
		url := strings.TrimSpace(toString(pickAny(entry, "url", "URL", "link", "Link")))
		if containsLegacyPublicBranding(url) {
			changed = true
			continue
		}
		filtered = append(filtered, entry)
	}
	if !changed {
		return raw, false
	}
	encoded, err := json.Marshal(filtered)
	if err != nil {
		return "[]", true
	}
	return string(encoded), true
}

func scrubLegacyAds(raw string) (string, bool) {
	if strings.TrimSpace(raw) == "" {
		return raw, false
	}
	var entries []map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &entries); err != nil {
		if containsLegacyPublicBranding(raw) {
			return "[]", true
		}
		return raw, false
	}
	changed := false
	for _, entry := range entries {
		link := strings.TrimSpace(toString(pickAny(entry, "linkURL", "LinkURL")))
		if containsLegacyPublicBranding(link) {
			entry["linkURL"] = ""
			delete(entry, "LinkURL")
			changed = true
		}
	}
	if !changed {
		return raw, false
	}
	encoded, err := json.Marshal(entries)
	if err != nil {
		return "[]", true
	}
	return string(encoded), true
}

func scrubLegacyFeedSources(raw string) (string, bool) {
	if strings.TrimSpace(raw) == "" {
		return raw, false
	}
	var entries []map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &entries); err != nil {
		if containsLegacyPublicBranding(raw) {
			return "[]", true
		}
		return raw, false
	}
	changed := false
	for _, entry := range entries {
		originalType := strings.TrimSpace(toString(pickAny(entry, "type", "Type")))
		lowerType := strings.ToLower(originalType)
		if containsLegacyPublicBranding(originalType) || lowerType == "custom" || lowerType == "本项目api" || lowerType == "本项目 api" {
			entry["type"] = "note"
			delete(entry, "Type")
			changed = true
		}
		name := strings.TrimSpace(toString(pickAny(entry, "name", "Name")))
		if containsLegacyPublicBranding(name) {
			entry["name"] = "本站内容"
			delete(entry, "Name")
			changed = true
		}
	}
	if !changed {
		return raw, false
	}
	encoded, err := json.Marshal(entries)
	if err != nil {
		return "[]", true
	}
	return string(encoded), true
}

func toString(value interface{}) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	raw, _ := json.Marshal(value)
	return string(raw)
}

func scrubPersistedLegacyPublicBranding(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var config models.SiteConfig
		if err := tx.Table("site_configs").First(&config).Error; err == nil {
			if scrubLegacySiteConfigValues(&config) {
				if err := tx.Save(&config).Error; err != nil {
					return err
				}
			}
		} else if err != gorm.ErrRecordNotFound {
			return err
		}

		for _, token := range []string{"noisework.cn", "noiseblogs.top", "github.com/rcy1314", "github.com/ynby233"} {
			if err := tx.Where("LOWER(link) LIKE ?", "%"+token+"%").Delete(&models.FriendLink{}).Error; err != nil {
				return err
			}
		}

		if err := tx.Model(&models.Message{}).
			Where("content = ?", legacyWelcomeMessage).
			Update("content", neutralWelcomeMessage).Error; err != nil {
			return err
		}

		if err := tx.Model(&models.User{}).
			Where("avatar_url = ?", legacyDefaultAvatarURL).
			Update("avatar_url", neutralAvatarURL).Error; err != nil {
			return err
		}

		for content, imageURL := range map[string]string{
			"这里有一些关于自己的美好记录。 #日记 #示例": "https://s2.loli.net/2025/12/16/nsROlxQD5EPZq6h.jpg",
			"探索未知的世界。 #Travel":        "https://s2.loli.net/2025/04/05/EnakPbZJjpGxRTw.jpg",
		} {
			if err := tx.Model(&models.Message{}).
				Where("content = ? AND image_url = ?", content, imageURL).
				Update("image_url", "").Error; err != nil {
				return err
			}
		}

		var messages []models.Message
		if err := tx.Select("id", "content", "image_url").
			Where("content LIKE ? OR content LIKE ? OR content LIKE ? OR content LIKE ? OR image_url LIKE ? OR image_url LIKE ? OR image_url LIKE ?",
				"%noise-full-image-attachments%", "%://%/api/%", "%echo-noise%", "%ech0-noise%", "%://%/api/%", "%echo-noise%", "%ech0-noise%").
			Find(&messages).Error; err != nil {
			return err
		}
		for _, message := range messages {
			updates := map[string]interface{}{}
			if content, changed := scrubTraceableMessageContent(message.Content); changed {
				updates["content"] = content
			}
			if imageURL, changed := scrubTraceableLocalURLs(message.ImageURL); changed {
				updates["image_url"] = imageURL
			}
			if len(updates) > 0 {
				if err := tx.Model(&models.Message{}).Where("id = ?", message.ID).Updates(updates).Error; err != nil {
					return err
				}
			}
		}

		var comments []models.Comment
		if err := tx.Select("id", "content").Where("content LIKE ? OR content LIKE ? OR content LIKE ?", "%://%/api/%", "%echo-noise%", "%ech0-noise%").Find(&comments).Error; err != nil {
			return err
		}
		for _, comment := range comments {
			if content, changed := scrubTraceableLocalURLs(comment.Content); changed {
				if err := tx.Model(&models.Comment{}).Where("id = ?", comment.ID).Update("content", content).Error; err != nil {
					return err
				}
			}
		}

		return nil
	})
}
