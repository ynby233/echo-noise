package syncmanager

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/storage"
)

var (
	mu            sync.Mutex
	configured    models.SiteConfig
	scheduledStop chan struct{}
	debounceTimer *time.Timer
)

const storageSyncConfirmFile = "data/storage_sync_confirmed"

func IsStorageSyncConfirmedLocal() bool {
	if _, err := os.Stat(storageSyncConfirmFile); err == nil {
		return true
	}
	return false
}

func SetStorageSyncConfirmedLocal() error {
	if err := os.MkdirAll(filepath.Dir(storageSyncConfirmFile), 0755); err != nil {
		return err
	}
	return os.WriteFile(storageSyncConfirmFile, []byte("ok"), 0644)
}

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
	confirmed := cfg.StorageSyncConfirmed && IsStorageSyncConfirmedLocal()
	instantEnabled := cfg.StorageEnabled && (cfg.StorageSyncRole == "" || cfg.StorageSyncRole == "primary") && cfg.StorageAutoSyncEnabled && cfg.StorageSyncMode == "instant" && confirmed
	if cfg.StorageEnabled && (cfg.StorageSyncRole == "" || cfg.StorageSyncRole == "primary") && cfg.StorageAutoSyncEnabled && cfg.StorageSyncMode == "scheduled" && cfg.StorageSyncIntervalMinute > 0 && confirmed {
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
		if cfg.StorageEnabled && cfg.StorageAutoSyncEnabled && !confirmed {
			log.Printf("检测到云同步未确认，启动时不启用任何自动同步")
		} else if instantEnabled {
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
		log.Printf("云端即时同步已启用，启动时不自动同步，等待数据变更触发")
	}
}

func Trigger() {
	mu.Lock()
	defer mu.Unlock()
	// 仅主节点在即时模式下触发
	if !(configured.StorageEnabled && (configured.StorageSyncRole == "" || configured.StorageSyncRole == "primary") && configured.StorageAutoSyncEnabled && configured.StorageSyncMode == "instant") {
		return
	}
	confirmed := configured.StorageSyncConfirmed && IsStorageSyncConfirmedLocal()
	if !confirmed {
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
	confirmed := cfg.StorageSyncConfirmed && IsStorageSyncConfirmedLocal()
	if !confirmed {
		return fmt.Errorf("检测到云同步首次启用/未确认：为避免覆盖，已阻止同步。请在后台先确认同步后再执行")
	}

	dbType := os.Getenv("DB_TYPE")
	if dbType == "" {
		dbType = "sqlite"
	}
	if dbType != "sqlite" {
		// 当前自动同步仅实现了 sqlite 的打包/恢复仲裁。
		// 非 sqlite 场景保持既有“上传备份”逻辑，避免错误覆盖。
		return syncUpload(cfg)
	}

	remoteMeta, err := storage.HeadObject(cfg, cfg.StorageBucket, "backup.zip")
	if err != nil {
		return err
	}

	localLatest := localLatestModTime(dbType)
	lastSync := cfg.StorageLastSyncTime
	localChanged := false
	if lastSync == nil {
		localChanged = !localLatest.IsZero()
	} else if !localLatest.IsZero() {
		localChanged = localLatest.After(*lastSync)
	}

	remoteChanged := false
	remoteTime := time.Time{}
	if remoteMeta != nil {
		if remoteMeta.LastModified != nil {
			remoteTime = *remoteMeta.LastModified
		}
		if cfg.StorageLastRemoteETag != "" {
			remoteChanged = remoteMeta.ETag != "" && remoteMeta.ETag != cfg.StorageLastRemoteETag
		} else if cfg.StorageLastRemoteModified != nil {
			remoteChanged = !remoteTime.IsZero() && remoteTime.After(*cfg.StorageLastRemoteModified)
		} else {
			// 第一次观测到云端对象，视为云端有数据
			remoteChanged = true
		}
	}

	// 决策：优先避免“本地旧数据覆盖云端新数据”
	if remoteMeta == nil {
		if !localLatest.IsZero() {
			return syncUpload(cfg)
		}
		return nil
	}

	// 如果本地从未同步过，但云端已有备份：优先拉取云端，避免首次启动就覆盖
	if cfg.StorageLastSyncTime == nil && cfg.StorageLastRemoteETag == "" && cfg.StorageLastRemoteModified == nil {
		return syncDownloadAndRestore(cfg, remoteMeta)
	}

	// 两边都变化时按 Last-Modified 与本地最新 mtime 仲裁
	if localChanged && remoteChanged {
		if !remoteTime.IsZero() && !localLatest.IsZero() {
			if remoteTime.After(localLatest.Add(2 * time.Second)) {
				return syncDownloadAndRestore(cfg, remoteMeta)
			}
			if localLatest.After(remoteTime.Add(2 * time.Second)) {
				return syncUpload(cfg)
			}
		}
		// 无法可靠仲裁时，不做任何覆盖，由管理员人工处理
		return fmt.Errorf("检测到云端与本地同时发生变更，无法自动仲裁（local=%v remote=%v）。为避免覆盖，已停止同步，请人工确认后再同步", localLatest, remoteTime)
	}
	if remoteChanged && !localChanged {
		return syncDownloadAndRestore(cfg, remoteMeta)
	}
	if localChanged && !remoteChanged {
		return syncUpload(cfg)
	}
	// 都没变：刷新记录（尤其是首次只有 remote meta 的场景）
	return syncPersistMeta(cfg, remoteMeta, cfg.StorageLastSyncTime)
}

func syncUpload(cfg models.SiteConfig) error {
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

	// 上传完成后重新 Head 一次获取最新 ETag/Last-Modified
	remoteMeta, _ := storage.HeadObject(cfg, cfg.StorageBucket, "backup.zip")
	now := time.Now()
	return syncPersistMeta(cfg, remoteMeta, &now)
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

func syncDownloadAndRestore(cfg models.SiteConfig, remoteMeta *storage.ObjectMeta) error {
	url, err := storage.PresignDownload(cfg, cfg.StorageBucket, "backup.zip", 1*time.Hour)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 180 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		io.Copy(io.Discard, resp.Body)
		return fmt.Errorf("下载云端备份失败: status=%d", resp.StatusCode)
	}

	tmpDir := os.TempDir()
	zipPath := filepath.Join(tmpDir, "cloud_backup.zip")
	_ = os.Remove(zipPath)
	out, err := os.Create(zipPath)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, resp.Body)
	out.Close()
	if err != nil {
		return err
	}

	if err := restoreFromBackupZip(zipPath); err != nil {
		return err
	}
	_ = os.Remove(zipPath)

	if err := database.ReconnectDB(); err != nil {
		return err
	}

	now := time.Now()
	return syncPersistMeta(cfg, remoteMeta, &now)
}

