package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/rcy1314/echo-noise/internal/models"
)

func managedHTTPSite(endpoint string) models.SiteConfig {
	return models.SiteConfig{StorageEnabled: true, StorageProvider: "s3", StorageEndpoint: endpoint, StorageRegion: "us-east-1", StorageBucket: "backups", StorageAccessKey: "dummy-access", StorageSecretKey: "dummy-secret", StorageUsePathStyle: true, StoragePublicBaseURL: "https://cdn.invalid/fixedfolder"}
}

func managedHTTPStore(t *testing.T, endpoint string) *ManagedStore {
	t.Helper()
	store, err := NewManagedStore(managedHTTPSite(endpoint), "data")
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func TestManagedS3HTTPFixedFolderAndPaginatedDiscovery(t *testing.T) {
	const prefix = "fixedfolder/noise-managed-backups/data/"
	const name = "938ba162-7f1d-45ef-8d00-df7c45822b79.zip"
	const second = "775b9fc4-dc10-4856-94dd-27d241de6b1b.zip"
	var puts, gets, lists, deletes atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("list-type") == "2" {
			lists.Add(1)
			if r.URL.Path != "/backups" && r.URL.Path != "/backups/" {
				t.Errorf("list path=%q", r.URL.Path)
			}
			if r.URL.Query().Get("prefix") != prefix {
				t.Errorf("prefix=%q", r.URL.Query().Get("prefix"))
			}
			w.Header().Set("Content-Type", "application/xml")
			if r.URL.Query().Get("continuation-token") == "" {
				fmt.Fprintf(w, `<ListBucketResult><IsTruncated>true</IsTruncated><NextContinuationToken>page-two</NextContinuationToken><Contents><Key>%s%s</Key><Size>7</Size><LastModified>2026-11-11T12:00:00Z</LastModified></Contents><Contents><Key>fixedfolder/backup.zip</Key><Size>7</Size></Contents><Contents><Key>%snested/file.zip</Key><Size>7</Size></Contents><Contents><Key>%s../outside.zip</Key><Size>7</Size></Contents><Contents><Key>fixedfolder/noise-managed-backups/attachments/unrelated.zip</Key><Size>7</Size></Contents></ListBucketResult>`, prefix, name, prefix, prefix)
			} else {
				if r.URL.Query().Get("continuation-token") != "page-two" {
					t.Errorf("token=%q", r.URL.Query().Get("continuation-token"))
				}
				fmt.Fprintf(w, `<ListBucketResult><IsTruncated>false</IsTruncated><Contents><Key>%s%s</Key><Size>9</Size><LastModified>2026-11-10T12:00:00Z</LastModified></Contents></ListBucketResult>`, prefix, second)
			}
			return
		}
		if r.URL.Path != "/backups/"+prefix+name {
			t.Errorf("object path=%q", r.URL.Path)
			http.Error(w, "bad key", http.StatusBadRequest)
			return
		}
		switch r.Method {
		case http.MethodPut:
			puts.Add(1)
			body, err := io.ReadAll(r.Body)
			if err != nil || !bytes.Contains(body, []byte("archive")) {
				t.Errorf("upload body=%q error=%v", body, err)
			}
			if r.Header.Get("Content-Type") != "application/zip" {
				t.Errorf("content type=%q", r.Header.Get("Content-Type"))
			}
			w.Header().Set("ETag", `"stored"`)
		case http.MethodGet:
			gets.Add(1)
			_, _ = io.WriteString(w, "archive")
		case http.MethodDelete:
			deletes.Add(1)
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("method=%s", r.Method)
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer server.Close()
	store := managedHTTPStore(t, server.URL)
	ctx := context.Background()
	if err := store.Upload(ctx, name, strings.NewReader("archive")); err != nil {
		t.Fatal(err)
	}
	reader, err := store.Open(ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(reader)
	_ = reader.Close()
	if err != nil || string(body) != "archive" {
		t.Fatalf("body=%q error=%v", body, err)
	}
	var downloaded bytes.Buffer
	if n, err := store.Download(ctx, name, &downloaded); err != nil || n != 7 || downloaded.String() != "archive" {
		t.Fatalf("download=%q n=%d error=%v", downloaded.String(), n, err)
	}
	objects, err := store.List(ctx)
	if err != nil || len(objects) != 2 || objects[0].Name != name || objects[1].Name != second || objects[1].Size != 9 {
		t.Fatalf("objects=%+v error=%v", objects, err)
	}
	if err := store.Delete(ctx, name); err != nil {
		t.Fatal(err)
	}
	if puts.Load() != 1 || gets.Load() != 2 || lists.Load() != 2 || deletes.Load() != 1 {
		t.Fatalf("requests put=%d get=%d list=%d delete=%d", puts.Load(), gets.Load(), lists.Load(), deletes.Load())
	}
}

func TestManagedS3HTTPListingRejectsIncompletePages(t *testing.T) {
	for _, mode := range []string{"forbidden-page-two", "repeated-token", "empty-token"} {
		t.Run(mode, func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				page := requests.Add(1)
				w.Header().Set("Content-Type", "application/xml")
				if mode == "forbidden-page-two" && page > 1 {
					w.WriteHeader(http.StatusForbidden)
					_, _ = io.WriteString(w, `<Error><Code>AccessDenied</Code></Error>`)
					return
				}
				token := "again"
				if mode == "empty-token" {
					token = ""
				}
				fmt.Fprintf(w, `<ListBucketResult><IsTruncated>true</IsTruncated><NextContinuationToken>%s</NextContinuationToken><Contents><Key>fixedfolder/noise-managed-backups/data/one.zip</Key><Size>5</Size></Contents></ListBucketResult>`, token)
			}))
			defer server.Close()
			objects, err := managedHTTPStore(t, server.URL).List(context.Background())
			if err == nil || objects != nil {
				t.Fatalf("partial list escaped: objects=%+v error=%v", objects, err)
			}
			want := int32(2)
			if mode == "empty-token" {
				want = 1
			}
			if requests.Load() != want {
				t.Fatalf("requests=%d want=%d", requests.Load(), want)
			}
		})
	}
}

