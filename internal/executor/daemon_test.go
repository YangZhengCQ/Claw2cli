package executor

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/user/claw2cli/internal/paths"
)

func TestIsProcessRunning(t *testing.T) {
	// Current process should be running
	if !isProcessRunning(os.Getpid()) {
		t.Error("current process should be running")
	}

	// Non-existent PID should not be running
	if isProcessRunning(9999999) {
		t.Error("PID 9999999 should not be running")
	}
}

func TestGetConnectorStatus_NoPIDFile(t *testing.T) {
	dir := t.TempDir()
	paths.SetBaseDir(dir)
	paths.EnsureDirs()

	_, err := GetConnectorStatus("nonexistent")
	if err == nil {
		t.Error("expected error for missing PID file")
	}
}

func TestGetConnectorStatus_StalePID(t *testing.T) {
	dir := t.TempDir()
	paths.SetBaseDir(dir)
	paths.EnsureDirs()

	// Write a PID file with a definitely-dead PID
	pidPath := paths.PIDPath("stale")
	os.WriteFile(pidPath, []byte("9999999\n"), 0644)

	status, err := GetConnectorStatus("stale")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.Running {
		t.Error("stale PID should not be running")
	}
}

func TestGetConnectorStatus_MalformedPID(t *testing.T) {
	dir := t.TempDir()
	paths.SetBaseDir(dir)
	paths.EnsureDirs()

	pidPath := paths.PIDPath("bad")
	os.WriteFile(pidPath, []byte("abc\n"), 0644)

	_, err := GetConnectorStatus("bad")
	if err == nil {
		t.Error("expected error for malformed PID")
	}
	if err != nil && !contains(err.Error(), "parse PID") {
		t.Errorf("expected error to contain 'parse PID', got %q", err.Error())
	}
}

func TestGetConnectorStatus_WithMetadata(t *testing.T) {
	dir := t.TempDir()
	paths.SetBaseDir(dir)
	paths.EnsureDirs()

	// Use current PID so it shows as "running"
	pid := os.Getpid()
	pidPath := paths.PIDPath("meta-test")
	os.WriteFile(pidPath, []byte(fmt.Sprintf("%d\n", pid)), 0644)

	// Write metadata JSON
	now := time.Now().Truncate(time.Second)
	meta := ConnectorStatus{
		Name:      "meta-test",
		PID:       pid,
		Running:   true,
		Socket:    paths.SocketPath("meta-test"),
		StartedAt: now,
	}
	metaData, _ := json.Marshal(meta)
	metaPath := filepath.Join(dir, "pids", "meta-test.json")
	os.WriteFile(metaPath, metaData, 0644)

	status, err := GetConnectorStatus("meta-test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !status.Running {
		t.Error("expected status to be running")
	}
	if status.StartedAt.IsZero() {
		t.Error("expected StartedAt to be populated")
	}
}

func TestListConnectors_Empty(t *testing.T) {
	dir := t.TempDir()
	paths.SetBaseDir(dir)
	paths.EnsureDirs()

	statuses, err := ListConnectors()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(statuses) != 0 {
		t.Errorf("expected empty list, got %d", len(statuses))
	}
}

func TestListConnectors_NoPidsDir(t *testing.T) {
	dir := t.TempDir()
	paths.SetBaseDir(dir)
	// Don't call EnsureDirs — pids dir won't exist

	statuses, err := ListConnectors()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if statuses != nil {
		t.Errorf("expected nil, got %v", statuses)
	}
}

func TestListConnectors_MixedFiles(t *testing.T) {
	dir := t.TempDir()
	paths.SetBaseDir(dir)
	paths.EnsureDirs()

	// Write valid PID files with dead PIDs (so they parse but show as not running)
	os.WriteFile(paths.PIDPath("a"), []byte("9999999\n"), 0644)
	os.WriteFile(paths.PIDPath("b"), []byte("9999998\n"), 0644)
	// Write a .json file that should be skipped
	os.WriteFile(filepath.Join(dir, "pids", "a.json"), []byte("{}"), 0644)

	statuses, err := ListConnectors()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(statuses) != 2 {
		t.Errorf("expected 2 statuses, got %d", len(statuses))
	}
}

