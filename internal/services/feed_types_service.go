package services

import (
	"regexp"
	"sync"
	"time"
)

// Feed public types and entry points keep configuration, cached reads and explicit refreshes together.
type InfoFeedSource struct {
	Type    string `json:"type"`
	Group   string `json:"group"`
	Name    string `json:"name"`
	URL     string `json:"url"`
	Enabled bool   `json:"enabled"`
	Visible bool   `json:"visible"`
	Local   bool   `json:"-"`
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

type infoFeedSnapshot struct {
	updatedAt time.Time
	items     []InfoFeedItem
	err       error
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
	feedSnapshotMu sync.RWMutex
	feedSnapshot   infoFeedSnapshot
	feedRefreshMu  sync.Mutex
	feedTickerMu   sync.Mutex
	feedTickerStop chan struct{}
)

const (
	infoFeedCacheTTL           = 90 * time.Second
	infoFeedUnlimitedFetchSize = 500
	infoFeedPublicRefreshTTL   = 15 * time.Second
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

func LoadInfoFeedItems(limit int) ([]InfoFeedItem, error) {
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
	if len(sources) == 0 {
		return []InfoFeedItem{}, nil
	}
	snapshot := readInfoFeedSnapshot()
	items := cloneInfoFeedItems(snapshot.items)
	if displayLimit > 0 && len(items) > displayLimit {
		items = items[:displayLimit]
	}
	if snapshot.err != nil && len(items) == 0 {
		return items, snapshot.err
	}
	return items, nil
}

// RefreshInfoFeedItems fetches every configured source immediately, replaces the
// shared snapshot, and returns the refreshed items to the caller.
func RefreshInfoFeedItems(limit int) ([]InfoFeedItem, error) {
	items, err := refreshInfoFeedSnapshotItems()
	if limit > 100 {
		limit = 100
	}
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return cloneInfoFeedItems(items), err
}

// RefreshPublicInfoFeedItems returns the current shared snapshot when it was
// refreshed recently; otherwise it performs one source refresh. This keeps the
// public refresh button responsive without allowing visitors to amplify source
// traffic through repeated POST requests.
func RefreshPublicInfoFeedItems(limit int) ([]InfoFeedItem, error) {
	items, err := refreshInfoFeedSnapshotItemsWithin(infoFeedPublicRefreshTTL)
	if limit > 100 {
		limit = 100
	}
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return cloneInfoFeedItems(items), err
}

// StartInfoFeedAutoRefresh 启动/重建信息流后台定时刷新任务。