func syncPersistMeta(cfg models.SiteConfig, remoteMeta *storage.ObjectMeta, syncTime *time.Time) error {
	updates := map[string]any{}
	if syncTime != nil {
		updates["storage_last_sync_time"] = syncTime
	}
	if remoteMeta != nil {
		updates["storage_last_remote_e_tag"] = remoteMeta.ETag
		updates["storage_last_remote_modified"] = remoteMeta.LastModified
	}
	if len(updates) == 0 {
		return nil
	}

	db, _ := database.GetDB()
	if err := db.Table("site_configs").Where("id = ?", cfg.ID).Updates(updates).Error; err != nil {
		errStr := strings.ToLower(err.Error())
		if strings.Contains(errStr, "no such column") || strings.Contains(errStr, "unknown column") {
			// 兼容旧库缺列：降级跳过 remote meta 字段更新，仅更新 last_sync_time
			updates2 := map[string]any{}
			if syncTime != nil {
				updates2["storage_last_sync_time"] = syncTime
			}
			if len(updates2) == 0 {
				return nil
			}
			if err2 := db.Table("site_configs").Where("id = ?", cfg.ID).Updates(updates2).Error; err2 == nil {
				log.Printf("检测到旧数据库缺少云端元信息字段，已跳过 remote meta 持久化")
				mu.Lock()
				if syncTime != nil {
					configured.StorageLastSyncTime = syncTime
				}
				mu.Unlock()
				return nil
			}
		}
		return err
	}

	mu.Lock()
	if syncTime != nil {
		configured.StorageLastSyncTime = syncTime
	}
	if remoteMeta != nil {
		configured.StorageLastRemoteETag = remoteMeta.ETag
		configured.StorageLastRemoteModified = remoteMeta.LastModified
	}
	mu.Unlock()
	return nil
}

func localLatestModTime(dbType string) time.Time {
	latest := time.Time{}
	if dbType == "sqlite" {
		dbPath := os.Getenv("DB_PATH")
		if dbPath == "" {
			dbPath = "/app/data/noise.db"
		}
		latest = maxTime(latest, fileModTime(dbPath))
	}
	latest = maxTime(latest, dirLatestModTime("./data/images"))
	latest = maxTime(latest, dirLatestModTime("./data/video"))
	return latest
}

func fileModTime(path string) time.Time {
	fi, err := os.Stat(path)
	if err != nil || fi.IsDir() {
		return time.Time{}
	}
	return fi.ModTime()
}

func dirLatestModTime(dir string) time.Time {
	latest := time.Time{}
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		latest = maxTime(latest, info.ModTime())
		return nil
	})
	return latest
}

func maxTime(a, b time.Time) time.Time {
	if a.IsZero() {
		return b
	}
	if b.IsZero() {
		return a
	}
	if b.After(a) {
		return b
	}
	return a
}

func restoreFromBackupZip(zipPath string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "/app/data/noise.db"
	}
	_ = os.MkdirAll(filepath.Dir(dbPath), 0755)
	_ = os.MkdirAll("./data/images", 0755)
	_ = os.MkdirAll("./data/video", 0755)

	for _, f := range r.File {
		name := filepath.ToSlash(f.Name)
		if strings.HasSuffix(name, "/") {
			continue
		}
		var target string
		switch {
		case name == "database.db":
			target = dbPath
		case strings.HasPrefix(name, "images/"):
			target = filepath.Join("./data/images", strings.TrimPrefix(name, "images/"))
		case strings.HasPrefix(name, "video/"):
			target = filepath.Join("./data/video", strings.TrimPrefix(name, "video/"))
		default:
			continue
		}
		if err := extractZipFileTo(f, target); err != nil {
			return err
		}
	}
	return nil
}

func extractZipFileTo(zf *zip.File, dest string) error {
	rc, err := zf.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, rc)
	return err
}
