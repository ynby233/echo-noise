package music

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func testMediaCache(t *testing.T, base string) *mediaCache {
	t.Helper()
	if runtime.GOOS != "linux" && runtime.GOOS != "windows" {
		t.Skip("native cache locking unavailable on this platform")
	}
	c := newMediaCache(Options{CacheDir: base})
	c.audioBudget, c.audioLimit = 16, 8
	c.assetBudget, c.assetLimit = 8, 4
	c.requestWait = 5 * time.Second
	t.Cleanup(c.close)
	return c
}

func cacheTestBuild(_ context.Context, file *os.File) error {
	_, err := file.WriteString("12345678")
	return err
}

func cacheTestWaiters(t *testing.T, c *mediaCache, key string, count int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		c.mu.Lock()
		flight := c.flights[key]
		ready := flight != nil && flight.waiters == count
		c.mu.Unlock()
		if ready {
			return
		}
		runtime.Gosched()
	}
	t.Fatalf("did not reach %d waiters", count)
}

func TestMediaCacheTenWaitersShareOneBuild(t *testing.T) {
	c := testMediaCache(t, filepath.Join(t.TempDir(), "cache"))
	gate := make(chan struct{})
	defer func() {
		select {
		case <-gate:
		default:
			close(gate)
		}
	}()
	var builds atomic.Int32
	results := make(chan *MediaFile, 10)
	errorsCh := make(chan error, 10)
	for range 10 {
		go func() {
			file, err := c.acquire(context.Background(), "shared", false, "audio/test", "shared", func(ctx context.Context, f *os.File) error {
				builds.Add(1)
				<-gate
				return cacheTestBuild(ctx, f)
			})
			results <- file
			errorsCh <- err
		}()
	}
	cacheTestWaiters(t, c, "shared", 10)
	close(gate)
	for range 10 {
		file := <-results
		if file != nil {
			t.Cleanup(func() { _ = file.Close() })
		}
		if err := <-errorsCh; err != nil {
			t.Fatal(err)
		}
	}
	if builds.Load() != 1 {
		t.Fatalf("builds = %d", builds.Load())
	}
	c.mu.Lock()
	refs := c.entries["shared"].refs
	c.mu.Unlock()
	if refs != 10 {
		t.Fatalf("refs = %d, want 10", refs)
	}
}

func TestMediaCacheFailedOldWaiterDoesNotReleaseNewEntry(t *testing.T) {
	c := testMediaCache(t, filepath.Join(t.TempDir(), "cache"))
	file, err := c.acquire(context.Background(), "key", false, "", "", cacheTestBuild)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = file.Close() })
	c.mu.Lock()
	newEntry := c.entries["key"]
	delete(c.entries, "key")
	old := &cacheFlight{done: make(chan struct{}), err: ErrUnavailable}
	c.flights["key"] = old
	c.mu.Unlock()
	result := make(chan error, 1)
	go func() {
		_, err := c.acquire(context.Background(), "key", false, "", "", cacheTestBuild)
		result <- err
	}()
	cacheTestWaiters(t, c, "key", 1)
	// Publish the later generation before the failed generation's waiter resumes.
	c.mu.Lock()
	delete(c.flights, "key")
	c.entries["key"] = newEntry
	close(old.done)
	c.mu.Unlock()
	if err := <-result; !errors.Is(err, ErrUnavailable) {
		t.Fatalf("error = %v", err)
	}
	c.mu.Lock()
	refs := newEntry.refs
	c.mu.Unlock()
	if refs != 1 {
		t.Fatalf("new generation refs = %d, want 1", refs)
	}
}

func TestMediaCacheCapacityKeepsActiveReferences(t *testing.T) {
	c := testMediaCache(t, filepath.Join(t.TempDir(), "cache"))
	first, err := c.acquire(context.Background(), "first", false, "", "", cacheTestBuild)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = first.Close() })
	second, err := c.acquire(context.Background(), "second", false, "", "", cacheTestBuild)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = second.Close() })
	var builds atomic.Int32
	build := func(ctx context.Context, f *os.File) error { builds.Add(1); return cacheTestBuild(ctx, f) }
	if _, err := c.acquire(context.Background(), "third", false, "", "", build); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("capacity error = %v", err)
	}
	if builds.Load() != 0 {
		t.Fatal("over-budget build started")
	}
	firstPath := first.File.Name()
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	third, err := c.acquire(context.Background(), "third", false, "", "", build)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = third.Close() })
	if _, err := os.Stat(firstPath); !os.IsNotExist(err) {
		t.Fatalf("evicted file still exists: %v", err)
	}
	if _, err := second.File.Stat(); err != nil {
		t.Fatalf("active file lost: %v", err)
	}
}

