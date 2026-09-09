package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// Generic JSON-source parsing owns candidate requests, profile enrichment and tolerant field extraction.
type platformParseOptions struct {
	ContentKeys     []string
	TitleKeys       []string
	LinkKeys        []string
	IDKeys          []string
	TimeKeys        []string
	ImageKeys       []string
	ResourcesKeys   []string
	AuthorKeys      []string
	AvatarKeys      []string
	DefaultLinkPath string
	DefaultTitle    string
	SourceType      string
	Limit           int
}

func buildGenericFeedItems(rows []interface{}, sourceName, base string, opt platformParseOptions) []InfoFeedItem {
	out := make([]InfoFeedItem, 0, len(rows))
	for _, row := range rows {
		m, ok := row.(map[string]interface{})
		if !ok {
			continue
		}
		rawContent := firstNonEmpty(
			readFlexibleTextByKeys(m, opt.ContentKeys...),
			readStringByKeys(m, opt.ContentKeys...),
		)
		content := normalizeRawContent(rawContent)
		extensionContent := buildExtensionRenderableContent(m)
		resourceContent := buildResourcesRenderableContent(base, m, opt.ResourcesKeys, content)
		content = joinRenderableContent(content, extensionContent, resourceContent)
		if content == "" {
			content = cleanText(rawContent)
		}
		rawTitle := cleanText(readStringByKeys(m, opt.TitleKeys...))
		title := rawTitle
		if title == "" {
			title = limitText(firstLine(cleanContentText(content)), 80)
		}
		if title == "" {
			title = limitText(firstLine(content), 80)
		}
		if title == "" {
			title = opt.DefaultTitle
		}
		link := strings.TrimSpace(readStringByKeys(m, opt.LinkKeys...))
		id := normalizeExternalID(readStringByKeys(m, opt.IDKeys...))
		if link == "" && id != "" && opt.DefaultLinkPath != "" {
			link = strings.TrimRight(base, "/") + fmt.Sprintf(opt.DefaultLinkPath, url.PathEscape(id))
		}
		if link == "" {
			continue
		}
		pubRaw := readStringByKeys(m, opt.TimeKeys...)
		tm := parseAnyTime(pubRaw)
		imageURL := resolveRelativeURL(base, firstNonEmpty(
			readStringByKeys(m, opt.ImageKeys...),
			extractImageFromResources(base, m, opt.ResourcesKeys),
		))
		if strings.TrimSpace(content) == "" && strings.TrimSpace(imageURL) == "" && strings.TrimSpace(rawTitle) == "" {
			continue
		}
		avatarURL := resolveRelativeURL(base, readStringByKeys(m, opt.AvatarKeys...))
		out = append(out, InfoFeedItem{
			Title:       title,
			Link:        link,
			Content:     content,
			Summary:     content,
			ImageURL:    imageURL,
			Source:      sourceName,
			Type:        strings.TrimSpace(opt.SourceType),
			Author:      cleanText(readStringByKeys(m, opt.AuthorKeys...)),
			AvatarURL:   avatarURL,
			PublishedAt: formatTime(tm, pubRaw),
			Timestamp:   tm.Unix(),
		})
		if opt.Limit > 0 && len(out) >= opt.Limit {
			break
		}
	}
	return out
}

func fetchJSONFromCandidates(client *http.Client, candidates []feedFetchRequest, sourceType string) (interface{}, error) {
	var lastErr error
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate.URL) == "" {
			continue
		}
		payload, err := fetchJSON(client, candidate, sourceType)
		if err != nil {
			lastErr = err
			continue
		}
		rows := extractRows(payload)
		if len(rows) > 0 {
			return payload, nil
		}
		lastErr = fmt.Errorf("%s 接口返回为空", sourceType)
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("%s 接口无可用地址", sourceType)
	}
	return nil, lastErr
}