func TestListConnectors_SkipsBadEntries(t *testing.T) {
	dir := t.TempDir()
	paths.SetBaseDir(dir)
	paths.EnsureDirs()

	// Write a PID file with unparseable content
	os.WriteFile(paths.PIDPath("bad"), []byte("abc\n"), 0644)

	statuses, err := ListConnectors()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(statuses) != 0 {
		t.Errorf("expected 0 statuses (bad entry skipped), got %d", len(statuses))
	}
}

func TestCleanupConnectorFiles(t *testing.T) {
	dir := t.TempDir()
	paths.SetBaseDir(dir)
	paths.EnsureDirs()

	// Create files to clean up
	os.WriteFile(paths.PIDPath("test"), []byte("123\n"), 0644)
	os.WriteFile(filepath.Join(dir, "pids", "test.json"), []byte("{}"), 0644)
	os.WriteFile(paths.SocketPath("test"), []byte(""), 0644)

	cleanupConnectorFiles("test")

	for _, f := range []string{
		paths.PIDPath("test"),
		filepath.Join(dir, "pids", "test.json"),
		paths.SocketPath("test"),
	} {
		if _, err := os.Stat(f); !os.IsNotExist(err) {
			t.Errorf("file should be cleaned up: %s", f)
		}
	}
}

func TestStartConnector_PermissionDenied(t *testing.T) {
	manifest := connectorManifestNoNetwork()
	err := StartConnector(manifest)
	if err == nil {
		t.Fatal("expected permission error")
	}
	if !contains(err.Error(), "permission") {
		t.Errorf("expected error to contain 'permission', got %q", err.Error())
	}
}

func TestStartConnector_AlreadyRunning(t *testing.T) {
	dir := t.TempDir()
	paths.SetBaseDir(dir)
	paths.EnsureDirs()

	// Write PID file with current process PID (which is running)
	pid := os.Getpid()
	os.WriteFile(paths.PIDPath("test-connector"), []byte(fmt.Sprintf("%d\n", pid)), 0644)

	manifest := connectorManifest()
	err := StartConnector(manifest)
	if err == nil {
		t.Fatal("expected 'already running' error")
	}
	if !contains(err.Error(), "already running") {
		t.Errorf("expected error to contain 'already running', got %q", err.Error())
	}
}

func TestStartConnector_Success(t *testing.T) {
	dir := t.TempDir()
	paths.SetBaseDir(dir)
	paths.EnsureDirs()

	origExec := osExecutable
	origCmd := execCommand
	defer func() {
		osExecutable = origExec
		execCommand = origCmd
	}()

	// Swap osExecutable to return "sleep"
	osExecutable = func() (string, error) {
		return "/bin/sleep", nil
	}
	// Swap execCommand to return a "sleep 0" command (exits immediately, but starts)
	execCommand = func(name string, args ...string) *exec.Cmd {
		cmd := exec.Command("sleep", "999")
		return cmd
	}

	manifest := connectorManifest()
	err := StartConnector(manifest)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify PID file was created
	pidPath := paths.PIDPath("test-connector")
	if _, err := os.Stat(pidPath); os.IsNotExist(err) {
		t.Error("PID file should have been created")
	}

	// Verify metadata JSON was created
	metaPath := filepath.Join(dir, "pids", "test-connector.json")
	if _, err := os.Stat(metaPath); os.IsNotExist(err) {
		t.Error("metadata JSON should have been created")
	}

	// Read metadata and verify fields
	metaData, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatalf("failed to read metadata: %v", err)
	}
	var meta ConnectorStatus
	if err := json.Unmarshal(metaData, &meta); err != nil {
		t.Fatalf("failed to parse metadata: %v", err)
	}
	if meta.Name != "test-connector" {
		t.Errorf("expected name 'test-connector', got %q", meta.Name)
	}
	if meta.StartedAt.IsZero() {
		t.Error("expected StartedAt to be set")
	}
}

