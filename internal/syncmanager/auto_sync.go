package syncmanager

import (
	"archive/zip"
	"bytes"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/lin-snow/ech0/internal/database"
	"github.com/lin-snow/ech0/internal/models"
	"github.com/lin-snow/ech0/internal/storage"
)

var (
	mu            sync.Mutex
	configured    models.SiteConfig
	scheduledStop chan struct{}
	debounceTimer *time.Timer
)

func Configure(cfg models.SiteConfig) {
	mu.Lock()
	// 兼容旧数据：若云存储配置完整但未显式开启自动同步，则按“默认开启”
	effectiveAuto := cfg.StorageAutoSyncEnabled
	if !effectiveAuto && cfg.StorageEnabled &&
		strings.TrimSpace(cfg.StorageProvider) != "" &&
		strings.TrimSpace(cfg.StorageEndpoint) != "" &&
		strings.TrimSpace(cfg.StorageBucket) != "" &&
		strings.TrimSpace(cfg.StorageAccessKey) != "" &&
		strings.TrimSpace(cfg.StorageSecretKey) != "" {
		effectiveAuto = true
		log.Printf("检测到旧配置且云存储完整，默认开启自动同步")
	}
	cfg.StorageAutoSyncEnabled = effectiveAuto
	configured = cfg
	if scheduledStop != nil {
		close(scheduledStop)
		scheduledStop = nil
	}
	instantEnabled := cfg.StorageEnabled && (cfg.StorageSyncRole == "" || cfg.StorageSyncRole == "primary") && cfg.StorageAutoSyncEnabled && cfg.StorageSyncMode == "instant"
	if cfg.StorageEnabled && (cfg.StorageSyncRole == "" || cfg.StorageSyncRole == "primary") && cfg.StorageAutoSyncEnabled && cfg.StorageSyncMode == "scheduled" && cfg.StorageSyncIntervalMinute > 0 {
		scheduledStop = make(chan struct{})
		interval := time.Duration(cfg.StorageSyncIntervalMinute) * time.Minute
		go func(stop <-chan struct{}) {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("云端自动同步任务崩溃: %v", r)
				}
			}()
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			log.Printf("云端自动同步任务已启动，间隔: %v", interval)
			if err := SyncNow(); err != nil {
				log.Printf("云端自动同步执行失败: %v", err)
			} else {
				log.Printf("云端自动同步执行成功")
			}
			for {
				select {
				case <-ticker.C:
					if err := SyncNow(); err != nil {
						log.Printf("云端自动同步执行失败: %v", err)
					} else {
						log.Printf("云端自动同步执行成功")
					}
				case <-stop:
					log.Printf("云端自动同步任务已停止")
					return
				}
			}
		}(scheduledStop)
	} else {
		if instantEnabled {
			log.Printf("云端即时同步已启用，等待触发")
		} else {
			log.Printf(
				"云端自动同步未启动: enabled=%v role=%s auto=%v mode=%s interval=%d",
				cfg.StorageEnabled,
				cfg.StorageSyncRole,
				cfg.StorageAutoSyncEnabled,
				cfg.StorageSyncMode,
				cfg.StorageSyncIntervalMinute,
			)
		}
	}
	mu.Unlock()
	if instantEnabled {
		go func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("云端即时同步崩溃: %v", r)
				}
			}()
			log.Printf("云端即时同步已启用，立即执行一次同步")
			if err := SyncNow(); err != nil {
				log.Printf("云端即时同步执行失败: %v", err)
			} else {
				log.Printf("云端即时同步执行成功")
			}
		}()
	}
}

func Trigger() {
	mu.Lock()
	defer mu.Unlock()
	// 仅主节点在即时模式下触发
	if !(configured.StorageEnabled && (configured.StorageSyncRole == "" || configured.StorageSyncRole == "primary") && configured.StorageAutoSyncEnabled && configured.StorageSyncMode == "instant") {
		return
	}
	if debounceTimer != nil {
		debounceTimer.Stop()
	}
	log.Printf("云端即时同步触发（防抖 15s）")
	debounceTimer = time.AfterFunc(15*time.Second, func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("云端即时同步崩溃: %v", r)
			}
		}()
		if err := SyncNow(); err != nil {
			log.Printf("云端即时同步执行失败: %v", err)
		} else {
			log.Printf("云端即时同步执行成功")
		}
	})
}

func SyncNow() error {
	mu.Lock()
	cfg := configured
	mu.Unlock()
	if !cfg.StorageEnabled {
		return nil
	}
	if cfg.StorageSyncRole == "secondary" {
		return nil
	}

	// 构建临时备份文件
	tmpDir := os.TempDir()
	backupPath := filepath.Join(tmpDir, "backup.zip")
	_ = os.Remove(backupPath)
	f, err := os.Create(backupPath)
	if err != nil {
		return err
	}
	zw := zip.NewWriter(f)

	// 数据库文件
	dbType := os.Getenv("DB_TYPE")
	if dbType == "" {
		dbType = "sqlite"
	}
	if dbType == "sqlite" {
		dbPath := os.Getenv("DB_PATH")
		if dbPath == "" {
			dbPath = "/app/data/noise.db"
		}
		if err := addFileToZip(zw, dbPath, "database.db"); err != nil { /* ignore missing */
		}
	}
	// images
	_ = addDirToZip(zw, "./data/images", "images")
	// video
	_ = addDirToZip(zw, "./data/video", "video")

	_ = zw.Close()
	f.Close()

	// 预签名上传
	url, err := storage.PresignUpload(cfg, cfg.StorageBucket, "backup.zip", 1*time.Hour, "application/zip")
	if err != nil {
		return err
	}
	// PUT 上传
	content, err := os.ReadFile(backupPath)
	if err != nil {
		return err
	}
	req, _ := http.NewRequest(http.MethodPut, url, bytes.NewReader(content))
	req.Header.Set("Content-Type", "application/zip")
	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	now := time.Now()
	db, _ := database.GetDB()
	_ = db.Table("site_configs").Where("id = ?", cfg.ID).Updates(map[string]any{"storage_last_sync_time": &now}).Error
	return nil
}

func addFileToZip(zw *zip.Writer, srcPath, zipName string) error {
	fi, err := os.Stat(srcPath)
	if err != nil || fi.IsDir() {
		return err
	}
	r, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer r.Close()
	w, err := zw.Create(zipName)
	if err != nil {
		return err
	}
	_, err = io.Copy(w, r)
	return err
}

func addDirToZip(zw *zip.Writer, dir, prefix string) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(dir, path)
		zipName := filepath.ToSlash(filepath.Join(prefix, rel))
		return addFileToZip(zw, path, zipName)
	})
}
