// Package syncmanager owns typed backup plans and cloud archive transfers.
// Main owns scheduler lifetime; LockOperation covers complete backup/restore
// operations. Legacy restore helpers remain for explicitly confirmed imports.
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

func PrepareUpdateShutdown(task string, cancel bool) error {
	if !operationMu.TryLock() {
		return errors.New("备份或同步正在进行")
	}
	defer operationMu.Unlock()
	return backupservice.ReserveUpdate(task, backupservice.DefaultLayout(), cancel)
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

// Configure updates the cloud channel and stops timers from the retired sync
// scheduler. Main owns Manager.Start/Stop through its worker context.
func Configure(cfg models.SiteConfig) {
	mu.Lock()
	defer mu.Unlock()
	configured = cfg
	if scheduledStop != nil {
		close(scheduledStop)
		scheduledStop = nil
	}
	if debounceTimer != nil {
		debounceTimer.Stop()
		debounceTimer = nil
	}
}

// Trigger remains for existing mutation hooks. Managed daily plans are the only
// automatic backup source; data changes must not enqueue legacy backup.zip jobs.
func Trigger() {}

// SyncNow is deliberately retired rather than silently creating an unbounded
// legacy archive. Call Manager.Create with an explicit kind and destination.
func SyncNow() error {
	return errors.New("旧版云同步已停用，请使用独立备份计划或手动生成备份")
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
