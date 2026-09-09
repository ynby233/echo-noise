package services

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

// Memos fetching owns identity enrichment and Memos-specific response mapping.
func fetchMemosSource(client *http.Client, source InfoFeedSource, limit int) ([]InfoFeedItem, error) {
	bases := candidateSourceBases(source.URL, "/api/v1/memos", "/api/v1/memo")
	if len(bases) == 0 {
		return nil, fmt.Errorf("memos 源地址无效: %s", source.URL)
	}
	base := ""
	var payload interface{}
	var err error
	for _, oneBase := range bases {
		if !isHTTPURL(oneBase) {
			continue
		}
		payload, err = fetchJSONFromCandidates(client, []feedFetchRequest{
			{
				Method: http.MethodGet,
				URL: fmt.Sprintf(
					"%s/api/v1/memos?pageSize=%d&filter=%s&orderBy=%s",
					oneBase,
					limit,
					url.QueryEscape(`visibility == "PUBLIC"`),
					url.QueryEscape("display_time desc"),
				),
			},
			{
				Method: http.MethodGet,
				URL: fmt.Sprintf(
					"%s/api/v1/memos?page_size=%d&filter=%s&order_by=%s",
					oneBase,
					limit,
					url.QueryEscape(`visibility == "PUBLIC"`),
					url.QueryEscape("display_time desc"),
				),
			},
			{Method: http.MethodGet, URL: fmt.Sprintf("%s/api/v1/memos?pageSize=%d", oneBase, limit)},
		}, "memos")
		if err == nil {
			base = oneBase
			break
		}
	}
	if err != nil {
		return nil, err
	}
	if base == "" {
		return nil, fmt.Errorf("memos 源地址无效: %s", source.URL)
	}
	rows := extractRows(payload)
	sourceName := strings.TrimSpace(source.Name)
	if sourceName == "" {
		sourceName = "Memos"
	}
	items := make([]InfoFeedItem, 0, len(rows))
	for _, row := range rows {
		m, ok := row.(map[string]interface{})
		if !ok {
			continue
		}
		rawContent := firstNonEmpty(
			readFlexibleTextByKeys(m, "content", "snippet", "memo.content", "memo.snippet", "body", "nodes", "memo.nodes", "property", "memo.property"),
			readStringByKeys(m, "content", "snippet", "memo.content", "memo.snippet", "body"),
		)
		content := normalizeRawContent(rawContent)
		if content == "" {
			content = cleanText(rawContent)
		}
		if content == "" {
			continue
		}
		title := limitText(firstLine(cleanContentText(content)), 80)
		if title == "" {
			title = limitText(firstLine(content), 80)
		}
		if title == "" {
			title = "Memos 动态"
		}
		name := strings.TrimSpace(readStringByKeys(m, "name", "memo.name"))
		link := strings.TrimSpace(readStringByKeys(m, "url", "link", "memo.url", "memo.link"))
		if link == "" && name != "" {
			link = strings.TrimRight(base, "/") + "/" + strings.TrimLeft(name, "/")
		}
		if link == "" {
			memoID := strings.TrimSpace(readStringByKeys(m, "uid", "id", "memo.uid", "memo.id"))
			if memoID != "" {
				link = strings.TrimRight(base, "/") + "/m/" + url.PathEscape(memoID)
			}
		}
		if link == "" {
			continue
		}
		pubRaw := firstNonEmpty(
			readStringByKeys(m, "displayTime"),
			readStringByKeys(m, "display_time"),
			readStringByKeys(m, "createTime"),
			readStringByKeys(m, "create_time"),
			readStringByKeys(m, "updateTime"),
			readStringByKeys(m, "memo.displayTime"),
			readStringByKeys(m, "memo.createTime"),
		)
		tm := parseAnyTime(pubRaw)
		img := extractImageFromResources(base, m, []string{"attachments", "resources", "memo.attachments", "memo.resources"})
		creatorRef := strings.TrimSpace(readStringByKeys(m, "creator", "memo.creator"))
		creatorIdentity := extractMemosUserIdentity(client, base, creatorRef)
		author := cleanText(firstNonEmpty(
			readStringByKeys(
				m,
				"creator.username",
				"creator.name",
				"creator.nickname",
				"creator.displayName",
				"creator.display_name",
				"user.username",
				"user.name",
				"user.nickname",
				"memo.creator.username",
				"memo.creator.name",
				"memo.creator.displayName",
				"memo.creator.display_name",
			),
			creatorIdentity.Name,
		))
		avatarURL := resolveRelativeURL(base, firstNonEmpty(
			readStringByKeys(m, "creator.avatarUrl", "creator.avatar_url", "creator.avatar", "creator.profile.avatarUrl"),
			readStringByKeys(m, "user.avatarUrl", "user.avatar_url", "user.avatar"),
			readStringByKeys(m, "memo.creator.avatarUrl", "memo.creator.avatar_url", "memo.creator.avatar"),
			creatorIdentity.Avatar,
		))
		items = append(items, InfoFeedItem{
			Title:       title,
			Link:        link,
			Content:     content,
			Summary:     content,
			ImageURL:    img,
			Source:      sourceName,
			Type:        "memos",
			Author:      author,
			AvatarURL:   avatarURL,
			PublishedAt: formatTime(tm, pubRaw),
			Timestamp:   tm.Unix(),
		})
		if len(items) >= limit {
			break
		}
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("memos 接口未返回可用内容")
	}
	return items, nil
}

type memosUserIdentity struct {
	Name   string
	Avatar string
}

var memosUserProfileCache sync.Map

func extractMemosUserIdentity(client *http.Client, base, creatorRef string) memosUserIdentity {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	creatorRef = strings.TrimSpace(creatorRef)
	if client == nil || base == "" || creatorRef == "" {
		return memosUserIdentity{}
	}
	cacheKey := base + "|" + creatorRef
	if cached, ok := memosUserProfileCache.Load(cacheKey); ok {
		if identity, ok := cached.(memosUserIdentity); ok {
			return identity
		}
	}
	userPath := creatorRef
	if strings.Contains(userPath, "/") {
		parts := strings.Split(userPath, "/")
		userPath = strings.TrimSpace(parts[len(parts)-1])
	}
	if userPath == "" {
		return memosUserIdentity{}
	}
	profileURL := fmt.Sprintf("%s/api/v1/users/%s", base, url.PathEscape(userPath))
	req, _ := http.NewRequest(http.MethodGet, profileURL, nil)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Echo-Noise-InfoFlow/1.0")
	resp, err := client.Do(req)
	if err != nil || resp == nil {
		return memosUserIdentity{}
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return memosUserIdentity{}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		return memosUserIdentity{}
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return memosUserIdentity{}
	}
	identity := memosUserIdentity{
		Name: cleanText(firstNonEmpty(
			readStringByKeys(payload, "displayName", "display_name", "nickname", "username", "name"),
		)),
		Avatar: resolveRelativeURL(base, firstNonEmpty(
			readStringByKeys(payload, "avatarUrl", "avatar_url", "avatar"),
		)),
	}
	memosUserProfileCache.Store(cacheKey, identity)
	return identity
}
