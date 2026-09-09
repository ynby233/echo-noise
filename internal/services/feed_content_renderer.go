package services

import (
	"html"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Content normalization builds safe card text, resource links and item presentation metadata.
func cleanText(raw string) string {
	s := strings.TrimSpace(raw)
	s = html.UnescapeString(s)
	s = reTag.ReplaceAllString(s, " ")
	s = reSpaces.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

func cleanContentText(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	s = html.UnescapeString(s)
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = reTag.ReplaceAllString(s, "\n")

	lines := strings.Split(s, "\n")
	cleaned := make([]string, 0, len(lines))
	emptyCount := 0
	for _, line := range lines {
		line = reInlineSpaces.ReplaceAllString(strings.TrimSpace(line), " ")
		if line == "" {
			emptyCount++
			if emptyCount <= 1 {
				cleaned = append(cleaned, "")
			}
			continue
		}
		emptyCount = 0
		cleaned = append(cleaned, line)
	}
	out := strings.TrimSpace(strings.Join(cleaned, "\n"))
	if out == "" {
		return ""
	}
	return out
}

func normalizeRawContent(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	if c := reCData.FindStringSubmatch(s); len(c) > 1 {
		s = c[1]
	}
	s = html.UnescapeString(s)
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return strings.TrimSpace(s)
}

func normalizeContentForCard(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	if !strings.Contains(s, "<") || !strings.Contains(s, ">") {
		return s
	}

	// 将常见 HTML 链接与图片转为 markdown 形式，保证前端统一渲染。
	s = reAnchorTag.ReplaceAllStringFunc(s, func(one string) string {
		m := reAnchorTag.FindStringSubmatch(one)
		if len(m) < 3 {
			return one
		}
		href := strings.TrimSpace(html.UnescapeString(m[1]))
		text := strings.TrimSpace(cleanText(m[2]))
		if href == "" {
			return text
		}
		if text == "" && isImageResourceURL(strings.ToLower(href)) {
			return "![](" + href + ")"
		}
		if strings.EqualFold(text, href) && isImageResourceURL(strings.ToLower(href)) {
			return "![](" + href + ")"
		}
		if text == "" {
			text = href
		}
		return "[" + text + "](" + href + ")"
	})
	s = reImgSrc.ReplaceAllString(s, "\n\n![]($1)\n\n")
	s = strings.ReplaceAll(s, "<br>", "\n")
	s = strings.ReplaceAll(s, "<br/>", "\n")
	s = strings.ReplaceAll(s, "<br />", "\n")
	s = strings.ReplaceAll(s, "</p>", "\n\n")
	s = strings.ReplaceAll(s, "</div>", "\n")
	s = strings.ReplaceAll(s, "</li>", "\n")
	s = reLiOpen.ReplaceAllString(s, "- ")
	s = reTag.ReplaceAllString(s, " ")
	s = reOrphanCloser.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")

	lines := strings.Split(s, "\n")
	cleaned := make([]string, 0, len(lines))
	empty := 0
	for _, line := range lines {
		line = reInlineSpaces.ReplaceAllString(strings.TrimSpace(line), " ")
		if line == "" {
			empty++
			if empty <= 1 {
				cleaned = append(cleaned, "")
			}
			continue
		}
		empty = 0
		cleaned = append(cleaned, line)
	}
	return strings.TrimSpace(strings.Join(cleaned, "\n"))
}

func buildExtensionRenderableContent(m map[string]interface{}) string {
	extObj := readMapByKey(m, "extension")
	if extObj == nil {
		extObj = readMapByKey(m, "echo.extension")
	}
	if extObj == nil {
		return ""
	}
	extType := strings.ToUpper(strings.TrimSpace(readStringByKeys(extObj, "type")))
	payload := readMapByKey(extObj, "payload")
	if payload == nil {
		return ""
	}
	switch extType {
	case "MUSIC":
		return strings.TrimSpace(readStringByKeys(payload, "url"))
	case "VIDEO":
		videoID := strings.TrimSpace(readStringByKeys(payload, "videoId", "videoID", "url"))
		if videoID == "" {
			return ""
		}
		if strings.HasPrefix(videoID, "http://") || strings.HasPrefix(videoID, "https://") {
			return videoID
		}
		if strings.HasPrefix(strings.ToUpper(videoID), "BV") {
			return "https://www.bilibili.com/video/" + videoID
		}
		return "https://www.youtube.com/watch?v=" + videoID
	case "GITHUBPROJ":
		return strings.TrimSpace(readStringByKeys(payload, "repoUrl", "repoURL", "url"))
	case "WEBSITE":
		site := strings.TrimSpace(readStringByKeys(payload, "site", "url"))
		title := strings.TrimSpace(readStringByKeys(payload, "title", "name"))
		if site == "" {
			return ""
		}
		if title == "" {
			return site
		}
		return "[" + title + "](" + site + ")"
	default:
		return ""
	}
}

func buildResourcesRenderableContent(base string, m map[string]interface{}, resourcesKeys []string, existingContent string) string {
	if len(resourcesKeys) == 0 {
		return ""
	}
	existingContentLower := strings.ToLower(html.UnescapeString(strings.TrimSpace(existingContent)))
	seen := map[string]struct{}{}
	lines := make([]string, 0)
	for _, key := range resourcesKeys {
		arr := readArrayByKeys(m, key)
		for _, item := range arr {
			one, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			candidate := firstNonEmpty(
				readStringByKeys(one, "url"),
				readStringByKeys(one, "uri"),
				readStringByKeys(one, "file.url"),
				readStringByKeys(one, "file.uri"),
				readStringByKeys(one, "external_link"),
				readStringByKeys(one, "externalLink"),
				readStringByKeys(one, "image_url"),
				readStringByKeys(one, "preview_url"),
				readStringByKeys(one, "publicUrl"),
			)
			candidate = resolveRelativeURL(base, candidate)
			candidate = strings.TrimSpace(candidate)
			if candidate == "" {
				continue
			}
			if resourceURLAlreadyInContent(existingContentLower, candidate) {
				continue
			}
			if _, ok := seen[candidate]; ok {
				continue
			}
			seen[candidate] = struct{}{}
			lower := strings.ToLower(candidate)
			if strings.Contains(lower, ".mp4") || strings.Contains(lower, ".webm") ||
				strings.Contains(lower, ".mov") || strings.Contains(lower, ".avi") {
				lines = append(lines, candidate)
				continue
			}
			if isImageResourceURL(lower) {
				lines = append(lines, "![]("+candidate+")")
			}
		}
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func resourceURLAlreadyInContent(contentLower, candidate string) bool {
	if strings.TrimSpace(contentLower) == "" || strings.TrimSpace(candidate) == "" {
		return false
	}
	candidateLower := strings.ToLower(strings.TrimSpace(candidate))
	if candidateLower == "" {
		return false
	}
	if strings.Contains(contentLower, candidateLower) {
		return true
	}
	trimmed := candidateLower
	if idx := strings.Index(trimmed, "?"); idx >= 0 {
		trimmed = trimmed[:idx]
	}
	if idx := strings.Index(trimmed, "#"); idx >= 0 {
		trimmed = trimmed[:idx]
	}
	if trimmed != "" && trimmed != candidateLower && strings.Contains(contentLower, trimmed) {
		return true
	}
	return false
}

func isImageResourceURL(raw string) bool {
	if raw == "" {
		return false
	}
	base := raw
	if idx := strings.Index(base, "?"); idx >= 0 {
		base = base[:idx]
	}
	if idx := strings.Index(base, "#"); idx >= 0 {
		base = base[:idx]
	}
	for _, ext := range []string{".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp", ".svg", ".avif", ".heic", ".heif"} {
		if strings.HasSuffix(base, ext) {
			return true
		}
	}
	return false
}

func joinRenderableContent(parts ...string) string {
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return strings.TrimSpace(strings.Join(out, "\n\n"))
}

func firstImage(rawItem, content string) string {
	if m := reImgSrc.FindStringSubmatch(rawItem); len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	if m := reMediaContent.FindStringSubmatch(rawItem); len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	if m := reImgSrc.FindStringSubmatch(content); len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

func firstLine(s string) string {
	parts := strings.Split(s, "\n")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			return p
		}
	}
	return ""
}

func limitText(s string, max int) string {
	runes := []rune(strings.TrimSpace(s))
	if len(runes) <= max {
		return strings.TrimSpace(s)
	}
	return strings.TrimSpace(string(runes[:max])) + "..."
}

func parseAnyTime(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}
	if n, err := strconv.ParseInt(raw, 10, 64); err == nil {
		if len(raw) >= 13 {
			n = n / 1000
		}
		if n > 0 {
			return time.Unix(n, 0)
		}
		return time.Time{}
	}
	if f, err := strconv.ParseFloat(raw, 64); err == nil && f > 0 {
		n := int64(f)
		if len(strings.SplitN(raw, ".", 2)[0]) >= 13 {
			n = n / 1000
		}
		if n > 0 {
			return time.Unix(n, 0)
		}
		return time.Time{}
	}
	layouts := []string{
		time.RFC3339, time.RFC3339Nano, time.RFC1123Z, time.RFC1123,
		time.RFC822Z, time.RFC822, "2006-01-02 15:04:05", "2006-01-02T15:04:05Z07:00",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, raw); err == nil {
			return t
		}
	}
	return time.Time{}
}

func formatTime(t time.Time, fallback string) string {
	if t.IsZero() {
		return fallback
	}
	return t.Format("2006-01-02 15:04:05")
}

func toBool(v interface{}) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		s := strings.ToLower(strings.TrimSpace(x))
		return s == "true" || s == "1" || s == "yes"
	case float64:
		return int(x) == 1
	case int:
		return x == 1
	default:
		return false
	}
}

