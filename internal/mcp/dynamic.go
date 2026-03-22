package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"time"

	"github.com/user/claw2cli/internal/protocol"
)

// DiscoverTools queries a running connector via UDS for its discovered tool schemas.
func DiscoverTools(connectorName string) ([]protocol.ToolSchema, error) {
	conn, err := attachConnectorFn(connectorName)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	reqID := fmt.Sprintf("mcp-discover-%d", time.Now().UnixNano())
	msg := protocol.NewCommand("c2c-mcp", "list_tools", reqID, nil)
	data, _ := json.Marshal(msg)
	conn.Write(append(data, '\n'))

	resultCh := make(chan []protocol.ToolSchema, 1)
	go func() {
		scanner := bufio.NewScanner(conn)
		scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
		for scanner.Scan() {
			var resp protocol.Message
			if json.Unmarshal(scanner.Bytes(), &resp) != nil {
				continue
			}
			if resp.ID == reqID && resp.Type == protocol.TypeResponse {
				var dp protocol.DiscoveryPayload
				if json.Unmarshal(resp.Payload, &dp) == nil {
					resultCh <- dp.Tools
				}
				return
			}
		}
	}()

	select {
	case tools := <-resultCh:
		return tools, nil
	case <-time.After(5 * time.Second):
		return nil, fmt.Errorf("timeout querying tools for %s", connectorName)
	}
}

// InvokeTool sends an invoke_tool command to a connector via UDS and waits for the result.
func InvokeTool(connectorName, toolName string, args json.RawMessage) (json.RawMessage, error) {
	conn, err := attachConnectorFn(connectorName)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	reqID := fmt.Sprintf("mcp-invoke-%d", time.Now().UnixNano())
	payload, _ := json.Marshal(map[string]interface{}{
		"tool": toolName,
		"args": args,
	})
	msg := protocol.NewCommand("c2c-mcp", "invoke_tool", reqID, payload)
	data, _ := json.Marshal(msg)
	conn.Write(append(data, '\n'))

	type result struct {
		payload json.RawMessage
		err     error
	}
	resultCh := make(chan result, 1)

	go func() {
		scanner := bufio.NewScanner(conn)
		scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
		for scanner.Scan() {
			var resp protocol.Message
			if json.Unmarshal(scanner.Bytes(), &resp) != nil {
				continue
			}
			if resp.ID != reqID {
				continue
			}
			if resp.Type == protocol.TypeResponse {
				resultCh <- result{payload: resp.Payload}
				return
			}
			if resp.Type == protocol.TypeError {
				resultCh <- result{err: fmt.Errorf("[%s] %s", resp.Code, resp.MessageStr)}
				return
			}
		}
		resultCh <- result{err: fmt.Errorf("connection closed")}
	}()

	select {
	case r := <-resultCh:
		return r.payload, r.err
	case <-time.After(30 * time.Second):
		return nil, fmt.Errorf("timeout invoking %s on %s", toolName, connectorName)
	}
}
