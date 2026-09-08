package main

import (
	"bytes"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestServerStartupReadiness exercises the actual process and readiness HTTP
// endpoint. It protects the SQLite one-connection startup deadlock: a log line
// or a spawned process is not enough if a worker has exhausted the only DB
// connection before the server can serve a request.
func TestServerStartupReadiness(t *testing.T) {
	if testing.Short() {
		t.Skip("subprocess readiness sampling is skipped in short mode")
	}
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(t.TempDir(), "echo-noise-server")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Dir = filepath.Join(repoRoot, "cmd", "server")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build server: %v\n%s", err, output)
	}

	for index := 0; index < 20; index++ {
		t.Run(fmt.Sprintf("empty-%02d", index+1), func(t *testing.T) {
			runtimeDir := t.TempDir()
			runServerUntilReady(t, binary, runtimeDir, filepath.Join(runtimeDir, "data", "database.db"))
		})
	}

	runtimeDir := t.TempDir()
	databasePath := filepath.Join(runtimeDir, "data", "database.db")
	runServerUntilReady(t, binary, runtimeDir, databasePath) // initialize it once
	for index := 0; index < 20; index++ {
		t.Run(fmt.Sprintf("existing-%02d", index+1), func(t *testing.T) {
			runServerUntilReady(t, binary, runtimeDir, databasePath)
		})
	}
}

func runServerUntilReady(t *testing.T, binary, runtimeDir, databasePath string) {
	t.Helper()
	port := freeLoopbackPort(t)
	writeServerTestConfig(t, runtimeDir, port)
	var output bytes.Buffer
	cmd := exec.Command(binary)
	cmd.Dir = runtimeDir
	cmd.Env = append(os.Environ(),
		"DB_TYPE=sqlite",
		"DB_PATH="+databasePath,
		"ATTACHMENT_BLOB_ROOT="+filepath.Join(runtimeDir, "external-blobs"),
		"SESSION_SECRET=startup-integration-test-only",
	)
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Start(); err != nil {
		t.Fatalf("start server: %v", err)
	}
	t.Cleanup(func() { stopServer(cmd) })

	deadline := time.Now().Add(6 * time.Second)
	endpoint := "http://127.0.0.1:" + strconv.Itoa(port) + "/api/health/ready"
	client := &http.Client{Timeout: 300 * time.Millisecond}
	for time.Now().Before(deadline) {
		response, err := client.Get(endpoint)
		if err == nil {
			response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return
			}
		}
		if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("server did not become HTTP-ready within 6s\n%s", strings.TrimSpace(output.String()))
}

func freeLoopbackPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}

func writeServerTestConfig(t *testing.T, runtimeDir string, port int) {
	t.Helper()
	configDir := filepath.Join(runtimeDir, "config")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}
	content := fmt.Sprintf("server:\n  port: %q\n  host: 127.0.0.1\n  mode: release\ndatabase:\n  type: sqlite\n  path: data/database.db\nupload:\n  maxsize: 52428800\n  savepath: data/images\nauth:\n  jwt:\n    secret: startup-test\n    expires: 3600\n    issuer: startup-test\n    audience: startup-test\n", strconv.Itoa(port))
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func stopServer(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil || (cmd.ProcessState != nil && cmd.ProcessState.Exited()) {
		return
	}
	// The test owns this subprocess and does not exercise shutdown here. A
	// forced stop avoids leaving a transient test server or port behind.
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
}