func enrichItemsWithUserProfile(client *http.Client, base string, items []InfoFeedItem) []InfoFeedItem {
	if client == nil || !isHTTPURL(strings.TrimSpace(base)) || len(items) == 0 {
		return items
	}
	cache := map[string]sourceUserProfile{}
	for i := range items {
		author := strings.TrimSpace(items[i].Author)
		if author == "" || strings.TrimSpace(items[i].AvatarURL) != "" {
			continue
		}
		key := strings.ToLower(author)
		profile, exists := cache[key]
		if !exists {
			profileURL := fmt.Sprintf("%s/api/users/profile?username=%s", strings.TrimRight(base, "/"), url.QueryEscape(author))
			req, _ := http.NewRequest(http.MethodGet, profileURL, nil)
			req.Header.Set("Accept", "application/json")
			req.Header.Set("User-Agent", "Echo-Noise-InfoFlow/1.0")
			resp, err := client.Do(req)
			if err == nil && resp != nil {
				body, readErr := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
				_ = resp.Body.Close()
				if readErr == nil && resp.StatusCode < 400 {
					var payload map[string]interface{}
					if json.Unmarshal(body, &payload) == nil {
						obj := readMapByKey(payload, "data")
						if obj == nil {
							obj = payload
						}
						profile = sourceUserProfile{
							Username: cleanText(firstNonEmpty(
								readStringByKeys(obj, "username", "name", "display_name", "displayName"),
								author,
							)),
							Avatar: resolveRelativeURL(base, firstNonEmpty(
								readStringByKeys(obj, "avatar_url", "avatarUrl", "avatar"),
								readStringByKeys(obj, "profile.avatar_url", "profile.avatarUrl", "profile.avatar"),
							)),
						}
					}
				}
			}
			cache[key] = profile
		}
		if strings.TrimSpace(profile.Username) != "" {
			items[i].Author = profile.Username
		}
		if strings.TrimSpace(profile.Avatar) != "" {
			items[i].AvatarURL = profile.Avatar
		}
	}
	return items
}

func fetchSourcePublicProfile(client *http.Client, base string) sourcePublicProfile {
	if client == nil || !isHTTPURL(strings.TrimSpace(base)) {
		return sourcePublicProfile{}
	}
	settingsURL := fmt.Sprintf("%s/api/settings", strings.TrimRight(base, "/"))
	req, _ := http.NewRequest(http.MethodGet, settingsURL, nil)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Echo-Noise-InfoFlow/1.0")
	resp, err := client.Do(req)
	if err != nil || resp == nil {
		return sourcePublicProfile{}
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return sourcePublicProfile{}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		return sourcePublicProfile{}
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return sourcePublicProfile{}
	}
	obj := readMapByKey(payload, "data")
	if obj == nil {
		obj = payload
	}
	name := cleanText(firstNonEmpty(
		readStringByKeys(obj, "server_name", "serverName", "name", "username"),
	))
	avatar := resolveRelativeURL(base, firstNonEmpty(
		readStringByKeys(obj, "server_logo", "serverLogo", "avatar_url", "avatarUrl", "avatar"),
	))
	return sourcePublicProfile{
		Name:   name,
		Avatar: avatar,
	}
}

