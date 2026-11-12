package services

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/glebarez/sqlite"
	"github.com/rcy1314/echo-noise/internal/attachments"
	"github.com/rcy1314/echo-noise/internal/backup"
	"github.com/rcy1314/echo-noise/internal/database"
	"github.com/rcy1314/echo-noise/internal/models"
	"github.com/rcy1314/echo-noise/internal/storage"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

const AttachmentInventoryName = "attachment-inventory.json"
const AttachmentRestoreOriginal = "original"
const AttachmentRestoreLocal = "local"

var ErrAttachmentFallbackRequired = errors.New("cloud attachments require explicitly selected local restore")

type AttachmentFallbackRequiredError struct{ Cause error }

func (e *AttachmentFallbackRequiredError) Error() string {
	return fmt.Sprintf("%s: %v", ErrAttachmentFallbackRequired, e.Cause)
}
func (e *AttachmentFallbackRequiredError) Unwrap() error { return e.Cause }
func (e *AttachmentFallbackRequiredError) Is(target error) bool {
	return target == ErrAttachmentFallbackRequired
}

// AttachmentArchiveEntry describes one physical object; references are stored separately.
// Size is -1 when an old cloud object has no persisted size. Export fills it in.
type AttachmentArchiveEntry struct {
	BlobID   uint   `json:"blob_id,omitempty"`
	LegacyID uint   `json:"legacy_id,omitempty"`
	Backend  string `json:"backend"`
	Key      string `json:"key"`
	Hash     string `json:"hash"`
	Size     int64  `json:"size"`
	File     string `json:"file"`
}

// AttachmentInventory contains no site configuration, credentials, users, or permissions.
type AttachmentInventory struct {
	Version    int                            `json:"version"`
	Scope      string                         `json:"scope"`
	Entries    []AttachmentArchiveEntry       `json:"entries"`
	Blobs      []models.AttachmentBlob        `json:"-"`
	References []models.AttachmentReference   `json:"-"`
	Legacy     []models.CloudAttachmentObject `json:"-"`
	Grants     []models.LocalAttachmentGrant  `json:"-"`
}

// The model JSON intentionally hides physical keys/hashes, so the archive uses
// independent aliases with explicit fields rather than serializing public API models.
type archiveBlob struct {
	ID                              uint
	Backend, Key, Hash, ContentType string
	Size                            int64
}
type archiveReference struct {
	ID, BlobID, OwnerUserID      uint
	PublicID, Kind, OriginalName string
}
type archiveLegacy struct {
	ID                                             uint
	PublicID, Key, OriginalName, ContentType, Hash string
}
type attachmentArchiveDocument struct {
	Version    int
	Scope      string
	Entries    []AttachmentArchiveEntry
	Blobs      []archiveBlob
	References []archiveReference
	Legacy     []archiveLegacy
	Grants     []models.LocalAttachmentGrant
}

// InventoryAttachmentArchive inventories physical registry blobs, including unreferenced
// ones, and the legacy cloud registry. It never silently excludes a registered backend.
func InventoryAttachmentArchive(ctx context.Context, db *gorm.DB, scope string) (AttachmentInventory, error) {
	inventory := AttachmentInventory{Version: 1, Scope: scope}
	if db == nil || (scope != "local" && scope != "all") {
		return inventory, errors.New("attachment inventory requires a database and scope local or all")
	}
	db = db.WithContext(ctx)
	if err := ctx.Err(); err != nil {
		return inventory, err
	}
	if db.Migrator().HasTable(&models.AttachmentBlob{}) {
		var blobs []models.AttachmentBlob
		if err := db.Order("id").Find(&blobs).Error; err != nil {
			return inventory, err
		}
		included := map[uint]bool{}
		for _, blob := range blobs {
			if blob.StorageBackend != "local" && blob.StorageBackend != "cloud" {
				return inventory, fmt.Errorf("attachment blob %d has unsupported backend %q", blob.ID, blob.StorageBackend)
			}
			if scope == "local" && blob.StorageBackend == "cloud" {
				continue
			}
			if !validAttachmentHash(blob.ContentHash) || blob.Size < 0 {
				return inventory, fmt.Errorf("attachment blob %d has invalid hash or size", blob.ID)
			}
			entry := AttachmentArchiveEntry{BlobID: blob.ID, Backend: blob.StorageBackend, Key: blob.StorageKey, Hash: blob.ContentHash, Size: blob.Size}
			entry.File = attachmentEntryFile(entry)
			inventory.Entries = append(inventory.Entries, entry)
			inventory.Blobs = append(inventory.Blobs, blob)
			included[blob.ID] = true
		}
		if db.Migrator().HasTable(&models.AttachmentReference{}) {
			var refs []models.AttachmentReference
			if err := db.Order("id").Find(&refs).Error; err != nil {
				return inventory, err
			}
			for _, ref := range refs {
				if included[ref.BlobID] {
					inventory.References = append(inventory.References, ref)
				}
				if !included[ref.BlobID] {
					found := false
					for _, blob := range blobs {
						if blob.ID == ref.BlobID {
							found = true
							break
						}
					}
					if !found {
						return inventory, fmt.Errorf("attachment reference %q points to missing blob %d", ref.PublicID, ref.BlobID)
					}
				}
			}
		}
	}
	if scope == "all" && db.Migrator().HasTable(&models.CloudAttachmentObject{}) {
		if err := db.Order("id").Find(&inventory.Legacy).Error; err != nil {
			return inventory, err
		}
		for _, object := range inventory.Legacy {
			entry := AttachmentArchiveEntry{LegacyID: object.ID, Backend: "cloud", Key: object.ObjectKey, Hash: object.ContentHash, Size: -1}
			entry.File = attachmentEntryFile(entry)
			inventory.Entries = append(inventory.Entries, entry)
		}
	}
	if db.Migrator().HasTable(&models.LocalAttachmentGrant{}) {
		query := db.Order("id")
		if scope == "local" {
			query = query.Where("kind <> ?", "cloud")
		}
		if err := query.Find(&inventory.Grants).Error; err != nil {
			return inventory, err
		}
	}
	return inventory, ctx.Err()
}

