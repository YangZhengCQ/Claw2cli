package executor

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/YangZhengCQ/Claw2cli/internal/parser"
	"github.com/YangZhengCQ/Claw2cli/internal/paths"
	"github.com/YangZhengCQ/Claw2cli/internal/protocol"
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

	// Redirect daemon stdout/stderr to log file (with size-based rotation)
	logPath := filepath.Join(paths.BaseDir(), "logs", manifest.Name+".log")
	if err := os.MkdirAll(filepath.Dir(logPath), 0700); err != nil {
		return fmt.Errorf("create log directory: %w", err)
	}
	rotateLogIfNeeded(logPath)
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		logFile.Close()
		return fmt.Errorf("start daemon: %w", err)
	}

	// cleanup kills the child process if post-start operations fail
	cleanup := func() {
		cmd.Process.Kill()
		cmd.Process.Wait()
		cleanupConnectorFiles(manifest.Name)
	}

	// Write PID file
	pidPath := paths.PIDPath(manifest.Name)
	pidData := fmt.Sprintf("%d\n", cmd.Process.Pid)
	if err := os.WriteFile(pidPath, []byte(pidData), 0600); err != nil {
		cleanup()
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
	metaData, err := json.Marshal(meta)
	if err != nil {
		cleanup()
		return fmt.Errorf("marshal metadata: %w", err)
	}
	if err := os.WriteFile(metaPath, metaData, 0600); err != nil {
		cleanup()
		return fmt.Errorf("write metadata: %w", err)
	}

	// Detach — don't wait for the child
	cmd.Process.Release()
	logFile.Close()

	// Wait for daemon to become ready
	if err := waitForReady(manifest.Name, 10*time.Second); err != nil {
		// Daemon started but didn't become ready — kill it
		if pidData, e := os.ReadFile(pidPath); e == nil {
			if pid, e := strconv.Atoi(strings.TrimSpace(string(pidData))); e == nil {
				if p, e := os.FindProcess(pid); e == nil {
					p.Signal(syscall.SIGKILL)
				}
			}
		}
		cleanupConnectorFiles(manifest.Name)
		return fmt.Errorf("daemon failed to start: %w", err)
	}

	return nil
}

// waitForReady polls the daemon's UDS socket until it responds to a ping.
func waitForReady(name string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	backoff := 50 * time.Millisecond

	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("unix", paths.SocketPath(name), 2*time.Second)
		if err != nil {
			time.Sleep(backoff)
			if backoff < 2*time.Second {
				backoff *= 2
			}
			continue
		}

		// Send ping
		msg := protocol.NewPing("c2c", "ready-check")
		data, _ := json.Marshal(msg)
		conn.Write(append(data, '\n'))
		conn.SetReadDeadline(time.Now().Add(3 * time.Second))

		scanner := bufio.NewScanner(conn)
		if scanner.Scan() {
			var resp protocol.Message
			if json.Unmarshal(scanner.Bytes(), &resp) == nil && resp.Type == protocol.TypePong {
				conn.Close()
				return nil
			}
		}
		conn.Close()
		time.Sleep(backoff)
		if backoff < 2*time.Second {
			backoff *= 2
		}
	}
	return fmt.Errorf("daemon did not become ready within %s", timeout)
}

// StopConnector tries graceful shutdown via socket first, then falls back to PID-based SIGTERM.
func StopConnector(name string) error {
	// Try graceful shutdown via socket first (preferred)
	socketPath := paths.SocketPath(name)
	conn, err := net.DialTimeout("unix", socketPath, 2*time.Second)
	if err == nil {
		defer conn.Close()
		msg := protocol.NewCommand("c2c", "shutdown", "stop-req", nil)
		data, _ := json.Marshal(msg)
		conn.Write(append(data, '\n'))
		// Wait for socket to close (daemon exited)
		conn.SetReadDeadline(time.Now().Add(7 * time.Second))
		buf := make([]byte, 1)
		conn.Read(buf) // blocks until close or deadline
		cleanupConnectorFiles(name)
		return nil
	}

	// Fallback: PID-based stop (stale daemon, socket already gone)
	status, err := GetConnectorStatus(name)
	if err != nil {
		return fmt.Errorf("get status: %w", err)
	}
	if !status.Running {
		cleanupConnectorFiles(name)
		return fmt.Errorf("connector %q is not running (stale PID file cleaned)", name)
	}

	process, err := osFindProcess(status.PID)
	if err != nil {
		return fmt.Errorf("find process: %w", err)
	}

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
	case <-time.After(5 * time.Second):
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
			log.Printf("skipping connector %q: %v", name, err)
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

// maxLogSize is the maximum log file size before rotation (10 MB).
const maxLogSize = 10 * 1024 * 1024

// rotateLogIfNeeded rotates the log file if it exceeds maxLogSize.
// Keeps one previous log as <name>.log.1.
func rotateLogIfNeeded(logPath string) {
	info, err := os.Stat(logPath)
	if err != nil || info.Size() < maxLogSize {
		return
	}
	prevPath := logPath + ".1"
	os.Remove(prevPath)
	os.Rename(logPath, prevPath)
}

// cleanupConnectorFiles removes PID, metadata, and socket files for a connector.
func cleanupConnectorFiles(name string) {
	os.Remove(paths.PIDPath(name))
	os.Remove(filepath.Join(paths.BaseDir(), "pids", name+".json"))
	os.Remove(paths.SocketPath(name))
}
