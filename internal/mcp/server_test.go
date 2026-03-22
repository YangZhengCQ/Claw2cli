package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	gomcp "github.com/mark3labs/mcp-go/mcp"
	"github.com/user/claw2cli/internal/executor"
	"github.com/user/claw2cli/internal/parser"
)

// --- ManifestToTool tests ---

func TestManifestToTool_Skill(t *testing.T) {
	m := &parser.PluginManifest{
		Name:   "google-search",
		Type:   parser.PluginTypeSkill,
		Source: "@test/google-search@1.0.0",
		Skill: &parser.SkillMetadata{
			Name:        "google-search",
			Description: "Search the web",
		},
	}

	tool := ManifestToTool(m)
	if tool.Name != "google-search" {
		t.Errorf("name=%q, want google-search", tool.Name)
	}
	if tool.Description != "Search the web" {
		t.Errorf("description=%q, want 'Search the web'", tool.Description)
	}
}

func TestManifestToTool_Connector(t *testing.T) {
	m := &parser.PluginManifest{
		Name:   "wechat",
		Type:   parser.PluginTypeConnector,
		Source: "@tencent-weixin/openclaw-weixin-cli@latest",
	}

	tool := ManifestToTool(m)
	if tool.Name != "wechat" {
		t.Errorf("name=%q, want wechat", tool.Name)
	}
}

func TestManifestToTool_NoSkillMetadata(t *testing.T) {
	m := &parser.PluginManifest{
		Name:   "plain",
		Type:   parser.PluginTypeSkill,
		Source: "@test/plain@1.0.0",
	}
	tool := ManifestToTool(m)
	if tool.Name != "plain" {
		t.Errorf("name=%q, want plain", tool.Name)
	}
	// Description should fall back to source
	if tool.Description != "@test/plain@1.0.0" {
		t.Errorf("description=%q, want source as fallback", tool.Description)
	}
}

// --- FilterPlugins tests ---

func TestFilterPlugins_NoFilter(t *testing.T) {
	plugins := []*parser.PluginManifest{
		{Name: "a"},
		{Name: "b"},
	}
	result := FilterPlugins(plugins, nil, nil)
	if len(result) != 2 {
		t.Errorf("expected 2, got %d", len(result))
	}
}

func TestFilterPlugins_ByName(t *testing.T) {
	plugins := []*parser.PluginManifest{
		{Name: "a", Type: parser.PluginTypeSkill},
		{Name: "b", Type: parser.PluginTypeConnector},
		{Name: "c", Type: parser.PluginTypeSkill},
	}
	result := FilterPlugins(plugins, []string{"a"}, []string{"b"})
	if len(result) != 2 {
		t.Errorf("expected 2, got %d", len(result))
	}
}

func TestFilterPlugins_NoMatch(t *testing.T) {
	plugins := []*parser.PluginManifest{
		{Name: "a"},
	}
	result := FilterPlugins(plugins, []string{"x"}, nil)
	if len(result) != 0 {
		t.Errorf("expected 0, got %d", len(result))
	}
}

// --- makeHandler tests ---

func buildRequest(params map[string]interface{}) gomcp.CallToolRequest {
	return gomcp.CallToolRequest{
		Params: gomcp.CallToolParams{
			Arguments: params,
		},
	}
}