func validAttachmentHash(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && value == strings.ToLower(value)
}

func attachmentEntryFile(entry AttachmentArchiveEntry) string {
	if entry.Backend == "local" {
		return "attachment-blobs/" + entry.Hash[:2] + "/" + entry.Hash
	}
	if entry.LegacyID != 0 {
		return fmt.Sprintf("cloud-attachment-content/legacy-%d.blob", entry.LegacyID)
	}
	return fmt.Sprintf("cloud-attachment-content/blob-%d.blob", entry.BlobID)
}

// FetchAttachmentArchiveContent fetches full content and checks persisted size and SHA256.
// On error dst may contain partial bytes; callers must discard their temporary destination.
func FetchAttachmentArchiveContent(ctx context.Context, entry AttachmentArchiveEntry, localRoot string, site models.SiteConfig, dst io.Writer) (int64, error) {
	if dst == nil {
		return 0, errors.New("attachment destination is nil")
	}
	hash := sha256.New()
	writer := io.MultiWriter(dst, hash)
	var n int64
	var err error
	switch entry.Backend {
	case "local":
		var source string
		source, err = safeAttachmentPath(localRoot, entry.Key, false)
		if err == nil {
			var file *os.File
			file, err = os.Open(source)
			if err == nil {
				n, err = storage.CopyContext(ctx, writer, file)
				closeErr := file.Close()
				if err == nil {
					err = closeErr
				}
			}
		}
		if errors.Is(err, os.ErrNotExist) {
			err = fmt.Errorf("%w: local blob %q", storage.ErrMissingContent, entry.Key)
		}
	case "cloud":
		n, err = storage.DownloadAttachmentObject(ctx, site, entry.Key, writer)
	default:
		err = fmt.Errorf("unsupported attachment backend %q", entry.Backend)
	}
	if err != nil {
		return n, fmt.Errorf("attachment %s/%q: %w", entry.Backend, entry.Key, err)
	}
	if entry.Size >= 0 && n != entry.Size {
		return n, fmt.Errorf("attachment %q size mismatch: expected %d, got %d", entry.Key, entry.Size, n)
	}
	if entry.Hash != "" && hex.EncodeToString(hash.Sum(nil)) != entry.Hash {
		return n, fmt.Errorf("attachment %q SHA256 mismatch", entry.Key)
	}
	return n, ctx.Err()
}

// safeAttachmentPath rejects traversal and every symlink component, even inside stage.
func safeAttachmentPath(root, relative string, create bool) (string, error) {
	if root == "" || relative == "" || strings.Contains(relative, "\\") || strings.Contains(relative, ":") || filepath.IsAbs(relative) {
		return "", errors.New("invalid attachment path")
	}
	parts := strings.Split(relative, "/")
	current := root
	if info, err := os.Lstat(root); err != nil {
		return "", err
	} else if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("attachment root is not a real directory")
	}
	for index, part := range parts {
		if part == "" || part == "." || part == ".." {
			return "", errors.New("invalid attachment path component")
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) && create {
			if index < len(parts)-1 {
				if err := os.Mkdir(current, 0755); err != nil {
					return "", err
				}
			}
			continue
		}
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("attachment path contains a symlink")
		}
		if index < len(parts)-1 && !info.IsDir() {
			return "", errors.New("attachment path parent is not a directory")
		}
		if index == len(parts)-1 && !info.Mode().IsRegular() {
			return "", errors.New("attachment content is not a regular file")
		}
	}
	return current, nil
}

