package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/YangZhengCQ/Claw2cli/internal/protocol"
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
	data, err := json.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("marshal discover request: %w", err)
	}
	if _, err := conn.Write(append(data, '\n')); err != nil {
		return nil, fmt.Errorf("write discover request: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Set read deadline so the scanner goroutine unblocks promptly on timeout
	// instead of leaking until conn.Close() fires.
	if tc, ok := conn.(interface{ SetReadDeadline(time.Time) error }); ok {
		tc.SetReadDeadline(time.Now().Add(6 * time.Second)) // slightly longer than context
	}

	type discoverResult struct {
		tools []protocol.ToolSchema
		err   error
	}
	resultCh := make(chan discoverResult, 1)
	go func() {
		scanner := bufio.NewScanner(conn)
		scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
		for scanner.Scan() {
			select {
			case <-ctx.Done():
				return
			default:
			}
			var resp protocol.Message
			if json.Unmarshal(scanner.Bytes(), &resp) != nil {
				continue
			}
			if resp.ID == reqID && resp.Type == protocol.TypeError {
				resultCh <- discoverResult{err: fmt.Errorf("[%s] %s", resp.Code, resp.MessageStr)}
				return
			}
			if resp.ID == reqID && resp.Type == protocol.TypeResponse {
				var dp protocol.DiscoveryPayload
				if json.Unmarshal(resp.Payload, &dp) == nil {
					resultCh <- discoverResult{tools: dp.Tools}
				}
				return
			}
		}
	}()

	select {
	case r := <-resultCh:
		return r.tools, r.err
	case <-ctx.Done():
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
	payload, err := json.Marshal(map[string]interface{}{
		"tool": toolName,
		"args": args,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal invoke payload: %w", err)
	}
	msg := protocol.NewCommand("c2c-mcp", "invoke_tool", reqID, payload)
	data, err := json.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("marshal invoke request: %w", err)
	}
	if _, err := conn.Write(append(data, '\n')); err != nil {
		return nil, fmt.Errorf("write invoke request: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Set read deadline so the scanner goroutine unblocks promptly on timeout
	if tc, ok := conn.(interface{ SetReadDeadline(time.Time) error }); ok {
		tc.SetReadDeadline(time.Now().Add(31 * time.Second))
	}

	type result struct {
		payload json.RawMessage
		err     error
	}
	resultCh := make(chan result, 1)

	go func() {
		scanner := bufio.NewScanner(conn)
		scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
		for scanner.Scan() {
			select {
			case <-ctx.Done():
				return
			default:
			}
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
		if err := scanner.Err(); err != nil {
			resultCh <- result{err: fmt.Errorf("read error: %w", err)}
		} else {
			resultCh <- result{err: fmt.Errorf("connection closed")}
		}
	}()

	select {
	case r := <-resultCh:
		return r.payload, r.err
	case <-ctx.Done():
		return nil, fmt.Errorf("timeout invoking %s on %s", toolName, connectorName)
	}
}
