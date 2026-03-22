package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/user/claw2cli/internal/parser"
	"github.com/user/claw2cli/internal/paths"
	"github.com/user/claw2cli/internal/protocol"
)

// daemonCmd is a hidden command used internally by StartConnector.
// It runs the actual plugin subprocess via the Node.js shim and manages the UDS listener.
var daemonCmd = &cobra.Command{
	Use:    "_daemon <connector>",
	Hidden: true,
	Args:   cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		return runDaemon(name)
	},
}

func init() {
	rootCmd.AddCommand(daemonCmd)
}

// shimDir returns the path to the shim directory bundled with the c2c binary.
// In development, it's relative to the source tree; in production, it's next to the binary.
func shimDir() string {
	self, err := os.Executable()
	if err != nil {
		return "shim"
	}
	// Check if shim/ exists next to the binary
	dir := filepath.Join(filepath.Dir(self), "shim")
	if _, err := os.Stat(dir); err == nil {
		return dir
	}
	// Fallback: look in the current working directory
	if _, err := os.Stat("shim"); err == nil {
		return "shim"
	}
	return dir
}

func runDaemon(name string) error {
	manifest, err := parser.LoadPlugin(name)
	if err != nil {
		return fmt.Errorf("load plugin: %w", err)
	}

	// Resolve shim path
	shim := shimDir()
	shimEntry := filepath.Join(shim, "c2c-shim.js")
	if _, err := os.Stat(shimEntry); os.IsNotExist(err) {
		return fmt.Errorf("shim not found at %s — is c2c installed correctly?", shimEntry)
	}

	// Build NODE_PATH to include both:
	// 1. Our fake @openclaw/plugin-sdk (shim/node_modules)
	// 2. Globally installed npm packages (so the actual plugin can be resolved)
	shimNodeModules := filepath.Join(shim, "node_modules")
	globalNodeModules := resolveGlobalNodeModules()

	nodePath := shimNodeModules
	if globalNodeModules != "" {
		nodePath = shimNodeModules + ":" + globalNodeModules
	}

	// Also ensure the actual plugin package is available
	// Run npx to ensure it's cached/installed
	ensurePluginInstalled(manifest.Source)

	// Start Node.js shim as subprocess
	pluginCmd := exec.Command("node", shimEntry, name)
	pluginCmd.Env = append(os.Environ(),
		"C2C_PLUGIN_NAME="+name,
		"C2C_PLUGIN_TYPE="+string(manifest.Type),
		"C2C_STORAGE_DIR="+paths.StorageDir(name),
		"C2C_BASE_DIR="+paths.BaseDir(),
		"C2C_PLUGIN_SOURCE="+manifest.Source,
		"NODE_PATH="+nodePath,
	)

	pluginStdout, err := pluginCmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	pluginStderr, err := pluginCmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("stderr pipe: %w", err)
	}
	pluginStdin, err := pluginCmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}

	if err := pluginCmd.Start(); err != nil {
		return fmt.Errorf("start shim: %w", err)
	}

	// Set up UDS listener
	socketPath := paths.SocketPath(name)
	os.Remove(socketPath) // Clean up stale socket
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		pluginCmd.Process.Kill()
		return fmt.Errorf("listen on socket: %w", err)
	}
	defer listener.Close()
	defer os.Remove(socketPath)

	// Client connections
	var clients sync.Map

	// Broadcast a protocol message to all connected UDS clients
	broadcast := func(msg *protocol.Message) {
		data, err := json.Marshal(msg)
		if err != nil {
			return
		}
		data = append(data, '\n')
		clients.Range(func(key, value interface{}) bool {
			conn := value.(net.Conn)
			conn.Write(data)
			return true
		})
	}

	// Read shim stdout (NDJSON from plugin-sdk shim) and broadcast to UDS clients
	go func() {
		scanner := bufio.NewScanner(pluginStdout)
		scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB buffer for large messages
		for scanner.Scan() {
			line := scanner.Bytes()

			// Parse the NDJSON message from the shim
			var msg protocol.Message
			if err := json.Unmarshal(line, &msg); err != nil {
				// Not valid NDJSON — wrap as log
				broadcast(protocol.NewLog(name, "info", string(line)))
				continue
			}

			// Ensure source is set
			if msg.Source == "" {
				msg.Source = name
			}

			broadcast(&msg)
		}
	}()

	// Read shim stderr and broadcast as error logs
	go func() {
		scanner := bufio.NewScanner(pluginStderr)
		for scanner.Scan() {
			broadcast(protocol.NewLog(name, "error", scanner.Text()))
		}
	}()

	// Accept UDS client connections
	go func() {
		clientID := 0
		for {
			conn, err := listener.Accept()
			if err != nil {
				return // listener closed
			}
			clientID++
			id := clientID
			clients.Store(id, conn)

			// Handle incoming commands from UDS clients → forward to shim stdin
			go func() {
				defer func() {
					clients.Delete(id)
					conn.Close()
				}()
				scanner := bufio.NewScanner(conn)
				for scanner.Scan() {
					line := scanner.Bytes()
					var msg protocol.Message
					if err := json.Unmarshal(line, &msg); err != nil {
						continue
					}
					if msg.Type == protocol.TypeCommand || msg.Type == protocol.TypeResponse {
						// Forward to shim's stdin (which routes to the plugin)
						pluginStdin.Write(line)
						pluginStdin.Write([]byte("\n"))
					}
				}
			}()
		}
	}()

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	// Wait for either signal or plugin exit
	doneCh := make(chan error, 1)
	go func() {
		doneCh <- pluginCmd.Wait()
	}()

	select {
	case sig := <-sigCh:
		log.Printf("received signal %v, shutting down", sig)
		pluginCmd.Process.Signal(syscall.SIGTERM)
		<-doneCh
	case err := <-doneCh:
		if err != nil {
			log.Printf("shim exited with error: %v", err)
		}
	}

	// Close all client connections
	clients.Range(func(key, value interface{}) bool {
		value.(net.Conn).Close()
		return true
	})

	_ = pluginStdin.Close()
	_, _ = io.Copy(io.Discard, pluginStdout)

	return nil
}

// resolveGlobalNodeModules finds the global npm node_modules directory.
func resolveGlobalNodeModules() string {
	out, err := exec.Command("npm", "root", "-g").Output()
	if err != nil {
		return ""
	}
	dir := string(out)
	// Trim newline
	if len(dir) > 0 && dir[len(dir)-1] == '\n' {
		dir = dir[:len(dir)-1]
	}
	return dir
}

// ensurePluginInstalled makes sure the npm plugin package is available globally.
func ensurePluginInstalled(source string) {
	// Use npm list to check if installed
	cmd := exec.Command("npm", "list", "-g", source, "--depth=0")
	if err := cmd.Run(); err != nil {
		// Not installed — install it
		log.Printf("Installing plugin package: %s", source)
		install := exec.Command("npm", "install", "-g", source)
		install.Stdout = os.Stderr
		install.Stderr = os.Stderr
		install.Run()
	}
}