// ExportAttachmentArchive writes only attachment data into an existing isolated stage.
// The manifest is published last; any missing/truncated/corrupt object fails the export.
func ExportAttachmentArchive(ctx context.Context, db *gorm.DB, stage, scope string) (int64, []string, error) {
	inventory, err := InventoryAttachmentArchive(ctx, db, scope)
	if err != nil {
		return 0, nil, err
	}
	var site models.SiteConfig
	for _, entry := range inventory.Entries {
		if entry.Backend == "cloud" {
			if err := db.WithContext(ctx).First(&site).Error; err != nil {
				return 0, nil, err
			}
			break
		}
	}
	var total int64
	for index := range inventory.Entries {
		entry := &inventory.Entries[index]
		destination, err := safeAttachmentPath(stage, entry.File, true)
		if err != nil {
			return total, nil, err
		}
		file, err := os.CreateTemp(filepath.Dir(destination), ".attachment-export-*")
		if err != nil {
			return total, nil, err
		}
		hash := sha256.New()
		n, fetchErr := FetchAttachmentArchiveContent(ctx, *entry, attachments.DefaultLocalRoot(), site, io.MultiWriter(file, hash))
		closeErr := file.Close()
		if fetchErr == nil {
			fetchErr = closeErr
		}
		if fetchErr == nil {
			fetchErr = os.Rename(file.Name(), destination)
		}
		_ = os.Remove(file.Name())
		if fetchErr != nil {
			return total, nil, fetchErr
		}
		entry.Size, entry.Hash = n, hex.EncodeToString(hash.Sum(nil))
		total += n
	}
	document := archiveDocument(inventory)
	data, err := json.Marshal(document)
	if err != nil {
		return total, nil, err
	}
	if err := ctx.Err(); err != nil {
		return total, nil, err
	}
	manifest, err := safeAttachmentPath(stage, AttachmentInventoryName, true)
	if err != nil {
		return total, nil, err
	}
	if err := os.WriteFile(manifest, data, 0600); err != nil {
		return total, nil, err
	}
	return int64(len(inventory.Entries)), nil, nil
}

func archiveDocument(inventory AttachmentInventory) attachmentArchiveDocument {
	doc := attachmentArchiveDocument{Version: 1, Scope: inventory.Scope, Entries: inventory.Entries, Grants: inventory.Grants}
	for _, blob := range inventory.Blobs {
		doc.Blobs = append(doc.Blobs, archiveBlob{ID: blob.ID, Backend: blob.StorageBackend, Key: blob.StorageKey, Hash: blob.ContentHash, Size: blob.Size, ContentType: blob.ContentType})
	}
	for _, ref := range inventory.References {
		doc.References = append(doc.References, archiveReference{ID: ref.ID, PublicID: ref.PublicID, BlobID: ref.BlobID, OwnerUserID: ref.OwnerUserID, Kind: ref.Kind, OriginalName: ref.OriginalName})
	}
	for _, old := range inventory.Legacy {
		doc.Legacy = append(doc.Legacy, archiveLegacy{ID: old.ID, PublicID: old.PublicID, Key: old.ObjectKey, Hash: old.ContentHash, OriginalName: old.OriginalName, ContentType: old.ContentType})
	}
	return doc
}

