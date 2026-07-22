package controllers

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestPublicVersionLabelDoesNotExposeBuildRevision(t *testing.T) {
	t.Setenv("APP_VERSION", "52f10102c8693dd9359d59c65a8e98007bb0115f")
	t.Setenv("ECHO_NOISE_VERSION", "sha-52f10102c869-mcp")
	t.Setenv("IMAGE_TAG", "ghcr.io/example/project:52f10102c869")

	if got := publicVersionLabel(); got != "installed" {
		t.Fatalf("publicVersionLabel() = %q, want %q", got, "installed")
	}
}

func TestGetVersionResponseDoesNotExposeBuildRevision(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("APP_VERSION", "52f10102c8693dd9359d59c65a8e98007bb0115f")
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	GetVersion(ctx)

	var response struct {
		Data struct {
			Version string `json:"version"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Data.Version != "installed" {
		t.Fatalf("public version = %q", response.Data.Version)
	}
	if strings.Contains(recorder.Body.String(), "52f10102") {
		t.Fatalf("response leaked build revision: %s", recorder.Body.String())
	}
}
