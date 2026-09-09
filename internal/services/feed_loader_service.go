package services

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// Source loading owns fan-out, result ordering and the short-lived fetch cache.
func loadInfoFeedItemsFromSources(baseURL string, limit int) ([]InfoFeedItem, error) {
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
			source.Local = isLocalInfoFeedSource(baseURL, source.URL)
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
			if source.Local {
				for i := range items {
					items[i].Link = localInfoFeedURLPath(items[i].Link, source.URL)
					items[i].ImageURL = localInfoFeedURLPath(items[i].ImageURL, source.URL)
					items[i].AvatarURL = localInfoFeedURLPath(items[i].AvatarURL, source.URL)
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