func readAttachmentDocument(ctx context.Context, stage string) (attachmentArchiveDocument, error) {
	var doc attachmentArchiveDocument
	manifest, err := safeAttachmentPath(stage, AttachmentInventoryName, false)
	if err != nil {
		return doc, err
	}
	info, err := os.Stat(manifest)
	if err != nil {
		return doc, err
	}
	if info.Size() > 32<<20 {
		return doc, errors.New("attachment inventory is too large")
	}
	data, err := os.ReadFile(manifest)
	if err != nil {
		return doc, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&doc); err != nil {
		return doc, err
	}
	var extra interface{}
	if decoder.Decode(&extra) != io.EOF {
		return doc, errors.New("trailing attachment inventory data")
	}
	if doc.Version != 1 || (doc.Scope != "local" && doc.Scope != "all") {
		return doc, errors.New("unsupported attachment inventory version or scope")
	}
	seen := map[string]AttachmentArchiveEntry{}
	for _, entry := range doc.Entries {
		if !validAttachmentHash(entry.Hash) || entry.Size < 0 || (entry.Backend != "local" && entry.Backend != "cloud") || (entry.BlobID == 0) == (entry.LegacyID == 0) || entry.File != attachmentEntryFile(entry) || entry.Key == "" {
			return doc, errors.New("invalid attachment inventory entry")
		}
		if prior, exists := seen[entry.File]; exists && (prior.Hash != entry.Hash || prior.Size != entry.Size) {
			return doc, errors.New("conflicting attachment content path")
		}
		seen[entry.File] = entry
		if doc.Scope == "local" && entry.Backend != "local" {
			return doc, errors.New("local attachment archive contains cloud data")
		}
		if err := verifyStagedAttachment(ctx, stage, entry); err != nil {
			return doc, err
		}
	}
	return doc, validateAttachmentRegistry(doc)
}

func verifyStagedAttachment(ctx context.Context, stage string, entry AttachmentArchiveEntry) error {
	_, err := FetchAttachmentArchiveContent(ctx, AttachmentArchiveEntry{Backend: "local", Key: entry.File, Hash: entry.Hash, Size: entry.Size}, stage, models.SiteConfig{}, io.Discard)
	return err
}

func validateAttachmentRegistry(doc attachmentArchiveDocument) error {
	blobs, legacy, publicIDs := map[uint]bool{}, map[uint]bool{}, map[string]bool{}
	validID := func(id string) bool {
		return id != "" && len(id) <= 64 && !strings.ContainsAny(id, "/\\?#%\x00 \t\r\n") && id != "." && id != ".."
	}
	validName := func(name string) bool {
		return name != "" && len(name) <= 255 && !strings.ContainsAny(name, "/\\\x00\r\n") && name != "." && name != ".."
	}
	physical := map[string]bool{}
	for _, blob := range doc.Blobs {
		if blob.ID == 0 || blobs[blob.ID] {
			return errors.New("invalid or duplicate registry blob")
		}
		if !validAttachmentHash(blob.Hash) || blob.Size < 0 || blob.Backend != "local" && blob.Backend != "cloud" || blob.Key == "" || physical[blob.Backend+":"+blob.Hash] {
			return errors.New("invalid or duplicate physical attachment blob")
		}
		physical[blob.Backend+":"+blob.Hash] = true
		blobs[blob.ID] = true
		matches := 0
		for _, entry := range doc.Entries {
			if entry.BlobID == blob.ID {
				if entry.Backend != blob.Backend || entry.Key != blob.Key || entry.Hash != blob.Hash || entry.Size != blob.Size {
					return errors.New("registry blob does not match content inventory")
				}
				matches++
			}
		}
		if matches != 1 {
			return errors.New("registry blob content is missing")
		}
	}
	for _, old := range doc.Legacy {
		if old.ID == 0 || legacy[old.ID] || !validID(old.PublicID) || !validName(old.OriginalName) || publicIDs[old.PublicID] {
			return errors.New("invalid legacy cloud attachment")
		}
		legacy[old.ID], publicIDs[old.PublicID] = true, true
		matches := 0
		for _, entry := range doc.Entries {
			if entry.LegacyID == old.ID {
				if entry.Backend != "cloud" || entry.Key != old.Key || old.Hash != "" && old.Hash != entry.Hash {
					return errors.New("legacy cloud content does not match inventory")
				}
				matches++
			}
		}
		if matches != 1 {
			return errors.New("legacy cloud content is missing")
		}
	}
	for _, ref := range doc.References {
		if !validID(ref.PublicID) || ref.OwnerUserID == 0 || publicIDs[ref.PublicID] || !blobs[ref.BlobID] || !validName(ref.OriginalName) || !(ref.Kind == "image" || ref.Kind == "video" || ref.Kind == "audio" || ref.Kind == "file") {
			return errors.New("invalid attachment reference")
		}
		publicIDs[ref.PublicID] = true
	}
	if len(blobs)+len(legacy) != len(doc.Entries) {
		return errors.New("attachment inventory contains unregistered content")
	}
	for _, grant := range doc.Grants {
		if grant.MessageID == 0 || grant.OwnerUserID == 0 || grant.Name == "" || !(grant.Kind == "cloud" || grant.Kind == "image" || grant.Kind == "video" || grant.Kind == "audio" || grant.Kind == "file") || !(grant.Visibility == "private" || grant.Visibility == "public" || grant.Visibility == "contacts" || grant.Visibility == "users") {
			return errors.New("invalid attachment visibility grant")
		}
	}
	return nil
}

