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

	"github.com/glebarez/sqlite"
	backupservice "github.com/rcy1314/echo-noise/internal/backup"
	"gorm.io/gorm"
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
	corruptPending := filepath.Join(filepath.Dir(databasePath), ".echo-noise-restore-pending.zip")
	if err := os.WriteFile(corruptPending, []byte("not a zip archive"), 0600); err != nil {
		t.Fatal(err)
	}
	runServerUntilReady(t, binary, runtimeDir, databasePath)
	if _, err := os.Stat(corruptPending); !os.IsNotExist(err) {
		t.Fatalf("invalid pending restore still blocks future startups: %v", err)
	}
	quarantined, err := filepath.Glob(filepath.Join(filepath.Dir(databasePath), ".echo-noise-restore-failed-*.zip"))
	if err != nil || len(quarantined) != 1 {
		t.Fatalf("quarantined restore packages = %#v, %v", quarantined, err)
	}
	testSuccessfulRestoreRoundTrip(t, binary, runtimeDir, databasePath)
	for index := 0; index < 20; index++ {
		t.Run(fmt.Sprintf("existing-%02d", index+1), func(t *testing.T) {
			runServerUntilReady(t, binary, runtimeDir, databasePath)
		})
	}
}

func testSuccessfulRestoreRoundTrip(t *testing.T, binary, runtimeDir, databasePath string) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+filepath.ToSlash(databasePath)+"?_pragma=journal_mode(WAL)"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE TABLE restore_probe (value TEXT NOT NULL)").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO restore_probe(value) VALUES (?)", "restored").Error; err != nil {
		t.Fatal(err)
	}
	sourceBlobs := filepath.Join(runtimeDir, "source-blobs")
	if err := os.MkdirAll(sourceBlobs, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceBlobs, "new.bin"), []byte("restored-blob"), 0600); err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(runtimeDir, "restore-round-trip.zip")
	if err := backupservice.CreateArchive(archivePath, db, backupservice.Layout{
		DatabasePath: databasePath,
		Roots:        []backupservice.Root{{ArchiveName: "attachment-blobs", Path: sourceBlobs}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("DROP TABLE restore_probe").Error; err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}
	targetBlobs := filepath.Join(runtimeDir, "external-blobs")
	if err := os.MkdirAll(targetBlobs, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(targetBlobs, "old.bin"), []byte("original-blob"), 0600); err != nil {
		t.Fatal(err)
	}
	layout := backupservice.Layout{
		DatabasePath: databasePath,
		Roots:        []backupservice.Root{{ArchiveName: "attachment-blobs", Path: targetBlobs}},
	}
	if err := backupservice.StageRestore(archivePath, layout); err != nil {
		t.Fatal(err)
	}
	runServerUntilReady(t, binary, runtimeDir, databasePath)

	restoredDB, err := gorm.Open(sqlite.Open("file:"+filepath.ToSlash(databasePath)+"?mode=ro"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	var value string
	if err := restoredDB.Raw("SELECT value FROM restore_probe LIMIT 1").Scan(&value).Error; err != nil {
		t.Fatal(err)
	}
	restoredSQL, err := restoredDB.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := restoredSQL.Close(); err != nil {
		t.Fatal(err)
	}
	if value != "restored" {
		t.Fatalf("restored database probe = %q", value)
	}
	if got, err := os.ReadFile(filepath.Join(targetBlobs, "new.bin")); err != nil || string(got) != "restored-blob" {
		t.Fatalf("restored attachment = %q, %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(targetBlobs, "old.bin")); !os.IsNotExist(err) {
		t.Fatalf("old attachment remained after restore: %v", err)
	}
	if backupservice.HasPendingRestore(layout) {
		t.Fatal("successful startup did not commit the pending restore")
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
	defer stopServer(cmd)

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
