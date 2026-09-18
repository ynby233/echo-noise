package buildinfo

import "testing"

func TestCurrentMetadataKeepsFullRevision(t *testing.T) {
	originalIdentity, originalRevision, originalBuiltAt, originalVersion := Identity, Revision, BuiltAt, Version
	t.Cleanup(func() {
		Identity, Revision, BuiltAt, Version = originalIdentity, originalRevision, originalBuiltAt, originalVersion
	})

	Identity = "dev"
	Revision = "8A5ED7759B123200D8200BBB9AB2E2386977AD38"
	BuiltAt = "2026-09-11T08:30:00Z"
	Version = "V4.0.6"

	got := CurrentMetadata()
	if got.Revision != "8a5ed7759b123200d8200bbb9ab2e2386977ad38" || got.BuiltAt != "2026-09-11T08:30:00Z" || got.Version != "v4.0.6" {
		t.Fatalf("CurrentMetadata() = %#v", got)
	}
	if got.Identity != "v4.0.6" {
		t.Fatalf("display identity = %q, want v4.0.6", got.Identity)
	}
}

func TestNormalizeIdentity(t *testing.T) {
	tests := map[string]string{
		"8A5ED7759B123200D8200BBB9AB2E2386977AD38": "8a5ed7759b12",
		"v4.0.6": "v4.0.6",
		"V4.0.6": "v4.0.6",
		"4.0.6":  "v4.0.6",
		"latest": "unknown",
		"":       "unknown",
	}
	for input, want := range tests {
		if got := NormalizeIdentity(input); got != want {
			t.Errorf("NormalizeIdentity(%q)=%q, want %q", input, got, want)
		}
	}
}

func TestCurrentPrefersCompiledIdentityAndFallsBackToBuildEnvironment(t *testing.T) {
	original := Identity
	t.Cleanup(func() { Identity = original })

	Identity = "0123456789abcdef"
	t.Setenv("BUILD_ID", "fedcba9876543210")
	if got := Current(); got != "0123456789ab" {
		t.Fatalf("compiled identity current=%q", got)
	}

	Identity = "dev"
	if got := Current(); got != "fedcba987654" {
		t.Fatalf("environment fallback current=%q", got)
	}
}
