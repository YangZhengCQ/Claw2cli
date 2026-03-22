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
	"strings"
	"sync"
	"syscall"
	"time"

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

// isForeground is true when runDaemon is called directly from `connect` (not via _daemon).
// In foreground mode, shim stderr goes directly to the terminal for QR codes etc.
var isForeground bool

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

	// Resolve the Node.js runner: prefer tsx (for ESM + TypeScript plugins), fallback to node
	nodeRunner := resolveNodeRunner()

	// Start shim as subprocess
	pluginCmd := exec.Command(nodeRunner, shimEntry, name)
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
	// In foreground mode, let stderr go directly to terminal (QR codes, prompts)
	var pluginStderr io.ReadCloser
	if isForeground {
		pluginCmd.Stderr = os.Stderr
	} else {
		pluginStderr, err = pluginCmd.StderrPipe()
		if err != nil {
			return fmt.Errorf("stderr pipe: %w", err)
		}
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
				// Not valid NDJSON — raw output (QR codes, etc.)
				if isForeground {
					fmt.Fprintln(os.Stderr, string(line))
				}
				broadcast(protocol.NewLog(name, "info", string(line)))
				continue
			}

			// In foreground mode, print messages to terminal
			if isForeground {
				switch msg.Type {
				case protocol.TypeLog:
					fmt.Fprintf(os.Stderr, "[%s] %s\n", msg.Level, msg.MessageStr)
				case protocol.TypeEvent:
					if msg.Topic == "message.received" {
						var p struct {
							From string `json:"from"`
							Body string `json:"body"`
						}
						if json.Unmarshal(msg.Payload, &p) == nil {
							fmt.Fprintf(os.Stderr, "\n💬 %s: %s\n", p.From, p.Body)
						}
					} else {
						fmt.Fprintf(os.Stderr, "[event] %s\n", msg.Topic)
					}
				case protocol.TypeError:
					fmt.Fprintf(os.Stderr, "[error] %s: %s\n", msg.Code, msg.MessageStr)
				}
			}

			// Ensure source is set
			if msg.Source == "" {
				msg.Source = name
			}

			broadcast(&msg)
		}
	}()

	// Read shim stderr and broadcast as error logs (background mode only;
	// in foreground mode stderr goes directly to the terminal)
	if pluginStderr != nil {
		go func() {
			scanner := bufio.NewScanner(pluginStderr)
			for scanner.Scan() {
				broadcast(protocol.NewLog(name, "error", scanner.Text()))
			}
		}()
	}

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
		// Wait up to 3 seconds for graceful exit, then force kill
		select {
		case <-doneCh:
		case <-time.After(3 * time.Second):
			log.Printf("shim did not exit in time, force killing")
			pluginCmd.Process.Kill()
			<-doneCh
		}
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

// resolveNodeRunner returns "tsx" if available globally, otherwise "node".
// tsx is needed because OpenClaw plugins are ESM + TypeScript.
func resolveNodeRunner() string {
	// Check if tsx is installed globally
	if _, err := exec.LookPath("tsx"); err == nil {
		return "tsx"
	}
	// Try to install tsx globally
	log.Printf("tsx not found, installing globally for TypeScript plugin support...")
	install := exec.Command("npm", "install", "-g", "tsx")
	install.Stdout = os.Stderr
	install.Stderr = os.Stderr
	if err := install.Run(); err == nil {
		if tsxPath, err := exec.LookPath("tsx"); err == nil {
			return tsxPath
		}
	}
	log.Printf("Warning: tsx not available, falling back to node (TypeScript plugins may not load)")
	return "node"
}

// resolvePluginPackage derives the actual plugin package name from the source.
// CLI wrapper packages like "@tencent-weixin/openclaw-weixin-cli" need the
// runtime package "@tencent-weixin/openclaw-weixin" to be installed instead.
func resolvePluginPackage(source string) string {
	pkg := source
	// Strip version suffix: "@scope/name@version" -> "@scope/name"
	if strings.HasPrefix(pkg, "@") {
		if idx := strings.LastIndex(pkg, "@"); idx > 0 {
			pkg = pkg[:idx]
		}
	} else if idx := strings.Index(pkg, "@"); idx > 0 {
		pkg = pkg[:idx]
	}
	// Strip -cli suffix: the CLI package is just an installer,
	// the actual plugin is the package without -cli
	pkg = strings.TrimSuffix(pkg, "-cli")
	return pkg
}

// ensurePluginInstalled makes sure the npm plugin package is available globally.
// It installs both the source package (CLI wrapper) and the actual runtime plugin.
func ensurePluginInstalled(source string) {
	pkgs := []string{source}

	// If source is a CLI wrapper (e.g. *-cli), also install the runtime plugin
	runtimePkg := resolvePluginPackage(source)
	if runtimePkg != "" && runtimePkg != strings.TrimSuffix(source, "@latest") {
		pkgs = append(pkgs, runtimePkg)
	}

	for _, pkg := range pkgs {
		cmd := exec.Command("npm", "list", "-g", pkg, "--depth=0")
		if err := cmd.Run(); err != nil {
			log.Printf("Installing plugin package: %s", pkg)
			install := exec.Command("npm", "install", "-g", pkg)
			install.Stdout = os.Stderr
			install.Stderr = os.Stderr
			install.Run()
		}
	}
}
