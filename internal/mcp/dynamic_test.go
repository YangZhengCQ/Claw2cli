package mcp

import (
	"encoding/json"
	"fmt"
	"net"
	"testing"

	"github.com/YangZhengCQ/Claw2cli/internal/protocol"
)

func TestDiscoverTools_Success(t *testing.T) {
	orig := attachConnectorFn
	defer func() { attachConnectorFn = orig }()

	serverConn, clientConn := net.Pipe()
	attachConnectorFn = func(name string) (net.Conn, error) {
		return clientConn, nil
	}

	// Server side: read command, write discovery response
	go func() {
		buf := make([]byte, 65536)
		n, _ := serverConn.Read(buf)
		var msg protocol.Message
		json.Unmarshal(buf[:n], &msg)

		tools := []protocol.ToolSchema{
			{Name: "test_tool", Description: "A test tool", InputSchema: json.RawMessage(`{}`)},
		}
		payload, _ := json.Marshal(protocol.DiscoveryPayload{Tools: tools})
		resp := protocol.NewResponse("test", msg.ID, payload)
		data, _ := json.Marshal(resp)
		serverConn.Write(append(data, '\n'))
		serverConn.Close()
	}()

	tools, err := DiscoverTools("test-connector")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	if tools[0].Name != "test_tool" {
		t.Errorf("expected tool name 'test_tool', got %q", tools[0].Name)
	}
}

func TestDiscoverTools_Timeout(t *testing.T) {
	orig := attachConnectorFn
	defer func() { attachConnectorFn = orig }()

	_, clientConn := net.Pipe()
	attachConnectorFn = func(name string) (net.Conn, error) {
		return clientConn, nil
	}

	// Don't write anything — should timeout
	// Note: this test may be slow due to the 5s timeout
	// We close the connection to make it fast
	go func() {
		// Close after a brief delay to trigger "connection closed" faster than timeout
		clientConn.Close()
	}()

	_, err := DiscoverTools("test-connector")
	if err == nil {
		t.Fatal("expected error (timeout or connection closed)")
	}
}

func TestDiscoverTools_DialError(t *testing.T) {
	orig := attachConnectorFn
	defer func() { attachConnectorFn = orig }()

	attachConnectorFn = func(name string) (net.Conn, error) {
		return nil, fmt.Errorf("connection refused")
	}

	_, err := DiscoverTools("test-connector")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestInvokeTool_Success(t *testing.T) {
	orig := attachConnectorFn
	defer func() { attachConnectorFn = orig }()

	serverConn, clientConn := net.Pipe()
	attachConnectorFn = func(name string) (net.Conn, error) {
		return clientConn, nil
	}

	// Server side: read command, write response
	go func() {
		buf := make([]byte, 65536)
		n, _ := serverConn.Read(buf)
		var msg protocol.Message
		json.Unmarshal(buf[:n], &msg)

		resp := protocol.NewResponse("test", msg.ID, json.RawMessage(`{"result":"ok"}`))
		data, _ := json.Marshal(resp)
		serverConn.Write(append(data, '\n'))
		serverConn.Close()
	}()

	result, err := InvokeTool("test-connector", "test_tool", json.RawMessage(`{"key":"value"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(result) != `{"result":"ok"}` {
		t.Errorf("unexpected result: %s", string(result))
	}
}

func TestInvokeTool_ErrorResponse(t *testing.T) {
	orig := attachConnectorFn
	defer func() { attachConnectorFn = orig }()

	serverConn, clientConn := net.Pipe()
	attachConnectorFn = func(name string) (net.Conn, error) {
		return clientConn, nil
	}

	// Server side: read command, write error response
	go func() {
		buf := make([]byte, 65536)
		n, _ := serverConn.Read(buf)
		var msg protocol.Message
		json.Unmarshal(buf[:n], &msg)

		resp := protocol.NewError("test", "TOOL_ERROR", "tool failed")
		resp.ID = msg.ID
		data, _ := json.Marshal(resp)
		serverConn.Write(append(data, '\n'))
		serverConn.Close()
	}()

	_, err := InvokeTool("test-connector", "test_tool", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "[TOOL_ERROR] tool failed" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestInvokeTool_DialError(t *testing.T) {
	orig := attachConnectorFn
	defer func() { attachConnectorFn = orig }()

	attachConnectorFn = func(name string) (net.Conn, error) {
		return nil, fmt.Errorf("connection refused")
	}

	_, err := InvokeTool("test-connector", "test_tool", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error")
	}
}
