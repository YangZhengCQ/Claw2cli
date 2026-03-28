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
	"time"

	"github.com/spf13/cobra"
	"github.com/YangZhengCQ/Claw2cli/internal/executor"
	"github.com/YangZhengCQ/Claw2cli/internal/nodeutil"
	"github.com/YangZhengCQ/Claw2cli/internal/parser"
	"github.com/YangZhengCQ/Claw2cli/internal/paths"
	"github.com/YangZhengCQ/Claw2cli/internal/protocol"
	"github.com/YangZhengCQ/Claw2cli/internal/registry"
	"github.com/YangZhengCQ/Claw2cli/internal/sandbox"
	"github.com/YangZhengCQ/Claw2cli/internal/store"
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

// shimDir delegates to paths.ShimDir for shim directory resolution.
func shimDir() string {
	return paths.ShimDir()
}

// Shutdown timeout for the shim subprocess after SIGTERM.
const shimShutdownTimeout = 9 * time.Second

// isForeground is true when runDaemon is called directly from `connect` (not via _daemon).
var isForeground bool

// noSandbox disables OS-level sandboxing when true (set via --no-sandbox flag).
var noSandbox bool


func runDaemon(name string) error {
	manifest, err := parser.LoadPlugin(name)
	if err != nil {
		return fmt.Errorf("load plugin: %w", err)
	}

	// Verify package integrity before execution
	if !skipVerify {
		if err := nodeutil.VerifyChecksum(manifest.Source, manifest.Checksum); err != nil {
			return fmt.Errorf("checksum verification: %w", err)
		}
	}

	// Resolve shim path
	shim := shimDir()
	shimEntry := filepath.Join(shim, "c2c-shim.js")
	if _, err := os.Stat(shimEntry); os.IsNotExist(err) {
		return fmt.Errorf("shim not found at %s — is c2c installed correctly?", shimEntry)
	}

	// Build NODE_PATH: fake SDK (shim) + local plugin packages
	shimNodeModules := filepath.Join(shim, "node_modules")

	// Verify local packages exist (no network calls at connect-time)
	s := store.New(name)
	if !s.IsInstalled() {
		return fmt.Errorf("packages not installed for %q — run 'c2c install %s' first", name, name)
	}

	// Resolve the tsx runner
	nodeRunner := store.ResolveTsx()

	nodePath := shimNodeModules + ":" + s.NodeModulesPath()
	// Keep global fallback for migration
	globalNodeModules := nodeutil.ResolveGlobalNodeModules()
	if globalNodeModules != "" {
		nodePath += ":" + globalNodeModules
	}

	// Start shim as subprocess
	// Use BuildEnv to filter out sensitive environment variables (AWS keys, tokens, etc.)
	pluginCmd := exec.Command(nodeRunner, shimEntry, name)
	pluginCmd.Env = append(executor.BuildEnv(manifest),
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

	// Apply OS-level sandbox based on declared permissions
	sandboxPaths := sandbox.SandboxPaths{
		ShimDir:     shim,
		NodeModules: s.NodeModulesPath(),
		NodeRunner:  nodeRunner,
		StorageDir:  paths.StorageDir(name),
	}
	if !noSandbox {
		if err := sandbox.Apply(pluginCmd, manifest, sandboxPaths); err != nil {
			log.Printf("warning: sandbox not available: %v", err)
		}
	}

	if err := pluginCmd.Start(); err != nil {
		return fmt.Errorf("start shim: %w", err)
	}

	// Set up UDS listener
	socketPath := paths.SocketPath(name)
	os.Remove(socketPath) // Clean up stale socket
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		_ = pluginCmd.Process.Kill()
		return fmt.Errorf("listen on socket: %w", err)
	}
	defer listener.Close()
	// Restrict socket access to the current user only
	if err := os.Chmod(socketPath, 0600); err != nil {
		log.Printf("warning: could not set socket permissions: %v", err)
	}
	// Note: do NOT defer os.Remove(socketPath) here.
	// StopConnector.cleanupConnectorFiles handles socket cleanup.
	// A deferred remove here would race with a newly started daemon's socket.

	// Client connections
	var clients sync.Map

	// Broadcast a protocol message to all connected UDS clients
	broadcast := func(msg *protocol.Message) {
		data, err := json.Marshal(msg)
		if err != nil {
			log.Printf("broadcast: marshal error: %v", err)
			return
		}
		data = append(data, '\n')
		clients.Range(func(key, value interface{}) bool {
			conn, ok := value.(net.Conn)
			if !ok {
				clients.Delete(key)
				return true
			}
			conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if _, err := conn.Write(data); err != nil {
				// Client disconnected; remove it
				clients.Delete(key)
				conn.Close()
			}
			return true
		})
	}

	// Read shim stdout (NDJSON from plugin-sdk shim) and broadcast to UDS clients
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("panic in stdout reader: %v", r)
			}
		}()
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

			// Cache discovery messages in tool registry
			if msg.Type == protocol.TypeDiscovery {
				var dp protocol.DiscoveryPayload
				if err := json.Unmarshal(msg.Payload, &dp); err != nil {
					log.Printf("warning: failed to unmarshal discovery payload: %v", err)
				} else if len(dp.Tools) > 0 {
					existing := registry.Get(name)
					registry.Store(name, append(existing, dp.Tools...))
					if isForeground {
						fmt.Fprintf(os.Stderr, "[discovery] %d tool(s) registered\n", len(dp.Tools))
					}
				}
			}

			broadcast(&msg)
		}
		if err := scanner.Err(); err != nil {
			log.Printf("shim stdout scanner error: %v", err)
		}
	}()

	// Read shim stderr and broadcast as error logs (background mode only;
	// in foreground mode stderr goes directly to the terminal)
	if pluginStderr != nil {
		go func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("panic in stderr reader: %v", r)
				}
			}()
			scanner := bufio.NewScanner(pluginStderr)
			for scanner.Scan() {
				broadcast(protocol.NewLog(name, "error", scanner.Text()))
			}
			if err := scanner.Err(); err != nil {
				log.Printf("shim stderr scanner error: %v", err)
			}
		}()
	}

	// stdinMu protects concurrent writes to pluginStdin from multiple UDS clients
	var stdinMu sync.Mutex

	// Set up shutdown signal channel (must be declared before accept loop
	// so client handler goroutines can send shutdown signals)
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	// Accept UDS client connections
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("panic in accept loop: %v", r)
			}
		}()
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
					if r := recover(); r != nil {
						log.Printf("panic in client handler: %v", r)
					}
					clients.Delete(id)
					conn.Close()
				}()
				scanner := bufio.NewScanner(conn)
				scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
				for scanner.Scan() {
					line := scanner.Bytes()
					var msg protocol.Message
					if err := json.Unmarshal(line, &msg); err != nil {
						log.Printf("malformed message from client %d: %v", id, err)
						continue
					}
					// Daemon-level commands (handled before plugin forwarding)
					if msg.Type == protocol.TypePing {
						resp := protocol.NewPong(name, msg.ID)
						if data, err := json.Marshal(resp); err == nil {
							conn.Write(append(data, '\n'))
						}
						continue
					}
					if msg.Type == protocol.TypeShutdown || (msg.Type == protocol.TypeCommand && msg.Action == "shutdown") {
						log.Printf("shutdown requested by client %d", id)
						// Signal the main goroutine to shut down
						select {
						case sigCh <- syscall.SIGTERM:
						default:
						}
						continue
					}
					if msg.Type == protocol.TypeCommand && msg.Action == "list_tools" {
						// Handle list_tools locally from cached registry
						tools := registry.Get(name)
						payload, err := json.Marshal(protocol.DiscoveryPayload{Tools: tools})
						if err != nil {
							log.Printf("marshal discovery payload: %v", err)
							continue
						}
						resp := protocol.NewResponse(name, msg.ID, payload)
						data, err := json.Marshal(resp)
						if err != nil {
							log.Printf("marshal response: %v", err)
							continue
						}
						if _, err := conn.Write(append(data, '\n')); err != nil {
							log.Printf("write to client %d: %v", id, err)
							return
						}
					} else if msg.Type == protocol.TypeCommand || msg.Type == protocol.TypeResponse {
						// Forward to shim's stdin (which routes to the plugin)
						stdinMu.Lock()
						_, err1 := pluginStdin.Write(line)
						_, err2 := pluginStdin.Write([]byte("\n"))
						stdinMu.Unlock()
						if err1 != nil || err2 != nil {
							log.Printf("write to shim stdin failed: %v / %v", err1, err2)
							// Send error response back to the requesting client
							if msg.ID != "" {
								errResp := protocol.NewError(name, "FORWARD_FAILED", "failed to forward command to plugin")
								errResp.ID = msg.ID
								if errData, err := json.Marshal(errResp); err == nil {
									conn.Write(append(errData, '\n'))
								}
							}
						}
					}
				}
			}()
		}
	}()

	// Wait for either signal or plugin exit
	doneCh := make(chan error, 1)
	go func() {
		doneCh <- pluginCmd.Wait()
	}()

	select {
	case sig := <-sigCh:
		log.Printf("received signal %v, shutting down", sig)
		pluginCmd.Process.Signal(syscall.SIGTERM)
		// Wait for graceful exit, then force kill
		select {
		case <-doneCh:
		case <-time.After(shimShutdownTimeout):
			log.Printf("shim did not exit in time, force killing")
			pluginCmd.Process.Kill()
			<-doneCh
		}
	case err := <-doneCh:
		if err != nil {
			return fmt.Errorf("shim exited unexpectedly: %w", err)
		}
	}

	// Clean up sandbox temp files
	sandbox.Cleanup()

	// Evict discovered tools on shutdown
	registry.Delete(name)

	// Close all client connections
	clients.Range(func(key, value interface{}) bool {
		if conn, ok := value.(net.Conn); ok {
			conn.Close()
		}
		return true
	})

	_ = pluginStdin.Close()
	_, _ = io.Copy(io.Discard, pluginStdout)

	return nil
}
