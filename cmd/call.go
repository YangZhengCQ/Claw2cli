package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/YangZhengCQ/Claw2cli/internal/executor"
	"github.com/YangZhengCQ/Claw2cli/internal/protocol"
)

var listTools bool
var callTimeout int

var callCmd = &cobra.Command{
	Use:   "call <connector> [tool-name] [json-args]",
	Short: "Invoke a discovered tool on a running connector",
	Long: `Generic RPC client for invoking tools discovered via capability discovery.

Examples:
  c2c call wechat --list-tools
  c2c call wechat wechat_send_text '{"to":"wxid_123@im.wechat","text":"hello"}'
  c2c call wechat wechat_send_media '{"to":"wxid_123@im.wechat","media":"/tmp/img.png"}'`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		conn, err := executor.AttachConnector(name)
		if err != nil {
			return err
		}
		defer conn.Close()

		reqID := fmt.Sprintf("call-%d", time.Now().UnixNano())

		if listTools {
			return queryListTools(conn, name, reqID)
		}

		if len(args) < 2 {
			return fmt.Errorf("usage: c2c call <connector> <tool-name> [json-args]\n       c2c call <connector> --list-tools")
		}

		toolName := args[1]
		var toolArgs json.RawMessage
		if len(args) >= 3 {
			toolArgs = json.RawMessage(args[2])
			// Validate JSON
			if !json.Valid(toolArgs) {
				return fmt.Errorf("invalid JSON args: %s", args[2])
			}
		} else {
			toolArgs = json.RawMessage(`{}`)
		}

		return invokeToolCall(conn, name, reqID, toolName, toolArgs)
	},
}

func init() {
	callCmd.Flags().BoolVar(&listTools, "list-tools", false, "List available tools for the connector")
	callCmd.Flags().IntVar(&callTimeout, "timeout", 30, "Timeout in seconds")
}

func queryListTools(conn net.Conn, name, reqID string) error {
	// Send list_tools command
	msg := protocol.NewCommand("c2c-call", "list_tools", reqID, nil)
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal command: %w", err)
	}
	if _, err := conn.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write command: %w", err)
	}

	return readResponse(conn, reqID, func(payload json.RawMessage) error {
		var dp protocol.DiscoveryPayload
		if err := json.Unmarshal(payload, &dp); err != nil {
			fmt.Println(string(payload))
			return nil
		}
		if len(dp.Tools) == 0 {
			fmt.Println("No tools discovered. Is the connector running?")
			return nil
		}
		fmt.Printf("Discovered %d tool(s) for %q:\n\n", len(dp.Tools), name)
		for _, t := range dp.Tools {
			fmt.Printf("  %s\n", t.Name)
			fmt.Printf("    %s\n", t.Description)
			if len(t.InputSchema) > 0 {
				var schema map[string]interface{}
				if json.Unmarshal(t.InputSchema, &schema) == nil {
					if props, ok := schema["properties"].(map[string]interface{}); ok {
						for pname, pval := range props {
							desc := ""
							if pm, ok := pval.(map[string]interface{}); ok {
								if d, ok := pm["description"].(string); ok {
									desc = " — " + d
								}
							}
							fmt.Printf("      --%s%s\n", pname, desc)
						}
					}
				}
			}
			fmt.Println()
		}
		return nil
	})
}

func invokeToolCall(conn net.Conn, name, reqID, toolName string, toolArgs json.RawMessage) error {
	payload, err := json.Marshal(map[string]interface{}{
		"tool": toolName,
		"args": json.RawMessage(toolArgs),
	})
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}
	msg := protocol.NewCommand("c2c-call", "invoke_tool", reqID, payload)
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal command: %w", err)
	}
	if _, err := conn.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write command: %w", err)
	}

	return readResponse(conn, reqID, func(payload json.RawMessage) error {
		// Pretty-print the result
		var pretty map[string]interface{}
		if json.Unmarshal(payload, &pretty) == nil {
			out, _ := json.MarshalIndent(pretty, "", "  ")
			fmt.Println(string(out))
		} else {
			fmt.Println(string(payload))
		}
		return nil
	})
}

func readResponse(conn net.Conn, reqID string, onResult func(json.RawMessage) error) error {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	resultCh := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(conn)
		scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
		for scanner.Scan() {
			var msg protocol.Message
			if json.Unmarshal(scanner.Bytes(), &msg) != nil {
				continue
			}
			if msg.ID != reqID {
				continue
			}
			if msg.Type == protocol.TypeResponse {
				resultCh <- onResult(msg.Payload)
				return
			}
			if msg.Type == protocol.TypeError {
				resultCh <- fmt.Errorf("[%s] %s", msg.Code, msg.MessageStr)
				return
			}
		}
		if err := scanner.Err(); err != nil {
			resultCh <- fmt.Errorf("read error: %w", err)
			return
		}
		resultCh <- fmt.Errorf("connection closed before response")
	}()

	select {
	case err := <-resultCh:
		return err
	case <-sigCh:
		return fmt.Errorf("interrupted")
	case <-time.After(time.Duration(callTimeout) * time.Second):
		return fmt.Errorf("timeout after %ds", callTimeout)
	}
}