func validateAttachmentMode(mode string) error {
	if mode != AttachmentRestoreOriginal && mode != AttachmentRestoreLocal {
		return errors.New("attachment restore mode must be explicitly original or local")
	}
	return nil
}

// PrepareAttachmentRestore checks complete archived content and current cloud availability.
// Original mode never writes. Local mode materializes only inside the isolated stage.
func PrepareAttachmentRestore(ctx context.Context, stage, mode string) error {
	if err := validateAttachmentMode(mode); err != nil {
		return err
	}
	doc, err := readAttachmentDocument(ctx, stage)
	if err != nil {
		return err
	}
	var site models.SiteConfig
	if mode == AttachmentRestoreOriginal {
		for _, entry := range doc.Entries {
			if entry.Backend != "cloud" {
				continue
			}
			db, err := database.GetDB()
			if err != nil {
				return &AttachmentFallbackRequiredError{Cause: err}
			}
			if err := db.WithContext(ctx).First(&site).Error; err != nil {
				return &AttachmentFallbackRequiredError{Cause: err}
			}
			break
		}
	}
	return prepareAttachmentRestoreWithDocument(ctx, stage, mode, site, doc)
}

// PrepareAttachmentRestoreWithSite permits callers with an explicit target configuration.
func PrepareAttachmentRestoreWithSite(ctx context.Context, stage, mode string, site models.SiteConfig) error {
	if err := validateAttachmentMode(mode); err != nil {
		return err
	}
	doc, err := readAttachmentDocument(ctx, stage)
	if err != nil {
		return err
	}
	return prepareAttachmentRestoreWithDocument(ctx, stage, mode, site, doc)
}

func prepareAttachmentRestoreWithDocument(ctx context.Context, stage, mode string, site models.SiteConfig, doc attachmentArchiveDocument) error {
	if mode == AttachmentRestoreOriginal {
		// Probe target bucket access, not the source keys: a new bucket may be
		// empty. Archived bytes are uploaded only at quiescent startup.
		for _, entry := range doc.Entries {
			if entry.Backend != "cloud" {
				continue
			}
			if !site.AttachmentStorageEnabled {
				return &AttachmentFallbackRequiredError{Cause: errors.New("附件云存储未启用，请明确确认是否全部恢复到本地")}
			}
			_, err := storage.DownloadAttachmentObject(ctx, site, restoredCloudKey(entry.Hash), io.Discard)
			if err != nil && !errors.Is(err, storage.ErrMissingContent) {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				return &AttachmentFallbackRequiredError{Cause: err}
			}
			break
		}
		return ctx.Err()
	}
	for _, entry := range doc.Entries {
		if entry.Backend != "cloud" {
			continue
		}
		destination, err := safeAttachmentPath(stage, "attachment-blobs/"+entry.Hash[:2]+"/"+entry.Hash, true)
		if err != nil {
			return err
		}
		if err := writeStagedAttachment(ctx, stage, entry, destination); err != nil {
			return err
		}
	}
	return nil
}

func restoredCloudKey(hash string) string { return "attachment-blobs/sha256/" + hash[:2] + "/" + hash }

func restoredEntryKey(entry AttachmentArchiveEntry, doc attachmentArchiveDocument) string {
	if entry.LegacyID != 0 {
		for _, old := range doc.Legacy {
			if old.ID == entry.LegacyID {
				return restoredCloudKey(entry.Hash) + "/legacy-" + old.PublicID
			}
		}
	}
	return restoredCloudKey(entry.Hash)
}

func writeStagedAttachment(ctx context.Context, stage string, entry AttachmentArchiveEntry, destination string) error {
	source, err := safeAttachmentPath(stage, entry.File, false)
	if err != nil {
		return err
	}
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.CreateTemp(filepath.Dir(destination), ".attachment-restore-*")
	if err != nil {
		return err
	}
	defer os.Remove(output.Name())
	_, err = storage.CopyContext(ctx, output, input)
	closeErr := output.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(output.Name(), destination)
}

