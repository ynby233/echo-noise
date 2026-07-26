package routers

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSetupRouterCompressesAndCachesFingerprintedNuxtAssets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("ACCESS_LOG", "false")
	t.Chdir(t.TempDir())
	assetDir := filepath.Join("public", "_nuxt")
	if err := os.MkdirAll(assetDir, 0o755); err != nil {
		t.Fatalf("create asset directory: %v", err)
	}
	wantBody := strings.Repeat("const payload = 'compressible asset';\n", 256)
	if err := os.WriteFile(filepath.Join(assetDir, "app.hash.js"), []byte(wantBody), 0o600); err != nil {
		t.Fatalf("write asset: %v", err)
	}

	r := SetupRouter()
	request := httptest.NewRequest(http.MethodGet, "/_nuxt/app.hash.js", nil)
	request.Header.Set("Accept-Encoding", "gzip")
	response := httptest.NewRecorder()
	r.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("asset status = %d: %s", response.Code, response.Body.String())
	}
	if cache := response.Header().Get("Cache-Control"); cache != "public, max-age=31536000, immutable" {
		t.Fatalf("asset Cache-Control = %q", cache)
	}
	if encoding := response.Header().Get("Content-Encoding"); encoding != "gzip" {
		t.Fatalf("asset Content-Encoding = %q", encoding)
	}
	if !strings.Contains(response.Header().Get("Vary"), "Accept-Encoding") {
		t.Fatalf("asset Vary = %q", response.Header().Get("Vary"))
	}
	reader, err := gzip.NewReader(response.Body)
	if err != nil {
		t.Fatalf("open compressed response: %v", err)
	}
	decompressed, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read compressed response: %v", err)
	}
	if err := reader.Close(); err != nil {
		t.Fatalf("close compressed response: %v", err)
	}
	if string(decompressed) != wantBody {
		t.Fatalf("decompressed asset mismatch: got %d bytes, want %d", len(decompressed), len(wantBody))
	}
}
