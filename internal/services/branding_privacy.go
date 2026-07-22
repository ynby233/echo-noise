package services

import (
	"encoding/json"
	"strings"

	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
)

const (
	neutralSiteTitle      = "个人站点"
	neutralOwnerName      = "站长"
	neutralDescription    = "欢迎访问"
	neutralRSSTitle       = "个人内容订阅"
	neutralRSSDescription = "个人内容更新"
	neutralPwaDescription = "个人内容与记录"
	neutralAnnouncement   = "欢迎访问！"
	legacyWelcomeMessage  = "欢迎来到说说笔记！默认用户名及密码均为admin，记得到后台页修改你的用户名或密码,密码带有加强设置，如需简单密码可在用户管理面板中展开后重置密码"
	neutralWelcomeMessage = "欢迎来到你的个人站点！默认用户名及密码均为admin，记得到后台页修改你的用户名或密码,密码带有加强设置，如需简单密码可在用户管理面板中展开后重置密码"
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

func containsLegacyPublicBranding(value string) bool {
	lower := strings.ToLower(value)
	for _, token := range legacyPublicBrandingTokens {
		if strings.Contains(lower, strings.ToLower(token)) {
			return true
		}
	}
	return false
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

	return changed
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

		return nil
	})
}
