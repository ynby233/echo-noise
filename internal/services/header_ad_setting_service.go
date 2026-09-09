package services

import (
	"encoding/json"
	"fmt"
	"github.com/rcy1314/echo-noise/internal/models"
	"strings"
)

// Header and advertisement settings normalize legacy backgrounds and presentation fields.
const defaultHeaderImageURL = "https://picsum.photos/1600/500"

const (
	defaultAdTextColor       = "#ffffff"
	defaultAdTextDisplayMode = "hover"
)

const (
	defaultLoginExpireDays  = 3
	defaultLoginExpireHours = 0
	maxLoginExpireDays      = 31
	maxLoginExpireHours     = 24
)

var legacyDefaultHeaderImageURLs = map[string]struct{}{
	"https://s2.loli.net/2025/03/26/d7iyuPYA8cRqD1K.jpg": {},
	"https://s2.loli.net/2025/03/27/KJ1trnU2ksbFEYM.jpg": {},
	"https://s2.loli.net/2025/03/27/MZqaLczCvwjSmW7.jpg": {},
	"https://s2.loli.net/2025/03/27/UMijKXwJ9yTqSeE.jpg": {},
	"https://s2.loli.net/2025/03/27/WJQIlkXvBg2afcR.jpg": {},
	"https://s2.loli.net/2025/03/27/oHNQtf4spkq2iln.jpg": {},
	"https://s2.loli.net/2025/03/27/PMRuX5loc6Uaimw.jpg": {},
	"https://s2.loli.net/2025/03/27/U2WIslbNyTLt4rD.jpg": {},
	"https://s2.loli.net/2025/03/27/xu1jZL5Og4pqT9d.jpg": {},
	"https://s2.loli.net/2025/03/27/OXqwzZ6v3PVIns9.jpg": {},
	"https://s2.loli.net/2025/03/27/HGuqlE6apgNywbh.jpg": {},
	"https://s2.loli.net/2025/03/27/7Zck3y6XTzhYPs5.jpg": {},
	"https://s2.loli.net/2025/03/27/wYy12qDMH6bGJOI.jpg": {},
	"https://s2.loli.net/2025/03/27/y67m2k5xcSdTsHN.jpg": {},
	defaultHeaderImageURL:                                {},
}

func defaultHeaderImages() []string {
	return []string{defaultHeaderImageURL}
}

func defaultHeaderImagesJSON() string {
	data, err := json.Marshal(defaultHeaderImages())
	if err != nil {
		return `["` + defaultHeaderImageURL + `"]`
	}
	return string(data)
}

func shouldCollapseLegacyBackgrounds(backgrounds []string) bool {
	hasBackground := false
	for _, raw := range backgrounds {
		url := strings.TrimSpace(raw)
		if url == "" {
			continue
		}
		hasBackground = true
		if _, ok := legacyDefaultHeaderImageURLs[url]; !ok {
			return false
		}
	}
	return hasBackground
}

func defaultHeaderBackgroundConfigs() []models.HeaderBackground {
	defaults := getDefaultConfig()["frontendSettings"].(map[string]interface{})["backgrounds"].([]string)
	backgrounds := make([]models.HeaderBackground, 0, len(defaults))
	for _, url := range defaults {
		if trimmed := strings.TrimSpace(url); trimmed != "" {
			backgrounds = append(backgrounds, models.HeaderBackground{URL: trimmed, TitleOpacity: 1, SubtitleOpacity: 1})
		}
	}
	return backgrounds
}

