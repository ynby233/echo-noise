package services

import (
	"fmt"
	"net/http"
	"strings"
)

// Mastodon fetching owns account discovery and status mapping.
func fetchMastodonSource(client *http.Client, source InfoFeedSource, limit int) ([]InfoFeedItem, error) {
	base, statusesURL, accountName, err := resolveMastodonStatusesURL(client, source.URL, limit)
	if err != nil {
		return nil, err
	}
	payload, err := fetchJSON(client, feedFetchRequest{Method: http.MethodGet, URL: statusesURL}, "mastodon")
	if err != nil {
		return nil, err
	}
	rows := extractRows(payload)
	sourceName := strings.TrimSpace(source.Name)
	if sourceName == "" {
		sourceName = strings.TrimSpace(accountName)
	}
	if sourceName == "" {
		sourceName = "Mastodon"
	}
	items := make([]InfoFeedItem, 0, len(rows))
	for _, row := range rows {
		m, ok := row.(map[string]interface{})
		if !ok {
			continue
		}
		contentHTML := readStringByKeys(m, "content")
		content := normalizeRawContent(contentHTML)
		if content == "" {
			content = cleanText(contentHTML)
		}
		content = normalizeContentForCard(content)
		if cw := strings.TrimSpace(readStringByKeys(m, "spoiler_text")); cw != "" {
			if content == "" {
				content = cw
			} else {
				content = cw + "\n\n" + content
			}
		}
		link := firstNonEmpty(readStringByKeys(m, "url"), readStringByKeys(m, "uri"))
		if link == "" {
			continue
		}
		imageURL := firstNonEmpty(
			readStringByKeys(readMapByKey(m, "card"), "image"),
			readStringByKeys(firstMapFromArray(readArrayByKeys(m, "media_attachments")), "preview_url", "url"),
		)
		imageURL = resolveRelativeURL(base, imageURL)
		title := limitText(firstLine(cleanContentText(content)), 80)
		if title == "" {
			title = limitText(firstLine(content), 80)
		}
		if title == "" {
			title = "Mastodon 动态"
		}
		pubRaw := readStringByKeys(m, "created_at")
		tm := parseAnyTime(pubRaw)
		items = append(items, InfoFeedItem{
			Title:       title,
			Link:        link,
			Content:     content,
			Summary:     content,
			ImageURL:    imageURL,
			Source:      sourceName,
			Type:        "mastodon",
			Author:      cleanText(firstNonEmpty(readStringByKeys(m, "account.display_name"), readStringByKeys(m, "account.username"), readStringByKeys(m, "account.acct"))),
			AvatarURL:   resolveRelativeURL(base, firstNonEmpty(readStringByKeys(m, "account.avatar_static"), readStringByKeys(m, "account.avatar"))),
			PublishedAt: formatTime(tm, pubRaw),
			Timestamp:   tm.Unix(),
		})
		if len(items) >= limit {
			break
		}
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("mastodon 接口未返回可用内容")
	}
	return items, nil
}