func TestManagedS3HTTPMissingPermissionAndPartialDownloads(t *testing.T) {
	for _, mode := range []string{"missing", "forbidden", "truncated", "partial"} {
		t.Run(mode, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch mode {
				case "missing":
					w.Header().Set("Content-Type", "application/xml")
					w.WriteHeader(http.StatusNotFound)
					_, _ = io.WriteString(w, `<Error><Code>NoSuchKey</Code></Error>`)
				case "forbidden":
					w.Header().Set("Content-Type", "application/xml")
					w.WriteHeader(http.StatusForbidden)
					_, _ = io.WriteString(w, `<Error><Code>AccessDenied</Code></Error>`)
				case "truncated":
					w.Header().Set("Content-Length", "100")
					_, _ = io.WriteString(w, "short")
				case "partial":
					w.Header().Set("Content-Range", "bytes 0-4/100")
					w.WriteHeader(http.StatusPartialContent)
					_, _ = io.WriteString(w, "short")
				}
			}))
			defer server.Close()
			store := managedHTTPStore(t, server.URL)
			var body bytes.Buffer
			_, err := store.Download(context.Background(), "one.zip", &body)
			if err == nil {
				t.Fatal("invalid download reported success")
			}
			if errors.Is(err, ErrMissingContent) != (mode == "missing") {
				t.Fatalf("incorrect missing classification: %v", err)
			}
			if mode == "missing" || mode == "forbidden" {
				reader, err := store.Open(context.Background(), "one.zip")
				if reader != nil {
					_ = reader.Close()
					t.Fatal("Open returned reader on HTTP error")
				}
				if err == nil {
					t.Fatal("Open delayed HTTP error until response body")
				}
			}
		})
	}
}

func TestManagedS3HTTPUploadErrorsAndCanceledRequests(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `<Error><Code>AccessDenied</Code></Error>`)
	}))
	defer server.Close()
	store := managedHTTPStore(t, server.URL)
	if err := store.Upload(context.Background(), "one.zip", strings.NewReader("archive")); err == nil {
		t.Fatal("rejected upload reported success")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := store.Upload(ctx, "one.zip", strings.NewReader("archive")); !errors.Is(err, context.Canceled) {
		t.Fatalf("upload error=%v", err)
	}
	if _, err := store.List(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("list error=%v", err)
	}
	if _, err := store.Open(ctx, "one.zip"); !errors.Is(err, context.Canceled) {
		t.Fatalf("open error=%v", err)
	}
	if err := store.Delete(ctx, "one.zip"); !errors.Is(err, context.Canceled) {
		t.Fatalf("delete error=%v", err)
	}
	for _, name := range []string{"../one.zip", "nested/one.zip", `nested\one.zip`, "one.txt"} {
		if err := store.Delete(context.Background(), name); err == nil {
			t.Fatalf("unsafe name accepted: %q", name)
		}
	}
	if requests.Load() != 1 {
		t.Fatalf("invalid or canceled requests reached S3: %d", requests.Load())
	}
}

func TestManagedS3HTTPCancelClosesBlockedBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reader, err := managedHTTPStore(t, server.URL).Open(ctx, "one.zip")
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	cancel()
	if _, err := io.ReadAll(reader); !errors.Is(err, context.Canceled) {
		t.Fatalf("read error=%v", err)
	}
}

func TestAttachmentS3HTTPUsesIndependentChannelAndPersistedKeyVerbatim(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/attachments/stored/folder/blob.bin" {
			t.Errorf("attachment path=%q", r.URL.Path)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_, _ = io.WriteString(w, "attachment")
	}))
	defer server.Close()
	site := models.SiteConfig{StorageEndpoint: "http://invalid.invalid", StorageBucket: "wrong", StoragePublicBaseURL: "https://invalid.invalid/wrong-prefix", AttachmentStorageProvider: "s3", AttachmentStorageEndpoint: server.URL, AttachmentStorageBucket: "attachments", AttachmentStorageRegion: "us-east-1", AttachmentStorageAccessKey: "dummy-access", AttachmentStorageSecretKey: "dummy-secret", AttachmentStorageUsePathStyle: true, AttachmentStoragePublicBaseURL: "https://invalid.invalid/also-wrong"}
	var body bytes.Buffer
	if n, err := DownloadAttachmentObject(context.Background(), site, "stored/folder/blob.bin", &body); err != nil || n != 10 || body.String() != "attachment" {
		t.Fatalf("body=%q n=%d error=%v", body.String(), n, err)
	}
}

func TestManagedCloudConfigurationErrorsExplainHowToRecover(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1); w.WriteHeader(500) }))
	defer server.Close()
	for _, test := range []struct {
		name          string
		site          models.SiteConfig
		kind, message string
	}{
		{"disabled", models.SiteConfig{StorageEndpoint: server.URL}, "data", "云备份连接尚未启用"},
		{"missing credentials", models.SiteConfig{StorageEnabled: true, StorageEndpoint: server.URL, StorageBucket: "backups"}, "data", "云备份连接配置不完整"},
		{"invalid kind", managedHTTPSite(server.URL), "unknown", "无效的云备份类型"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewManagedStore(test.site, test.kind)
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("unexpected configuration error: %v", err)
			}
		})
	}
	if requests.Load() != 0 {
		t.Fatal("invalid configuration contacted remote storage")
	}
}
