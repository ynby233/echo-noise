package music

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	mediaAudioBudget  int64 = 1 << 30
	mediaAssetBudget  int64 = 64 << 20
	mediaAudioLimit   int64 = 256 << 20
	mediaCoverLimit   int64 = 1 << 20
	mediaRequestWait        = 25 * time.Second
	mediaBuildTimeout       = 120 * time.Second
	mediaCacheAge           = 7 * 24 * time.Hour
	mediaCacheMarker        = ".echo-noise-music-cache-v1"
)

type cacheEntry struct {
	path  string
	size  int64
	asset bool
	refs  int
	last  time.Time
}

type cacheFlight struct {
	done     chan struct{}
	err      error
	asset    bool
	reserved int64
	waiters  int
	entry    *cacheEntry
}

// All entry references and reservations are changed under mu. A complete file
// is published only after generation, validation, and a successful rename.
type mediaCache struct {
	mu           sync.Mutex
	opts         Options
	dir          string
	lockFile     *os.File
	shutdownDone chan struct{}
	jobsDone     bool
	entries      map[string]*cacheEntry
	flights      map[string]*cacheFlight
	ctx          context.Context
	cancel       context.CancelFunc
	jobs         sync.WaitGroup
	slots        chan struct{}
	closed       bool
	audioBudget  int64
	assetBudget  int64
	audioLimit   int64
	assetLimit   int64
	requestWait  time.Duration
	buildTimeout time.Duration
}

func newMediaCache(opts Options) *mediaCache {
	ctx, cancel := context.WithCancel(context.Background())
	return &mediaCache{opts: opts, entries: make(map[string]*cacheEntry), flights: make(map[string]*cacheFlight), ctx: ctx, cancel: cancel, slots: make(chan struct{}, 2), shutdownDone: make(chan struct{}), audioBudget: mediaAudioBudget, assetBudget: mediaAssetBudget, audioLimit: mediaAudioLimit, assetLimit: mediaCoverLimit, requestWait: mediaRequestWait, buildTimeout: mediaBuildTimeout}
}

func (c *mediaCache) now() time.Time {
	if c.opts.Now != nil {
		return c.opts.Now()
	}
	return time.Now()
}

// start is called once by Service.Start before accepting lifecycle work.
func (c *mediaCache) start(ctx context.Context) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || len(c.flights) != 0 {
		return
	}
	c.cancel()
	c.ctx, c.cancel = context.WithCancel(ctx)
}

// wait seals the cache and retains ownership until builders and response
// descriptors have all finished. A timed-out caller does not release the lock.
func (c *mediaCache) wait(ctx context.Context) error {
	c.mu.Lock()
	if !c.closed {
		c.closed = true
		c.cancel()
		go func() {
			c.jobs.Wait()
			c.mu.Lock()
			defer c.mu.Unlock()
			c.jobsDone = true
			c.finishLocked()
		}()
	}
	done := c.shutdownDone
	c.mu.Unlock()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *mediaCache) close() { _ = c.wait(context.Background()) }

