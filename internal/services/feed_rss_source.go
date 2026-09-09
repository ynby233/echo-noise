package services

import (
	"fmt"
	"io"
	"net/http"
	"strings"
)

// RSS fetching translates one RSS or Atom endpoint into normalized feed items.
func fetchRSSSource(client *http.Client, source InfoFeedSource, limit int) ([]InfoFeedItem, error) {
	if !strings.HasPrefix(source.URL, "http://") && !strings.HasPrefix(source.URL, "https://") {
		return nil, fmt.Errorf("rss 源地址无效: %s", source.URL)
	}
	req, _ := http.NewRequest(http.MethodGet, source.URL, nil)
	req.Header.Set("User-Agent", "Echo-Noise-InfoFlow/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("rss 请求失败: %d", resp.StatusCode)
	}
	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 3*1024*1024))
	if err != nil {
		return nil, err
	}
	body := string(bodyBytes)
	isAtom := strings.Contains(body, "<feed") || strings.Contains(body, "<entry")
	sourceName := strings.TrimSpace(source.Name)
	if sourceName == "" {
		sourceName = extractTagValue(body, "title")
	}
	if sourceName == "" {
		sourceName = "RSS"
	}

	blocks := reItemRss.FindAllStringSubmatch(body, -1)
	if isAtom {
		blocks = reItemAtom.FindAllStringSubmatch(body, -1)
	}
	items := make([]InfoFeedItem, 0, len(blocks))
	for _, m := range blocks {
		if len(m) < 2 {
			continue
		}
		raw := m[1]
		title := cleanText(extractTagValue(raw, "title"))
		link := extractFeedItemLink(raw)
		if link == "" {
			link = cleanText(firstNonEmpty(
				extractTagValue(raw, "guid"),
				extractTagValue(raw, "id"),
			))
		}
		link = resolveRelativeURLWithSource(source.URL, link)
		pub := cleanText(firstNonEmpty(
			extractTagValue(raw, "pubDate"),
			extractTagValue(raw, "published"),
			extractTagValue(raw, "updated"),
			extractTagValueByLocalName(raw, "pubDate"),
			extractTagValueByLocalName(raw, "published"),
			extractTagValueByLocalName(raw, "updated"),
		))
		rawContent := extractRSSItemContent(raw)
		content := normalizeRawContent(rawContent)
		if content == "" {
			content = cleanText(rawContent)
		}
		content = normalizeContentForCard(content)
		if title == "" || link == "" {
			continue
		}
		img := resolveRelativeURLWithSource(source.URL, firstImage(raw, rawContent))
		tm := parseAnyTime(pub)
		items = append(items, InfoFeedItem{
			Title:       title,
			Link:        link,
			Content:     content,
			Summary:     content,
			ImageURL:    img,
			Source:      sourceName,
			Type:        "rss",
			PublishedAt: formatTime(tm, pub),
			Timestamp:   tm.Unix(),
		})
	}
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}
