//go:build windows

package music

import (
	"os"

	"golang.org/x/sys/windows"
)

func init() {
	lockCacheFile = func(file *os.File) error {
		var overlapped windows.Overlapped
		return windows.LockFileEx(windows.Handle(file.Fd()), windows.LOCKFILE_FAIL_IMMEDIATELY|windows.LOCKFILE_EXCLUSIVE_LOCK, 0, 1, 0, &overlapped)
	}
	unlockCacheFile = func(file *os.File) error {
		var overlapped windows.Overlapped
		return windows.UnlockFileEx(windows.Handle(file.Fd()), 0, 1, 0, &overlapped)
	}
}