// cacheCanonical resolves existing ancestors too, so a non-existent cache
// below an attachment/backup symlink is still rejected before MkdirAll.
func cacheCanonical(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	var suffix []string
	base := abs
	for {
		_, err = os.Lstat(base)
		if err == nil {
			break
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(base)
		if parent == base {
			return "", err
		}
		suffix = append(suffix, filepath.Base(base))
		base = parent
	}
	resolved, err := filepath.EvalSymlinks(base)
	if err != nil {
		return "", err
	}
	for i := len(suffix) - 1; i >= 0; i-- {
		resolved = filepath.Join(resolved, suffix[i])
	}
	return filepath.Clean(resolved), nil
}

func cacheContains(root, child string) bool {
	rel, err := filepath.Rel(root, child)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

func (c *mediaCache) isolated(path string) bool {
	candidate, err := cacheCanonical(path)
	if err != nil {
		return false
	}
	roots := append([]string(nil), c.opts.ExcludedRoots...)
	if c.opts.RootDir != "" {
		roots = append(roots, c.opts.RootDir)
	}
	for _, root := range roots {
		if root == "" {
			continue
		}
		canonical, err := cacheCanonical(root)
		if err != nil || cacheContains(canonical, candidate) || cacheContains(candidate, canonical) {
			return false
		}
	}
	return true
}

// Platforms without a native helper refuse cache ownership.
var lockCacheFile = func(*os.File) error { return ErrUnavailable }
var unlockCacheFile = func(*os.File) error { return nil }

func (c *mediaCache) ensureDirLocked() error {
	if c.dir != "" {
		if c.lockFile == nil || !c.isolated(c.dir) {
			return ErrUnavailable
		}
		return nil
	}
	base := c.opts.CacheDir
	if base == "" {
		base = filepath.Join("data", "music-cache")
	}
	if !c.isolated(base) {
		base = filepath.Join(os.TempDir(), "echo-noise-music-cache")
		if !c.isolated(base) {
			return ErrUnavailable
		}
	}
	if err := os.MkdirAll(base, 0700); err != nil {
		return ErrUnavailable
	}
	baseInfo, err := os.Lstat(base)
	if err != nil || !baseInfo.IsDir() || !c.isolated(base) {
		return ErrUnavailable
	}
	lockPath := filepath.Join(base, ".music-cache.lock")
	if !c.isolated(lockPath) {
		return ErrUnavailable
	}
	before, err := os.Lstat(lockPath)
	if err != nil && !os.IsNotExist(err) || err == nil && !before.Mode().IsRegular() {
		return ErrUnavailable
	}
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return ErrUnavailable
	}
	owned := false
	defer func() {
		if !owned {
			_ = unlockCacheFile(file)
			_ = file.Close()
		}
	}()
	info, err := file.Stat()
	current, pathErr := os.Lstat(lockPath)
	if err != nil || pathErr != nil || !info.Mode().IsRegular() || !current.Mode().IsRegular() || !os.SameFile(info, current) || before != nil && !os.SameFile(before, info) {
		return ErrUnavailable
	}
	if err := lockCacheFile(file); err != nil {
		return ErrUnavailable
	}
	dir := filepath.Join(base, "runtime")
	if !c.isolated(dir) {
		return ErrUnavailable
	}
	if err := os.Mkdir(dir, 0700); err != nil && !os.IsExist(err) {
		return ErrUnavailable
	}
	if err := c.reclaimRuntimeLocked(dir); err != nil {
		return ErrUnavailable
	}
	c.dir, c.lockFile = dir, file
	owned = true
	return nil
}

// Validate the entire directory before removing anything. Unknown regular files
// are preserved; symlinks, subdirectories and special files fail closed.
func (c *mediaCache) reclaimRuntimeLocked(dir string) error {
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || !c.isolated(dir) {
		return ErrUnavailable
	}
	items, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	marked := false
	var derived []string
	for _, item := range items {
		path := filepath.Join(dir, item.Name())
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || !c.isolated(path) {
			return ErrUnavailable
		}
		if item.Name() == mediaCacheMarker {
			marked = true
		} else if cacheDerivedName(item.Name()) {
			derived = append(derived, path)
		}
	}
	if !marked {
		if len(items) != 0 {
			return ErrUnavailable
		}
		marker, err := os.OpenFile(filepath.Join(dir, mediaCacheMarker), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		return marker.Close()
	}
	for _, path := range derived {
		if err := os.Remove(path); err != nil {
			return err
		}
	}
	return nil
}

func cacheDerivedName(name string) bool {
	if strings.HasPrefix(name, ".building-") {
		suffix := strings.TrimPrefix(name, ".building-")
		if suffix == "" {
			return false
		}
		for _, ch := range suffix {
			if ch < '0' || ch > '9' {
				return false
			}
		}
		return true
	}
	if len(name) != 64+len(".media") || !strings.HasSuffix(name, ".media") {
		return false
	}
	_, err := hex.DecodeString(strings.TrimSuffix(name, ".media"))
	return err == nil
}

func (c *mediaCache) finishLocked() {
	if !c.closed || !c.jobsDone || c.shutdownDone == nil {
		return
	}
	for _, entry := range c.entries {
		if entry.refs != 0 {
			return
		}
	}
	if c.lockFile != nil {
		// Leave the marked directory for the next owner if cleanup fails.
		_ = c.reclaimRuntimeLocked(c.dir)
		_ = unlockCacheFile(c.lockFile)
		_ = c.lockFile.Close()
		c.lockFile = nil
	}
	select {
	case <-c.shutdownDone:
	default:
		close(c.shutdownDone)
	}
}

func (c *mediaCache) removeLocked(key string, entry *cacheEntry) bool {
	if entry.refs != 0 {
		return false
	}
	if err := os.Remove(entry.path); err != nil && !os.IsNotExist(err) {
		return false
	}
	delete(c.entries, key)
	return true
}

func (c *mediaCache) reserveLocked(asset bool, size int64) bool {
	now := c.now()
	for key, entry := range c.entries {
		if now.Sub(entry.last) >= mediaCacheAge {
			c.removeLocked(key, entry)
		}
	}
	budget := c.audioBudget
	if asset {
		budget = c.assetBudget
	}
	for {
		used := int64(0)
		var oldest *cacheEntry
		var oldestKey string
		for key, entry := range c.entries {
			if entry.asset != asset {
				continue
			}
			used += entry.size
			if entry.refs == 0 && (oldest == nil || entry.last.Before(oldest.last)) {
				oldest, oldestKey = entry, key
			}
		}
		for _, flight := range c.flights {
			if flight.asset == asset {
				used += flight.reserved
			}
		}
		if size <= budget && used <= budget-size {
			return true
		}
		if oldest == nil || !c.removeLocked(oldestKey, oldest) {
			return false
		}
	}
}

func (c *mediaCache) openLocked(key, mime, name string) (*MediaFile, error) {
	entry := c.entries[key]
	if entry == nil {
		return nil, ErrUnavailable
	}
	if !c.isolated(c.dir) {
		return nil, ErrUnavailable
	}
	before, err := os.Lstat(entry.path)
	if err != nil || !before.Mode().IsRegular() {
		return nil, ErrUnavailable
	}
	file, err := os.Open(entry.path)
	if err != nil {
		return nil, ErrUnavailable
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || !os.SameFile(before, info) || info.Size() != entry.size {
		_ = file.Close()
		return nil, ErrUnavailable
	}
	entry.refs++
	entry.last = c.now()
	hash := sha256.Sum256([]byte(key))
	return &MediaFile{File: file, Info: info, MIME: mime, ETag: "\"" + hex.EncodeToString(hash[:]) + "\"", Name: name, release: func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		entry.refs--
		entry.last = c.now()
		c.finishLocked()
	}}, nil
}

func (c *mediaCache) acquire(ctx context.Context, key string, asset bool, mime, name string, build func(context.Context, *os.File) error) (*MediaFile, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	c.mu.Lock()
	if c.closed || c.ctx.Err() != nil {
		c.mu.Unlock()
		return nil, ErrUnavailable
	}
	if entry := c.entries[key]; entry != nil {
		if entry.refs == 0 && c.now().Sub(entry.last) >= mediaCacheAge {
			c.removeLocked(key, entry)
		}
		if c.entries[key] != nil {
			result, err := c.openLocked(key, mime, name)
			c.mu.Unlock()
			return result, err
		}
	}
	flight := c.flights[key]
	if flight == nil {
		if err := c.ensureDirLocked(); err != nil {
			c.mu.Unlock()
			return nil, err
		}
		limit := c.audioLimit
		if asset {
			limit = c.assetLimit
		}
		if !c.reserveLocked(asset, limit) {
			c.mu.Unlock()
			return nil, ErrUnavailable
		}
		flight = &cacheFlight{done: make(chan struct{}), asset: asset, reserved: limit}
		c.flights[key] = flight
		c.jobs.Add(1)
		go c.generate(key, flight, build)
	}
	flight.waiters++
	c.mu.Unlock()
	timer := time.NewTimer(c.requestWait)
	defer timer.Stop()
	var waitErr error
	select {
	case <-ctx.Done():
		waitErr = ctx.Err()
	case <-timer.C:
		waitErr = ErrBusy
	case <-flight.done:
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	defer c.finishLocked()
	flight.waiters--
	// Publication pins one reference per waiting request until each waiter
	// opens its own descriptor or abandons the wait.
	if entry := flight.entry; entry != nil {
		entry.refs--
	}
	if waitErr != nil {
		return nil, waitErr
	}
	if flight.err != nil {
		return nil, flight.err
	}
	return c.openLocked(key, mime, name)
}

func (c *mediaCache) generate(key string, flight *cacheFlight, build func(context.Context, *os.File) error) {
	defer c.jobs.Done()
	ctx, cancel := context.WithTimeout(c.ctx, c.buildTimeout)
	defer cancel()
	var err error
	select {
	case c.slots <- struct{}{}:
		defer func() { <-c.slots }()
	case <-ctx.Done():
		err = ErrUnavailable
	}
	var temp *os.File
	var tempPath, finalPath string
	var size int64
	if err == nil {
		temp, err = os.CreateTemp(c.dir, ".building-")
		if err == nil {
			tempPath = temp.Name()
			err = build(ctx, temp)
			if err == nil {
				err = ctx.Err()
			}
			if err == nil {
				var info os.FileInfo
				info, err = temp.Stat()
				if err == nil {
					size = info.Size()
					if size <= 0 || size > flight.reserved {
						err = ErrUnavailable
					}
				}
			}
			if err == nil {
				err = temp.Sync()
			}
			closeErr := temp.Close()
			if err == nil {
				err = closeErr
			}
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err == nil && (c.closed || c.ctx.Err() != nil || !c.isolated(c.dir)) {
		err = ErrUnavailable
	}
	if err == nil {
		hash := sha256.Sum256([]byte(key))
		finalPath = filepath.Join(c.dir, hex.EncodeToString(hash[:])+".media")
		err = os.Rename(tempPath, finalPath)
	}
	if err != nil {
		if tempPath != "" {
			_ = os.Remove(tempPath)
		}
		flight.err = ErrUnavailable
	} else {
		entry := &cacheEntry{path: finalPath, size: size, asset: flight.asset, refs: flight.waiters, last: c.now()}
		flight.entry = entry
		c.entries[key] = entry
	}
	delete(c.flights, key)
	close(flight.done)
}

// boundedMediaWriter stops a misbehaving encoder before it fills the disk.
type boundedMediaWriter struct {
	writer    io.Writer
	remaining int64
}

func (w *boundedMediaWriter) Write(p []byte) (int, error) {
	if int64(len(p)) > w.remaining {
		return 0, errors.New("media output limit")
	}
	n, err := w.writer.Write(p)
	w.remaining -= int64(n)
	return n, err
}