func fetchJSON(client *http.Client, req feedFetchRequest, sourceType string) (interface{}, error) {
	method := strings.ToUpper(strings.TrimSpace(req.Method))
	if method == "" {
		method = http.MethodGet
	}
	var bodyReader io.Reader
	if req.Body != nil {
		raw, err := json.Marshal(req.Body)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(raw)
	}
	httpReq, err := http.NewRequest(method, req.URL, bodyReader)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Accept", "application/json, text/plain, */*")
	httpReq.Header.Set("User-Agent", "Echo-Noise-InfoFlow/1.0")
	if req.Body != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	}
	for key, value := range req.Headers {
		if strings.TrimSpace(key) != "" && strings.TrimSpace(value) != "" {
			httpReq.Header.Set(key, value)
		}
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return nil, fmt.Errorf("%s 请求失败: %d %s", sourceType, resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 3*1024*1024))
	if err != nil {
		return nil, err
	}
	var payload interface{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("%s 返回非 JSON: %w", sourceType, err)
	}
	return payload, nil
}

func extractRows(raw interface{}) []interface{} {
	return extractRowsWithDepth(raw, 0)
}

func extractRowsWithDepth(raw interface{}, depth int) []interface{} {
	if depth > 5 || raw == nil {
		return nil
	}
	if arr, ok := raw.([]interface{}); ok {
		return arr
	}
	obj, ok := raw.(map[string]interface{})
	if !ok {
		return nil
	}
	for _, key := range []string{"items", "list", "rows", "records", "memos", "statuses", "data", "result"} {
		if next, exists := obj[key]; exists {
			if rows := extractRowsWithDepth(next, depth+1); len(rows) > 0 {
				return rows
			}
		}
	}
	if readStringByKeys(obj, "content", "text", "title", "url", "link") != "" {
		return []interface{}{obj}
	}
	return nil
}

func normalizeSourceBase(raw string, cutPaths ...string) string {
	base := strings.TrimRight(strings.TrimSpace(raw), "/")
	lower := strings.ToLower(base)
	for _, p := range cutPaths {
		pathLower := strings.ToLower(strings.TrimSpace(p))
		if pathLower == "" {
			continue
		}
		if idx := strings.Index(lower, pathLower); idx > 0 {
			base = strings.TrimRight(base[:idx], "/")
			lower = strings.ToLower(base)
		}
	}
	return strings.TrimRight(base, "/")
}

func candidateSourceBases(raw string, cutPaths ...string) []string {
	bases := make([]string, 0, 3)
	seen := map[string]struct{}{}
	appendBase := func(base string) {
		base = strings.TrimRight(strings.TrimSpace(base), "/")
		if !isHTTPURL(base) {
			return
		}
		if _, ok := seen[base]; ok {
			return
		}
		seen[base] = struct{}{}
		bases = append(bases, base)
	}

	normalized := normalizeSourceBase(raw, cutPaths...)
	appendBase(normalized)
	if parsed, err := url.Parse(strings.TrimSpace(raw)); err == nil && parsed != nil && parsed.Scheme != "" && parsed.Host != "" {
		appendBase(parsed.Scheme + "://" + parsed.Host)
	}
	return bases
}

func fetchByAutoType(client *http.Client, source InfoFeedSource, limit int) ([]InfoFeedItem, error) {
	switch inferSourceTypeByURL(source.URL) {
	case "ech0":
		return fetchEch0Source(client, source, limit)
	case "memos":
		return fetchMemosSource(client, source, limit)
	case "mastodon":
		return fetchMastodonSource(client, source, limit)
	default:
		return fetchNoteSource(client, source, limit)
	}
}

func inferSourceTypeByURL(raw string) string {
	u := strings.ToLower(strings.TrimSpace(raw))
	if u == "" {
		return "note"
	}
	if strings.Contains(u, "/api/echo/query") || strings.Contains(u, "/echo/query") ||
		strings.Contains(u, "/api/echo/page") || strings.Contains(u, "/echo/page") {
		return "ech0"
	}
	if strings.Contains(u, "/api/v1/memos") || strings.Contains(u, "/api/v1/memo") {
		return "memos"
	}
	if strings.Contains(u, "/api/v1/accounts/") || strings.Contains(u, "/@") {
		return "mastodon"
	}
	if looksLikeNoteEndpoint(u) {
		return "note"
	}
	return "note"
}

func isHTTPURL(raw string) bool {
	return strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://")
}

func readStringByKeys(m map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		val := readAnyByPath(m, key)
		switch x := val.(type) {
		case string:
			if strings.TrimSpace(x) != "" {
				return strings.TrimSpace(x)
			}
		case float64:
			if x != 0 {
				return strconv.FormatFloat(x, 'f', -1, 64)
			}
		case int:
			if x != 0 {
				return strconv.Itoa(x)
			}
		case json.Number:
			return strings.TrimSpace(x.String())
		}
	}
	return ""
}

func readFlexibleTextByKeys(m map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		value := readAnyByPath(m, key)
		if value == nil {
			continue
		}
		if out := extractTextFromAny(value, 0); strings.TrimSpace(out) != "" {
			return strings.TrimSpace(out)
		}
	}
	return ""
}

func extractTextFromAny(raw interface{}, depth int) string {
	if raw == nil || depth > 6 {
		return ""
	}

	switch x := raw.(type) {
	case string:
		text := strings.TrimSpace(x)
		if text == "" {
			return ""
		}
		if (strings.HasPrefix(text, "{") && strings.HasSuffix(text, "}")) ||
			(strings.HasPrefix(text, "[") && strings.HasSuffix(text, "]")) {
			var parsed interface{}
			if err := json.Unmarshal([]byte(text), &parsed); err == nil {
				if parsedText := extractTextFromAny(parsed, depth+1); strings.TrimSpace(parsedText) != "" {
					return strings.TrimSpace(parsedText)
				}
			}
		}
		return text
	case json.Number:
		return strings.TrimSpace(x.String())
	case float64:
		return strings.TrimSpace(strconv.FormatFloat(x, 'f', -1, 64))
	case float32:
		return strings.TrimSpace(strconv.FormatFloat(float64(x), 'f', -1, 64))
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case []interface{}:
		parts := make([]string, 0, len(x))
		seen := map[string]struct{}{}
		for _, one := range x {
			part := strings.TrimSpace(extractTextFromAny(one, depth+1))
			if part == "" {
				continue
			}
			if _, ok := seen[part]; ok {
				continue
			}
			seen[part] = struct{}{}
			parts = append(parts, part)
		}
		return strings.TrimSpace(strings.Join(parts, "\n\n"))
	case map[string]interface{}:
		parts := make([]string, 0, 8)
		seen := map[string]struct{}{}
		appendPart := func(value interface{}) {
			part := strings.TrimSpace(extractTextFromAny(value, depth+1))
			if part == "" {
				return
			}
			if _, ok := seen[part]; ok {
				return
			}
			seen[part] = struct{}{}
			parts = append(parts, part)
		}

		for _, key := range []string{
			"content", "text", "markdown", "body", "summary", "description", "snippet",
			"raw", "plainText", "plain", "html", "value", "message",
		} {
			if v := readAnyByPath(x, key); v != nil {
				appendPart(v)
				continue
			}
			if v, ok := readMapValueInsensitive(x, key); ok {
				appendPart(v)
			}
		}
		for _, key := range []string{
			"children", "nodes", "blocks", "parts", "segments", "list", "items",
			"paragraphs", "resources", "attachments", "payload",
		} {
			if v := readAnyByPath(x, key); v != nil {
				appendPart(v)
				continue
			}
			if v, ok := readMapValueInsensitive(x, key); ok {
				appendPart(v)
			}
		}
		return strings.TrimSpace(strings.Join(parts, "\n\n"))
	default:
		out := strings.TrimSpace(fmt.Sprintf("%v", raw))
		if out == "<nil>" || out == "map[]" || out == "[]" {
			return ""
		}
		return out
	}
}

func readAnyByPath(m map[string]interface{}, path string) interface{} {
	if m == nil {
		return nil
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	parts := strings.Split(path, ".")
	var cur interface{} = m
	for _, part := range parts {
		obj, ok := cur.(map[string]interface{})
		if !ok {
			return nil
		}
		next, exists := obj[part]
		if !exists {
			next, exists = readMapValueInsensitive(obj, part)
		}
		if !exists {
			return nil
		}
		cur = next
	}
	return cur
}

func normalizeLookupKey(raw string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(raw)), "_", "")
}

func readMapValueInsensitive(m map[string]interface{}, key string) (interface{}, bool) {
	target := normalizeLookupKey(key)
	for k, v := range m {
		if normalizeLookupKey(k) == target {
			return v, true
		}
	}
	return nil, false
}

func readMapByKey(m map[string]interface{}, key string) map[string]interface{} {
	if m == nil {
		return nil
	}
	val := readAnyByPath(m, key)
	obj, _ := val.(map[string]interface{})
	return obj
}

func readArrayByKeys(m map[string]interface{}, keys ...string) []interface{} {
	for _, key := range keys {
		if arr, ok := readAnyByPath(m, key).([]interface{}); ok && len(arr) > 0 {
			return arr
		}
	}
	return nil
}

func firstMapFromArray(arr []interface{}) map[string]interface{} {
	for _, it := range arr {
		if m, ok := it.(map[string]interface{}); ok {
			return m
		}
	}
	return nil
}

func extractImageFromResources(base string, m map[string]interface{}, resourcesKeys []string) string {
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
				readStringByKeys(one, "external_link"),
				readStringByKeys(one, "image_url"),
				readStringByKeys(one, "externalLink"),
				readStringByKeys(one, "publicUrl"),
				readStringByKeys(one, "preview_url"),
			)
			if candidate != "" {
				return resolveRelativeURL(base, candidate)
			}
		}
	}
	return ""
}

func resolveRelativeURL(base, raw string) string {
	u := strings.TrimSpace(raw)
	if u == "" {
		return ""
	}
	if strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://") {
		return u
	}
	if strings.HasPrefix(u, "/") {
		return strings.TrimRight(base, "/") + u
	}
	return u
}

func resolveRelativeURLWithSource(sourceURL, raw string) string {
	u := strings.TrimSpace(raw)
	if u == "" {
		return ""
	}
	if strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://") {
		return u
	}
	base, err := url.Parse(strings.TrimSpace(sourceURL))
	if err == nil && base != nil {
		ref, refErr := url.Parse(u)
		if refErr == nil {
			return base.ResolveReference(ref).String()
		}
	}
	return resolveRelativeURL(sourceURL, u)
}

func normalizeExternalID(raw string) string {
	v := strings.TrimSpace(raw)
	if v == "" {
		return ""
	}
	if strings.Contains(v, "/") {
		parts := strings.Split(v, "/")
		for i := len(parts) - 1; i >= 0; i-- {
			if strings.TrimSpace(parts[i]) != "" {
				v = parts[i]
				break
			}
		}
	}
	if idx := strings.Index(v, "?"); idx > 0 {
		v = v[:idx]
	}
	return strings.TrimSpace(v)
}

func resolveMastodonStatusesURL(client *http.Client, raw string, limit int) (string, string, string, error) {
	target := strings.TrimSpace(raw)
	if !isHTTPURL(target) {
		return "", "", "", fmt.Errorf("mastodon 源地址无效: %s", raw)
	}
	parsed, err := url.Parse(target)
	if err != nil {
		return "", "", "", err
	}
	base := parsed.Scheme + "://" + parsed.Host
	path := strings.TrimSpace(parsed.Path)
	if strings.Contains(path, "/api/v1/accounts/") && strings.Contains(path, "/statuses") {
		statusURL, limitErr := ensureURLHasLimit(target, limit)
		return base, statusURL, "", limitErr
	}

	account := strings.TrimSpace(parsed.Query().Get("acct"))
	if account == "" && strings.HasPrefix(path, "/@") {
		account = strings.TrimPrefix(path, "/@")
		if idx := strings.Index(account, "/"); idx >= 0 {
			account = account[:idx]
		}
	}
	if account == "" {
		return "", "", "", fmt.Errorf("mastodon 源请填写账号主页（如 /@name）或 statuses API 地址")
	}

	lookupURL := fmt.Sprintf("%s/api/v1/accounts/lookup?acct=%s", base, url.QueryEscape(account))
	payload, err := fetchJSON(client, feedFetchRequest{Method: http.MethodGet, URL: lookupURL}, "mastodon")
	if err != nil {
		return "", "", "", err
	}
	accountObj, ok := payload.(map[string]interface{})
	if !ok {
		return "", "", "", fmt.Errorf("mastodon 账号查询返回异常")
	}
	accountID := strings.TrimSpace(readStringByKeys(accountObj, "id"))
	if accountID == "" {
		return "", "", "", fmt.Errorf("mastodon 账号查询失败: 缺少 id")
	}
	accountName := firstNonEmpty(readStringByKeys(accountObj, "display_name"), readStringByKeys(accountObj, "username"), account)
	statusesURL := fmt.Sprintf("%s/api/v1/accounts/%s/statuses?limit=%d&exclude_replies=true", base, url.PathEscape(accountID), limit)
	return base, statusesURL, accountName, nil
}

func ensureURLHasLimit(raw string, limit int) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	q := parsed.Query()
	if strings.TrimSpace(q.Get("limit")) == "" {
		q.Set("limit", strconv.Itoa(limit))
	}
	parsed.RawQuery = q.Encode()
	return parsed.String(), nil
}
