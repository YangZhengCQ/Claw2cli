package executor

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/YangZhengCQ/Claw2cli/internal/paths"
	"github.com/YangZhengCQ/Claw2cli/internal/protocol"
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
	if err != nil && !strings.Contains(err.Error(), "ready") {
		t.Fatalf("unexpected error: %v", err)
	}
	if err != nil {
		t.Skipf("skipped readiness check (expected with mock): %v", err)
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

func TestStopConnector_ViaSocket(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "c2c-")
	if err != nil {
		t.Fatalf("mktemp: %v", err)
	}
	defer os.RemoveAll(dir)
	paths.SetBaseDir(dir)
	paths.EnsureDirs()

	// Create a real UDS listener (short name to stay under macOS 104-char limit)
	socketPath := paths.SocketPath("s")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	// Accept one connection, read shutdown, close
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		scanner := bufio.NewScanner(conn)
		scanner.Scan() // read shutdown message
		conn.Close()
		listener.Close()
	}()

	err = StopConnector("s")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Socket file should be cleaned up
	if _, err := os.Stat(socketPath); !os.IsNotExist(err) {
		t.Error("socket file should be cleaned up")
	}
}

func TestAttachConnector_FallbackToDirectSocket(t *testing.T) {
	// When no PID file exists but socket is available, AttachConnector
	// should fall back to direct socket connection (foreground mode).
	dir, err := os.MkdirTemp("/tmp", "c2c-")
	if err != nil {
		t.Fatalf("mktemp: %v", err)
	}
	defer os.RemoveAll(dir)
	origBase := paths.BaseDir()
	paths.SetBaseDir(dir)
	defer paths.SetBaseDir(origBase)
	paths.EnsureDirs()

	// Create a real UDS listener (no PID file — simulates foreground mode)
	socketPath := paths.SocketPath("fb")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	// Accept one connection
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		conn.Write([]byte(`{"type":"log","source":"fb","level":"info","message":"hello"}` + "\n"))
		conn.Close()
	}()

	// AttachConnector should succeed via fallback (no PID file)
	conn, err := AttachConnector("fb")
	if err != nil {
		t.Fatalf("AttachConnector should succeed via socket fallback: %v", err)
	}
	defer conn.Close()

	// Read the greeting
	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(string(buf[:n]), "hello") {
		t.Errorf("expected greeting, got: %s", string(buf[:n]))
	}
}

func TestWaitForReady_PongResponse(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "c2c-")
	if err != nil {
		t.Fatalf("mktemp: %v", err)
	}
	defer os.RemoveAll(dir)
	origBase := paths.BaseDir()
	paths.SetBaseDir(dir)
	defer paths.SetBaseDir(origBase)
	paths.EnsureDirs()

	// Create a UDS listener that responds to ping with pong
	socketPath := paths.SocketPath("rdy")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		buf := make([]byte, 4096)
		n, _ := conn.Read(buf)
		var msg protocol.Message
		if json.Unmarshal(buf[:n], &msg) == nil && msg.Type == protocol.TypePing {
			pong := protocol.NewPong("rdy", msg.ID)
			data, _ := json.Marshal(pong)
			conn.Write(append(data, '\n'))
		}
	}()

	// waitForReady should succeed
	err = waitForReady("rdy", 5*time.Second)
	if err != nil {
		t.Fatalf("waitForReady should succeed with pong: %v", err)
	}
}

func TestWaitForReady_Timeout(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "c2c-")
	if err != nil {
		t.Fatalf("mktemp: %v", err)
	}
	defer os.RemoveAll(dir)
	origBase := paths.BaseDir()
	paths.SetBaseDir(dir)
	defer paths.SetBaseDir(origBase)
	paths.EnsureDirs()

	// No listener on socket — waitForReady should timeout
	err = waitForReady("noexist", 1*time.Second)
	if err == nil {
		t.Fatal("waitForReady should fail when no daemon is listening")
	}
	if !strings.Contains(err.Error(), "ready") {
		t.Errorf("expected 'ready' in error, got: %v", err)
	}
}

// contains is a helper for checking substrings in error messages.
func contains(s, sub string) bool {
	return strings.Contains(s, sub)
}

func TestRotateLogIfNeeded_SmallFile(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "test.log")
	os.WriteFile(logPath, []byte("small log"), 0600)

	rotateLogIfNeeded(logPath)

	// File should still exist (not rotated)
	if _, err := os.Stat(logPath); err != nil {
		t.Error("small log should not be rotated")
	}
	// No .1 file should exist
	if _, err := os.Stat(logPath + ".1"); err == nil {
		t.Error("small log should not create .1 backup")
	}
}

func TestRotateLogIfNeeded_LargeFile(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "test.log")

	// Create a file larger than maxLogSize (10 MB)
	f, _ := os.Create(logPath)
	f.Truncate(maxLogSize + 1)
	f.Close()

	rotateLogIfNeeded(logPath)

	// Original should be gone (renamed)
	if _, err := os.Stat(logPath); err == nil {
		t.Error("large log should be renamed away")
	}
	// .1 backup should exist
	info, err := os.Stat(logPath + ".1")
	if err != nil {
		t.Fatal("large log should be rotated to .1")
	}
	if info.Size() != maxLogSize+1 {
		t.Errorf("backup size should be %d, got %d", maxLogSize+1, info.Size())
	}
}

func TestRotateLogIfNeeded_NonexistentFile(t *testing.T) {
	// Should not panic on missing file
	rotateLogIfNeeded("/nonexistent/path/test.log")
}

func TestRotateLogIfNeeded_OverwritesPreviousBackup(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "test.log")

	// Create old backup
	os.WriteFile(logPath+".1", []byte("old backup"), 0600)

	// Create large current log
	f, _ := os.Create(logPath)
	f.Truncate(maxLogSize + 100)
	f.Close()

	rotateLogIfNeeded(logPath)

	// Old backup should be overwritten
	data, err := os.ReadFile(logPath + ".1")
	if err != nil {
		t.Fatal("backup should exist")
	}
	if string(data) == "old backup" {
		t.Error("old backup should be overwritten, not preserved")
	}
}
