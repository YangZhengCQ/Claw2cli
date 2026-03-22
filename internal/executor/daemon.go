package executor

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/user/claw2cli/internal/parser"
	"github.com/user/claw2cli/internal/paths"
)

// ConnectorStatus represents the state of a connector daemon.
type ConnectorStatus struct {
	Name      string    `json:"name"`
	PID       int       `json:"pid"`
	Running   bool      `json:"running"`
	Socket    string    `json:"socket"`
	StartedAt time.Time `json:"started_at,omitempty"`
}

// StartConnector launches a background daemon for a connector plugin.
// It re-invokes the c2c binary with the hidden _daemon subcommand.
func StartConnector(manifest *parser.PluginManifest) error {
	if err := CheckPermissions(manifest); err != nil {
		return fmt.Errorf("permission check: %w", err)
	}

	// Check if already running
	status, err := GetConnectorStatus(manifest.Name)
	if err == nil && status.Running {
		return fmt.Errorf("connector %q is already running (PID %d)", manifest.Name, status.PID)
	}

	// Ensure storage directory exists
	if err := paths.EnsureStorageDir(manifest.Name); err != nil {
		return fmt.Errorf("create storage dir: %w", err)
	}

	// Find our own binary path for re-invocation
	self, err := osExecutable()
	if err != nil {
		return fmt.Errorf("find executable: %w", err)
	}

	// Launch the daemon as a detached subprocess
	cmd := execCommand(self, "_daemon", manifest.Name)
	cmd.Env = BuildEnv(manifest)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true, // Create new session, detach from terminal
	}

	// Redirect daemon stdout/stderr to log file
	logPath := filepath.Join(paths.BaseDir(), "logs", manifest.Name+".log")
	os.MkdirAll(filepath.Dir(logPath), 0755)
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		logFile.Close()
		return fmt.Errorf("start daemon: %w", err)
	}

	// Write PID file
	pidPath := paths.PIDPath(manifest.Name)
	pidData := fmt.Sprintf("%d\n", cmd.Process.Pid)
	if err := os.WriteFile(pidPath, []byte(pidData), 0644); err != nil {
		return fmt.Errorf("write PID file: %w", err)
	}

	// Write metadata for status queries
	metaPath := filepath.Join(paths.BaseDir(), "pids", manifest.Name+".json")
	meta := ConnectorStatus{
		Name:      manifest.Name,
		PID:       cmd.Process.Pid,
		Running:   true,
		Socket:    paths.SocketPath(manifest.Name),
		StartedAt: time.Now(),
	}
	metaData, _ := json.Marshal(meta)
	os.WriteFile(metaPath, metaData, 0644)

	// Detach — don't wait for the child
	cmd.Process.Release()
	logFile.Close()

	return nil
}

// StopConnector sends SIGTERM to a running connector daemon and cleans up.
func StopConnector(name string) error {
	status, err := GetConnectorStatus(name)
	if err != nil {
		return fmt.Errorf("get status: %w", err)
	}
	if !status.Running {
		// Clean up stale files
		cleanupConnectorFiles(name)
		return fmt.Errorf("connector %q is not running (stale PID file cleaned)", name)
	}

	process, err := osFindProcess(status.PID)
	if err != nil {
		return fmt.Errorf("find process: %w", err)
	}

	// Send SIGTERM for graceful shutdown
	if err := process.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("send SIGTERM: %w", err)
	}

	// Wait briefly for process to exit
	done := make(chan struct{})
	go func() {
		process.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Process exited gracefully
	case <-time.After(5 * time.Second):
		// Force kill
		process.Signal(syscall.SIGKILL)
		<-done
	}

	cleanupConnectorFiles(name)
	return nil
}

// GetConnectorStatus reads the status of a connector by name.
func GetConnectorStatus(name string) (*ConnectorStatus, error) {
	pidPath := paths.PIDPath(name)
	data, err := os.ReadFile(pidPath)
	if err != nil {
		return nil, fmt.Errorf("read PID file: %w", err)
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return nil, fmt.Errorf("parse PID: %w", err)
	}

	status := &ConnectorStatus{
		Name:    name,
		PID:     pid,
		Socket:  paths.SocketPath(name),
		Running: isProcessRunning(pid),
	}

	// Try to read metadata for StartedAt
	metaPath := filepath.Join(paths.BaseDir(), "pids", name+".json")
	if metaData, err := os.ReadFile(metaPath); err == nil {
		var meta ConnectorStatus
		if json.Unmarshal(metaData, &meta) == nil {
			status.StartedAt = meta.StartedAt
		}
	}

	return status, nil
}

// ListConnectors returns the status of all connectors with PID files.
func ListConnectors() ([]ConnectorStatus, error) {
	pidsDir := filepath.Join(paths.BaseDir(), "pids")
	entries, err := os.ReadDir(pidsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var statuses []ConnectorStatus
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".pid") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".pid")
		status, err := GetConnectorStatus(name)
		if err != nil {
			continue
		}
		statuses = append(statuses, *status)
	}
	return statuses, nil
}

// AttachConnector connects to a running connector's Unix Domain Socket.
// It first tries the PID-based status check (background mode), then falls back
// to directly connecting to the socket path (foreground mode has no PID file).
func AttachConnector(name string) (net.Conn, error) {
	// Try PID-based status check first (background daemon mode)
	status, err := GetConnectorStatus(name)
	if err == nil && status.Running {
		conn, err := netDial("unix", status.Socket)
		if err != nil {
			return nil, fmt.Errorf("connect to socket: %w", err)
		}
		return conn, nil
	}

	// Fallback: try connecting to socket directly (foreground mode — no PID file)
	socketPath := paths.SocketPath(name)
	conn, err := netDial("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("connector %q is not running (no PID file and socket unreachable)", name)
	}
	return conn, nil
}

// isProcessRunning checks if a process with the given PID is alive.
func isProcessRunning(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// On Unix, FindProcess always succeeds. Send signal 0 to check.
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

// cleanupConnectorFiles removes PID, metadata, and socket files for a connector.
func cleanupConnectorFiles(name string) {
	os.Remove(paths.PIDPath(name))
	os.Remove(filepath.Join(paths.BaseDir(), "pids", name+".json"))
	os.Remove(paths.SocketPath(name))
}
