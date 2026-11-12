package syncmanager

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	backup "github.com/rcy1314/echo-noise/internal/backup"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/storage"
	"gorm.io/gorm"
)

const managedPrefix = "noise-managed-backups/"

type managedObject struct {
	Key      string
	Modified time.Time
	Size     int64
}

type managedCloud interface {
	Enabled() bool
	Upload(context.Context, string, string) error
	Open(context.Context, string) (io.ReadCloser, error)
	List(context.Context, string) ([]managedObject, error)
	Delete(context.Context, string) error
}

type productionManagedCloud struct{}

func (productionManagedCloud) Enabled() bool {
	mu.Lock()
	defer mu.Unlock()
	return configured.StorageEnabled
}

func managedKey(kind, id string) string { return managedPrefix + kind + "/" + id + ".zip" }

// Require the canonical spelling used in local filenames and S3 keys. uuid.Parse
// alone also accepts URNs and brace-wrapped IDs, which are not managed names.
func validRecordID(id string) bool {
	parsed, err := uuid.Parse(id)
	return err == nil && parsed.String() == id && parsed != uuid.Nil
}

func managedObjectID(kind, key string) (string, bool) {
	prefix := managedPrefix + kind + "/"
	if !validKind(kind) || !strings.HasPrefix(key, prefix) || !strings.HasSuffix(key, ".zip") {
		return "", false
	}
	id := strings.TrimSuffix(strings.TrimPrefix(key, prefix), ".zip")
	return id, validRecordID(id) && key == managedKey(kind, id)
}

func managedStoreForKey(key string) (*storage.ManagedStore, string, error) {
	parts := strings.Split(strings.TrimPrefix(key, managedPrefix), "/")
	if !strings.HasPrefix(key, managedPrefix) || len(parts) != 2 {
		return nil, "", errors.New("无效的托管云备份键")
	}
	if _, ok := managedObjectID(parts[0], key); !ok {
		return nil, "", errors.New("无效的托管云备份键")
	}
	mu.Lock()
	cfg := configured
	mu.Unlock()
	store, err := storage.NewManagedStore(cfg, parts[0])
	return store, parts[1], err
}

func (productionManagedCloud) Upload(ctx context.Context, key, path string) error {
	mu.Lock()
	secondary := configured.StorageSyncRole == "secondary"
	mu.Unlock()
	if secondary {
		return errors.New("次节点不能上传云备份")
	}
	store, name, err := managedStoreForKey(key)
	if err != nil {
		return err
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return store.Upload(ctx, name, file)
}

func (productionManagedCloud) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	store, name, err := managedStoreForKey(key)
	if err != nil {
		return nil, err
	}
	return store.Open(ctx, name)
}

func (productionManagedCloud) List(ctx context.Context, kind string) ([]managedObject, error) {
	mu.Lock()
	cfg := configured
	mu.Unlock()
	store, err := storage.NewManagedStore(cfg, kind)
	if err != nil {
		return nil, err
	}
	// Store.List completes every page in the configured folder before returning.
	objects, err := store.List(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]managedObject, 0, len(objects))
	for _, object := range objects {
		key := managedPrefix + kind + "/" + object.Name
		if _, ok := managedObjectID(kind, key); !ok {
			continue
		}
		result = append(result, managedObject{Key: key, Modified: object.LastModified, Size: object.Size})
	}
	return result, nil
}

func (productionManagedCloud) Delete(ctx context.Context, key string) error {
	store, name, err := managedStoreForKey(key)
	if err != nil {
		return err
	}
	return store.Delete(ctx, name)
}

