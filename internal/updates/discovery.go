package updates

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

type Status string

const (
	StatusCurrent           Status = "current"
	StatusSameSourceRebuild Status = "same_source_rebuild"
	StatusUpdateAvailable   Status = "update_available"
	StatusChannelBehind     Status = "channel_behind"
	StatusDiverged          Status = "diverged"
	StatusUnknownHistory    Status = "unknown_history"
	StatusNoRelease         Status = "no_release"
	StatusReleasePending    Status = "release_pending"
	StatusMissing           Status = "missing"
	StatusUnsupported       Status = "unsupported"
	StatusInvalidTarget     Status = "invalid_target"
	StatusCheckFailed       Status = "check_failed"
	StatusSourceReady       Status = "source_ready"
	StatusSourceSkipped     Status = "source_skipped"
	StatusBuildPending      Status = "build_pending"
	StatusBuilding          Status = "building"
	StatusPublishing        Status = "publishing"
	StatusBuildFailed       Status = "build_failed"
	StatusBuildCancelled    Status = "build_cancelled"
)

type Installed struct {
	Revision string `json:"revision,omitempty"`
	Digest   string `json:"digest,omitempty"`
}

type Platform struct {
	OS           string `json:"os"`
	Architecture string `json:"architecture"`
}

type Channel struct {
	Name        string     `json:"name"`
	Image       string     `json:"image"`
	Tag         string     `json:"tag"`
	Digest      string     `json:"digest,omitempty"`
	Revision    string     `json:"revision,omitempty"`
	Version     string     `json:"version,omitempty"`
	BuiltAt     string     `json:"built_at,omitempty"`
	Platforms   []Platform `json:"platforms,omitempty"`
	Status      Status     `json:"status"`
	Installable bool       `json:"installable"`
	HasUpdate   bool       `json:"has_update"`
	Error       string     `json:"error,omitempty"`
}

type Report struct {
	Installed Installed `json:"installed"`
	Source    Source    `json:"latest_source"`
	Channels  []Channel `json:"channels"`
}

type Source struct {
	Revision string `json:"revision,omitempty"`
	Status   Status `json:"status"`
	Error    string `json:"error,omitempty"`
}

func (r Report) Channel(name string) Channel {
	for _, channel := range r.Channels {
		if channel.Name == name {
			return channel
		}
	}
	return Channel{Name: name, Status: StatusMissing}
}

type Discovery struct {
	client       *http.Client
	githubBase   string
	registryBase string
	tokenURL     string
}

func NewDiscovery(client *http.Client, githubBase, registryBase, tokenURL string) *Discovery {
	return &Discovery{
		client:       client,
		githubBase:   strings.TrimRight(githubBase, "/"),
		registryBase: strings.TrimRight(registryBase, "/"),
		tokenURL:     tokenURL,
	}
}

func (d *Discovery) Discover(ctx context.Context, installed Installed, platform Platform) Report {
	edge := d.discoverImageChannel(ctx, "edge", "edge-mcp", installed, platform)
	return Report{
		Installed: installed,
		Source:    d.discoverSource(ctx, edge.Revision),
		Channels: []Channel{
			d.discoverStable(ctx, installed, platform),
			edge,
		},
	}
}

func (d *Discovery) discoverSource(ctx context.Context, edgeRevision string) Source {
	var commit struct {
		SHA string `json:"sha"`
	}
	if err := d.getJSON(ctx, d.githubBase+"/commits/main", &commit); err != nil {
		return Source{Status: StatusCheckFailed, Error: err.Error()}
	}
	commit.SHA = strings.ToLower(strings.TrimSpace(commit.SHA))
	if !validRevision(commit.SHA) {
		return Source{Status: StatusCheckFailed, Error: "main 缺少完整提交身份"}
	}
	source := Source{Revision: commit.SHA}
	if edgeRevision == commit.SHA {
		source.Status = StatusSourceReady
		return source
	}

	var runs struct {
		WorkflowRuns []struct {
			HeadSHA    string `json:"head_sha"`
			Status     string `json:"status"`
			Conclusion string `json:"conclusion"`
		} `json:"workflow_runs"`
	}
	if err := d.getJSON(ctx, d.githubBase+"/actions/workflows/docker-publish.yml/runs?branch=main&event=push&per_page=30", &runs); err != nil {
		source.Status, source.Error = StatusCheckFailed, err.Error()
		return source
	}
	for _, run := range runs.WorkflowRuns {
		if !strings.EqualFold(run.HeadSHA, commit.SHA) {
			continue
		}
		if run.Status != "completed" {
			source.Status = StatusBuilding
			return source
		}
		switch run.Conclusion {
		case "success":
			source.Status = StatusPublishing
		case "cancelled", "skipped":
			source.Status = StatusBuildCancelled
		default:
			source.Status = StatusBuildFailed
		}
		return source
	}

	if validRevision(edgeRevision) {
		var comparison struct {
			Status string `json:"status"`
			Files  []struct {
				Filename string `json:"filename"`
			} `json:"files"`
		}
		if err := d.getJSON(ctx, d.githubBase+"/compare/"+edgeRevision+"..."+commit.SHA, &comparison); err == nil && comparison.Status == "ahead" && documentationOnly(comparison.Files) {
			source.Status = StatusSourceSkipped
			return source
		}
	}
	source.Status = StatusBuildPending
	return source
}

