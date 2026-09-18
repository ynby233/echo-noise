package updates

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStableRemainsValidWhenLatestReleaseIsOlder(t *testing.T) {
	server := newStableDiscoveryServer(t, stableDiscoveryFixture{
		LatestVersion:  "v1.0.0",
		LatestRevision: currentRevision,
		StableVersion:  "v2.0.0",
		StableRevision: targetRevision,
	})

	report := NewDiscovery(server.Client(), server.URL+"/github", server.URL+"/registry", server.URL+"/token").Discover(t.Context(), Installed{Revision: currentRevision}, Platform{OS: "linux", Architecture: "amd64"})
	stable := report.Channel("stable")
	if stable.Status != StatusUpdateAvailable || !stable.Installable || !stable.HasUpdate || stable.Version != "v2.0.0" {
		t.Fatalf("stable = %#v", stable)
	}
}

func TestStableAndLatestReleaseBuildHaveIndependentStatus(t *testing.T) {
	server := newStableDiscoveryServer(t, stableDiscoveryFixture{
		LatestVersion:   "v2.0.0",
		LatestRevision:  targetRevision,
		LatestRunStatus: "in_progress",
		StableVersion:   "v1.0.0",
		StableRevision:  currentRevision,
	})

	report := NewDiscovery(server.Client(), server.URL+"/github", server.URL+"/registry", server.URL+"/token").Discover(t.Context(), Installed{Revision: currentRevision}, Platform{OS: "linux", Architecture: "amd64"})
	stable := report.Channel("stable")
	if stable.Status != StatusCurrent || !stable.Installable || stable.Version != "v1.0.0" {
		t.Fatalf("stable = %#v", stable)
	}
	payload, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(payload, &body); err != nil {
		t.Fatal(err)
	}
	latest, ok := body["latest_release"].(map[string]any)
	if !ok || latest["version"] != "v2.0.0" || latest["status"] != string(StatusBuilding) {
		t.Fatalf("latest_release = %#v; report = %s", body["latest_release"], payload)
	}
}

type stableDiscoveryFixture struct {
	LatestVersion     string
	LatestRevision    string
	LatestRunStatus   string
	StableVersion     string
	StableRevision    string
	RunConclusion     string
	StableHTTP        int
	ReleaseHTTP       int
	TagHTTP           int
	CompareHTTP       int
	StableReleaseMode string
	RunsJSON          string
}

