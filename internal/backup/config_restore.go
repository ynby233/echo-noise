package backup

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/viper"
)

// Environment restoration accepts application settings only. It never executes
// runtime.env or restores process loader/search-path settings.
func allowedRuntimeKey(key string) bool {
	switch key {
	case "TZ", "ACCESS_LOG", "LOG_LEVEL", "SESSION_SECRET", "NOISE_SESSION_SECRET", "SESSION_MAX_AGE", "COOKIE_SECURE", "COOKIE_SAMESITE", "TRUSTED_PROXIES", "CORS_ORIGINS", "BASE_PATH", "DB_TYPE", "DB_PATH", "DB_HOST", "DB_PORT", "DB_NAME", "DB_USER", "DB_PASSWORD", "DB_SSL_MODE", "ATTACHMENT_BLOB_ROOT", "ATTACHMENT_ZIP_MAX_BYTES", "NOISE_PLAIN_PASSWORD_STORE", "WEB_PUSH_VAPID_PUBLIC_KEY", "WEB_PUSH_VAPID_PRIVATE_KEY", "WEB_PUSH_VAPID_PRIVATE_KEY_FILE", "WEB_PUSH_VAPID_SUBJECT", "NOTE_HOST", "NOTE_HTTP_PORT", "NOTE_TOKEN", "MCP_ENABLED":
		return true
	case "UPDATE_EXECUTOR_WAKE_URL", "UPDATE_EXECUTOR_WAKE_TOKEN_FILE",
		"MCP_TRANSPORT", "MCP_HTTP_HOST", "MCP_HTTP_PORT", "MCP_ALLOWED_HOSTS", "MCP_ALLOWED_ORIGINS",
		"DB_TIMEZONE", "DB_CHARSET", "HTTP_PORT":
		return true
	}
	return false
}

func literalEnvironment(data []byte) map[string]string {
	result := make(map[string]string)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(strings.TrimPrefix(line, "export "), "=")
		if !found {
			continue
		}
		if len(value) >= 2 && (value[0] == '\'' || value[0] == '"') {
			value = value[1 : len(value)-1]
		}
		result[key] = value
	}
	return result
}

func sameRestorePath(a, b string) bool {
	left, err := filepath.Abs(filepath.Clean(a))
	if err != nil {
		return false
	}
	right, err := filepath.Abs(filepath.Clean(b))
	if err != nil {
		return false
	}
	if runtime.GOOS == "windows" {
		return strings.EqualFold(left, right)
	}
	return left == right
}

// A configuration package must not redirect InitDB or attachment workers away
// from the targets that were preflighted and restored. A new deployment chooses
// its mount locations before importing data, not through a package side effect.
func validateRestoreConfiguration(stage string, layout Layout) error {
	directory := filepath.Join(stage, "server-config")
	env := map[string]string{}
	if data, err := os.ReadFile(filepath.Join(directory, "runtime.env")); err == nil {
		env = literalEnvironment(data)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	paths := map[string]string{"DB_PATH": layout.DatabasePath, "NOISE_PLAIN_PASSWORD_STORE": layout.passwordPath()}
	for _, root := range layout.Roots {
		if root.ArchiveName == "attachment-blobs" {
			paths["ATTACHMENT_BLOB_ROOT"] = root.Path
		}
	}
	for key, expected := range paths {
		if value, found := env[key]; found && (value == "" || expected == "" || !sameRestorePath(value, expected)) {
			return fmt.Errorf("配置中的 %s 与当前数据目录不同；请先调整备份配置，避免恢复后读取另一份数据", key)
		}
	}
	currentType := strings.TrimSpace(os.Getenv("DB_TYPE"))
	if currentType == "" {
		currentType = "sqlite"
	}
	if value, found := env["DB_TYPE"]; found && value != currentType {
		return errors.New("配置恢复不能切换数据库类型")
	}
	if value, found := env["WEB_PUSH_VAPID_PRIVATE_KEY_FILE"]; found && value != "" {
		if err := rejectSymlinkPath(value); err != nil {
			return err
		}
		info, err := os.Stat(value)
		if err != nil || !info.Mode().IsRegular() {
			return errors.New("配置引用的推送私钥文件尚未挂载，请先单独恢复该文件")
		}
	}
	source := filepath.Join(directory, "config.yaml")
	if _, err := os.Stat(source); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	cfg := viper.New()
	cfg.SetConfigFile(source)
	if err := cfg.ReadInConfig(); err != nil {
		return err
	}
	if os.Getenv("DB_TYPE") == "" {
		if value := cfg.GetString("database.type"); value != "" && value != currentType {
			return errors.New("配置恢复不能切换数据库类型")
		}
	}
	effectiveDBPath := os.Getenv("DB_PATH")
	if value, ok := env["DB_PATH"]; ok {
		effectiveDBPath = value
	}
	if effectiveDBPath == "" {
		effectiveDBPath = cfg.GetString("database.path")
		if effectiveDBPath == "" {
			effectiveDBPath = "data/noise.db"
		}
	}
	if !sameRestorePath(effectiveDBPath, layout.DatabasePath) {
		return errors.New("配置中的数据库路径与当前恢复目标不一致，请先调整配置")
	}
	if value := cfg.GetString("upload.savepath"); value != "" {
		for _, root := range layout.Roots {
			if root.ArchiveName == "images" && !sameRestorePath(value, root.Path) {
				return errors.New("配置中的图片路径与当前恢复目标不一致，请先调整配置")
			}
		}
	}
	return nil
}
