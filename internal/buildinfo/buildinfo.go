package buildinfo

import (
	"os"
	"regexp"
	"strings"
)

// Identity is replaced at build time with -X. Development binaries keep the
// fallback and may read BUILD_ID/APP_VERSION from their runtime environment.
var Identity = "dev"

var (
	semverIdentityPattern = regexp.MustCompile(`(?i)^v?(\d+)\.(\d+)\.(\d+)$`)
	shaIdentityPattern    = regexp.MustCompile(`(?i)^[0-9a-f]{7,40}$`)
)

func NormalizeIdentity(raw string) string {
	value := strings.TrimSpace(raw)
	if matches := semverIdentityPattern.FindStringSubmatch(value); len(matches) == 4 {
		return "v" + matches[1] + "." + matches[2] + "." + matches[3]
	}
	if shaIdentityPattern.MatchString(value) {
		value = strings.ToLower(value)
		if len(value) > 12 {
			value = value[:12]
		}
		return value
	}
	return "unknown"
}

func Current() string {
	if value := NormalizeIdentity(Identity); value != "unknown" {
		return value
	}
	for _, key := range []string{"BUILD_ID", "APP_VERSION", "ECHO_NOISE_VERSION", "IMAGE_TAG"} {
		if value := NormalizeIdentity(os.Getenv(key)); value != "unknown" {
			return value
		}
	}
	return "unknown"
}