func TestStartConnector_ExecutableError(t *testing.T) {
	dir := t.TempDir()
	paths.SetBaseDir(dir)
	paths.EnsureDirs()

	origExec := osExecutable
	defer func() { osExecutable = origExec }()

	osExecutable = func() (string, error) {
		return "", fmt.Errorf("cannot find executable")
	}

	manifest := connectorManifest()
	err := StartConnector(manifest)
	if err == nil {
		t.Fatal("expected error when executable not found")
	}
	if !contains(err.Error(), "find executable") {
		t.Errorf("expected error to contain 'find executable', got %q", err.Error())
	}
}

func TestStopConnector_NoPIDFile(t *testing.T) {
	dir := t.TempDir()
	paths.SetBaseDir(dir)
	paths.EnsureDirs()

	err := StopConnector("nonexistent")
	if err == nil {
		t.Fatal("expected error for missing PID file")
	}
	if !contains(err.Error(), "get status") {
		t.Errorf("expected error to contain 'get status', got %q", err.Error())
	}
}

func TestStopConnector_StalePID(t *testing.T) {
	dir := t.TempDir()
	paths.SetBaseDir(dir)
	paths.EnsureDirs()

	// Write PID file with dead PID
	os.WriteFile(paths.PIDPath("stale"), []byte("9999999\n"), 0644)

	err := StopConnector("stale")
	if err == nil {
		t.Fatal("expected error for stale PID")
	}
	if !contains(err.Error(), "not running") {
		t.Errorf("expected error to contain 'not running', got %q", err.Error())
	}
	// Stale files should be cleaned up
	if _, err := os.Stat(paths.PIDPath("stale")); !os.IsNotExist(err) {
		t.Error("PID file should have been cleaned up")
	}
}

func TestStopConnector_RunningProcess(t *testing.T) {
	dir := t.TempDir()
	paths.SetBaseDir(dir)
	paths.EnsureDirs()

	// Start a real sleep process that we can stop
	cmd := exec.Command("sleep", "999")
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start sleep: %v", err)
	}
	defer cmd.Process.Kill()

	pid := cmd.Process.Pid
	os.WriteFile(paths.PIDPath("stopper"), []byte(fmt.Sprintf("%d\n", pid)), 0644)

	err := StopConnector("stopper")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// PID file should be cleaned up
	if _, err := os.Stat(paths.PIDPath("stopper")); !os.IsNotExist(err) {
		t.Error("PID file should have been cleaned up after stop")
	}
}

func TestAttachConnector_NotRunning(t *testing.T) {
	dir := t.TempDir()
	paths.SetBaseDir(dir)
	paths.EnsureDirs()

	// Write PID with dead PID
	os.WriteFile(paths.PIDPath("dead"), []byte("9999999\n"), 0644)

	_, err := AttachConnector("dead")
	if err == nil {
		t.Fatal("expected error for non-running connector")
	}
	if !contains(err.Error(), "not running") {
		t.Errorf("expected error to contain 'not running', got %q", err.Error())
	}
}

func TestAttachConnector_DialError(t *testing.T) {
	dir := t.TempDir()
	paths.SetBaseDir(dir)
	paths.EnsureDirs()

	// Write PID with current process PID (running)
	pid := os.Getpid()
	os.WriteFile(paths.PIDPath("dial-fail"), []byte(fmt.Sprintf("%d\n", pid)), 0644)

	origDial := netDial
	defer func() { netDial = origDial }()

	netDial = func(network, address string) (net.Conn, error) {
		return nil, fmt.Errorf("dial failed: connection refused")
	}

	_, err := AttachConnector("dial-fail")
	if err == nil {
		t.Fatal("expected error for dial failure")
	}
	if !contains(err.Error(), "connect to socket") {
		t.Errorf("expected error to contain 'connect to socket', got %q", err.Error())
	}
}

func TestAttachConnector_NoPIDFile(t *testing.T) {
	dir := t.TempDir()
	paths.SetBaseDir(dir)
	paths.EnsureDirs()

	_, err := AttachConnector("missing")
	if err == nil {
		t.Fatal("expected error for missing connector")
	}
}

// contains is a helper for checking substrings in error messages.
func contains(s, sub string) bool {
	return len(s) >= len(sub) && containsSub(s, sub)
}

func containsSub(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