func newStableDiscoveryServer(t *testing.T, fixture stableDiscoveryFixture) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/github/releases/latest":
			fmt.Fprintf(w, `{"tag_name":%q,"draft":false,"prerelease":false,"published_at":"2026-09-18T08:00:00Z"}`, fixture.LatestVersion)
		case "/github/releases/tags/" + fixture.StableVersion:
			if fixture.ReleaseHTTP != 0 {
				w.WriteHeader(fixture.ReleaseHTTP)
				return
			}
			fmt.Fprintf(w, `{"tag_name":%q,"draft":%t,"prerelease":%t,"published_at":"2026-09-17T08:00:00Z"}`, fixture.StableVersion, fixture.StableReleaseMode == "draft", fixture.StableReleaseMode == "prerelease")
		case "/github/git/ref/tags/" + fixture.LatestVersion:
			fmt.Fprintf(w, `{"object":{"type":"commit","sha":%q}}`, fixture.LatestRevision)
		case "/github/git/ref/tags/" + fixture.StableVersion:
			if fixture.TagHTTP != 0 {
				w.WriteHeader(fixture.TagHTTP)
				return
			}
			revision := fixture.StableRevision
			if fixture.StableReleaseMode == "mismatch" {
				revision = tagObject
			}
			fmt.Fprintf(w, `{"object":{"type":"commit","sha":%q}}`, revision)
		case "/github/compare/" + currentRevision + "..." + targetRevision:
			if fixture.CompareHTTP != 0 {
				w.WriteHeader(fixture.CompareHTTP)
				return
			}
			fmt.Fprint(w, `{"status":"ahead","files":[{"filename":"internal/server.go"}]}`)
		case "/github/compare/" + targetRevision + "..." + currentRevision:
			fmt.Fprint(w, `{"status":"behind"}`)
		case "/github/commits/main":
			fmt.Fprintf(w, `{"sha":%q}`, targetRevision)
		case "/github/actions/workflows/docker-publish.yml/runs":
			if r.URL.Query().Get("head_sha") != "" && fixture.RunsJSON != "" {
				fmt.Fprint(w, fixture.RunsJSON)
				return
			}
			if r.URL.Query().Get("head_sha") != "" && fixture.LatestRunStatus != "" {
				fmt.Fprintf(w, `{"workflow_runs":[{"head_sha":%q,"head_branch":%q,"created_at":"2026-09-18T08:01:00Z","event":"release","status":%q,"conclusion":%q}]}`, fixture.LatestRevision, fixture.LatestVersion, fixture.LatestRunStatus, fixture.RunConclusion)
				return
			}
			fmt.Fprint(w, `{"workflow_runs":[]}`)
		case "/token":
			fmt.Fprint(w, `{"token":"public-read-token"}`)
		case "/registry/manifests/stable-mcp":
			if fixture.StableHTTP != 0 {
				w.WriteHeader(fixture.StableHTTP)
				return
			}
			writeIndex(w, fixture.StableRevision, fixture.StableVersion, "linux", "amd64")
		case "/registry/manifests/edge-mcp":
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func TestStableCandidateDoesNotInvalidateExistingArtifact(t *testing.T) {
	for _, test := range []struct {
		status, conclusion string
		want               Status
	}{
		{"queued", "", StatusBuildPending},
		{"in_progress", "", StatusBuilding},
		{"completed", "failure", StatusBuildFailed},
		{"completed", "cancelled", StatusBuildCancelled},
		{"completed", "success", StatusPublishing},
		{"", "", StatusReleasePending},
	} {
		for _, missing := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s-%s-missing=%t", test.status, test.conclusion, missing), func(t *testing.T) {
				fixture := stableDiscoveryFixture{LatestVersion: "v2.0.0", LatestRevision: targetRevision, LatestRunStatus: test.status, RunConclusion: test.conclusion, StableVersion: "v1.0.0", StableRevision: currentRevision}
				if missing {
					fixture.StableHTTP = http.StatusNotFound
				}
				server := newStableDiscoveryServer(t, fixture)
				report := NewDiscovery(server.Client(), server.URL+"/github", server.URL+"/registry", server.URL+"/token").Discover(t.Context(), Installed{Revision: currentRevision}, Platform{OS: "linux", Architecture: "amd64"})
				if report.Release.Status != test.want || report.Release.Version != "v2.0.0" {
					t.Fatalf("release = %#v", report.Release)
				}
				stable := report.Channel("stable")
				if missing {
					if stable.Installable || stable.Status != StatusReleasePending {
						t.Fatalf("stable = %#v", stable)
					}
				} else if stable.Status != StatusCurrent || !stable.Installable || stable.Version != "v1.0.0" {
					t.Fatalf("stable = %#v", stable)
				}
			})
		}
	}
}

func TestInvalidLatestReleaseCannotHideValidStable(t *testing.T) {
	server := newStableDiscoveryServer(t, stableDiscoveryFixture{LatestVersion: "invalid-version", LatestRevision: currentRevision, StableVersion: "v2.0.0", StableRevision: targetRevision})
	report := NewDiscovery(server.Client(), server.URL+"/github", server.URL+"/registry", server.URL+"/token").Discover(t.Context(), Installed{Revision: currentRevision}, Platform{OS: "linux", Architecture: "amd64"})
	if stable := report.Channel("stable"); !stable.Installable || !stable.HasUpdate || stable.Status != StatusUpdateAvailable {
		t.Fatalf("stable = %#v", stable)
	}
	if report.Release.Status != StatusInvalidTarget {
		t.Fatalf("candidate = %#v", report.Release)
	}
}