// discoverLocal requires operationMu. Callers already creating a backup must
// not acquire it again. Only validated archives in the dedicated directory are
// eligible for discovery; unknown or damaged files are left untouched.
func (m *Manager) discoverLocal(ctx context.Context, kind string) error {
	kinds := []string{kind}
	if kind == "" {
		kinds = []string{"data", "attachments", "server-config"}
	}
	db, err := m.store()
	if err != nil {
		return err
	}
	for _, current := range kinds {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !validKind(current) {
			return errors.New("无效的备份类型")
		}
		if err := m.safeDirectory(current, false); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return err
		}
		entries, err := os.ReadDir(m.Directory(current))
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return err
			}
			name := entry.Name()
			id := strings.TrimSuffix(name, ".zip")
			if name != id+".zip" || !validRecordID(id) {
				continue
			}
			path := filepath.Join(m.Directory(current), name)
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 {
				continue
			}
			inspection, err := backup.InspectTypedArchive(path)
			if err != nil || inspection.Legacy || string(inspection.Kind) != current {
				continue
			}
			err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
				var record models.ManagedBackupRecord
				err := tx.First(&record, "id = ?", id).Error
				if errors.Is(err, gorm.ErrRecordNotFound) {
					finished := info.ModTime().UTC()
					record = models.ManagedBackupRecord{ID: id, Kind: current, CreatedAt: finished, FinishedAt: &finished, Trigger: "discovered", Target: "local", Scope: "unknown", Status: "success", Size: info.Size(), Local: true}
					return tx.Create(&record).Error
				}
				if err != nil {
					return err
				}
				if record.Kind != current || record.Status != "success" || record.Size != info.Size() {
					return nil
				}
				return tx.Model(&record).Update("local", true).Error
			})
			if err != nil {
				return err
			}
		}
	}
	return ctx.Err()
}

// Discover rebuilds cloud history on a clean database. It lists only the managed
// namespaces, never legacy backup.zip or attachment content. A complete listing
// is required before updating availability; failed listings leave audit rows as-is.
// No archive is copied locally and discovery never initiates a restore.
func (m *Manager) Discover(ctx context.Context, kind string) error {
	if kind != "" && !validKind(kind) {
		return errors.New("无效的备份类型")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if !m.cloud.Enabled() {
		return nil
	}
	releaseCloud, err := lockContext(ctx, &m.cloudMu)
	if err != nil {
		return err
	}
	defer releaseCloud()
	if err := ctx.Err(); err != nil {
		return err
	}
	kinds := []string{kind}
	if kind == "" {
		kinds = []string{"data", "attachments", "server-config"}
	}
	byKind := make(map[string][]managedObject, len(kinds))
	for _, current := range kinds {
		objects, err := m.cloud.List(ctx, current)
		if err != nil {
			return fmt.Errorf("发现 %s 云备份失败: %w", current, err)
		}
		byKind[current] = objects
	}
	db, err := m.store()
	if err != nil {
		return err
	}
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, current := range kinds {
			seen := make(map[string]bool)
			for _, object := range byKind[current] {
				id, ok := managedObjectID(current, object.Key)
				if !ok || object.Size <= 0 || object.Modified.IsZero() {
					continue
				}
				if seen[id] {
					return errors.New("云备份列表包含重复记录")
				}
				seen[id] = true
				var record models.ManagedBackupRecord
				err := tx.First(&record, "id = ?", id).Error
				if errors.Is(err, gorm.ErrRecordNotFound) {
					finished := object.Modified.UTC()
					record = models.ManagedBackupRecord{ID: id, Kind: current, CreatedAt: finished, FinishedAt: &finished, Trigger: "discovered", Target: "cloud", Scope: "unknown", Status: "success", Size: object.Size, Cloud: true, CloudKey: object.Key}
					if err := tx.Create(&record).Error; err != nil {
						return err
					}
					continue
				}
				if err != nil {
					return err
				}
				if record.Kind != current {
					return errors.New("云备份 ID 与已有备份类型冲突")
				}
				// Do not turn a running or failed upload into a successful backup.
				if record.Status != "success" {
					continue
				}
				if record.Size != object.Size {
					return errors.New("云备份大小与已保存的备份记录不一致")
				}
				if err := tx.Model(&record).Updates(map[string]interface{}{"cloud": true, "cloud_key": object.Key}).Error; err != nil {
					return err
				}
			}
			var records []models.ManagedBackupRecord
			if err := tx.Where("kind = ? AND cloud = ? AND status = ?", current, true, "success").Find(&records).Error; err != nil {
				return err
			}
			for _, record := range records {
				if seen[record.ID] {
					continue
				}
				if err := tx.Model(&record).Update("cloud", false).Error; err != nil {
					return err
				}
			}
		}
		return ctx.Err()
	})
}