// PrepareAttachmentDatabase snapshots the TARGET database, then merges attachment
// records only. Callers must quiesce writes and atomically swap this candidate with
// staged media. This helper never opens archived site configuration or permissions.
// It is called only at startup, before InitDB or any worker can write.
func PrepareAttachmentDatabase(ctx context.Context, stage, currentDBPath, mode string) (string, error) {
	if err := validateAttachmentMode(mode); err != nil {
		return "", err
	}
	doc, err := readAttachmentDocument(ctx, stage)
	if err != nil {
		return "", err
	}
	if info, err := os.Stat(currentDBPath); err != nil || !info.Mode().IsRegular() {
		return "", errors.New("current attachment restore database is unavailable")
	}
	db, err := backup.OpenOffline(currentDBPath, false)
	if err != nil {
		return "", err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return "", err
	}
	defer sqlDB.Close()
	if mode == AttachmentRestoreOriginal {
		var site models.SiteConfig
		for _, entry := range doc.Entries {
			if entry.Backend != "cloud" {
				continue
			}
			if site.ID == 0 {
				if err := db.WithContext(ctx).First(&site).Error; err != nil {
					return "", &AttachmentFallbackRequiredError{Cause: err}
				}
			}
			source, err := safeAttachmentPath(stage, entry.File, false)
			if err != nil {
				return "", err
			}
			content, err := os.Open(source)
			if err != nil {
				return "", err
			}
			err = storage.UploadAttachmentArchiveObject(ctx, site, restoredEntryKey(entry, doc), content, "application/octet-stream", entry.Hash, entry.Size)
			closeErr := content.Close()
			if err == nil {
				err = closeErr
			}
			if err != nil {
				return "", &AttachmentFallbackRequiredError{Cause: err}
			}
			verified := entry
			verified.Key = restoredEntryKey(entry, doc)
			if _, err := FetchAttachmentArchiveContent(ctx, verified, "", site, io.Discard); err != nil {
				return "", &AttachmentFallbackRequiredError{Cause: err}
			}
		}
	}
	if mode == AttachmentRestoreLocal {
		if err := prepareAttachmentRestoreWithDocument(ctx, stage, mode, models.SiteConfig{}, doc); err != nil {
			return "", err
		}
	}
	candidate, err := safeAttachmentPath(stage, "attachment-candidate.db", true)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(candidate); err == nil {
		return "", errors.New("attachment database candidate already exists")
	}
	if err := db.WithContext(ctx).Exec("VACUUM INTO ?", candidate).Error; err != nil {
		return "", err
	}
	ready := false
	defer func() {
		if !ready {
			_ = os.Remove(candidate)
		}
	}()
	candidateDB, err := gorm.Open(sqlite.Open(candidate), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return "", err
	}
	candidateSQL, err := candidateDB.DB()
	if err != nil {
		return "", err
	}
	defer candidateSQL.Close()
	if err := candidateDB.Exec("PRAGMA journal_mode=DELETE").Error; err != nil {
		return "", err
	}
	if err := candidateDB.WithContext(ctx).AutoMigrate(&models.AttachmentBlob{}, &models.AttachmentReference{}, &models.CloudAttachmentObject{}, &models.LocalAttachmentGrant{}); err != nil {
		return "", err
	}
	if err := candidateDB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := mergeAttachmentDocument(tx, doc, mode); err != nil {
			return err
		}
		return rememberAttachmentRestore(tx, doc, mode)
	}); err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	var integrity string
	if err := candidateDB.Raw("PRAGMA quick_check").Scan(&integrity).Error; err != nil {
		return "", err
	}
	if integrity != "ok" {
		return "", fmt.Errorf("attachment candidate integrity check: %s", integrity)
	}
	if err := candidateSQL.Close(); err != nil {
		return "", err
	}
	ready = true
	return candidate, nil
}