func TestStableStillRequiresItsPublishedReleaseAndMatchingTag(t *testing.T) {
	for _, mode := range []string{"deleted", "draft", "prerelease", "mismatch"} {
		t.Run(mode, func(t *testing.T) {
			fixture := stableDiscoveryFixture{LatestVersion: "v1.0.0", LatestRevision: currentRevision, StableVersion: "v2.0.0", StableRevision: targetRevision, StableReleaseMode: mode}
			if mode == "deleted" {
				fixture.ReleaseHTTP = http.StatusNotFound
			}
			server := newStableDiscoveryServer(t, fixture)
			stable := NewDiscovery(server.Client(), server.URL+"/github", server.URL+"/registry", server.URL+"/token").Discover(t.Context(), Installed{Revision: currentRevision}, Platform{OS: "linux", Architecture: "amd64"}).Channel("stable")
			if stable.Status != StatusInvalidTarget || stable.Installable || stable.HasUpdate {
				t.Fatalf("stable = %#v", stable)
			}
		})
	}
}

func TestStableNetworkFailuresKeepCheckFailed(t *testing.T) {
	for _, target := range []string{"registry", "release", "tag", "compare"} {
		for _, code := range []int{http.StatusForbidden, http.StatusInternalServerError, http.StatusServiceUnavailable} {
			t.Run(fmt.Sprintf("%s-%d", target, code), func(t *testing.T) {
				fixture := stableDiscoveryFixture{LatestVersion: "v1.0.0", LatestRevision: currentRevision, StableVersion: "v2.0.0", StableRevision: targetRevision}
				switch target {
				case "registry":
					fixture.StableHTTP = code
				case "release":
					fixture.ReleaseHTTP = code
				case "tag":
					fixture.TagHTTP = code
				case "compare":
					fixture.CompareHTTP = code
				}
				server := newStableDiscoveryServer(t, fixture)
				stable := NewDiscovery(server.Client(), server.URL+"/github", server.URL+"/registry", server.URL+"/token").Discover(t.Context(), Installed{Revision: currentRevision}, Platform{OS: "linux", Architecture: "amd64"}).Channel("stable")
				if stable.Status != StatusCheckFailed || stable.HasUpdate || stable.Installable || stable.Error == "" {
					t.Fatalf("stable = %#v", stable)
				}
			})
		}
	}
}

func TestLatestReleaseRequiresMatchingRunEvidenceAndUsesNewestRerun(t *testing.T) {
	for _, test := range []struct {
		name, runs string
		want       Status
	}{
		{"push-is-not-stable", fmt.Sprintf(`{"workflow_runs":[{"head_sha":%q,"event":"push","head_branch":"v2.0.0","created_at":"2026-09-18T08:01:00Z","status":"completed","conclusion":"failure"}]}`, targetRevision), StatusReleasePending},
		{"different-tag-same-commit", fmt.Sprintf(`{"workflow_runs":[{"head_sha":%q,"event":"release","head_branch":"v3.0.0","created_at":"2026-09-18T08:01:00Z","status":"completed","conclusion":"failure"}]}`, targetRevision), StatusReleasePending},
		{"old-run", fmt.Sprintf(`{"workflow_runs":[{"head_sha":%q,"event":"release","head_branch":"v2.0.0","created_at":"2026-09-17T08:01:00Z","status":"completed","conclusion":"failure"}]}`, targetRevision), StatusReleasePending},
		{"stable-dispatch", fmt.Sprintf(`{"workflow_runs":[{"head_sha":%q,"event":"workflow_dispatch","head_branch":"main","display_title":"stable v2.0.0","created_at":"2026-09-18T08:01:00Z","status":"completed","conclusion":"failure"}]}`, targetRevision), StatusBuildFailed},
		{"latest-rerun", fmt.Sprintf(`{"workflow_runs":[{"head_sha":%q,"event":"release","head_branch":"v2.0.0","created_at":"2026-09-18T08:02:00Z","updated_at":"2026-09-18T08:03:00Z","status":"completed","conclusion":"failure"},{"head_sha":%q,"event":"release","head_branch":"v2.0.0","created_at":"2026-09-18T08:01:00Z","updated_at":"2026-09-18T08:04:00Z","status":"in_progress"}]}`, targetRevision, targetRevision), StatusBuilding},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := newStableDiscoveryServer(t, stableDiscoveryFixture{LatestVersion: "v2.0.0", LatestRevision: targetRevision, StableVersion: "v1.0.0", StableRevision: currentRevision, RunsJSON: test.runs})
			report := NewDiscovery(server.Client(), server.URL+"/github", server.URL+"/registry", server.URL+"/token").Discover(t.Context(), Installed{Revision: currentRevision}, Platform{OS: "linux", Architecture: "amd64"})
			if report.Release.Status != test.want || !report.Channel("stable").Installable {
				t.Fatalf("report=%#v", report)
			}
		})
	}
}

