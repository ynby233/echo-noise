// Package syncmanager owns scheduled and manual cloud archive transfers.
// Configure controls scheduler lifetime; LockOperation must cover the complete
// archive/upload/restore operation. Success metadata is persisted only after a
// 2xx upload and the required remote verification have both completed.
package syncmanager

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	backupservice "github.com/rcy1314/echo-noise/internal/backup"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/storage"
)

var (
	mu            sync.Mutex
	operationMu   sync.Mutex
	configured    models.SiteConfig
	scheduledStop chan struct{}
	debounceTimer *time.Timer
)

// LockOperation serializes cloud backup upload/restore entry points with
// scheduled and manual synchronization. Callers must defer the release func.
func LockOperation() func() {
	operationMu.Lock()
	return operationMu.Unlock
}

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
	// An explicit disabled setting must stay disabled even when legacy storage
	// credentials remain present. Enabling it is an administrator decision.
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
	releaseOperation := LockOperation()
	defer releaseOperation()
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
		return syncUploadLegacy(cfg)
	}
	if backupservice.HasPendingRestore(backupservice.DefaultLayout()) {
		return backupservice.ErrRestartRequired
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
	tmpDir, err := os.MkdirTemp("", "echo-noise-sync-*")
	if err != nil {
		return fmt.Errorf("创建同步临时目录失败: %w", err)
	}
	defer os.RemoveAll(tmpDir)
	backupPath := filepath.Join(tmpDir, "backup.zip")
	if err := backupservice.CreateArchive(backupPath, database.DB, backupservice.DefaultLayout()); err != nil {
		return fmt.Errorf("创建同步备份失败: %w", err)
	}

	return uploadAndPersist(cfg, backupPath)
}

func syncUploadLegacy(cfg models.SiteConfig) error {
	tmpDir, err := os.MkdirTemp("", "echo-noise-sync-legacy-*")
	if err != nil {
		return fmt.Errorf("创建同步临时目录失败: %w", err)
	}
	defer os.RemoveAll(tmpDir)
	backupPath := filepath.Join(tmpDir, "backup.zip")
	if err := createLegacyArchive(backupPath, filepath.Join("data", "images"), filepath.Join("data", "video")); err != nil {
		return fmt.Errorf("创建兼容同步备份失败: %w", err)
	}
	return uploadAndPersist(cfg, backupPath)
}

func uploadAndPersist(cfg models.SiteConfig, backupPath string) error {
	url, err := storage.PresignUpload(cfg, cfg.StorageBucket, "backup.zip", 1*time.Hour, "application/zip")
	if err != nil {
		return err
	}
	return completeUpload(
		func() error { return uploadArchive(url, backupPath, &http.Client{Timeout: 120 * time.Second}) },
		func() (*storage.ObjectMeta, error) { return storage.HeadObject(cfg, cfg.StorageBucket, "backup.zip") },
		func(remoteMeta *storage.ObjectMeta, syncTime *time.Time) error {
			return syncPersistMeta(cfg, remoteMeta, syncTime)
		},
	)
}

func completeUpload(
	upload func() error,
	head func() (*storage.ObjectMeta, error),
	persist func(*storage.ObjectMeta, *time.Time) error,
) error {
	if err := upload(); err != nil {
		return err
	}
	// Only a successful HTTP upload is allowed to advance LastSyncTime. A Head
	// failure also leaves the previous synchronization state untouched.
	remoteMeta, err := head()
	if err != nil {
		return fmt.Errorf("确认云端备份失败: %w", err)
	}
	now := time.Now()
	return persist(remoteMeta, &now)
}

func createLegacyArchive(destination, imageRoot, videoRoot string) (err error) {
	file, err := os.Create(destination)
	if err != nil {
		return err
	}
	archive := zip.NewWriter(file)
	for _, root := range []struct {
		path string
		name string
	}{{imageRoot, "images"}, {videoRoot, "video"}} {
		if info, statErr := os.Stat(root.path); statErr == nil && info.IsDir() {
			if err := addDirToZip(archive, root.path, root.name); err != nil {
				_ = archive.Close()
				_ = file.Close()
				return err
			}
		} else if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
			_ = archive.Close()
			_ = file.Close()
			return statErr
		}
	}
	if err := archive.Close(); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func uploadArchive(uploadURL, archivePath string, client *http.Client) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()
	req, err := http.NewRequest(http.MethodPut, uploadURL, file)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/zip")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, resp.Body)
		return fmt.Errorf("上传云端备份失败: status=%d", resp.StatusCode)
	}
	_, err = io.Copy(io.Discard, resp.Body)
	return err
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
			return err
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

	tmpDir, err := os.MkdirTemp("", "echo-noise-sync-restore-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)
	zipPath := filepath.Join(tmpDir, "cloud_backup.zip")
	out, err := os.Create(zipPath)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, resp.Body)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	result, err := backupservice.StageRestoreWithResult(zipPath, backupservice.DefaultLayout())
	if err != nil {
		return fmt.Errorf("暂存云端恢复包失败: %w", err)
	}
	return &backupservice.RestartRequiredError{Warning: result.Warning}
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
		dbPath := backupservice.DefaultLayout().DatabasePath
		latest = maxTime(latest, fileModTime(dbPath))
		latest = maxTime(latest, fileModTime(dbPath+"-wal"))
		latest = maxTime(latest, fileModTime(dbPath+"-shm"))
		for _, root := range backupservice.DefaultLayout().Roots {
			latest = maxTime(latest, dirLatestModTime(root.Path))
		}
	}
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