func toInt(v interface{}, def int) int {
	switch x := v.(type) {
	case int:
		return x
	case float64:
		return int(x)
	case string:
		n := strings.TrimSpace(x)
		if n == "" {
			return def
		}
		if p, err := strconv.Atoi(n); err == nil {
			return p
		}
		return def
	default:
		return def
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func looksLikeNoteEndpoint(u string) bool {
	return strings.Contains(u, "/api/messages") || strings.Contains(u, "#/messages")
}

func resolveSourceURL(baseURL, raw string) string {
	u := strings.TrimSpace(raw)
	if u == "" {
		return ""
	}
	if strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://") {
		return u
	}
	if strings.HasPrefix(u, "/") && strings.TrimSpace(baseURL) != "" {
		base, err := url.Parse(baseURL)
		if err != nil {
			return u
		}
		ref, err := url.Parse(u)
		if err != nil {
			return u
		}
		return base.ResolveReference(ref).String()
	}
	return u
}

func isLocalInfoFeedSource(baseURL, raw string) bool {
	candidate := strings.TrimSpace(raw)
	if candidate == "" {
		return false
	}
	if !isHTTPURL(candidate) {
		return strings.HasPrefix(candidate, "/")
	}

	parsed, err := url.Parse(candidate)
	if err != nil || parsed == nil {
		return false
	}
	hostname := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	if hostname == "localhost" || hostname == "127.0.0.1" || hostname == "::1" {
		return true
	}

	base, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || base == nil {
		return false
	}
	return strings.EqualFold(parsed.Host, base.Host)
}

func localInfoFeedURLPath(raw, sourceURL string) string {
	value := strings.TrimSpace(raw)
	if value == "" || !isHTTPURL(value) {
		return value
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed == nil {
		return value
	}
	source, err := url.Parse(strings.TrimSpace(sourceURL))
	if err != nil || source == nil {
		return value
	}
	hostname := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	loopback := hostname == "localhost" || hostname == "127.0.0.1" || hostname == "::1"
	if !loopback && !strings.EqualFold(parsed.Host, source.Host) {
		return value
	}
	path := parsed.EscapedPath()
	if path == "" {
		path = "/"
	}
	if parsed.RawQuery != "" {
		path += "?" + parsed.RawQuery
	}
	if parsed.Fragment != "" {
		path += "#" + parsed.EscapedFragment()
	}
	return path
}
