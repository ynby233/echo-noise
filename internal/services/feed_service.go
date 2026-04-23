package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type InfoFeedSource struct {
	Type    string `json:"type"`
	Group   string `json:"group"`
	Name    string `json:"name"`
	URL     string `json:"url"`
	Enabled bool   `json:"enabled"`
	Visible bool   `json:"visible"`
}

type InfoFeedItem struct {
	Title       string `json:"title"`
	Link        string `json:"link"`
	Content     string `json:"content"`
	Summary     string `json:"summary"`
	ImageURL    string `json:"imageURL"`
	Source      string `json:"source"`
	Type        string `json:"type"`
	Author      string `json:"author"`
	AvatarURL   string `json:"avatarURL"`
	PublishedAt string `json:"publishedAt"`
	Timestamp   int64  `json:"timestamp"`
}

type infoFeedCacheEntry struct {
	cachedAt time.Time
	items    []InfoFeedItem
	err      error
}

var (
	reCData        = regexp.MustCompile(`(?s)<!\[CDATA\[(.*?)\]\]>`)
	reTag          = regexp.MustCompile(`(?s)<[^>]+>`)
	reSpaces       = regexp.MustCompile(`\s+`)
	reInlineSpaces = regexp.MustCompile(`[ \t]+`)
	reOrphanCloser = regexp.MustCompile(`(?m)^\s*/>\s*$`)
	reItemRss      = regexp.MustCompile(`(?s)<item\b[^>]*>(.*?)</item>`)
	reItemAtom     = regexp.MustCompile(`(?s)<entry\b[^>]*>(.*?)</entry>`)
	reImgSrc       = regexp.MustCompile(`(?is)<img[^>]+src=["']([^"']+)["']`)
	reMediaContent = regexp.MustCompile(`(?is)<media:content[^>]+url=["']([^"']+)["']`)
	reLinkTag      = regexp.MustCompile(`(?is)<link\b[^>]*>`)
	reHref         = regexp.MustCompile(`(?is)<link[^>]+href=["']([^"']+)["']`)
	reAnchorTag    = regexp.MustCompile(`(?is)<a[^>]+href=["']([^"']+)["'][^>]*>(.*?)</a>`)
	reLiOpen       = regexp.MustCompile(`(?is)<li[^>]*>`)
	infoFeedCache  sync.Map
)

const (
	infoFeedCacheTTL           = 90 * time.Second
	infoFeedUnlimitedFetchSize = 500
)

func GetInfoFeedConfig() (bool, int, []InfoFeedSource, error) {
	cfg, err := GetFrontendConfig()
	if err != nil {
		return false, 0, nil, err
	}
	fs, _ := cfg["frontendSettings"].(map[string]interface{})
	enabled := toBool(fs["feedEnabled"])
	limit := toInt(fs["feedLimit"], 0)
	if limit < 0 {
		limit = 0
	}
	if limit > 100 {
		limit = 100
	}
	sources := parseInfoFeedSources(fs["feedSources"])
	return enabled, limit, sources, nil
}

func LoadInfoFeedItems(baseURL string, limit int) ([]InfoFeedItem, error) {
	enabled, cfgLimit, sources, err := GetInfoFeedConfig()
	if err != nil {
		return nil, err
	}
	if !enabled {
		return []InfoFeedItem{}, nil
	}
	displayLimit := limit
	if displayLimit <= 0 {
		displayLimit = cfgLimit
	}
	if displayLimit < 0 {
		displayLimit = 0
	}
	if displayLimit > 100 {
		displayLimit = 100
	}
	fetchLimit := displayLimit
	if fetchLimit <= 0 {
		fetchLimit = infoFeedUnlimitedFetchSize
	}
	if len(sources) == 0 {
		return []InfoFeedItem{}, nil
	}
	cacheKey := buildInfoFeedCacheKey(baseURL, displayLimit, sources)
	if cached, ok := readInfoFeedCache(cacheKey); ok {
		return cached.items, cached.err
	}

	client := &http.Client{Timeout: 12 * time.Second}
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		all     = make([]InfoFeedItem, 0, fetchLimit*len(sources))
		onceErr error
	)
	for _, src := range sources {
		source := src
		if !source.Enabled || !source.Visible {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			source.URL = resolveSourceURL(baseURL, source.URL)
			var items []InfoFeedItem
			var ferr error
			st := strings.ToLower(strings.TrimSpace(source.Type))
			switch st {
			case "note", "custom":
				items, ferr = fetchByAutoType(client, source, fetchLimit)
			case "说说笔记":
				items, ferr = fetchByAutoType(client, source, fetchLimit)
			case "ech0":
				items, ferr = fetchEch0Source(client, source, fetchLimit)
			case "memos":
				items, ferr = fetchMemosSource(client, source, fetchLimit)
			case "mastodon":
				items, ferr = fetchMastodonSource(client, source, fetchLimit)
			default:
				items, ferr = fetchRSSSource(client, source, fetchLimit)
				// RSS 失败或内容为空时自动按 URL 回退到非 RSS 解析，兼容配置类型不准确的场景。
				if ferr != nil || len(items) == 0 {
					autoItems, autoErr := fetchByAutoType(client, source, fetchLimit)
					if autoErr == nil && len(autoItems) > 0 {
						items, ferr = autoItems, nil
					} else if ferr == nil {
						ferr = autoErr
					}
				}
			}
			mu.Lock()
			defer mu.Unlock()
			if ferr != nil && onceErr == nil {
				onceErr = ferr
			}
			all = append(all, items...)
		}()
	}
	wg.Wait()

	dedup := make([]InfoFeedItem, 0, len(all))
	seen := map[string]struct{}{}
	for _, it := range all {
		key := strings.TrimSpace(it.Link) + "|" + strings.TrimSpace(it.Title)
		if key == "|" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		dedup = append(dedup, it)
	}

	sort.Slice(dedup, func(i, j int) bool {
		return dedup[i].Timestamp > dedup[j].Timestamp
	})
	if displayLimit > 0 && len(dedup) > displayLimit {
		dedup = dedup[:displayLimit]
	}
	if len(dedup) > 0 || onceErr == nil {
		writeInfoFeedCache(cacheKey, dedup, onceErr)
	}
	return dedup, onceErr
}

