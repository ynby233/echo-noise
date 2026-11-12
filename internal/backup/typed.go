package backup

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	appconfig "github.com/rcy1314/echo-noise/config"
	"github.com/spf13/viper"
	"gorm.io/gorm"
)

type ArchiveKind string

const (
	KindData             ArchiveKind = "data"
	KindAttachments      ArchiveKind = "attachments"
	KindServerConfig     ArchiveKind = "server-config"
	manifestName                     = "manifest.json"
	passwordArchiveName              = "plain-passwords.db"
	MaxArchiveEntries                = 100000
	MaxArchiveFileBytes  uint64      = 8 << 30
	MaxArchiveTotalBytes uint64      = 32 << 30
	maxManifestBytes                 = 4 << 20
	maxConfigBytes                   = 4 << 20
)

// AttachmentHooks lets the storage owner export inventory and materialize remote
// objects without making backup depend on HTTP services. Hooks may write only
// inside stage. PrepareDatabase returns a closed, self-contained SQLite candidate
// that changes only attachment metadata in the current database. It must never
// change site configuration, users or permissions. All hooks must honor ctx.
// PrepareRestore must check remote availability in original mode, returning a
// FallbackRequiredError when an explicit local migration is needed.
type AttachmentHooks struct {
	Export          func(ctx context.Context, db *gorm.DB, stage, scope string) (int64, []string, error)
	PrepareRestore  func(ctx context.Context, stage, mode string) error
	PrepareDatabase func(ctx context.Context, stage, currentDatabase, mode string) (string, error)
	// PrepareDataDatabase reapplies durable attachment migrations to a staged
	// data database while the current database is closed at startup.
	PrepareDataDatabase func(ctx context.Context, stage, currentDatabase string) error
}

var attachmentHookRegistry struct {
	sync.RWMutex
	hooks *AttachmentHooks
}

// ConfigureAttachmentHooks installs the storage integration used by DefaultLayout.
// Call once at process startup, before serving backup requests or starting jobs.
func ConfigureAttachmentHooks(hooks AttachmentHooks) {
	attachmentHookRegistry.Lock()
	defer attachmentHookRegistry.Unlock()
	copy := hooks
	attachmentHookRegistry.hooks = &copy
}

func defaultAttachmentHooks() *AttachmentHooks {
	attachmentHookRegistry.RLock()
	defer attachmentHookRegistry.RUnlock()
	return attachmentHookRegistry.hooks
}

type ManifestEntry struct {
	Path   string `json:"path"`
	Size   uint64 `json:"size"`
	SHA256 string `json:"sha256"`
}

type Manifest struct {
	Version         int             `json:"version"`
	Kind            ArchiveKind     `json:"kind"`
	CreatedAt       time.Time       `json:"createdAt"`
	Components      []string        `json:"components"`
	Entries         []ManifestEntry `json:"entries"`
	Warnings        []string        `json:"warnings,omitempty"`
	AttachmentCount int64           `json:"attachmentCount"`
	Scope           string          `json:"scope,omitempty"`
	AttachmentMode  string          `json:"attachmentMode,omitempty"`
}

type TypedArchiveInspection struct {
	Kind             ArchiveKind `json:"kind"`
	Legacy           bool        `json:"legacy"`
	Components       []string    `json:"components"`
	Warnings         []string    `json:"warnings"`
	AttachmentCount  int64       `json:"attachmentCount"`
	CloudAvailable   *bool       `json:"cloudAvailable,omitempty"`
	FallbackRequired bool        `json:"fallbackRequired"`
}

// FallbackRequiredError means original-location restore could not verify cloud
// storage. Callers must ask for explicit local-migration confirmation; they must
// never silently retry in local mode.
type FallbackRequiredError struct{ Cause error }

func (e *FallbackRequiredError) Error() string {
	return "cloud attachment storage unavailable; explicit local migration confirmation required"
}
func (e *FallbackRequiredError) Unwrap() error { return e.Cause }

func (l Layout) passwordPath() string {
	if strings.TrimSpace(l.PlainPasswordStorePath) != "" {
		return l.PlainPasswordStorePath
	}
	return filepath.Join(filepath.Dir(l.DatabasePath), passwordArchiveName)
}

func (l Layout) configRoot() string {
	if strings.TrimSpace(l.ConfigDir) != "" {
		return l.ConfigDir
	}
	return "config"
}

