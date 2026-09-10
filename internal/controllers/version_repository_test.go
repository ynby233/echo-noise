package controllers

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLatestRepositoryVersionFallsBackToNewestTag(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/releases/latest":
			http.NotFound(w, r)
		case "/tags":
			_, _ = w.Write([]byte(`[{"name":"V3.3","commit":{"sha":"abc123"}}]`))
		case "/commits/abc123":
			_, _ = w.Write([]byte(`{"commit":{"committer":{"date":"2026-05-12T10:46:45Z"}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	got, err := latestRepositoryVersion(server.Client(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if got.TagName != "V3.3" || got.PublishedAt != "2026-05-12T10:46:45Z" {
		t.Fatalf("latestRepositoryVersion() = %#v", got)
	}
}

func TestLatestRepositoryVersionPrefersPublishedRelease(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/releases/latest" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"tag_name":"v4.0.0","published_at":"2026-09-10T08:00:00Z"}`))
	}))
	defer server.Close()

	got, err := latestRepositoryVersion(server.Client(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if got.TagName != "v4.0.0" || got.PublishedAt != "2026-09-10T08:00:00Z" {
		t.Fatalf("latestRepositoryVersion() = %#v", got)
	}
}