const (
	currentRevision = "1111111111111111111111111111111111111111"
	targetRevision  = "2222222222222222222222222222222222222222"
	tagObject       = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
)

func TestDiscoverStableReleasePeelsAnnotatedTag(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/github/releases/latest", "/github/releases/tags/v1.2.3":
			fmt.Fprint(w, `{"tag_name":"v1.2.3","draft":false,"prerelease":false,"published_at":"2026-09-11T08:00:00Z"}`)
		case "/github/git/ref/tags/v1.2.3":
			fmt.Fprintf(w, `{"object":{"type":"tag","sha":"%s"}}`, tagObject)
		case "/github/git/tags/" + tagObject:
			fmt.Fprintf(w, `{"object":{"type":"commit","sha":"%s"}}`, targetRevision)
		case "/github/compare/" + currentRevision + "..." + targetRevision:
			fmt.Fprint(w, `{"status":"ahead","files":[{"filename":"internal/server.go"}]}`)
		case "/token":
			fmt.Fprint(w, `{"token":"public-read-token"}`)
		case "/registry/manifests/stable-mcp":
			w.Header().Set("Docker-Content-Digest", "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
			fmt.Fprintf(w, `{"schemaVersion":2,"mediaType":"application/vnd.oci.image.index.v1+json","annotations":{"org.opencontainers.image.revision":"%s","org.opencontainers.image.version":"v1.2.3","org.opencontainers.image.created":"2026-09-11T08:10:00Z"},"manifests":[{"digest":"sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc","platform":{"os":"linux","architecture":"amd64"}}]}`, targetRevision)
		case "/registry/manifests/edge-mcp":
			writeIndex(w, targetRevision, "222222222222", "linux", "amd64")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	discovery := NewDiscovery(server.Client(), server.URL+"/github", server.URL+"/registry", server.URL+"/token")
	report := discovery.Discover(t.Context(), Installed{Revision: currentRevision}, Platform{OS: "linux", Architecture: "amd64"})
	stable := report.Channel("stable")
	if stable.Status != StatusUpdateAvailable || !stable.Installable || stable.Revision != targetRevision || stable.Version != "v1.2.3" {
		t.Fatalf("stable channel = %#v", stable)
	}
	if stable.Digest != "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" {
		t.Fatalf("stable digest = %q", stable.Digest)
	}
	if edge := report.Channel("edge"); edge.Status != StatusUpdateAvailable || !edge.Installable || edge.Revision != stable.Revision {
		t.Fatalf("same-commit edge channel = %#v", edge)
	}
}

func TestDiscoverDistinguishesDocumentationOnlySourceFromInstallableEdge(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/github/releases/latest":
			http.NotFound(w, r)
		case "/github/commits/main":
			fmt.Fprintf(w, `{"sha":"%s"}`, targetRevision)
		case "/github/actions/workflows/docker-publish.yml/runs":
			fmt.Fprint(w, `{"workflow_runs":[]}`)
		case "/github/compare/" + currentRevision + "..." + targetRevision:
			fmt.Fprint(w, `{"status":"ahead","files":[{"filename":"README.md"},{"filename":"docs/maintenance.md"}]}`)
		case "/token":
			fmt.Fprint(w, `{"token":"public-read-token"}`)
		case "/registry/manifests/stable-mcp":
			http.NotFound(w, r)
		case "/registry/manifests/edge-mcp":
			writeIndex(w, currentRevision, "111111111111", "linux", "amd64")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	report := NewDiscovery(server.Client(), server.URL+"/github", server.URL+"/registry", server.URL+"/token").Discover(t.Context(), Installed{Revision: currentRevision}, Platform{OS: "linux", Architecture: "amd64"})
	if report.Source.Status != StatusSourceSkipped || report.Source.Revision != targetRevision {
		t.Fatalf("source = %#v", report.Source)
	}
	if edge := report.Channel("edge"); edge.Status != StatusCurrent || !edge.Installable {
		t.Fatalf("edge = %#v", edge)
	}
}

func TestDiscoverReportsFailedEdgeBuild(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/github/releases/latest", "/registry/manifests/stable-mcp", "/registry/manifests/edge-mcp":
			http.NotFound(w, r)
		case "/github/commits/main":
			fmt.Fprintf(w, `{"sha":"%s"}`, targetRevision)
		case "/github/actions/workflows/docker-publish.yml/runs":
			fmt.Fprintf(w, `{"workflow_runs":[{"head_sha":"%s","status":"completed","conclusion":"failure"}]}`, targetRevision)
		case "/token":
			fmt.Fprint(w, `{"token":"public-read-token"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	report := NewDiscovery(server.Client(), server.URL+"/github", server.URL+"/registry", server.URL+"/token").Discover(t.Context(), Installed{}, Platform{OS: "linux", Architecture: "amd64"})
	if report.Source.Status != StatusBuildFailed {
		t.Fatalf("source = %#v", report.Source)
	}
}

func TestDiscoverRejectsUnsupportedArchitecture(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/github/releases/latest":
			http.NotFound(w, r)
		case "/github/commits/main":
			fmt.Fprintf(w, `{"sha":"%s"}`, targetRevision)
		case "/github/actions/workflows/docker-publish.yml/runs":
			fmt.Fprint(w, `{"workflow_runs":[]}`)
		case "/token":
			fmt.Fprint(w, `{"token":"public-read-token"}`)
		case "/registry/manifests/stable-mcp":
			http.NotFound(w, r)
		case "/registry/manifests/edge-mcp":
			writeIndex(w, targetRevision, "222222222222", "linux", "arm64")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	edge := NewDiscovery(server.Client(), server.URL+"/github", server.URL+"/registry", server.URL+"/token").Discover(t.Context(), Installed{}, Platform{OS: "linux", Architecture: "amd64"}).Channel("edge")
	if edge.Status != StatusUnsupported || edge.Installable {
		t.Fatalf("edge = %#v", edge)
	}
}

func TestPublishedReleaseWithoutManifestIsPending(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/github/releases/latest":
			fmt.Fprint(w, `{"tag_name":"v1.2.3","draft":false,"prerelease":false,"published_at":"2026-09-11T08:00:00Z"}`)
		case "/github/git/ref/tags/v1.2.3":
			fmt.Fprintf(w, `{"object":{"type":"commit","sha":"%s"}}`, targetRevision)
		case "/registry/manifests/stable-mcp", "/registry/manifests/edge-mcp":
			http.NotFound(w, r)
		case "/token":
			fmt.Fprint(w, `{"token":"public-read-token"}`)
		case "/github/actions/workflows/docker-publish.yml/runs":
			fmt.Fprint(w, `{"workflow_runs":[]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	report := NewDiscovery(server.Client(), server.URL+"/github", server.URL+"/registry", server.URL+"/token").Discover(t.Context(), Installed{}, Platform{OS: "linux", Architecture: "amd64"})
	if report.Release.Status != StatusReleasePending || report.Channel("stable").Installable {
		t.Fatalf("report = %#v", report)
	}
}

func TestDiscoverFailsClosedOnRateLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "rate limited", http.StatusForbidden)
	}))
	defer server.Close()

	report := NewDiscovery(server.Client(), server.URL+"/github", server.URL+"/registry", server.URL+"/token").Discover(t.Context(), Installed{}, Platform{OS: "linux", Architecture: "amd64"})
	if report.Source.Status != StatusCheckFailed || report.Channel("stable").Status != StatusCheckFailed || report.Channel("edge").Status != StatusCheckFailed {
		t.Fatalf("rate-limited report = %#v", report)
	}
	if report.Channel("stable").HasUpdate || report.Channel("edge").HasUpdate {
		t.Fatalf("rate limiting must not report an update decision: %#v", report)
	}
}

