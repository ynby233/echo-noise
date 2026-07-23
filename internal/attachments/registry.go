package attachments

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/rcy1314/echo-noise/internal/models"
	"gorm.io/gorm"
)

var (
	registryMu       sync.Mutex
	sha256Pattern    = regexp.MustCompile(`^[a-f0-9]{64}$`)
	validKinds       = map[string]struct{}{"image": {}, "video": {}, "audio": {}, "file": {}}
	errInvalidCreate = errors.New("invalid attachment reference input")
)

type CreateInput struct {
	Kind         string
	OwnerUserID  uint
	OriginalName string
	ContentType  string
	ContentHash  string
	Size         int64
}

// BlobStore is the storage seam behind the registry. Implementations keep
// physical object details private while the registry owns deduplication and
// logical-reference identity.
type BlobStore interface {
	ID() string
	Key(contentHash string) string
	Exists(ctx context.Context, key string) (bool, error)
	Put(ctx context.Context, key string, content io.ReadSeeker, contentType, contentHash string, size int64) error
	Delete(ctx context.Context, key string) error
}

type Registry struct {
	db *gorm.DB
}

type ResolvedReference struct {
	Reference models.AttachmentReference
	Blob      models.AttachmentBlob
}

func NewRegistry(db *gorm.DB) *Registry {
	return &Registry{db: db}
}

func ReferenceURL(reference models.AttachmentReference, backend string) string {
	prefix := "/api/files/refs/"
	if backend == "cloud" {
		prefix = "/api/cloud-attachments/"
	} else {
		switch reference.Kind {
		case "image":
			prefix = "/api/images/refs/"
		case "video":
			prefix = "/api/video/refs/"
		case "audio":
			prefix = "/api/audio/refs/"
		}
	}
	return prefix + reference.PublicID + "/" + url.PathEscape(reference.OriginalName)
}

func (r *Registry) Resolve(publicID string) (ResolvedReference, error) {
	if r == nil || r.db == nil || strings.TrimSpace(publicID) == "" {
		return ResolvedReference{}, errInvalidCreate
	}
	var reference models.AttachmentReference
	if err := r.db.Where("public_id = ?", strings.TrimSpace(publicID)).First(&reference).Error; err != nil {
		return ResolvedReference{}, err
	}
	var blob models.AttachmentBlob
	if err := r.db.First(&blob, reference.BlobID).Error; err != nil {
		return ResolvedReference{}, err
	}
	return ResolvedReference{Reference: reference, Blob: blob}, nil
}

func (r *Registry) List(kind, backend string) ([]ResolvedReference, error) {
	if r == nil || r.db == nil {
		return nil, errInvalidCreate
	}
	var references []models.AttachmentReference
	if err := r.db.Where("kind = ?", strings.ToLower(strings.TrimSpace(kind))).Order("created_at DESC, id DESC").Find(&references).Error; err != nil {
		return nil, err
	}
	if len(references) == 0 {
		return []ResolvedReference{}, nil
	}
	blobIDs := make([]uint, 0, len(references))
	for _, reference := range references {
		blobIDs = append(blobIDs, reference.BlobID)
	}
	var blobs []models.AttachmentBlob
	query := r.db.Where("id IN ?", blobIDs)
	if strings.TrimSpace(backend) != "" {
		query = query.Where("storage_backend = ?", strings.TrimSpace(backend))
	}
	if err := query.Find(&blobs).Error; err != nil {
		return nil, err
	}
	blobByID := make(map[uint]models.AttachmentBlob, len(blobs))
	for _, blob := range blobs {
		blobByID[blob.ID] = blob
	}
	resolved := make([]ResolvedReference, 0, len(references))
	for _, reference := range references {
		if blob, ok := blobByID[reference.BlobID]; ok {
			resolved = append(resolved, ResolvedReference{Reference: reference, Blob: blob})
		}
	}
	return resolved, nil
}