// Retention only prunes a destination replaced by this successful backup. All
// candidates and the replacement are checked before deleting anything. Unknown,
// failed and incomplete objects never count toward the retained successful set.
func (m *Manager) retain(ctx context.Context, db *gorm.DB, plan models.ManagedBackupPlan, replacement models.ManagedBackupRecord) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := validatePlan(plan); err != nil {
		return err
	}
	if replacement.Status != "success" || replacement.Kind != plan.Kind {
		return errors.New("保留清理需要已成功创建的同类备份")
	}
	var local []models.ManagedBackupRecord
	var remote []managedObject
	if replacement.Local {
		if err := m.discoverLocal(ctx, plan.Kind); err != nil {
			return err
		}
		if err := m.safeDirectory(plan.Kind, false); err != nil {
			return err
		}
		var records []models.ManagedBackupRecord
		if err := db.Where("kind = ? AND local = ? AND status = ?", plan.Kind, true, "success").Order("created_at DESC, id DESC").Find(&records).Error; err != nil {
			return err
		}
		found := false
		for _, record := range records {
			path, err := m.recordPath(record)
			if err != nil {
				return err
			}
			info, err := os.Lstat(path)
			if errors.Is(err, os.ErrNotExist) && record.ID != replacement.ID {
				continue
			}
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() != record.Size {
				return errors.New("本地备份内容异常，已停止保留清理")
			}
			if record.ID == replacement.ID {
				found = true
				continue
			}
			local = append(local, record)
		}
		if !found {
			return errors.New("新本地备份不可用，已停止保留清理")
		}
	}
	if replacement.Cloud {
		objects, err := m.cloud.List(ctx, plan.Kind)
		if err != nil {
			return err
		}
		seen := make(map[string]bool)
		found := false
		for _, object := range objects {
			id, ok := managedObjectID(plan.Kind, object.Key)
			if !ok {
				continue
			}
			if seen[id] || object.Size <= 0 || object.Modified.IsZero() {
				return errors.New("云备份列表不完整，已停止保留清理")
			}
			seen[id] = true
			if id == replacement.ID {
				if object.Size != replacement.Size {
					return errors.New("新云备份大小不一致，已停止保留清理")
				}
				found = true
				continue
			}
			var known models.ManagedBackupRecord
			err := db.First(&known, "id = ?", id).Error
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			if err == nil && (known.Kind != plan.Kind || known.Status != "success" || known.Size != object.Size) {
				continue
			}
			remote = append(remote, object)
		}
		if !found {
			return errors.New("未发现刚上传的云备份，已停止保留清理")
		}
		sort.Slice(remote, func(i, j int) bool {
			if remote[i].Modified.Equal(remote[j].Modified) {
				return remote[i].Key > remote[j].Key
			}
			return remote[i].Modified.After(remote[j].Modified)
		})
	}
	// The replacement always occupies one retained slot, even with clock skew.
	for i, record := range local {
		if i < plan.KeepLocal-1 {
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		path, err := m.recordPath(record)
		if err != nil {
			return err
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := db.Model(&record).Update("local", false).Error; err != nil {
			return err
		}
	}
	for i, object := range remote {
		if i < plan.KeepCloud-1 {
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := m.cloud.Delete(ctx, object.Key); err != nil {
			return err
		}
		if err := db.Model(&models.ManagedBackupRecord{}).Where("kind = ? AND cloud_key = ?", plan.Kind, object.Key).Update("cloud", false).Error; err != nil {
			return err
		}
	}
	return nil
}
