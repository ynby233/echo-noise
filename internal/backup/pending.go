package backup

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

func validatePendingCombination(layout Layout, next TypedArchiveInspection) error {
	for _, kind := range []ArchiveKind{KindData, KindAttachments, KindServerConfig} {
		selected := layout
		selected.pendingKind = kind
		for _, p := range []string{PendingRestorePath(selected), previousPendingRestorePath(selected)} {
			if _, err := os.Lstat(p); errors.Is(err, os.ErrNotExist) {
				continue
			} else if err != nil {
				return err
			}
			prior, err := InspectTypedArchive(p)
			if err != nil {
				return err
			}
			if (next.Legacy || prior.Legacy) && prior.Kind != next.Kind {
				return errors.New("旧版整站备份不能与分类恢复同时暂存，请先完成重启恢复")
			}
		}
	}
	return nil
}

// RestoreRuntimeEnvironment uses the same literal-assignment subset accepted at
// inspection. It never executes a shell. Call only after a server-config restore,
// before loading application settings or starting any database/worker.
func (a *AppliedRestore) RestoreRuntimeEnvironment(layout Layout) error {
	if a == nil {
		return nil
	}
	found := strings.Contains(filepath.Base(a.pending), "server-config")
	for _, child := range a.groups {
		if strings.Contains(filepath.Base(child.pending), "server-config") {
			found = true
		}
	}
	if !found {
		return nil
	}
	p := filepath.Join(layout.configRoot(), "runtime.env")
	if _, err := os.Stat(p); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	if err := validateConfigFile(p, "runtime.env"); err != nil {
		return err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, value, _ := strings.Cut(line, "=")
		if len(value) >= 2 && (value[0] == '\'' || value[0] == '"') {
			value = value[1 : len(value)-1]
		}
		if err := os.Setenv(key, value); err != nil {
			return err
		}
	}
	return nil
}