func clampHeaderTextOpacity(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func normalizeHeaderBackground(raw interface{}) (models.HeaderBackground, bool) {
	switch bg := raw.(type) {
	case string:
		url := strings.TrimSpace(bg)
		if url == "" {
			return models.HeaderBackground{}, false
		}
		return models.HeaderBackground{URL: url, TitleOpacity: 1, SubtitleOpacity: 1}, true
	case map[string]interface{}:
		url, _ := bg["url"].(string)
		url = strings.TrimSpace(url)
		if url == "" {
			return models.HeaderBackground{}, false
		}
		normalized := models.HeaderBackground{URL: url, TitleOpacity: 1, SubtitleOpacity: 1}
		if v, ok := bg["titleColor"].(string); ok {
			normalized.TitleColor = strings.TrimSpace(v)
		}
		if v, ok := bg["subtitleColor"].(string); ok {
			normalized.SubtitleColor = strings.TrimSpace(v)
		}
		if v, ok := bg["titleOpacity"].(float64); ok {
			normalized.TitleOpacity = clampHeaderTextOpacity(v)
		}
		if v, ok := bg["subtitleOpacity"].(float64); ok {
			normalized.SubtitleOpacity = clampHeaderTextOpacity(v)
		}
		return normalized, true
	case models.HeaderBackground:
		bg.URL = strings.TrimSpace(bg.URL)
		if bg.URL == "" {
			return models.HeaderBackground{}, false
		}
		bg.TitleColor = strings.TrimSpace(bg.TitleColor)
		bg.SubtitleColor = strings.TrimSpace(bg.SubtitleColor)
		bg.TitleOpacity = clampHeaderTextOpacity(bg.TitleOpacity)
		bg.SubtitleOpacity = clampHeaderTextOpacity(bg.SubtitleOpacity)
		return bg, true
	default:
		return models.HeaderBackground{}, false
	}
}

func normalizeHeaderBackgrounds(raw interface{}) []models.HeaderBackground {
	var backgrounds []models.HeaderBackground
	switch list := raw.(type) {
	case []interface{}:
		backgrounds = make([]models.HeaderBackground, 0, len(list))
		for _, item := range list {
			if bg, ok := normalizeHeaderBackground(item); ok {
				backgrounds = append(backgrounds, bg)
			}
		}
	case []string:
		backgrounds = make([]models.HeaderBackground, 0, len(list))
		for _, item := range list {
			if bg, ok := normalizeHeaderBackground(item); ok {
				backgrounds = append(backgrounds, bg)
			}
		}
	case []models.HeaderBackground:
		backgrounds = make([]models.HeaderBackground, 0, len(list))
		for _, item := range list {
			if bg, ok := normalizeHeaderBackground(item); ok {
				backgrounds = append(backgrounds, bg)
			}
		}
	}
	if len(backgrounds) == 0 {
		return defaultHeaderBackgroundConfigs()
	}
	return backgrounds
}

func normalizeAdTextColor(raw interface{}) string {
	value := strings.ToLower(strings.TrimSpace(fmt.Sprintf("%v", raw)))
	if len(value) != 7 || value[0] != '#' {
		return defaultAdTextColor
	}
	for _, char := range value[1:] {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return defaultAdTextColor
		}
	}
	return value
}

func normalizeAdTextDisplayMode(raw interface{}) string {
	if strings.EqualFold(strings.TrimSpace(fmt.Sprintf("%v", raw)), "always") {
		return "always"
	}
	return defaultAdTextDisplayMode
}

func normalizeLeftAds(raw interface{}) []map[string]string {
	var items []map[string]interface{}
	switch value := raw.(type) {
	case string:
		if strings.TrimSpace(value) == "" || json.Unmarshal([]byte(value), &items) != nil {
			return []map[string]string{}
		}
	default:
		encoded, err := json.Marshal(value)
		if err != nil || json.Unmarshal(encoded, &items) != nil {
			return []map[string]string{}
		}
	}

	normalized := make([]map[string]string, 0, len(items))
	for _, item := range items {
		imageURL := strings.TrimSpace(fmt.Sprintf("%v", pickAny(item, "imageURL", "ImageURL")))
		if imageURL == "" || imageURL == "<nil>" {
			continue
		}
		normalized = append(normalized, map[string]string{
			"imageURL":        imageURL,
			"linkURL":         strings.TrimSpace(fmt.Sprintf("%v", pickAny(item, "linkURL", "LinkURL"))),
			"description":     strings.TrimSpace(fmt.Sprintf("%v", pickAny(item, "description", "Description"))),
			"textColor":       normalizeAdTextColor(pickAny(item, "textColor", "TextColor")),
			"textDisplayMode": normalizeAdTextDisplayMode(pickAny(item, "textDisplayMode", "TextDisplayMode")),
		})
	}
	return normalized
}