func buildInfoFeedCacheKey(baseURL string, limit int, sources []InfoFeedSource) string {
	payload := struct {
		BaseURL string           `json:"baseURL"`
		Limit   int              `json:"limit"`
		Sources []InfoFeedSource `json:"sources"`
	}{
		BaseURL: strings.TrimSpace(baseURL),
		Limit:   limit,
		Sources: sources,
	}
	bs, err := json.Marshal(payload)
	if err != nil {
		return fmt.Sprintf("%s|%d|%d", strings.TrimSpace(baseURL), limit, len(sources))
	}
	return string(bs)
}

func cloneInfoFeedItems(items []InfoFeedItem) []InfoFeedItem {
	if len(items) == 0 {
		return []InfoFeedItem{}
	}
	cloned := make([]InfoFeedItem, len(items))
	copy(cloned, items)
	return cloned
}

func readInfoFeedCache(cacheKey string) (infoFeedCacheEntry, bool) {
	if raw, ok := infoFeedCache.Load(cacheKey); ok {
		if entry, ok := raw.(infoFeedCacheEntry); ok {
			if time.Since(entry.cachedAt) <= infoFeedCacheTTL {
				entry.items = cloneInfoFeedItems(entry.items)
				return entry, true
			}
			infoFeedCache.Delete(cacheKey)
		}
	}
	return infoFeedCacheEntry{}, false
}

func writeInfoFeedCache(cacheKey string, items []InfoFeedItem, fetchErr error) {
	infoFeedCache.Store(cacheKey, infoFeedCacheEntry{
		cachedAt: time.Now(),
		items:    cloneInfoFeedItems(items),
		err:      fetchErr,
	})
}

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

