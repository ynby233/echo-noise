package buildinfo

import (
	"os"
	"regexp"
	"strings"
	"time"
)

// Identity is replaced at build time with -X. Development binaries keep the
// fallback and may read BUILD_ID/APP_VERSION from their runtime environment.
var Identity = "dev"

// Docker builds set these separately so update checks never have to infer a
// commit from a display version.
var (
	Revision = ""
	BuiltAt  = ""
	Version  = "dev"
)

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
	if value := NormalizeIdentity(Version); value != "unknown" {
		return value
	}
	for _, key := range []string{"BUILD_ID", "APP_VERSION", "ECHO_NOISE_VERSION", "IMAGE_TAG"} {
		if value := NormalizeIdentity(os.Getenv(key)); value != "unknown" {
			return value
		}
	}
	return "unknown"
}

type Metadata struct {
	Identity string `json:"identity"`
	Version  string `json:"version"`
	Revision string `json:"revision"`
	BuiltAt  string `json:"built_at"`
}

func CurrentMetadata() Metadata {
	revision := normalizeRevision(Revision)
	if revision == "" {
		revision = normalizeRevision(os.Getenv("BUILD_REVISION"))
	}
	if revision == "" {
		revision = normalizeRevision(Identity)
	}
	builtAt := normalizeBuildTime(BuiltAt)
	if builtAt == "" {
		builtAt = normalizeBuildTime(os.Getenv("BUILD_TIME"))
	}
	version := NormalizeIdentity(Version)
	if version == "unknown" {
		version = Current()
	}
	return Metadata{Identity: Current(), Version: version, Revision: revision, BuiltAt: builtAt}
}

func normalizeRevision(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	if len(value) == 40 && shaIdentityPattern.MatchString(value) {
		return value
	}
	return ""
}

func normalizeBuildTime(raw string) string {
	value := strings.TrimSpace(raw)
	if _, err := time.Parse(time.RFC3339, value); err != nil {
		return ""
	}
	return value
}
