//go:build linux

package music

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// openRoot rejects symlinks in every component, including the configured root.
// All subsequent operations are relative to this pinned directory descriptor.
func openRoot(root string) (*os.File, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	fd, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	for _, part := range strings.Split(filepath.Clean(abs), "/") {
		if part == "" {
			continue
		}
		next, e := unix.Openat(fd, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		unix.Close(fd)
		if e != nil {
			return nil, e
		}
		fd = next
	}
	return os.NewFile(uintptr(fd), abs), nil
}

func openAt(root *os.File, rel string, directory bool) (*os.File, error) {
	parts, err := secureParts(rel)
	if err != nil {
		return nil, err
	}
	fd, err := unix.Dup(int(root.Fd()))
	if err != nil {
		return nil, err
	}
	unix.CloseOnExec(fd)
	for i, part := range parts {
		flags := unix.O_RDONLY | unix.O_NOFOLLOW | unix.O_CLOEXEC | unix.O_NONBLOCK
		if i < len(parts)-1 || directory {
			flags |= unix.O_DIRECTORY
		}
		next, e := unix.Openat(fd, part, flags, 0)
		unix.Close(fd)
		if e != nil {
			return nil, e
		}
		fd = next
	}
	f := os.NewFile(uintptr(fd), rel)
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	if (directory && !info.IsDir()) || (!directory && !info.Mode().IsRegular()) {
		f.Close()
		return nil, ErrNotFound
	}
	return f, nil
}

func secureWalk(ctx context.Context, root *os.File, visit func(string, os.FileInfo) error) error {
	return walkDirectory(ctx, root, "", 0, visit)
}

func walkDirectory(ctx context.Context, dir *os.File, prefix string, depth int, visit func(string, os.FileInfo) error) error {
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
			name := entry.Name()
			if name == "." || name == ".." || strings.ContainsAny(name, "/\\\x00") {
				continue
			}
			rel := name
			if prefix != "" {
				rel = prefix + "/" + name
			}
			// Do not trust dirent type: validate the actual inode with openat.
			child, e := openAt(dir, name, true)
			if e == nil {
				info, se := child.Stat()
				if se == nil {
					se = visit(rel, info)
				}
				if se == nil {
					se = walkDirectory(ctx, child, rel, depth+1, visit)
				}
				child.Close()
				if errors.Is(se, errSkipDirectory) {
					continue
				}
				if se != nil {
					return se
				}
				continue
			}
			child, e = openAt(dir, name, false)
			if e != nil {
				if errors.Is(e, unix.ENOENT) || errors.Is(e, unix.ELOOP) || errors.Is(e, unix.ENOTDIR) || errors.Is(e, ErrNotFound) {
					continue
				}
				return e
			}
			info, se := child.Stat()
			child.Close()
			if se != nil {
				return se
			}
			if se = visit(rel, info); se != nil {
				return se
			}
		}
		if err != nil {
			if errors.Is(err, os.ErrClosed) {
				return err
			}
			if err.Error() == "EOF" {
				return nil
			}
			return err
		}
	}
}