func TestMediaCacheCloseAndRestartReclaimDerivedData(t *testing.T) {
	base := filepath.Join(t.TempDir(), "cache")
	c := testMediaCache(t, base)
	file, err := c.acquire(context.Background(), "old", false, "", "", cacheTestBuild)
	if err != nil {
		t.Fatal(err)
	}
	path, dir := file.File.Name(), c.dir
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	c.close()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("close did not reclaim cache: %v", err)
	}
	// Simulate a crash leaving previously marked, derived files behind.
	stale := filepath.Join(dir, strings.Repeat("a", 64)+".media")
	temp := filepath.Join(dir, ".building-12345")
	unknown := filepath.Join(dir, "keep.txt")
	for _, p := range []string{stale, temp, unknown} {
		if err := os.WriteFile(p, []byte("old"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	next := testMediaCache(t, base)
	fresh, err := next.acquire(context.Background(), "fresh", false, "", "", cacheTestBuild)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fresh.Close() })
	if next.dir != dir {
		t.Fatalf("runtime changed: %q -> %q", dir, next.dir)
	}
	for _, p := range []string{stale, temp} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatalf("stale file survived: %s: %v", p, err)
		}
	}
	if data, err := os.ReadFile(unknown); err != nil || string(data) != "old" {
		t.Fatalf("unknown data changed: %q, %v", data, err)
	}
}

func TestMediaCacheUnmarkedDataProtectedAndFailureUnlocks(t *testing.T) {
	base := filepath.Join(t.TempDir(), "cache")
	dir := filepath.Join(base, "runtime")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, strings.Repeat("b", 64)+".media")
	if err := os.WriteFile(path, []byte("user data"), 0600); err != nil {
		t.Fatal(err)
	}
	c := testMediaCache(t, base)
	if _, err := c.acquire(context.Background(), "key", false, "", "", cacheTestBuild); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("unmarked error = %v", err)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "user data" {
		t.Fatalf("unmarked data changed: %q, %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(dir, mediaCacheMarker)); !os.IsNotExist(err) {
		t.Fatalf("marker created in unmarked directory: %v", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	next := testMediaCache(t, base)
	file, err := next.acquire(context.Background(), "key", false, "", "", cacheTestBuild)
	if err != nil {
		t.Fatalf("failed acquisition leaked lock: %v", err)
	}
	t.Cleanup(func() { _ = file.Close() })
}

func TestMediaCacheSecondOwnerRefusedUntilResponsesClose(t *testing.T) {
	base := filepath.Join(t.TempDir(), "cache")
	c := testMediaCache(t, base)
	file, err := c.acquire(context.Background(), "key", false, "", "", cacheTestBuild)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = file.Close() })
	other := testMediaCache(t, base)
	assertRefused := func() {
		t.Helper()
		if _, err := other.acquire(context.Background(), "other", false, "", "", cacheTestBuild); !errors.Is(err, ErrUnavailable) {
			t.Fatalf("second cache error = %v", err)
		}
		if _, err := file.File.Stat(); err != nil {
			t.Fatalf("live owner data lost: %v", err)
		}
	}
	assertRefused()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := c.wait(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("wait with active response = %v", err)
	}
	assertRefused()
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := c.wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	fresh, err := other.acquire(context.Background(), "other", false, "", "", cacheTestBuild)
	if err != nil {
		t.Fatalf("lock not released after last response: %v", err)
	}
	t.Cleanup(func() { _ = fresh.Close() })
}

func TestMediaCacheRejectsUnsafeRuntimeBeforeCleanup(t *testing.T) {
	for _, kind := range []string{"directory", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			base := filepath.Join(t.TempDir(), "cache")
			dir := filepath.Join(base, "runtime")
			if err := os.MkdirAll(dir, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, mediaCacheMarker), nil, 0600); err != nil {
				t.Fatal(err)
			}
			stale := filepath.Join(dir, strings.Repeat("c", 64)+".media")
			if err := os.WriteFile(stale, []byte("derived"), 0600); err != nil {
				t.Fatal(err)
			}
			unsafe := filepath.Join(dir, "unsafe")
			if kind == "directory" {
				if err := os.Mkdir(unsafe, 0700); err != nil {
					t.Fatal(err)
				}
			} else {
				outside := filepath.Join(t.TempDir(), "outside")
				if err := os.WriteFile(outside, []byte("protected"), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, unsafe); err != nil {
					t.Skipf("symlink unavailable: %v", err)
				}
			}
			c := testMediaCache(t, base)
			if _, err := c.acquire(context.Background(), "key", false, "", "", cacheTestBuild); !errors.Is(err, ErrUnavailable) {
				t.Fatalf("unsafe runtime error = %v", err)
			}
			if _, err := os.Stat(stale); err != nil {
				t.Fatalf("cleanup ran before full validation: %v", err)
			}
		})
	}
}

func TestServiceWaitTimeoutStillSealsAndReleasesCache(t *testing.T) {
	base := filepath.Join(t.TempDir(), "cache")
	s := NewService(nil, Options{CacheDir: base})
	file, err := s.cache.acquire(context.Background(), "key", false, "", "", cacheTestBuild)
	if err != nil {
		t.Fatal(err)
	}
	s.wg.Add(1)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := s.Wait(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("wait error = %v", err)
	}
	s.cache.mu.Lock()
	sealed := s.cache.closed
	s.cache.mu.Unlock()
	if !sealed {
		t.Fatal("timed out service did not seal cache")
	}
	s.wg.Done()
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	finish, cancelFinish := context.WithTimeout(context.Background(), time.Second)
	defer cancelFinish()
	if err := s.cache.wait(finish); err != nil {
		t.Fatal(err)
	}
	other := testMediaCache(t, base)
	fresh, err := other.acquire(context.Background(), "new", false, "", "", cacheTestBuild)
	if err != nil {
		t.Fatal("service timeout leaked cache lock", err)
	}
	defer fresh.Close()
}