func (d *Discovery) discoverStable(ctx context.Context, installed Installed, platform Platform) Channel {
	channel := Channel{Name: "stable", Image: "ghcr.io/ynby233/echo-noise", Tag: "stable-mcp"}
	var release struct {
		TagName     string `json:"tag_name"`
		Draft       bool   `json:"draft"`
		Prerelease  bool   `json:"prerelease"`
		PublishedAt string `json:"published_at"`
	}
	if err := d.getJSON(ctx, d.githubBase+"/releases/latest", &release); err != nil {
		if errors.Is(err, errNotFound) {
			channel.Status = StatusNoRelease
			return channel
		}
		channel.Status, channel.Error = StatusCheckFailed, err.Error()
		return channel
	}
	if release.Draft || release.Prerelease || release.PublishedAt == "" || !semverPattern.MatchString(release.TagName) {
		channel.Status = StatusNoRelease
		return channel
	}
	revision, err := d.resolveTag(ctx, release.TagName)
	if err != nil {
		channel.Status, channel.Error = StatusCheckFailed, err.Error()
		return channel
	}
	channel = d.discoverImageChannel(ctx, "stable", "stable-mcp", installed, platform)
	if channel.Status == StatusMissing {
		channel.Status = StatusReleasePending
		return channel
	}
	if channel.Revision != revision || channel.Version != normalizeVersion(release.TagName) {
		channel.Status, channel.Installable, channel.HasUpdate = StatusInvalidTarget, false, false
	}
	return channel
}

func (d *Discovery) discoverImageChannel(ctx context.Context, name, tag string, installed Installed, platform Platform) Channel {
	channel := Channel{Name: name, Image: "ghcr.io/ynby233/echo-noise", Tag: tag}
	manifest, digest, err := d.fetchManifest(ctx, tag)
	if err != nil {
		if errors.Is(err, errNotFound) {
			channel.Status = StatusMissing
			return channel
		}
		channel.Status, channel.Error = StatusCheckFailed, err.Error()
		return channel
	}
	channel.Digest = digest
	channel.Revision = strings.ToLower(strings.TrimSpace(manifest.Annotations["org.opencontainers.image.revision"]))
	channel.Version = normalizeVersion(manifest.Annotations["org.opencontainers.image.version"])
	channel.BuiltAt = strings.TrimSpace(manifest.Annotations["org.opencontainers.image.created"])
	for _, item := range manifest.Manifests {
		if item.Platform.OS != "" && item.Platform.Architecture != "" {
			channel.Platforms = append(channel.Platforms, item.Platform)
		}
	}
	if !validRevision(channel.Revision) || channel.Version == "" || !validDigest(channel.Digest) {
		channel.Status = StatusInvalidTarget
		return channel
	}
	if !supports(channel.Platforms, platform) {
		channel.Status = StatusUnsupported
		return channel
	}
	channel.Installable = true
	if strings.EqualFold(strings.TrimSpace(installed.Revision), channel.Revision) && validDigest(installed.Digest) && installed.Digest != channel.Digest {
		channel.Status = StatusSameSourceRebuild
		return channel
	}
	channel.Status, channel.HasUpdate, channel.Error = d.compare(ctx, installed.Revision, channel.Revision)
	return channel
}