func validKind(kind ArchiveKind) bool {
	return kind == KindData || kind == KindAttachments || kind == KindServerConfig
}

// CreateTypedArchive writes an owner-readable ZIP only after a complete export
// and inspection. scope is local|all for attachments, default for other kinds.
// Authorization (including owner-only config access) belongs to the controller.
func CreateTypedArchive(ctx context.Context, destination string, db *gorm.DB, layout Layout, kind ArchiveKind, scope string) error {
	if !validKind(kind) {
		return errors.New("unsupported archive kind")
	}
	if kind == KindAttachments {
		if scope == "" {
			scope = "local"
		}
		if scope != "local" && scope != "all" {
			return errors.New("unsupported attachment scope")
		}
	} else if scope == "default" {
		scope = ""
	} else if scope != "" {
		return errors.New("scope is only valid for attachments")
	}
	archiveMu.Lock()
	defer archiveMu.Unlock()
	restoreMu.Lock()
	reserved := updateReservation != ""
	restoreMu.Unlock()
	if reserved {
		return errors.New("更新停机已准备，不能启动备份")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateLayout(layout); err != nil {
		return err
	}
	if err := rejectDestinationInsideSources(destination, layout); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(filepath.Dir(destination), ".echo-noise-typed-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	m := Manifest{Version: 1, Kind: kind, CreatedAt: time.Now().UTC(), Scope: scope}
	switch kind {
	case KindData:
		if db == nil {
			return errors.New("database not initialized")
		}
		if err := rejectSymlinkPath(layout.DatabasePath); err != nil {
			return err
		}
		if err := snapshotSQLite(db.WithContext(ctx), filepath.Join(stage, "database.db")); err != nil {
			return err
		}
		if _, err := os.Lstat(layout.passwordPath()); errors.Is(err, os.ErrNotExist) {
			if err := requireNoPasswordDependencies(filepath.Join(stage, "database.db")); err != nil {
				return err
			}
			if err := createEmptyPasswordStore(filepath.Join(stage, passwordArchiveName)); err != nil {
				return err
			}
			m.Warnings = append(m.Warnings, "本实例尚无独立密码库，备份包含空密码库；账户密码哈希保留，已有明文密码无法补回。")
		} else if err != nil {
			return err
		} else if err := snapshotPassword(ctx, layout.passwordPath(), filepath.Join(stage, passwordArchiveName)); err != nil {
			return err
		}
		m.Warnings = append(m.Warnings, "主数据库与独立密码库分别生成一致快照；跨库未锁定，并发修改密码可能导致两份快照时间不同。")
		if err := validatePasswordPair(filepath.Join(stage, "database.db"), filepath.Join(stage, passwordArchiveName)); err != nil {
			return err
		}
	case KindServerConfig:
		for _, name := range []string{"config.yaml", "runtime.env"} {
			source := filepath.Join(layout.configRoot(), name)
			if _, err := os.Lstat(source); errors.Is(err, os.ErrNotExist) {
				continue
			} else if err != nil {
				return err
			}
			if err := rejectSymlinkPath(source); err != nil {
				return err
			}
			if err := validateConfigFile(source, name); err != nil {
				return err
			}
			if err := copyPath(source, filepath.Join(stage, "server-config", name), 0600); err != nil {
				return err
			}
		}
		if _, err := os.Stat(filepath.Join(stage, "server-config")); err != nil {
			return errors.New("no allowlisted configuration files found")
		}
	case KindAttachments:
		if layout.AttachmentHooks == nil || layout.AttachmentHooks.Export == nil {
			return errors.New("attachment inventory export hook is required")
		}
		m.AttachmentCount, m.Warnings, err = layout.AttachmentHooks.Export(ctx, db, stage, scope)
		if err != nil {
			return err
		}
		if err := copyMediaToStage(stage, layout); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return publishTypedArchive(ctx, destination, stage, &m)
}

func snapshotPassword(ctx context.Context, source, destination string) error {
	if err := rejectSymlinkPath(source); err != nil {
		return err
	}
	if info, err := os.Stat(source); err != nil || !info.Mode().IsRegular() {
		return fmt.Errorf("independent password store is missing or not a regular file: %s", source)
	}
	db, err := OpenOffline(source, false)
	if err != nil {
		return fmt.Errorf("open independent password store: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	defer sqlDB.Close()
	if err := db.WithContext(ctx).Exec("VACUUM INTO ?", destination).Error; err != nil {
		return fmt.Errorf("snapshot independent password store: %w", err)
	}
	return validatePasswordSQLite(destination)
}

func validatePasswordSQLite(source string) error {
	store, err := OpenOffline(source, false)
	if err != nil {
		return err
	}
	db, err := store.DB()
	if err != nil {
		return err
	}
	defer db.Close()
	var check string
	if err := db.QueryRow("PRAGMA quick_check").Scan(&check); err != nil {
		return err
	}
	if check != "ok" {
		return errors.New("password store integrity check failed")
	}
	var count int
	if err := db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name='plain_password_records'").Scan(&count); err != nil {
		return err
	}
	if count != 1 {
		return errors.New("password store is missing plain_password_records")
	}
	return nil
}

func copyMediaToStage(stage string, layout Layout) error {
	for _, root := range layout.Roots {
		if err := rejectSymlinkPath(root.Path); err != nil {
			return err
		}
		target := filepath.Join(stage, root.ArchiveName)
		if _, err := os.Stat(root.Path); errors.Is(err, os.ErrNotExist) {
			if err := os.MkdirAll(target, 0700); err != nil {
				return err
			}
			continue
		} else if err != nil {
			return err
		}
		// Export may already have downloaded remote objects into this tree.
		if err := os.MkdirAll(target, 0700); err != nil {
			return err
		}
		if err := mergeMedia(root.Path, target); err != nil {
			return err
		}
	}
	return nil
}

func mergeMedia(source, target string) error {
	return filepath.Walk(source, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || (!info.IsDir() && !info.Mode().IsRegular()) {
			return fmt.Errorf("unsupported media source: %s", p)
		}
		rel, err := filepath.Rel(source, p)
		if err != nil {
			return err
		}
		dst := filepath.Join(target, rel)
		if info.IsDir() {
			return os.MkdirAll(dst, 0700)
		}
		if _, err := os.Stat(dst); err == nil {
			a, err := hashFile(p)
			if err != nil {
				return err
			}
			b, err := hashFile(dst)
			if err != nil {
				return err
			}
			if a != b {
				return fmt.Errorf("conflicting exported attachment: %s", rel)
			}
			return nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return copyPath(p, dst, 0600)
	})
}

func hashFile(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func publishTypedArchive(ctx context.Context, destination, stage string, m *Manifest) error {
	m.Entries = nil
	components := map[string]bool{}
	var total uint64
	err := filepath.Walk(stage, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if p == stage {
			return nil
		}
		if info.Mode()&os.ModeSymlink != 0 || (!info.IsDir() && !info.Mode().IsRegular()) {
			return errors.New("unsupported staged entry")
		}
		rel, err := filepath.Rel(stage, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == manifestName {
			return errors.New("hook must not write manifest.json")
		}
		if err := allowedTypedPath(m.Kind, rel, info.IsDir()); err != nil {
			return err
		}
		components[strings.Split(rel, "/")[0]] = true
		if info.IsDir() {
			return nil
		}
		if info.Size() < 0 || uint64(info.Size()) > MaxArchiveFileBytes {
			return errors.New("archive file size limit exceeded")
		}
		total += uint64(info.Size())
		if total > MaxArchiveTotalBytes || len(m.Entries) >= MaxArchiveEntries-1 {
			return errors.New("archive limits exceeded")
		}
		hash, err := hashFile(p)
		if err != nil {
			return err
		}
		m.Entries = append(m.Entries, ManifestEntry{Path: rel, Size: uint64(info.Size()), SHA256: hash})
		return nil
	})
	if err != nil {
		return err
	}
	m.Components = nil
	for c := range components {
		m.Components = append(m.Components, c)
	}
	sort.Strings(m.Components)
	temp, err := os.CreateTemp(filepath.Dir(destination), ".echo-noise-archive-*.zip")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	z := zip.NewWriter(temp)
	writeErr := filepath.Walk(stage, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if p == stage {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		rel, err := filepath.Rel(stage, p)
		if err != nil {
			return err
		}
		if info.IsDir() {
			return addDirectoryHeader(z, filepath.ToSlash(rel))
		}
		return addFile(z, p, filepath.ToSlash(rel))
	})
	if writeErr == nil {
		var out io.Writer
		out, writeErr = z.Create(manifestName)
		if writeErr == nil {
			writeErr = json.NewEncoder(out).Encode(m)
		}
	}
	closeErr := z.Close()
	syncErr := temp.Sync()
	fileErr := temp.Close()
	for _, err := range []error{writeErr, closeErr, syncErr, fileErr} {
		if err != nil {
			return err
		}
	}
	if _, err := InspectTypedArchive(tempName); err != nil {
		return err
	}
	return os.Rename(tempName, destination)
}

// InspectTypedArchive verifies every entry's CRC, size, and (for typed archives)
// manifest digest without extracting files or opening a live database.
func InspectTypedArchive(source string) (TypedArchiveInspection, error) {
	inspection, _, err := readArchive(source)
	return inspection, err
}

func readArchive(source string) (TypedArchiveInspection, *Manifest, error) {
	bad := func(err error) (TypedArchiveInspection, *Manifest, error) { return TypedArchiveInspection{}, nil, err }
	if err := rejectSymlinkPath(source); err != nil {
		return bad(err)
	}
	z, err := zip.OpenReader(source)
	if err != nil {
		return bad(err)
	}
	defer z.Close()
	if len(z.File) == 0 || len(z.File) > MaxArchiveEntries {
		return bad(errors.New("archive entry count limit exceeded"))
	}
	files := map[string]*zip.File{}
	caseNames := map[string]bool{}
	pathKinds := map[string]string{}
	var total uint64
	var manifestFile *zip.File
	for _, f := range z.File {
		name, err := safeArchiveName(f.Name)
		if err != nil {
			return bad(err)
		}
		if f.Mode()&os.ModeSymlink != 0 || (!f.FileInfo().IsDir() && !f.Mode().IsRegular()) {
			return bad(fmt.Errorf("unsupported archive entry: %s", name))
		}
		if strings.HasSuffix(f.Name, "/") != f.FileInfo().IsDir() {
			return bad(errors.New("archive entry type disagrees with path"))
		}
		for current := name; current != "."; current = path.Dir(current) {
			folded := strings.ToLower(current)
			if prior, ok := pathKinds[folded]; ok && prior != current {
				return bad(errors.New("archive path casing collision"))
			}
			pathKinds[folded] = current
		}
		if caseNames[strings.ToLower(name)] {
			return bad(fmt.Errorf("duplicate archive path: %s", name))
		}
		caseNames[strings.ToLower(name)] = true
		if f.UncompressedSize64 > MaxArchiveFileBytes || total > MaxArchiveTotalBytes-f.UncompressedSize64 {
			return bad(errors.New("archive size limit exceeded"))
		}
		total += f.UncompressedSize64
		files[name] = f
		if name == manifestName {
			manifestFile = f
		}
	}
	for name, f := range files {
		for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
			if entry := files[parent]; entry != nil && !entry.FileInfo().IsDir() {
				return bad(errors.New("archive file/directory path collision"))
			}
		}
		if f.FileInfo().IsDir() && f.UncompressedSize64 != 0 {
			return bad(errors.New("directory contains file data"))
		}
	}
	inspection := TypedArchiveInspection{Legacy: manifestFile == nil, Components: []string{}, Warnings: []string{}}
	var m *Manifest
	if manifestFile != nil {
		if manifestFile.FileInfo().IsDir() || manifestFile.UncompressedSize64 > maxManifestBytes {
			return bad(errors.New("invalid archive manifest size"))
		}
		in, err := manifestFile.Open()
		if err != nil {
			return bad(err)
		}
		decoder := json.NewDecoder(io.LimitReader(in, maxManifestBytes+1))
		decoder.DisallowUnknownFields()
		m = &Manifest{}
		err = decoder.Decode(m)
		if err == nil {
			var extra interface{}
			if decoder.Decode(&extra) != io.EOF {
				err = errors.New("trailing manifest data")
			}
		}
		closeErr := in.Close()
		if err != nil {
			return bad(err)
		}
		if closeErr != nil {
			return bad(closeErr)
		}
		if m.Version != 1 || !validKind(m.Kind) || m.AttachmentCount < 0 || len(m.Entries) > MaxArchiveEntries-1 {
			return bad(errors.New("unsupported or invalid manifest"))
		}
		if m.Kind == KindAttachments {
			if m.Scope != "local" && m.Scope != "all" {
				return bad(errors.New("invalid manifest attachment scope"))
			}
			if m.AttachmentMode != "" && m.AttachmentMode != "original" && m.AttachmentMode != "local" {
				return bad(errors.New("invalid manifest attachment mode"))
			}
		} else if m.Scope != "" || m.AttachmentMode != "" {
			return bad(errors.New("unexpected attachment options"))
		}
		inspection.Kind, inspection.AttachmentCount = m.Kind, m.AttachmentCount
		inspection.Warnings = append(inspection.Warnings, m.Warnings...)
	} else {
		inspection.Kind = KindData
		if f := files["database.db"]; f == nil || f.FileInfo().IsDir() {
			return bad(errors.New("legacy archive is missing database.db"))
		}
		inspection.Warnings = append(inspection.Warnings, "旧版备份可能缺少独立密码库和配置；缺少密码库时将清空目标密码库，请恢复后补录不可用的凭据。")
	}
	entries := map[string]ManifestEntry{}
	if m != nil {
		for _, entry := range m.Entries {
			name, err := safeArchiveName(entry.Path)
			if err != nil {
				return bad(err)
			}
			if name != entry.Path || len(entry.SHA256) != 64 {
				return bad(errors.New("invalid manifest entry"))
			}
			if _, err := hex.DecodeString(entry.SHA256); err != nil {
				return bad(err)
			}
			if _, exists := entries[name]; exists {
				return bad(errors.New("duplicate manifest entry"))
			}
			f := files[name]
			if f == nil || f.FileInfo().IsDir() || f.UncompressedSize64 != entry.Size {
				return bad(fmt.Errorf("manifest payload missing or wrong size: %s", name))
			}
			entries[name] = entry
		}
	}
	components := map[string]bool{}
	for name, f := range files {
		if name == manifestName {
			continue
		}
		if m != nil {
			if err := allowedTypedPath(m.Kind, name, f.FileInfo().IsDir()); err != nil {
				return bad(err)
			}
		}
		if inspection.Legacy && name != "database.db" && name != passwordArchiveName {
			root := strings.Split(name, "/")[0]
			if root != "attachment-blobs" && root != "images" && root != "video" && root != "audio" && root != "attachments" && root != "protected-config" {
				return bad(fmt.Errorf("unsupported legacy payload: %s", name))
			}
			if !f.FileInfo().IsDir() && root == name {
				return bad(errors.New("legacy media root must be a directory"))
			}
		}
		components[strings.Split(name, "/")[0]] = true
		if f.FileInfo().IsDir() {
			continue
		}
		entry, listed := entries[name]
		if m != nil && !listed {
			return bad(fmt.Errorf("unlisted archive payload: %s", name))
		}
		in, err := f.Open()
		if err != nil {
			return bad(err)
		}
		h := sha256.New()
		copied, copyErr := io.Copy(h, io.LimitReader(in, int64(f.UncompressedSize64)+1))
		closeErr := in.Close()
		if copyErr != nil {
			return bad(copyErr)
		}
		if closeErr != nil {
			return bad(closeErr)
		}
		if copied != int64(f.UncompressedSize64) {
			return bad(errors.New("archive payload size mismatch"))
		}
		if m != nil && hex.EncodeToString(h.Sum(nil)) != entry.SHA256 {
			return bad(fmt.Errorf("archive checksum mismatch: %s", name))
		}
	}
	for c := range components {
		inspection.Components = append(inspection.Components, c)
	}
	sort.Strings(inspection.Components)
	if m != nil {
		if strings.Join(m.Components, "\x00") != strings.Join(inspection.Components, "\x00") {
			return bad(errors.New("manifest components mismatch"))
		}
		switch m.Kind {
		case KindData:
			if files["database.db"] == nil || files[passwordArchiveName] == nil {
				return bad(errors.New("data archive requires database and independent password store"))
			}
		case KindServerConfig:
			if len(entries) == 0 {
				return bad(errors.New("empty configuration archive"))
			}
			inspection.Warnings = append(inspection.Warnings, "仅包含 config.yaml 与 runtime.env；挂载密钥、容器设置及外部服务配置须另行保管。恢复不得变更数据库或附件目录。")
		case KindAttachments:
			if files["attachment-inventory.json"] == nil {
				return bad(errors.New("attachment inventory missing"))
			}
			if hasComponent(inspection, "cloud-attachment-content") {
				inspection.Warnings = append(inspection.Warnings, "恢复原云位置前将检查目标云存储；改存本地须明确确认。")
			}
		}
	}
	return inspection, m, nil
}

func allowedTypedPath(kind ArchiveKind, name string, directory bool) error {
	root := strings.Split(name, "/")[0]
	switch kind {
	case KindData:
		if !directory && (name == "database.db" || name == passwordArchiveName) {
			return nil
		}
	case KindServerConfig:
		if directory && name == "server-config" {
			return nil
		}
		if !directory && (name == "server-config/runtime.env" || name == "server-config/config.yaml") {
			return nil
		}
	case KindAttachments:
		if !directory && name == "attachment-inventory.json" {
			return nil
		}
		if root == "attachment-blobs" || root == "cloud-attachment-content" || root == "images" || root == "video" || root == "audio" || root == "attachments" {
			if directory || strings.Contains(name, "/") {
				return nil
			}
		}
	}
	return fmt.Errorf("payload %s is not allowed for archive kind %s", name, kind)
}

func StageTypedRestore(ctx context.Context, source string, layout Layout, attachmentMode string) (StageResult, error) {
	restoreMu.Lock()
	defer restoreMu.Unlock()
	if updateReservation != "" {
		return StageResult{}, errors.New("更新停机已准备，不能同时恢复备份")
	}
	if attachmentMode == "" {
		attachmentMode = "original"
	}
	if attachmentMode != "original" && attachmentMode != "local" {
		return StageResult{}, errors.New("unsupported attachment restore mode")
	}
	inspection, m, err := readArchive(source)
	if err != nil {
		return StageResult{}, err
	}
	layout.pendingKind = inspection.Kind
	if err := validatePendingCombination(layout, inspection); err != nil {
		return StageResult{}, err
	}
	if err := validateLayout(layout); err != nil {
		return StageResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return StageResult{}, err
	}
	if err := os.MkdirAll(filepath.Dir(layout.DatabasePath), 0700); err != nil {
		return StageResult{}, err
	}
	stage, err := os.MkdirTemp(filepath.Dir(layout.DatabasePath), ".echo-noise-typed-restore-*")
	if err != nil {
		return StageResult{}, err
	}
	defer os.RemoveAll(stage)
	if err := extractArchive(source, stage); err != nil {
		return StageResult{}, err
	}
	if err := validateStagedPayload(stage, inspection); err != nil {
		return StageResult{}, err
	}
	if inspection.Kind == KindAttachments && !inspection.Legacy {
		if layout.AttachmentHooks == nil || layout.AttachmentHooks.PrepareRestore == nil || layout.AttachmentHooks.PrepareDatabase == nil {
			return StageResult{}, errors.New("attachment restore hooks are required")
		}
		if err := layout.AttachmentHooks.PrepareRestore(ctx, stage, attachmentMode); err != nil {
			return StageResult{}, err
		}
		m.AttachmentMode = attachmentMode
	}
	pairs, err := selectedRestorePairs(stage, layout, inspection)
	if err != nil {
		return StageResult{}, err
	}
	if err := preflightPairs(pairs); err != nil {
		return StageResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return StageResult{}, err
	}
	publishSource := source
	if m != nil && m.Kind == KindAttachments {
		if err := os.Remove(filepath.Join(stage, manifestName)); err != nil {
			return StageResult{}, err
		}
		publishSource = filepath.Join(filepath.Dir(stage), filepath.Base(stage)+".zip")
		defer os.Remove(publishSource)
		if err := publishTypedArchive(ctx, publishSource, stage, m); err != nil {
			return StageResult{}, err
		}
		inspection, err = InspectTypedArchive(publishSource)
		if err != nil {
			return StageResult{}, err
		}
	}
	if err := publishPendingRestore(publishSource, layout); err != nil {
		return StageResult{}, err
	}
	result := StageResult{Inspection: inspection}
	if inspection.Legacy && !hasComponent(inspection, "attachment-blobs") && !hasComponent(inspection, "attachments") {
		result.Warning = incompleteMigrationWarning
	}
	result.Warning = strings.TrimSpace(strings.Join(append(inspection.Warnings, result.Warning), "；"))
	return result, nil
}

func hasComponent(i TypedArchiveInspection, component string) bool {
	for _, c := range i.Components {
		if c == component {
			return true
		}
	}
	return false
}

func validateStagedPayload(stage string, inspection TypedArchiveInspection) error {
	if inspection.Legacy || inspection.Kind == KindData {
		if err := validateSQLite(filepath.Join(stage, "database.db")); err != nil {
			return fmt.Errorf("invalid backup database: %w", err)
		}
	}
	if hasComponent(inspection, passwordArchiveName) {
		if err := validatePasswordSQLite(filepath.Join(stage, passwordArchiveName)); err != nil {
			return err
		}
	}
	if (inspection.Legacy || inspection.Kind == KindData) && hasComponent(inspection, passwordArchiveName) {
		if err := validatePasswordPair(filepath.Join(stage, "database.db"), filepath.Join(stage, passwordArchiveName)); err != nil {
			return err
		}
	}
	if inspection.Kind == KindServerConfig {
		for _, name := range []string{"config.yaml", "runtime.env"} {
			p := filepath.Join(stage, "server-config", name)
			if _, err := os.Stat(p); errors.Is(err, os.ErrNotExist) {
				continue
			} else if err != nil {
				return err
			}
			if err := validateConfigFile(p, name); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateConfigFile(p, name string) error {
	info, err := os.Stat(p)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() > maxConfigBytes {
		return errors.New("invalid configuration file size or type")
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	if strings.ContainsRune(string(data), '\x00') {
		return errors.New("configuration contains NUL")
	}
	if name == "config.yaml" {
		v := viper.New()
		v.SetConfigType("yaml")
		if err := v.ReadConfig(strings.NewReader(string(data))); err != nil {
			return fmt.Errorf("invalid config.yaml: %w", err)
		}
		var parsed appconfig.AppConfig
		if err := v.Unmarshal(&parsed); err != nil {
			return fmt.Errorf("invalid config.yaml values: %w", err)
		}
		return nil
	}
	// runtime.env is sourced by the container entrypoint. Accept literal
	// assignments only, never executable shell substitutions or directives.
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, value, ok := strings.Cut(line, "=")
		if !ok || key == "" {
			return errors.New("invalid runtime.env assignment")
		}
		for index, c := range key {
			if !(c == '_' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || index > 0 && c >= '0' && c <= '9') {
				return errors.New("invalid runtime.env key")
			}
		}
		if !allowedRuntimeKey(key) {
			return fmt.Errorf("runtime.env 包含不支持恢复的变量 %s", key)
		}
		if strings.ContainsAny(value, "`;$<>|&\\") {
			return errors.New("runtime.env must contain literal values")
		}
		if strings.HasPrefix(value, "\"") || strings.HasPrefix(value, "'") {
			if len(value) < 2 || value[len(value)-1] != value[0] || strings.ContainsRune(value[1:len(value)-1], rune(value[0])) {
				return errors.New("invalid runtime.env quoting")
			}
		} else if strings.ContainsAny(value, " \t\"'") {
			return errors.New("runtime.env values with spaces must be quoted")
		}
	}
	return nil
}

func selectedRestorePairs(stage string, layout Layout, inspection TypedArchiveInspection) ([]renamePair, error) {
	pairs := []renamePair{}
	if inspection.Legacy || inspection.Kind == KindData {
		pairs = append(pairs, renamePair{filepath.Join(stage, "database.db"), layout.DatabasePath})
	}
	if hasComponent(inspection, passwordArchiveName) {
		pairs = append(pairs, renamePair{filepath.Join(stage, passwordArchiveName), layout.passwordPath()})
	}
	if inspection.Kind == KindServerConfig {
		if err := validateRestoreConfiguration(stage, layout); err != nil {
			return nil, err
		}
		for _, name := range []string{"config.yaml", "runtime.env"} {
			p := filepath.Join(stage, "server-config", name)
			if _, err := os.Stat(p); err == nil {
				pairs = append(pairs, renamePair{p, filepath.Join(layout.configRoot(), name)})
			} else if !errors.Is(err, os.ErrNotExist) {
				return nil, err
			}
		}
	}
	if inspection.Legacy || inspection.Kind == KindAttachments {
		known := map[string]bool{}
		for _, root := range layout.Roots {
			known[root.ArchiveName] = true
			p := filepath.Join(stage, root.ArchiveName)
			if info, err := os.Stat(p); err == nil && info.IsDir() {
				if inspection.Kind == KindAttachments && !inspection.Legacy {
					if _, err := os.Stat(root.Path); err == nil {
						if err := mergeMedia(root.Path, p); err != nil {
							return nil, err
						}
					} else if !errors.Is(err, os.ErrNotExist) {
						return nil, err
					}
				}
				pairs = append(pairs, renamePair{p, root.Path})
			} else if err != nil && !errors.Is(err, os.ErrNotExist) {
				return nil, err
			}
		}
		for _, c := range inspection.Components {
			if c == "attachment-blobs" || c == "images" || c == "video" || c == "audio" || c == "attachments" {
				if !known[c] {
					return nil, fmt.Errorf("missing destination for archive media root %s", c)
				}
			}
		}
	}
	return pairs, nil
}

func validateLayout(layout Layout) error {
	if strings.TrimSpace(layout.DatabasePath) == "" {
		return errors.New("database path is empty")
	}
	paths := []string{layout.DatabasePath, layout.passwordPath(), layout.configRoot()}
	names := map[string]bool{}
	for _, root := range layout.Roots {
		name, err := safeArchiveName(root.ArchiveName)
		if err != nil || name != root.ArchiveName || strings.Contains(name, "/") || name == "database.db" || name == passwordArchiveName || name == manifestName || name == "server-config" || strings.TrimSpace(root.Path) == "" || names[name] {
			return errors.New("invalid or duplicate media layout root")
		}
		names[name] = true
		paths = append(paths, root.Path)
	}
	for n, p := range paths {
		if err := rejectSymlinkPath(p); err != nil {
			return err
		}
		a, err := filepath.Abs(p)
		if err != nil {
			return err
		}
		for _, other := range paths[:n] {
			b, err := filepath.Abs(other)
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(a, b)
			if err != nil {
				continue
			}
			reverse, err := filepath.Rel(b, a)
			if err != nil {
				continue
			}
			inside := func(r string) bool {
				return r == "." || r != ".." && !strings.HasPrefix(r, ".."+string(filepath.Separator))
			}
			if inside(rel) || inside(reverse) {
				return errors.New("overlapping restore destinations")
			}
		}
	}
	return nil
}

func rejectSymlinkPath(p string) error {
	absolute, err := filepath.Abs(p)
	if err != nil {
		return err
	}
	for current := absolute; ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink path is unsupported: %s", current)
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if filepath.Dir(current) == current {
			break
		}
	}
	return nil
}

func rejectDestinationInsideSources(destination string, layout Layout) error {
	dst, err := filepath.Abs(destination)
	if err != nil {
		return err
	}
	if err := rejectSymlinkPath(dst); err != nil {
		return err
	}
	paths := []string{layout.DatabasePath, layout.passwordPath(), layout.configRoot()}
	for _, root := range layout.Roots {
		paths = append(paths, root.Path)
	}
	for _, source := range paths {
		a, err := filepath.Abs(source)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(a, dst)
		if err != nil {
			continue
		}
		if rel == "." || rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return errors.New("backup destination is inside a backup source")
		}
	}
	return nil
}

// preflightPairs checks permissions and opens existing target files without
// truncation. Read-only secret/config mounts fail before pending publication.
// Startup repeats this check before preparing or replacing any selected target.
func preflightPairs(pairs []renamePair) error {
	for _, pair := range pairs {
		if err := rejectSymlinkPath(pair.to); err != nil {
			return err
		}
		info, err := os.Stat(pair.to)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err == nil && info.Mode().Perm()&0222 == 0 {
			return fmt.Errorf("restore target is read-only: %s", pair.to)
		}
		parent := filepath.Dir(pair.to)
		if err == nil && info.IsDir() {
			parent = pair.to
		}
		if err := os.MkdirAll(parent, 0700); err != nil {
			return err
		}
		parentInfo, err := os.Stat(parent)
		if err != nil {
			return err
		}
		if parentInfo.Mode().Perm()&0222 == 0 {
			return fmt.Errorf("restore destination is read-only: %s", parent)
		}
		probe, err := os.CreateTemp(parent, ".echo-noise-restore-probe-*")
		if err != nil {
			return fmt.Errorf("restore destination is not writable: %w", err)
		}
		probeName := probe.Name()
		closeErr := probe.Close()
		removeErr := os.Remove(probeName)
		if closeErr != nil {
			return closeErr
		}
		if removeErr != nil {
			return removeErr
		}
		if info != nil && !info.IsDir() {
			// Opening without truncation explicitly rejects read-only file mounts.
			file, err := os.OpenFile(pair.to, os.O_WRONLY, 0)
			if err != nil {
				return fmt.Errorf("restore file is not writable: %s: %w", pair.to, err)
			}
			if err := file.Close(); err != nil {
				return err
			}
		}
	}
	return nil
}