func (r *Registry) Create(ctx context.Context, store BlobStore, input CreateInput, content io.ReadSeeker) (models.AttachmentReference, error) {
	if r == nil || r.db == nil || store == nil || content == nil || input.OwnerUserID == 0 || input.Size < 0 {
		return models.AttachmentReference{}, errInvalidCreate
	}
	input.Kind = strings.ToLower(strings.TrimSpace(input.Kind))
	if _, ok := validKinds[input.Kind]; !ok {
		return models.AttachmentReference{}, errInvalidCreate
	}
	input.OriginalName = filepath.Base(strings.TrimSpace(input.OriginalName))
	if input.OriginalName == "" || input.OriginalName == "." || strings.ContainsAny(input.OriginalName, `/\`) {
		return models.AttachmentReference{}, errInvalidCreate
	}
	input.ContentHash = strings.ToLower(strings.TrimSpace(input.ContentHash))
	if !sha256Pattern.MatchString(input.ContentHash) {
		return models.AttachmentReference{}, errInvalidCreate
	}

	registryMu.Lock()
	defer registryMu.Unlock()

	var blob models.AttachmentBlob
	err := r.db.Where("storage_backend = ? AND content_hash = ?", store.ID(), input.ContentHash).First(&blob).Error
	newBlob := false
	if errors.Is(err, gorm.ErrRecordNotFound) {
		blob = models.AttachmentBlob{
			StorageBackend: store.ID(),
			StorageKey:     store.Key(input.ContentHash),
			ContentHash:    input.ContentHash,
			Size:           input.Size,
			ContentType:    strings.TrimSpace(input.ContentType),
		}
		newBlob = true
	} else if err != nil {
		return models.AttachmentReference{}, err
	} else if blob.Size != input.Size {
		return models.AttachmentReference{}, fmt.Errorf("attachment hash collision or corrupt metadata")
	}

	exists := false
	if !newBlob {
		exists, err = store.Exists(ctx, blob.StorageKey)
		if err != nil {
			return models.AttachmentReference{}, err
		}
	}
	if !exists {
		if _, err := content.Seek(0, io.SeekStart); err != nil {
			return models.AttachmentReference{}, err
		}
		if err := store.Put(ctx, blob.StorageKey, content, blob.ContentType, blob.ContentHash, blob.Size); err != nil {
			return models.AttachmentReference{}, err
		}
	}
	if newBlob {
		if err := r.db.Create(&blob).Error; err != nil {
			// Another process may have won the unique (backend, hash) race.
			// Resolve that winner instead of deleting the deterministic object
			// key, which could break the winner's reference.
			var winner models.AttachmentBlob
			if lookupErr := r.db.Where("storage_backend = ? AND content_hash = ?", store.ID(), input.ContentHash).First(&winner).Error; lookupErr == nil && winner.Size == input.Size {
				blob = winner
			} else {
				return models.AttachmentReference{}, err
			}
		}
	}

	reference := models.AttachmentReference{
		PublicID:     uuid.NewString(),
		BlobID:       blob.ID,
		OwnerUserID:  input.OwnerUserID,
		Kind:         input.Kind,
		OriginalName: input.OriginalName,
	}
	if err := r.db.Create(&reference).Error; err != nil {
		return models.AttachmentReference{}, err
	}
	return reference, nil
}

// DeleteReference removes one logical attachment. The physical blob is only
// removed after the last reference is gone. Registry operations share a lock
// so an upload cannot reuse a blob while its final reference is being removed.
func (r *Registry) DeleteReference(ctx context.Context, store BlobStore, publicID string) error {
	if r == nil || r.db == nil || store == nil || strings.TrimSpace(publicID) == "" {
		return errInvalidCreate
	}

	registryMu.Lock()
	defer registryMu.Unlock()

	var reference models.AttachmentReference
	if err := r.db.Where("public_id = ?", strings.TrimSpace(publicID)).First(&reference).Error; err != nil {
		return err
	}
	var blob models.AttachmentBlob
	if err := r.db.First(&blob, reference.BlobID).Error; err != nil {
		return err
	}
	if blob.StorageBackend != store.ID() {
		return errors.New("attachment storage backend mismatch")
	}

	remaining := int64(0)
	if err := r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&reference).Error; err != nil {
			return err
		}
		return tx.Model(&models.AttachmentReference{}).Where("blob_id = ?", blob.ID).Count(&remaining).Error
	}); err != nil {
		return err
	}
	if remaining > 0 {
		return nil
	}
	if err := store.Delete(ctx, blob.StorageKey); err != nil {
		// The now-unreferenced blob remains registered and can be reused or
		// cleaned up safely later; logical deletion still succeeded.
		return nil
	}
	// A failed metadata cleanup is also safe: the unreferenced row will be
	// repaired or reused on the next upload of the same content.
	_ = r.db.Delete(&blob).Error
	return nil
}

type LocalStore struct {
	root string
}

func NewLocalStore(root string) *LocalStore {
	return &LocalStore{root: filepath.Clean(root)}
}

func DefaultLocalRoot() string {
	if configured := strings.TrimSpace(os.Getenv("ATTACHMENT_BLOB_ROOT")); configured != "" {
		return filepath.Clean(configured)
	}
	if _, err := os.Stat("/data"); err == nil {
		return filepath.Join("/data", "attachment-blobs")
	}
	if _, err := os.Stat("/app/data"); err == nil {
		return filepath.Join("/app/data", "attachment-blobs")
	}
	return filepath.Join(".", "data", "attachment-blobs")
}

func (s *LocalStore) Root() string { return s.root }
func (s *LocalStore) ID() string   { return "local" }

func (s *LocalStore) Key(contentHash string) string {
	return filepath.ToSlash(filepath.Join(contentHash[:2], contentHash))
}

func (s *LocalStore) path(key string) (string, error) {
	rootAbs, err := filepath.Abs(s.root)
	if err != nil {
		return "", err
	}
	pathAbs, err := filepath.Abs(filepath.Join(rootAbs, filepath.FromSlash(key)))
	if err != nil || (pathAbs != rootAbs && !strings.HasPrefix(pathAbs, rootAbs+string(filepath.Separator))) {
		return "", errInvalidCreate
	}
	return pathAbs, nil
}

func (s *LocalStore) Exists(_ context.Context, key string) (bool, error) {
	path, err := s.path(key)
	if err != nil {
		return false, err
	}
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return err == nil && !info.IsDir(), err
}

func (s *LocalStore) Put(_ context.Context, key string, content io.ReadSeeker, _, _ string, _ int64) error {
	path, err := s.path(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".attachment-blob-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := io.Copy(tmp, content); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func (s *LocalStore) Delete(_ context.Context, key string) error {
	path, err := s.path(key)
	if err != nil {
		return err
	}
	err = os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func (s *LocalStore) Open(key string) (*os.File, os.FileInfo, error) {
	path, err := s.path(key)
	if err != nil {
		return nil, nil, err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	info, err := file.Stat()
	if err != nil || info.IsDir() {
		file.Close()
		if err == nil {
			err = os.ErrNotExist
		}
		return nil, nil, err
	}
	return file, info, nil
}
