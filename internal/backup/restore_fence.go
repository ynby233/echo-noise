package backup

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// The marker intentionally fails closed after abrupt process termination. It
// does not guess which files were applied or initiate an unconfirmed recovery.
// Existing rollback copies and pending packages are retained for manual repair.
func beginRestoreFence(layout Layout) (string, error) {
	marker := filepath.Join(filepath.Dir(layout.DatabasePath), ".echo-noise-restore-in-progress")
	if _, err := os.Lstat(marker); err == nil {
		return "", &RestoreApplyError{Cause: fmt.Errorf("检测到上次恢复过程中服务异常中断；已停止启动以避免使用不一致数据。请保留待恢复包和回退副本，人工核对后再处理标记 %s", marker)}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if !HasPendingRestore(layout) {
		return "", nil
	}
	file, err := os.OpenFile(marker, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return "", err
	}
	_, writeErr := fmt.Fprintf(file, "restore started %s\n", time.Now().UTC().Format(time.RFC3339Nano))
	syncErr := file.Sync()
	closeErr := file.Close()
	if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
		_ = os.Remove(marker)
		return "", err
	}
	return marker, nil
}