func TestSameRevisionWithDifferentDigestIsNotAnAutomaticUpdate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/github/releases/latest":
			http.NotFound(w, r)
		case "/github/commits/main":
			fmt.Fprintf(w, `{"sha":"%s"}`, targetRevision)
		case "/token":
			fmt.Fprint(w, `{"token":"public-read-token"}`)
		case "/registry/manifests/stable-mcp":
			http.NotFound(w, r)
		case "/registry/manifests/edge-mcp":
			writeIndex(w, targetRevision, "222222222222", "linux", "amd64")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	installed := Installed{Revision: targetRevision, Digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
	edge := NewDiscovery(server.Client(), server.URL+"/github", server.URL+"/registry", server.URL+"/token").Discover(t.Context(), installed, Platform{OS: "linux", Architecture: "amd64"}).Channel("edge")
	if edge.Status != StatusSameSourceRebuild || edge.HasUpdate {
		t.Fatalf("edge = %#v", edge)
	}
}

func TestDiscoverClassifiesChannelHistory(t *testing.T) {
	for _, test := range []struct {
		comparison string
		want       Status
		update     bool
	}{
		{comparison: "ahead", want: StatusUpdateAvailable, update: true},
		{comparison: "behind", want: StatusChannelBehind},
		{comparison: "diverged", want: StatusDiverged},
	} {
		t.Run(test.comparison, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/github/releases/latest", "/registry/manifests/stable-mcp":
					http.NotFound(w, r)
				case "/github/commits/main":
					fmt.Fprintf(w, `{"sha":"%s"}`, targetRevision)
				case "/github/compare/" + currentRevision + "..." + targetRevision:
					fmt.Fprintf(w, `{"status":%q}`, test.comparison)
				case "/token":
					fmt.Fprint(w, `{"token":"public-read-token"}`)
				case "/registry/manifests/edge-mcp":
					writeIndex(w, targetRevision, "222222222222", "linux", "amd64")
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()

			edge := NewDiscovery(server.Client(), server.URL+"/github", server.URL+"/registry", server.URL+"/token").Discover(t.Context(), Installed{Revision: currentRevision}, Platform{OS: "linux", Architecture: "amd64"}).Channel("edge")
			if edge.Status != test.want || edge.HasUpdate != test.update {
				t.Fatalf("edge = %#v", edge)
			}
		})
	}
}

func writeIndex(w http.ResponseWriter, revision, version, osName, architecture string) {
	w.Header().Set("Docker-Content-Digest", "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	fmt.Fprintf(w, `{"schemaVersion":2,"annotations":{"org.opencontainers.image.revision":"%s","org.opencontainers.image.version":"%s","org.opencontainers.image.created":"2026-09-11T08:10:00Z"},"manifests":[{"platform":{"os":"%s","architecture":"%s"}}]}`, revision, version, osName, architecture)
}