func (d *Discovery) compare(ctx context.Context, installed, target string) (Status, bool, string) {
	installed = strings.ToLower(strings.TrimSpace(installed))
	if !validRevision(installed) {
		return StatusUnknownHistory, false, ""
	}
	if installed == target {
		return StatusCurrent, false, ""
	}
	var comparison struct {
		Status string `json:"status"`
	}
	if err := d.getJSON(ctx, d.githubBase+"/compare/"+installed+"..."+target, &comparison); err != nil {
		return StatusCheckFailed, false, err.Error()
	}
	switch comparison.Status {
	case "ahead":
		return StatusUpdateAvailable, true, ""
	case "behind":
		return StatusChannelBehind, false, ""
	case "identical":
		return StatusCurrent, false, ""
	default:
		return StatusDiverged, false, ""
	}
}

func (d *Discovery) resolveTag(ctx context.Context, tag string) (string, error) {
	var ref struct {
		Object gitObject `json:"object"`
	}
	if err := d.getJSON(ctx, d.githubBase+"/git/ref/tags/"+url.PathEscape(tag), &ref); err != nil {
		return "", err
	}
	object := ref.Object
	for range 5 {
		if object.Type == "commit" && validRevision(object.SHA) {
			return strings.ToLower(object.SHA), nil
		}
		if object.Type != "tag" || !validRevision(object.SHA) {
			break
		}
		var tagObject struct {
			Object gitObject `json:"object"`
		}
		if err := d.getJSON(ctx, d.githubBase+"/git/tags/"+object.SHA, &tagObject); err != nil {
			return "", err
		}
		object = tagObject.Object
	}
	return "", fmt.Errorf("Release Tag 没有解析到提交")
}

type gitObject struct {
	Type string `json:"type"`
	SHA  string `json:"sha"`
}

type imageIndex struct {
	Annotations map[string]string `json:"annotations"`
	Manifests   []struct {
		Platform Platform `json:"platform"`
	} `json:"manifests"`
}

func (d *Discovery) fetchManifest(ctx context.Context, tag string) (imageIndex, string, error) {
	var token struct {
		Token string `json:"token"`
	}
	if err := d.getJSON(ctx, d.tokenURL, &token); err != nil {
		return imageIndex{}, "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.registryBase+"/manifests/"+url.PathEscape(tag), nil)
	if err != nil {
		return imageIndex{}, "", err
	}
	req.Header.Set("Authorization", "Bearer "+token.Token)
	req.Header.Set("Accept", "application/vnd.oci.image.index.v1+json, application/vnd.docker.distribution.manifest.list.v2+json")
	resp, err := d.client.Do(req)
	if err != nil {
		return imageIndex{}, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return imageIndex{}, "", errNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return imageIndex{}, "", fmt.Errorf("GHCR 返回 %s", resp.Status)
	}
	var manifest imageIndex
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		return imageIndex{}, "", err
	}
	return manifest, strings.TrimSpace(resp.Header.Get("Docker-Content-Digest")), nil
}

func (d *Discovery) getJSON(ctx context.Context, rawURL string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "echo-noise")
	resp, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return errNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("版本源返回 %s", resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(target)
}

func supports(platforms []Platform, wanted Platform) bool {
	for _, platform := range platforms {
		if platform.OS == wanted.OS && platform.Architecture == wanted.Architecture {
			return true
		}
	}
	return false
}

func documentationOnly(files []struct {
	Filename string `json:"filename"`
}) bool {
	if len(files) == 0 || len(files) >= 300 {
		return false
	}
	for _, file := range files {
		name := strings.ToLower(strings.TrimSpace(file.Filename))
		if !strings.HasPrefix(name, "docs/") && !strings.HasSuffix(name, ".md") && !strings.HasPrefix(name, "license") {
			return false
		}
	}
	return true
}

var (
	errNotFound   = errors.New("not found")
	hex40Pattern  = regexp.MustCompile(`(?i)^[0-9a-f]{40}$`)
	digestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	semverPattern = regexp.MustCompile(`(?i)^v[0-9]+\.[0-9]+\.[0-9]+$`)
	sha12Pattern  = regexp.MustCompile(`(?i)^[0-9a-f]{12}$`)
)

func validRevision(value string) bool { return hex40Pattern.MatchString(value) }
func validDigest(value string) bool   { return digestPattern.MatchString(value) }

func normalizeVersion(value string) string {
	value = strings.TrimSpace(value)
	if semverPattern.MatchString(value) {
		return "v" + strings.TrimPrefix(strings.ToLower(value), "v")
	}
	if sha12Pattern.MatchString(value) {
		return strings.ToLower(value)
	}
	return ""
}
