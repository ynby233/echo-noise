package services

import (
	"fmt"
	"strings"
)

// Feed configuration parsing normalizes source rows and source-type aliases.
func parseInfoFeedSources(raw interface{}) []InfoFeedSource {
	out := make([]InfoFeedSource, 0)
	switch arr := raw.(type) {
	case []interface{}:
		for _, it := range arr {
			m, ok := it.(map[string]interface{})
			if !ok {
				continue
			}
			src := InfoFeedSource{
				Type:    normalizeFeedSourceTypeValue(m["type"]),
				Group:   strings.TrimSpace(fmt.Sprintf("%v", m["group"])),
				Name:    strings.TrimSpace(fmt.Sprintf("%v", m["name"])),
				URL:     strings.TrimSpace(fmt.Sprintf("%v", m["url"])),
				Enabled: toBool(firstNonEmpty(fmt.Sprintf("%v", m["enabled"]), "true")),
				Visible: toBool(firstNonEmpty(fmt.Sprintf("%v", m["visible"]), "true")),
			}
			if src.URL == "" {
				continue
			}
			if src.Type == "" {
				src.Type = "rss"
			}
			if strings.TrimSpace(src.Group) == "" {
				src.Group = "默认分组"
			}
			if fmt.Sprintf("%v", m["enabled"]) == "<nil>" {
				src.Enabled = true
			}
			if fmt.Sprintf("%v", m["visible"]) == "<nil>" {
				src.Visible = true
			}
			out = append(out, src)
		}
	case []map[string]interface{}:
		for _, m := range arr {
			src := InfoFeedSource{
				Type:    normalizeFeedSourceTypeValue(m["type"]),
				Group:   strings.TrimSpace(fmt.Sprintf("%v", m["group"])),
				Name:    strings.TrimSpace(fmt.Sprintf("%v", m["name"])),
				URL:     strings.TrimSpace(fmt.Sprintf("%v", m["url"])),
				Enabled: toBool(firstNonEmpty(fmt.Sprintf("%v", m["enabled"]), "true")),
				Visible: toBool(firstNonEmpty(fmt.Sprintf("%v", m["visible"]), "true")),
			}
			if src.URL == "" {
				continue
			}
			if src.Type == "" {
				src.Type = "rss"
			}
			if strings.TrimSpace(src.Group) == "" {
				src.Group = "默认分组"
			}
			if fmt.Sprintf("%v", m["enabled"]) == "<nil>" {
				src.Enabled = true
			}
			if fmt.Sprintf("%v", m["visible"]) == "<nil>" {
				src.Visible = true
			}
			out = append(out, src)
		}
	}
	return out
}

func normalizeFeedSourceTypeValue(raw interface{}) string {
	candidate := raw
	if obj, ok := raw.(map[string]interface{}); ok {
		candidate = firstNonEmpty(
			readStringByKeys(obj, "value"),
			readStringByKeys(obj, "type"),
			readStringByKeys(obj, "label"),
		)
	}
	t := strings.ToLower(strings.TrimSpace(fmt.Sprintf("%v", candidate)))
	switch t {
	case "rss":
		return "rss"
	case "note", "custom", "说说笔记", "本项目api", "本项目 api":
		return "note"
	case "ech0":
		return "ech0"
	case "memos":
		return "memos"
	case "mastodon":
		return "mastodon"
	default:
		return "rss"
	}
}