func fetchNoteSource(client *http.Client, source InfoFeedSource, limit int) ([]InfoFeedItem, error) {
	bases := candidateSourceBases(source.URL, "/api/messages", "#/messages")
	if len(bases) == 0 {
		return nil, fmt.Errorf("信息流 note 源为空")
	}
	base := ""
	var payload map[string]interface{}
	var lastErr error
	for _, oneBase := range bases {
		if !isHTTPURL(oneBase) {
			continue
		}
		apiURL := fmt.Sprintf("%s/api/messages/page?page=1&pageSize=%d", oneBase, limit)
		req, _ := http.NewRequest(http.MethodGet, apiURL, nil)
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", "Echo-Noise-InfoFlow/1.0")
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		raw, readErr := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
		_ = resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			continue
		}
		if resp.StatusCode >= 400 {
			lastErr = fmt.Errorf("note 请求失败: %d", resp.StatusCode)
			continue
		}
		var parsed map[string]interface{}
		if err := json.Unmarshal(raw, &parsed); err != nil {
			lastErr = err
			continue
		}
		if len(extractRows(parsed)) == 0 {
			lastErr = fmt.Errorf("note 接口返回为空")
			continue
		}
		base = oneBase
		payload = parsed
		break
	}
	if base == "" {
		if lastErr == nil {
			lastErr = fmt.Errorf("note 源地址无效: %s", source.URL)
		}
		return nil, lastErr
	}
	rows := extractRows(payload)
	sourceName := strings.TrimSpace(source.Name)
	if sourceName == "" {
		sourceName = "同部署项目"
	}
	type noteProfileInfo struct {
		Username string
		Avatar   string
	}
	profileCache := map[string]noteProfileInfo{}
	loadUserProfile := func(username string) noteProfileInfo {
		name := strings.TrimSpace(username)
		if name == "" {
			return noteProfileInfo{}
		}
		key := strings.ToLower(name)
		if cached, ok := profileCache[key]; ok {
			return cached
		}
		profileURL := fmt.Sprintf("%s/api/users/profile?username=%s", base, url.QueryEscape(name))
		req, _ := http.NewRequest(http.MethodGet, profileURL, nil)
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", "Echo-Noise-InfoFlow/1.0")
		resp, err := client.Do(req)
		if err != nil {
			profileCache[key] = noteProfileInfo{}
			return noteProfileInfo{}
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 400 {
			profileCache[key] = noteProfileInfo{}
			return noteProfileInfo{}
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
		if err != nil {
			profileCache[key] = noteProfileInfo{}
			return noteProfileInfo{}
		}
		var profilePayload map[string]interface{}
		if err := json.Unmarshal(body, &profilePayload); err != nil {
			profileCache[key] = noteProfileInfo{}
			return noteProfileInfo{}
		}
		profileObj := readMapByKey(profilePayload, "data")
		if profileObj == nil {
			profileObj = profilePayload
		}
		info := noteProfileInfo{
			Username: cleanText(firstNonEmpty(
				readStringByKeys(profileObj, "username", "name", "displayName", "display_name"),
				name,
			)),
			Avatar: resolveRelativeURL(base, firstNonEmpty(
				readStringByKeys(profileObj, "avatar_url", "avatarUrl", "avatar"),
				readStringByKeys(profileObj, "profile.avatar_url", "profile.avatarUrl", "profile.avatar"),
			)),
		}
		profileCache[key] = info
		return info
	}
	out := make([]InfoFeedItem, 0, len(rows))
	for _, row := range rows {
		m, ok := row.(map[string]interface{})
		if !ok {
			continue
		}
		id := toInt(readAnyByPath(m, "id"), 0)
		if id <= 0 {
			id = toInt(readAnyByPath(m, "ID"), 0)
		}
		rawContent := firstNonEmpty(
			readFlexibleTextByKeys(m, "content", "text", "summary", "body", "markdown"),
			fmt.Sprintf("%v", m["content"]),
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
			title = "未命名内容"
		}
		link := strings.TrimSpace(firstNonEmpty(
			readStringByKeys(m, "url", "link", "permalink"),
			readStringByKeys(m, "message_url", "messageUrl"),
		))
		if link == "" && id > 0 {
			link = strings.TrimSpace(fmt.Sprintf("%s/#/messages/%d", base, id))
		}
		link = resolveRelativeURL(base, link)
		if link == "" {
			continue
		}
		pubRaw := firstNonEmpty(
			readStringByKeys(m, "created_at", "createdAt", "createTime", "created", "publishedAt", "display_time", "displayTime"),
			strings.TrimSpace(fmt.Sprintf("%v", m["created_at"])),
		)
		img := resolveRelativeURL(base, firstNonEmpty(
			readStringByKeys(m, "image_url", "imageURL", "imageUrl", "cover", "cover_url", "coverUrl", "thumbnail"),
			extractImageFromResources(base, m, []string{"attachments", "resources", "files", "echo_files"}),
		))
		tm := parseAnyTime(pubRaw)
		author := cleanText(firstNonEmpty(
			readStringByKeys(m, "username"),
			readStringByKeys(m, "userName"),
			readStringByKeys(m, "user.username"),
			readStringByKeys(m, "user.name"),
			readStringByKeys(m, "creator.username"),
			readStringByKeys(m, "creator.name"),
			readStringByKeys(m, "nickname"),
			readStringByKeys(m, "author"),
			readStringByKeys(m, "display_name"),
		))
		avatarURL := resolveRelativeURL(base, firstNonEmpty(
			readStringByKeys(m, "avatar"),
			readStringByKeys(m, "avatar_url"),
			readStringByKeys(m, "avatarUrl"),
			readStringByKeys(m, "avatarURL"),
			readStringByKeys(m, "user.avatar"),
			readStringByKeys(m, "user.avatar_url"),
			readStringByKeys(m, "user.avatarUrl"),
			readStringByKeys(m, "user.avatarURL"),
			readStringByKeys(m, "creator.avatar"),
			readStringByKeys(m, "creator.avatar_url"),
			readStringByKeys(m, "creator.avatarUrl"),
			readStringByKeys(m, "creator.avatarURL"),
		))
		if author != "" && avatarURL == "" {
			profile := loadUserProfile(author)
			if strings.TrimSpace(profile.Avatar) != "" {
				avatarURL = profile.Avatar
			}
			if strings.TrimSpace(profile.Username) != "" {
				author = profile.Username
			}
		}
		out = append(out, InfoFeedItem{
			Title:       title,
			Link:        link,
			Content:     content,
			Summary:     content,
			ImageURL:    img,
			Source:      sourceName,
			Type:        "note",
			Author:      author,
			AvatarURL:   avatarURL,
			PublishedAt: formatTime(tm, pubRaw),
			Timestamp:   tm.Unix(),
		})
	}
	return out, nil
}

type feedFetchRequest struct {
	Method  string
	URL     string
	Body    interface{}
	Headers map[string]string
}

type sourceUserProfile struct {
	Username string
	Avatar   string
}

type sourcePublicProfile struct {
	Name   string
	Avatar string
}

func fetchEch0Source(client *http.Client, source InfoFeedSource, limit int) ([]InfoFeedItem, error) {
	bases := candidateSourceBases(source.URL, "/api/echo/query", "/echo/query", "/api/echo/page", "/echo/page")
	if len(bases) == 0 {
		return nil, fmt.Errorf("ech0 源地址无效: %s", source.URL)
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
				Method: http.MethodPost,
				URL:    fmt.Sprintf("%s/api/echo/query", oneBase),
				Body: map[string]interface{}{
					"page":      1,
					"pageSize":  limit,
					"search":    "",
					"tagIds":    []string{},
					"sortBy":    "created_at",
					"sortOrder": "desc",
					"dateFrom":  0,
					"dateTo":    0,
				},
			},
			// 向后兼容旧接口
			{Method: http.MethodGet, URL: fmt.Sprintf("%s/api/echo/page?page=1&pageSize=%d", oneBase, limit)},
			{Method: http.MethodPost, URL: fmt.Sprintf("%s/api/echo/page", oneBase), Body: map[string]interface{}{"page": 1, "pageSize": limit}},
		}, "ech0")
		if err == nil {
			base = oneBase
			break
		}
	}
	if err != nil {
		return nil, err
	}
	if base == "" {
		return nil, fmt.Errorf("ech0 源地址无效: %s", source.URL)
	}
	rows := extractRows(payload)
	sourceName := strings.TrimSpace(source.Name)
	if sourceName == "" {
		sourceName = "Ech0"
	}
	items := buildGenericFeedItems(rows, sourceName, base, platformParseOptions{
		ContentKeys:     []string{"content", "text", "summary", "description", "body", "markdown", "echo.content", "echo.text", "data.content", "message.content"},
		TitleKeys:       []string{"title", "name", "subject", "echo.title"},
		LinkKeys:        []string{"url", "link", "permalink", "path"},
		IDKeys:          []string{"id", "echo.id", "message.id", "uuid"},
		TimeKeys:        []string{"created_at", "createdAt", "create_time", "createTime", "publishedAt"},
		ImageKeys:       []string{"image_url", "imageURL", "imageUrl", "cover", "cover_url", "coverUrl", "thumbnail", "thumb", "echo.image_url"},
		ResourcesKeys:   []string{"echo_files", "echoFiles", "files", "resources", "attachments", "images"},
		AuthorKeys:      []string{"username", "user.username", "user.name", "user.nickname", "creator.username", "creator.name", "creator.nickname", "nickname", "author", "display_name", "echo.username"},
		AvatarKeys:      []string{"avatar", "avatar_url", "avatarUrl", "avatarURL", "user.avatar", "user.avatar_url", "user.avatarUrl", "creator.avatar", "creator.avatar_url", "creator.avatarUrl", "echo.avatar_url"},
		DefaultLinkPath: "/echo/%s",
		DefaultTitle:    "Ech0 动态",
		SourceType:      "ech0",
		Limit:           limit,
	})
	items = enrichItemsWithUserProfile(client, base, items)
	publicProfile := fetchSourcePublicProfile(client, base)
	for i := range items {
		if strings.TrimSpace(items[i].AvatarURL) == "" && strings.TrimSpace(publicProfile.Avatar) != "" {
			items[i].AvatarURL = publicProfile.Avatar
		}
		if strings.TrimSpace(items[i].Author) == "" && strings.TrimSpace(publicProfile.Name) != "" {
			items[i].Author = strings.TrimSpace(publicProfile.Name)
		}
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("ech0 接口未返回可用内容")
	}
	return items, nil
}

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
		resourceContent := buildResourcesRenderableContent(base, m, opt.ResourcesKeys)
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

func buildResourcesRenderableContent(base string, m map[string]interface{}, resourcesKeys []string) string {
	if len(resourcesKeys) == 0 {
		return ""
	}
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
			lines = append(lines, "![]("+candidate+")")
		}
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
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
