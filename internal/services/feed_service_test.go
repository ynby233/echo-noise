package services

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rcy1314/echo-noise/internal/models"
)

func TestRefreshInfoFeedItemsReplacesSnapshotAndKeepsLocalMessageLinksRelative(t *testing.T) {
	db := setupUserServiceTestDB(t)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("open sql db handle: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	content := "before manual refresh"

	var upstream *httptest.Server
	upstream = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rss" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = fmt.Fprintf(w, `<?xml version="1.0"?><rss><channel><item><title>local item</title><link>%s/#/messages/233</link><description>%s</description><pubDate>Fri, 17 Jul 2026 00:00:00 GMT</pubDate></item></channel></rss>`, upstream.URL, content)
	}))
	defer upstream.Close()

	if err := db.Create(&models.SiteConfig{
		FeedEnabled:   true,
		FeedSources:   `[{"type":"rss","group":"test","name":"local","url":"/rss","enabled":true,"visible":true}]`,
		FeedLimit:     100,
		SitePublicURL: upstream.URL,
	}).Error; err != nil {
		t.Fatalf("create feed config: %v", err)
	}

	writeInfoFeedSnapshot(nil, nil, time.Now())
	first, err := RefreshInfoFeedItems(0)
	if err != nil {
		t.Fatalf("initial refresh: %v", err)
	}
	if len(first) != 1 || !strings.Contains(first[0].Content, "before manual refresh") {
		t.Fatalf("initial refresh items = %#v", first)
	}
	if first[0].Link != "/#/messages/233" {
		t.Fatalf("local message link = %q, want relative deep link", first[0].Link)
	}

	content = "after manual refresh"
	stale, err := LoadInfoFeedItems(0)
	if err != nil {
		t.Fatalf("load snapshot: %v", err)
	}
	if len(stale) != 1 || !strings.Contains(stale[0].Content, "before manual refresh") {
		t.Fatalf("snapshot changed without refresh: %#v", stale)
	}

	refreshed, err := RefreshInfoFeedItems(0)
	if err != nil {
		t.Fatalf("manual refresh: %v", err)
	}
	if len(refreshed) != 1 || !strings.Contains(refreshed[0].Content, "after manual refresh") {
		t.Fatalf("manual refresh items = %#v", refreshed)
	}

	latest, err := LoadInfoFeedItems(0)
	if err != nil {
		t.Fatalf("load refreshed snapshot: %v", err)
	}
	if len(latest) != 1 || !strings.Contains(latest[0].Content, "after manual refresh") {
		t.Fatalf("refreshed snapshot = %#v", latest)
	}
}

func TestRefreshPublicInfoFeedItemsReusesRecentSnapshot(t *testing.T) {
	previous := readInfoFeedSnapshot()
	t.Cleanup(func() {
		writeInfoFeedSnapshot(previous.items, previous.err, previous.updatedAt)
	})

	writeInfoFeedSnapshot([]InfoFeedItem{{Title: "cached public item"}}, nil, time.Now())
	items, err := RefreshPublicInfoFeedItems(0)
	if err != nil {
		t.Fatalf("public refresh with a recent snapshot: %v", err)
	}
	if len(items) != 1 || items[0].Title != "cached public item" {
		t.Fatalf("public refresh must reuse the recent shared snapshot, got %#v", items)
	}
}
