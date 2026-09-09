package services

import (
	"log"
	"strings"
	"time"

	"github.com/rcy1314/echo-noise/config"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
)

// The scheduler owns background refresh timing and the shared snapshot lifecycle.
func StartInfoFeedAutoRefresh() {
	enabled, _, _, err := GetInfoFeedConfig()
	if err != nil {
		log.Printf("信息流自动刷新配置读取失败: %v", err)
		return
	}
	interval := time.Duration(getInfoFeedRefreshSeconds()) * time.Second

	feedTickerMu.Lock()
	if feedTickerStop != nil {
		close(feedTickerStop)
		feedTickerStop = nil
	}
	if !enabled {
		writeInfoFeedSnapshot([]InfoFeedItem{}, nil, time.Now())
		log.Printf("信息流自动刷新未启动: feedEnabled=false")
		feedTickerMu.Unlock()
		return
	}
	stop := make(chan struct{})
	feedTickerStop = stop
	feedTickerMu.Unlock()

	log.Printf("信息流后台自动刷新已启动，间隔: %v", interval)
	go func(stop <-chan struct{}, interval time.Duration) {
		refreshInfoFeedSnapshot()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				refreshInfoFeedSnapshot()
			case <-stop:
				log.Printf("信息流后台自动刷新已停止")
				return
			}
		}
	}(stop, interval)
}

func getInfoFeedRefreshSeconds() int {
	cfg, err := GetFrontendConfig()
	if err != nil {
		return 7200
	}
	fs, _ := cfg["frontendSettings"].(map[string]interface{})
	seconds := toInt(fs["feedRefreshSeconds"], 7200)
	if seconds < 10 {
		seconds = 10
	}
	if seconds > 86400 {
		seconds = 86400
	}
	return seconds
}

func readInfoFeedSnapshot() infoFeedSnapshot {
	feedSnapshotMu.RLock()
	defer feedSnapshotMu.RUnlock()
	return infoFeedSnapshot{
		updatedAt: feedSnapshot.updatedAt,
		items:     cloneInfoFeedItems(feedSnapshot.items),
		err:       feedSnapshot.err,
	}
}

func writeInfoFeedSnapshot(items []InfoFeedItem, fetchErr error, updatedAt time.Time) {
	feedSnapshotMu.Lock()
	feedSnapshot = infoFeedSnapshot{
		updatedAt: updatedAt,
		items:     cloneInfoFeedItems(items),
		err:       fetchErr,
	}
	feedSnapshotMu.Unlock()
}

func refreshInfoFeedSnapshot() {
	items, err := refreshInfoFeedSnapshotItems()
	if err != nil && len(items) == 0 {
		log.Printf("信息流后台刷新失败: %v", err)
		return
	}
	if err != nil {
		log.Printf("信息流后台刷新部分失败: %v（保留可用内容 %d 条）", err, len(items))
	} else {
		log.Printf("信息流后台刷新成功: %d 条", len(items))
	}
}

func refreshInfoFeedSnapshotItems() ([]InfoFeedItem, error) {
	return refreshInfoFeedSnapshotItemsWithin(0)
}

func refreshInfoFeedSnapshotItemsWithin(reuseWithin time.Duration) ([]InfoFeedItem, error) {
	feedRefreshMu.Lock()
	defer feedRefreshMu.Unlock()
	if reuseWithin > 0 {
		snapshot := readInfoFeedSnapshot()
		if !snapshot.updatedAt.IsZero() && time.Since(snapshot.updatedAt) < reuseWithin {
			return cloneInfoFeedItems(snapshot.items), snapshot.err
		}
	}

	baseURL := resolveSchedulerBaseURL()
	items, err := loadInfoFeedItemsFromSources(baseURL, 0)
	writeInfoFeedSnapshot(items, err, time.Now())
	return cloneInfoFeedItems(items), err
}

func resolveSchedulerBaseURL() string {
	db, err := database.GetDB()
	if err == nil && db != nil {
		var siteCfg models.SiteConfig
		if err := db.Table("site_configs").First(&siteCfg).Error; err == nil {
			u := strings.TrimSpace(siteCfg.SitePublicURL)
			if strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://") {
				return strings.TrimRight(u, "/")
			}
		}
	}
	host := strings.TrimSpace(config.Config.Server.Host)
	port := strings.TrimSpace(config.Config.Server.Port)
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	if port == "" {
		port = "1314"
	}
	return "http://" + host + ":" + port
}
