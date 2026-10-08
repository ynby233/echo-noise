//go:build !linux

package music

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
)

// This fallback verifies every ancestor before and after opening and compares
// identities. Linux uses descriptor-relative openat instead.
func verifiedPath(root, rel string, directory bool) (*os.File, error) {
	if rel != "" {
		if _, err := secureParts(rel); err != nil {
			return nil, err
		}
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	abs = filepath.Clean(abs)
	target := abs
	if rel != "" {
		target = filepath.Join(abs, filepath.FromSlash(rel))
	}
	inside, err := filepath.Rel(abs, target)
	if err != nil || inside == ".." || filepath.IsAbs(inside) {
		return nil, ErrNotFound
	}
	var paths []string
	for p := target; ; p = filepath.Dir(p) {
		paths = append(paths, p)
		if filepath.Dir(p) == p {
			break
		}
	}
	infos := make([]os.FileInfo, len(paths))
	for i, p := range paths {
		infos[i], err = os.Lstat(p)
		if err != nil {
			return nil, err
		}
		if infos[i].Mode()&os.ModeSymlink != 0 {
			return nil, ErrNotFound
		}
		if i > 0 && !infos[i].IsDir() {
			return nil, ErrNotFound
		}
	}
	resolved, err := filepath.EvalSymlinks(target)
	if err != nil || filepath.Clean(resolved) != target {
		return nil, ErrNotFound
	}
	if (directory && !infos[0].IsDir()) || (!directory && !infos[0].Mode().IsRegular()) {
		return nil, ErrNotFound
	}
	f, err := os.Open(target)
	if err != nil {
		return nil, err
	}
	actual, err := f.Stat()
	if err != nil || !os.SameFile(infos[0], actual) {
		f.Close()
		return nil, ErrNotFound
	}
	for i, p := range paths {
		after, e := os.Lstat(p)
		if e != nil || after.Mode()&os.ModeSymlink != 0 || !os.SameFile(infos[i], after) {
			f.Close()
			return nil, ErrNotFound
		}
	}
	resolved, err = filepath.EvalSymlinks(target)
	if err != nil || filepath.Clean(resolved) != target {
		f.Close()
		return nil, ErrNotFound
	}
	return f, nil
}

func openRoot(root string) (*os.File, error) { return verifiedPath(root, "", true) }
func openAt(root *os.File, rel string, directory bool) (*os.File, error) {
	before, err := root.Stat()
	if err != nil {
		return nil, err
	}
	current, err := os.Lstat(root.Name())
	if err != nil || current.Mode()&os.ModeSymlink != 0 || !os.SameFile(before, current) {
		return nil, ErrNotFound
	}
	return verifiedPath(root.Name(), rel, directory)
}
func secureWalk(ctx context.Context, root *os.File, visit func(string, os.FileInfo) error) error {
	var walk func(*os.File, string, int) error
	walk = func(dir *os.File, prefix string, depth int) error {
		if depth > maxWalkDepth {
			return errScanLimit
		}
		for {
			if err := ctx.Err(); err != nil {
				return err
			}
			entries, err := dir.ReadDir(128)
			for _, entry := range entries {
				if err := ctx.Err(); err != nil {
					return err
				}
				if entry.Type()&os.ModeSymlink != 0 {
					continue
				}
				rel := entry.Name()
				if prefix != "" {
					rel = prefix + "/" + rel
				}
				child, e := openAt(dir, entry.Name(), entry.IsDir())
				if errors.Is(e, os.ErrNotExist) || errors.Is(e, ErrNotFound) {
					continue
				}
				if e != nil {
					return e
				}
				info, e := child.Stat()
				if e == nil {
					e = visit(rel, info)
				}
				if e == nil && info.IsDir() {
					e = walk(child, rel, depth+1)
				}
				child.Close()
				if errors.Is(e, errSkipDirectory) {
					continue
				}
				if e != nil {
					return e
				}
			}
			if errors.Is(err, io.EOF) {
				return nil
			}
			if err != nil {
				return err
			}
		}
	}
	return walk(root, "", 0)
}