func mergeAttachmentDocument(tx *gorm.DB, doc attachmentArchiveDocument, mode string) error {
	blobIDs := map[uint]uint{}
	for _, blob := range doc.Blobs {
		backend, key := blob.Backend, blob.Key
		if backend == "cloud" {
			key = restoredCloudKey(blob.Hash)
		} else {
			key = blob.Hash[:2] + "/" + blob.Hash
		}
		if mode == AttachmentRestoreLocal {
			backend, key = "local", blob.Hash[:2]+"/"+blob.Hash
		}
		stored, err := mergeArchiveBlob(tx, backend, key, blob.Hash, blob.ContentType, blob.Size)
		if err != nil {
			return err
		}
		blobIDs[blob.ID] = stored.ID
	}
	for _, ref := range doc.References {
		var existing models.AttachmentReference
		err := tx.Where("public_id = ?", ref.PublicID).First(&existing).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err == nil && (existing.OwnerUserID != ref.OwnerUserID || existing.Kind != ref.Kind || existing.OriginalName != ref.OriginalName) {
			return fmt.Errorf("attachment public ID %q conflicts with target reference", ref.PublicID)
		}
		original := models.AttachmentReference{PublicID: ref.PublicID, BlobID: blobIDs[ref.BlobID], OwnerUserID: ref.OwnerUserID, Kind: ref.Kind, OriginalName: ref.OriginalName}
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "public_id"}}, DoUpdates: clause.AssignmentColumns([]string{"blob_id"})}).Create(&original).Error; err != nil {
			return err
		}
		if err := tx.Where("public_id = ?", ref.PublicID).Delete(&models.CloudAttachmentObject{}).Error; err != nil {
			return err
		}
		if mode == AttachmentRestoreLocal {
			if err := rewriteAttachmentURLs(tx, ref.PublicID, ref.Kind); err != nil {
				return err
			}
		}
	}
	for _, old := range doc.Legacy {
		var content AttachmentArchiveEntry
		for _, entry := range doc.Entries {
			if entry.LegacyID == old.ID {
				content = entry
				break
			}
		}
		if mode == AttachmentRestoreOriginal {
			var existing models.CloudAttachmentObject
			err := tx.Where("public_id = ?", old.PublicID).First(&existing).Error
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			if err == nil && (existing.OriginalName != old.OriginalName || existing.ContentHash != "" && existing.ContentHash != content.Hash) {
				return errors.New("legacy public ID conflicts with target")
			}
			object := models.CloudAttachmentObject{PublicID: old.PublicID, ObjectKey: restoredEntryKey(content, doc), OriginalName: old.OriginalName, ContentType: old.ContentType, ContentHash: content.Hash}
			if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "public_id"}}, DoUpdates: clause.AssignmentColumns([]string{"object_key", "content_hash", "legacy_object_key", "legacy_cleanup_pending"})}).Create(&object).Error; err != nil {
				return err
			}
			continue
		}
		var existing models.AttachmentReference
		err := tx.Where("public_id = ?", old.PublicID).First(&existing).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		owner := existing.OwnerUserID
		if errors.Is(err, gorm.ErrRecordNotFound) {
			owner, err = legacyArchiveOwner(tx, old.PublicID)
			if err != nil {
				return err
			}
		} else if existing.OriginalName != old.OriginalName {
			return errors.New("legacy public ID conflicts with target reference")
		}
		blob, err := mergeArchiveBlob(tx, "local", content.Hash[:2]+"/"+content.Hash, content.Hash, old.ContentType, content.Size)
		if err != nil {
			return err
		}
		kind := existing.Kind
		if kind == "" {
			kind = "file"
		}
		ref := models.AttachmentReference{PublicID: old.PublicID, BlobID: blob.ID, OwnerUserID: owner, Kind: kind, OriginalName: old.OriginalName}
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "public_id"}}, DoUpdates: clause.AssignmentColumns([]string{"blob_id"})}).Create(&ref).Error; err != nil {
			return err
		}
		if err := rewriteAttachmentURLs(tx, old.PublicID, kind); err != nil {
			return err
		}
		if err := tx.Where("public_id = ?", old.PublicID).Delete(&models.CloudAttachmentObject{}).Error; err != nil {
			return err
		}
	}
	for _, grant := range doc.Grants {
		// Grants for old messages are useful only when that same message and
		// owner exists on the target. Never grant visibility to an unrelated ID.
		var message models.Message
		if !tx.Migrator().HasTable(&models.Message{}) {
			continue
		}
		err := tx.First(&message, grant.MessageID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			continue
		}
		if err != nil {
			return err
		}
		if message.UserID != grant.OwnerUserID {
			return errors.New("attachment grant owner conflicts with target message")
		}
		grant.ID = 0
		// Keep historical restrictions even after the physical storage changes.
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&grant).Error; err != nil {
			return err
		}
	}
	if mode == AttachmentRestoreLocal {
		for _, blob := range doc.Blobs {
			if blob.Backend != "cloud" {
				continue
			}
			// Match physical identity, never archive IDs which may belong to another target blob.
			if err := tx.Where("storage_backend = ? AND content_hash = ? AND NOT EXISTS (SELECT 1 FROM attachment_references WHERE attachment_references.blob_id = attachment_blobs.id)", "cloud", blob.Hash).Delete(&models.AttachmentBlob{}).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func mergeArchiveBlob(tx *gorm.DB, backend, key, hash, contentType string, size int64) (models.AttachmentBlob, error) {
	var existing models.AttachmentBlob
	err := tx.Where("storage_backend = ? AND content_hash = ?", backend, hash).First(&existing).Error
	if err == nil {
		if existing.Size != size {
			return existing, errors.New("target attachment blob metadata conflicts with archive")
		}
		if existing.StorageKey != key {
			if err := tx.Model(&existing).Update("storage_key", key).Error; err != nil {
				return existing, err
			}
			existing.StorageKey = key
		}
		return existing, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return existing, err
	}
	existing = models.AttachmentBlob{StorageBackend: backend, StorageKey: key, ContentHash: hash, Size: size, ContentType: contentType}
	if err := tx.Create(&existing).Error; err != nil {
		return existing, err
	}
	return existing, nil
}

func rewriteAttachmentURLs(tx *gorm.DB, publicID, kind string) error {
	ref := models.AttachmentReference{PublicID: publicID, Kind: kind, OriginalName: "placeholder"}
	newPrefix := strings.TrimSuffix(attachments.ReferenceURL(ref, "local"), url.PathEscape(ref.OriginalName))
	for table, columns := range map[string][]string{
		"messages": {"content", "image_url"}, "comments": {"content"},
		"user_notifications": {"content", "deletion_snapshot_json"}, "announcements": {"content"},
		"users":        {"avatar_url"},
		"site_configs": {"avatar_url", "backgrounds", "rss_favicon_url", "pwa_icon_url", "welcome_avatar_url"},
	} {
		if !tx.Migrator().HasTable(table) {
			continue
		}
		for _, column := range columns {
			if !tx.Migrator().HasColumn(table, column) {
				continue
			}
			var rows []struct {
				ID    uint
				Value string
			}
			if err := tx.Table(table).Select("id, " + column + " AS value").Where(column + " IS NOT NULL").Scan(&rows).Error; err != nil {
				return err
			}
			for _, row := range rows {
				value := rewriteManagedAttachmentURLText(row.Value, publicID, newPrefix)
				if value == row.Value {
					continue
				}
				if err := tx.Table(table).Where("id = ?", row.ID).UpdateColumn(column, value).Error; err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// Match whole managed URLs, including the origin. The left boundary prevents
// rewriting a managed-looking path embedded in an unrelated URL or query value.
func rewriteManagedAttachmentURLText(value, publicID, newPrefix string) string {
	for _, sourcePrefix := range []string{"/api/cloud-attachments/" + publicID + "/", newPrefix} {
		for _, escaped := range []bool{false, true} {
			slash, prefix, replacement := "/", sourcePrefix, newPrefix
			if escaped {
				slash = `\/`
				prefix = strings.ReplaceAll(prefix, "/", slash)
				replacement = strings.ReplaceAll(replacement, "/", slash)
			}
			s := regexp.QuoteMeta(slash)
			origin := `(?:(?:https?:)?` + s + s + `[^/\\\s<>"'()?#]+)?`
			pattern := regexp.MustCompile(`(^|[\s<>"'\x60(\[])` + origin + regexp.QuoteMeta(prefix))
			value = pattern.ReplaceAllStringFunc(value, func(match string) string {
				boundary := ""
				if match != "" && strings.ContainsAny(match[:1], " \t\r\n<>\"'`([") {
					boundary = match[:1]
				}
				return boundary + replacement
			})
		}
	}
	return value
}

func legacyArchiveOwner(tx *gorm.DB, publicID string) (uint, error) {
	contains := func(value string) bool {
		return rewriteManagedAttachmentURLText(value, publicID, "/__attachment_owner__/") != value
	}
	owners := map[uint]bool{}
	if tx.Migrator().HasTable(&models.Message{}) {
		var messages []models.Message
		if err := tx.Find(&messages).Error; err != nil {
			return 0, err
		}
		for _, message := range messages {
			if contains(message.Content) || contains(message.ImageURL) {
				owners[message.UserID] = true
			}
		}
	}
	if tx.Migrator().HasTable(&models.Comment{}) {
		var comments []models.Comment
		if err := tx.Find(&comments).Error; err != nil {
			return 0, err
		}
		for _, comment := range comments {
			if !contains(comment.Content) {
				continue
			}
			if comment.UserID == nil || *comment.UserID == 0 {
				return 0, errors.New("legacy cloud attachment has ambiguous guest ownership")
			}
			owners[*comment.UserID] = true
		}
	}
	if len(owners) != 1 || owners[0] {
		return 0, errors.New("legacy cloud attachment requires one unambiguous owner for local migration")
	}
	for owner := range owners {
		return owner, nil
	}
	return 0, errors.New("legacy cloud attachment owner is missing")
}