func TestMakeHandler_Skill(t *testing.T) {
	orig := runSkillFn
	defer func() { runSkillFn = orig }()

	called := false
	runSkillFn = func(ctx context.Context, m *parser.PluginManifest, args []string, timeout time.Duration) (*executor.SkillResult, error) {
		called = true
		return &executor.SkillResult{Stdout: "ok"}, nil
	}

	m := &parser.PluginManifest{Name: "test", Type: parser.PluginTypeSkill}
	handler := makeHandler(m)
	result, err := handler(context.Background(), buildRequest(map[string]interface{}{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("expected runSkillFn to be called")
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestMakeHandler_Connector(t *testing.T) {
	orig := startConnectorFn
	defer func() { startConnectorFn = orig }()

	called := false
	startConnectorFn = func(m *parser.PluginManifest) error {
		called = true
		return nil
	}

	m := &parser.PluginManifest{Name: "test", Type: parser.PluginTypeConnector}
	handler := makeHandler(m)
	result, err := handler(context.Background(), buildRequest(map[string]interface{}{"action": "start"}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("expected startConnectorFn to be called")
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestMakeHandler_UnknownType(t *testing.T) {
	m := &parser.PluginManifest{Name: "test", Type: parser.PluginType("widget")}
	handler := makeHandler(m)
	_, err := handler(context.Background(), buildRequest(map[string]interface{}{}))
	if err == nil {
		t.Fatal("expected error for unknown type")
	}
	if !strings.Contains(err.Error(), "unknown plugin type") {
		t.Errorf("expected 'unknown plugin type', got %q", err.Error())
	}
}

// --- handleSkill tests ---

func TestHandleSkill_JSONOutput(t *testing.T) {
	orig := runSkillFn
	defer func() { runSkillFn = orig }()

	runSkillFn = func(ctx context.Context, m *parser.PluginManifest, args []string, timeout time.Duration) (*executor.SkillResult, error) {
		return &executor.SkillResult{
			Stdout: `{"key":"value"}`,
			Output: json.RawMessage(`{"key":"value"}`),
		}, nil
	}

	m := &parser.PluginManifest{Name: "test", Type: parser.PluginTypeSkill}
	result, err := handleSkill(context.Background(), m, buildRequest(map[string]interface{}{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := getTextContent(result)
	if !strings.Contains(text, "key") {
		t.Errorf("expected formatted JSON with 'key', got %q", text)
	}
}

func TestHandleSkill_PlainText(t *testing.T) {
	orig := runSkillFn
	defer func() { runSkillFn = orig }()

	runSkillFn = func(ctx context.Context, m *parser.PluginManifest, args []string, timeout time.Duration) (*executor.SkillResult, error) {
		return &executor.SkillResult{Stdout: "plain text output"}, nil
	}

	m := &parser.PluginManifest{Name: "test", Type: parser.PluginTypeSkill}
	result, err := handleSkill(context.Background(), m, buildRequest(map[string]interface{}{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := getTextContent(result)
	if text != "plain text output" {
		t.Errorf("expected 'plain text output', got %q", text)
	}
}

func TestHandleSkill_Error(t *testing.T) {
	orig := runSkillFn
	defer func() { runSkillFn = orig }()

	runSkillFn = func(ctx context.Context, m *parser.PluginManifest, args []string, timeout time.Duration) (*executor.SkillResult, error) {
		return nil, fmt.Errorf("skill failed")
	}

	m := &parser.PluginManifest{Name: "test", Type: parser.PluginTypeSkill}
	result, err := handleSkill(context.Background(), m, buildRequest(map[string]interface{}{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError to be true")
	}
	text := getTextContent(result)
	if !strings.Contains(text, "skill failed") {
		t.Errorf("expected error message, got %q", text)
	}
}

func TestHandleSkill_WithArgs(t *testing.T) {
	orig := runSkillFn
	defer func() { runSkillFn = orig }()

	var capturedArgs []string
	runSkillFn = func(ctx context.Context, m *parser.PluginManifest, args []string, timeout time.Duration) (*executor.SkillResult, error) {
		capturedArgs = args
		return &executor.SkillResult{Stdout: "ok"}, nil
	}

	m := &parser.PluginManifest{Name: "test", Type: parser.PluginTypeSkill}
	handleSkill(context.Background(), m, buildRequest(map[string]interface{}{"args": "--query hello world"}))
	if len(capturedArgs) != 3 {
		t.Errorf("expected 3 args, got %d: %v", len(capturedArgs), capturedArgs)
	}
}

func TestHandleSkill_EmptyArgs(t *testing.T) {
	orig := runSkillFn
	defer func() { runSkillFn = orig }()

	var capturedArgs []string
	runSkillFn = func(ctx context.Context, m *parser.PluginManifest, args []string, timeout time.Duration) (*executor.SkillResult, error) {
		capturedArgs = args
		return &executor.SkillResult{Stdout: "ok"}, nil
	}

	m := &parser.PluginManifest{Name: "test", Type: parser.PluginTypeSkill}
	handleSkill(context.Background(), m, buildRequest(map[string]interface{}{}))
	if capturedArgs != nil {
		t.Errorf("expected nil args, got %v", capturedArgs)
	}
}

// --- handleConnector tests ---

func TestHandleConnector_Start_Success(t *testing.T) {
	orig := startConnectorFn
	defer func() { startConnectorFn = orig }()

	startConnectorFn = func(m *parser.PluginManifest) error { return nil }

	m := &parser.PluginManifest{Name: "wechat", Type: parser.PluginTypeConnector}
	result, err := handleConnector(m, buildRequest(map[string]interface{}{"action": "start"}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := getTextContent(result)
	if !strings.Contains(text, "started") {
		t.Errorf("expected 'started', got %q", text)
	}
}

func TestHandleConnector_Start_Error(t *testing.T) {
	orig := startConnectorFn
	defer func() { startConnectorFn = orig }()

	startConnectorFn = func(m *parser.PluginManifest) error { return fmt.Errorf("already running") }

	m := &parser.PluginManifest{Name: "wechat", Type: parser.PluginTypeConnector}
	result, err := handleConnector(m, buildRequest(map[string]interface{}{"action": "start"}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError")
	}
}

func TestHandleConnector_Stop_Success(t *testing.T) {
	orig := stopConnectorFn
	defer func() { stopConnectorFn = orig }()

	stopConnectorFn = func(name string) error { return nil }

	m := &parser.PluginManifest{Name: "wechat", Type: parser.PluginTypeConnector}
	result, err := handleConnector(m, buildRequest(map[string]interface{}{"action": "stop"}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := getTextContent(result)
	if !strings.Contains(text, "stopped") {
		t.Errorf("expected 'stopped', got %q", text)
	}
}

func TestHandleConnector_Stop_Error(t *testing.T) {
	orig := stopConnectorFn
	defer func() { stopConnectorFn = orig }()

	stopConnectorFn = func(name string) error { return fmt.Errorf("not running") }

	m := &parser.PluginManifest{Name: "wechat", Type: parser.PluginTypeConnector}
	result, err := handleConnector(m, buildRequest(map[string]interface{}{"action": "stop"}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError")
	}
}

func TestHandleConnector_Status_Running(t *testing.T) {
	orig := getConnectorStatusFn
	defer func() { getConnectorStatusFn = orig }()

	getConnectorStatusFn = func(name string) (*executor.ConnectorStatus, error) {
		return &executor.ConnectorStatus{
			Name:    name,
			PID:     12345,
			Running: true,
		}, nil
	}

	m := &parser.PluginManifest{Name: "wechat", Type: parser.PluginTypeConnector}
	result, err := handleConnector(m, buildRequest(map[string]interface{}{"action": "status"}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := getTextContent(result)
	if !strings.Contains(text, "12345") {
		t.Errorf("expected PID in output, got %q", text)
	}
}

func TestHandleConnector_Status_NotRunning(t *testing.T) {
	orig := getConnectorStatusFn
	defer func() { getConnectorStatusFn = orig }()

	getConnectorStatusFn = func(name string) (*executor.ConnectorStatus, error) {
		return nil, fmt.Errorf("no PID file")
	}

	m := &parser.PluginManifest{Name: "wechat", Type: parser.PluginTypeConnector}
	result, err := handleConnector(m, buildRequest(map[string]interface{}{"action": "status"}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := getTextContent(result)
	if !strings.Contains(text, "Not running") {
		t.Errorf("expected 'Not running', got %q", text)
	}
}

func TestHandleConnector_MissingAction(t *testing.T) {
	m := &parser.PluginManifest{Name: "wechat", Type: parser.PluginTypeConnector}
	_, err := handleConnector(m, buildRequest(map[string]interface{}{}))
	if err == nil {
		t.Fatal("expected error for missing action")
	}
	if !strings.Contains(err.Error(), "action is required") {
		t.Errorf("expected 'action is required', got %q", err.Error())
	}
}

func TestHandleConnector_CustomAction_Success(t *testing.T) {
	orig := attachConnectorFn
	defer func() { attachConnectorFn = orig }()

	// Use net.Pipe to simulate UDS
	serverConn, clientConn := net.Pipe()
	attachConnectorFn = func(name string) (net.Conn, error) {
		return clientConn, nil
	}

	// Server side: read command, write response
	go func() {
		buf := make([]byte, 65536)
		n, _ := serverConn.Read(buf)
		_ = n // consume the command
		serverConn.Write([]byte(`{"type":"response","payload":{"ok":true}}`))
		serverConn.Close()
	}()

	m := &parser.PluginManifest{Name: "wechat", Type: parser.PluginTypeConnector}
	result, err := handleConnector(m, buildRequest(map[string]interface{}{
		"action":  "send_message",
		"payload": `{"text":"hello"}`,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := getTextContent(result)
	if !strings.Contains(text, "response") {
		t.Errorf("expected response, got %q", text)
	}
}

func TestHandleConnector_CustomAction_DialError(t *testing.T) {
	orig := attachConnectorFn
	defer func() { attachConnectorFn = orig }()

	attachConnectorFn = func(name string) (net.Conn, error) {
		return nil, fmt.Errorf("connection refused")
	}

	m := &parser.PluginManifest{Name: "wechat", Type: parser.PluginTypeConnector}
	result, err := handleConnector(m, buildRequest(map[string]interface{}{
		"action": "send_message",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError")
	}
	text := getTextContent(result)
	if !strings.Contains(text, "Cannot connect") {
		t.Errorf("expected 'Cannot connect', got %q", text)
	}
}

func TestHandleConnector_CustomAction_ReadError(t *testing.T) {
	orig := attachConnectorFn
	defer func() { attachConnectorFn = orig }()

	// Server immediately closes without writing
	_, clientConn := net.Pipe()
	attachConnectorFn = func(name string) (net.Conn, error) {
		clientConn.Close() // close immediately so Read fails
		return clientConn, nil
	}

	m := &parser.PluginManifest{Name: "wechat", Type: parser.PluginTypeConnector}
	result, err := handleConnector(m, buildRequest(map[string]interface{}{
		"action": "send_message",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := getTextContent(result)
	if !strings.Contains(text, "no response") {
		t.Errorf("expected 'no response', got %q", text)
	}
}

// --- helpers ---

func getTextContent(result *gomcp.CallToolResult) string {
	if result == nil || len(result.Content) == 0 {
		return ""
	}
	if tc, ok := result.Content[0].(gomcp.TextContent); ok {
		return tc.Text
	}
	return ""
}
