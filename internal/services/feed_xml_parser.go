package services

import (
	"fmt"
	"html"
	"regexp"
	"strings"
)

// XML helpers parse RSS and Atom item fields without owning network behavior.
func extractTagValue(text, tag string) string {
	pattern := fmt.Sprintf(`(?is)<%s\b[^>]*>(.*?)</%s>`, regexp.QuoteMeta(tag), regexp.QuoteMeta(tag))
	re := regexp.MustCompile(pattern)
	m := re.FindStringSubmatch(text)
	if len(m) < 2 {
		return ""
	}
	val := strings.TrimSpace(m[1])
	if c := reCData.FindStringSubmatch(val); len(c) > 1 {
		val = c[1]
	}
	return val
}

func extractTagValueByLocalName(text, localName string) string {
	name := strings.TrimSpace(localName)
	if name == "" {
		return ""
	}
	pattern := fmt.Sprintf(`(?is)<(?:[\w-]+:)?%s\b[^>]*>(.*?)</(?:[\w-]+:)?%s>`, regexp.QuoteMeta(name), regexp.QuoteMeta(name))
	re := regexp.MustCompile(pattern)
	m := re.FindStringSubmatch(text)
	if len(m) < 2 {
		return ""
	}
	val := strings.TrimSpace(m[1])
	if c := reCData.FindStringSubmatch(val); len(c) > 1 {
		val = c[1]
	}
	return val
}

func extractTagAttrValueByLocalName(text, localName, attr string) string {
	name := strings.TrimSpace(localName)
	a := strings.TrimSpace(attr)
	if name == "" || a == "" {
		return ""
	}
	tagPattern := fmt.Sprintf(`(?is)<(?:[\w-]+:)?%s\b([^>]*)/?>`, regexp.QuoteMeta(name))
	tagRe := regexp.MustCompile(tagPattern)
	matches := tagRe.FindAllStringSubmatch(text, -1)
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		attrs := m[1]
		attrPattern := fmt.Sprintf(`(?is)\b%s\s*=\s*["']([^"']+)["']`, regexp.QuoteMeta(a))
		attrRe := regexp.MustCompile(attrPattern)
		if hit := attrRe.FindStringSubmatch(attrs); len(hit) > 1 {
			return strings.TrimSpace(html.UnescapeString(hit[1]))
		}
	}
	return ""
}

func extractRSSItemContent(raw string) string {
	// 优先命中正文语义最强的字段。
	for _, key := range []string{"content:encoded", "encoded", "content", "description", "summary", "media:description"} {
		if val := extractTagValue(raw, key); strings.TrimSpace(val) != "" {
			return strings.TrimSpace(val)
		}
	}
	for _, key := range []string{"encoded", "content", "description", "summary"} {
		if val := extractTagValueByLocalName(raw, key); strings.TrimSpace(val) != "" {
			return strings.TrimSpace(val)
		}
	}
	// 一些 Atom/RSS 变体会把文本放在标签属性中（例如 value）。
	for _, key := range []string{"content", "summary", "description"} {
		for _, attr := range []string{"value", "src"} {
			if val := extractTagAttrValueByLocalName(raw, key, attr); strings.TrimSpace(val) != "" {
				return strings.TrimSpace(val)
			}
		}
	}
	// 最后兜底：去掉常见元数据标签后提取剩余文本，避免卡片只剩标题。
	fallback := raw
	for _, key := range []string{"title", "link", "guid", "id", "pubDate", "published", "updated", "author", "category"} {
		pattern := fmt.Sprintf(`(?is)<(?:[\w-]+:)?%s\b[^>]*>.*?</(?:[\w-]+:)?%s>`, regexp.QuoteMeta(key), regexp.QuoteMeta(key))
		fallback = regexp.MustCompile(pattern).ReplaceAllString(fallback, " ")
	}
	fallback = cleanContentText(fallback)
	return strings.TrimSpace(fallback)
}

func extractFeedItemLink(raw string) string {
	link := cleanText(extractTagValue(raw, "link"))
	if link != "" {
		return link
	}
	linkTags := reLinkTag.FindAllString(raw, -1)
	for _, tag := range linkTags {
		href := strings.TrimSpace(extractAttrValue(tag, "href"))
		if href == "" {
			continue
		}
		rel := strings.ToLower(strings.TrimSpace(extractAttrValue(tag, "rel")))
		if rel == "" || rel == "alternate" {
			return href
		}
	}
	if href := reHref.FindStringSubmatch(raw); len(href) > 1 {
		return strings.TrimSpace(href[1])
	}
	return ""
}

func extractAttrValue(rawTag, attr string) string {
	if strings.TrimSpace(rawTag) == "" || strings.TrimSpace(attr) == "" {
		return ""
	}
	pattern := fmt.Sprintf(`(?is)\b%s\s*=\s*["']([^"']+)["']`, regexp.QuoteMeta(attr))
	re := regexp.MustCompile(pattern)
	if match := re.FindStringSubmatch(rawTag); len(match) > 1 {
		return strings.TrimSpace(match[1])
	}
	return ""
}
