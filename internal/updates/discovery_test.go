package updates

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

const (
	currentRevision = "1111111111111111111111111111111111111111"
	targetRevision  = "2222222222222222222222222222222222222222"
	tagObject       = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
)

func TestDiscoverStableReleasePeelsAnnotatedTag(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/github/releases/latest":
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
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	stable := NewDiscovery(server.Client(), server.URL+"/github", server.URL+"/registry", server.URL+"/token").Discover(t.Context(), Installed{}, Platform{OS: "linux", Architecture: "amd64"}).Channel("stable")
	if stable.Status != StatusReleasePending || stable.Installable {
		t.Fatalf("stable = %#v", stable)
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
