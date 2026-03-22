package executor

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/user/claw2cli/internal/parser"
)

func TestBuildNpxArgs(t *testing.T) {
	args := buildNpxArgs("@test/plugin@1.0.0", []string{"--query", "hello"})
	expected := []string{"-y", "@test/plugin@1.0.0", "--query", "hello"}

	if len(args) != len(expected) {
		t.Fatalf("args length=%d, want %d", len(args), len(expected))
	}
	for i, a := range args {
		if a != expected[i] {
			t.Errorf("args[%d]=%q, want %q", i, a, expected[i])
		}
	}
}

func TestBuildEnv(t *testing.T) {
	m := &parser.PluginManifest{
		Name: "test-plugin",
		Type: parser.PluginTypeSkill,
	}

	env := BuildEnv(m)

	found := map[string]bool{}
	for _, e := range env {
		if e == "C2C_PLUGIN_NAME=test-plugin" {
			found["name"] = true
		}
		if e == "C2C_PLUGIN_TYPE=skill" {
			found["type"] = true
		}
	}

	if !found["name"] {
		t.Error("C2C_PLUGIN_NAME not found in env")
	}
	if !found["type"] {
		t.Error("C2C_PLUGIN_TYPE not found in env")
	}
}

// fakeExecCommandCtx replaces execCommandCtx for testing.
// It returns a command that runs the given shell script via bash -c.
func fakeExecCommandCtx(script string) func(ctx context.Context, name string, args ...string) *exec.Cmd {
	return func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "bash", "-c", script)
	}
}

func skillManifest() *parser.PluginManifest {
	return &parser.PluginManifest{
		Name:   "test-skill",
		Type:   parser.PluginTypeSkill,
		Source: "@test/plugin@1.0.0",
	}
}

func connectorManifest() *parser.PluginManifest {
	return &parser.PluginManifest{
		Name:        "test-connector",
		Type:        parser.PluginTypeConnector,
		Source:      "@test/connector@1.0.0",
		Permissions: []parser.Permission{"network"},
	}
}

func connectorManifestNoNetwork() *parser.PluginManifest {
	return &parser.PluginManifest{
		Name:   "test-connector",
		Type:   parser.PluginTypeConnector,
		Source: "@test/connector@1.0.0",
	}
}

func TestRunSkill_JSONStdout(t *testing.T) {
	orig := execCommandCtx
	defer func() { execCommandCtx = orig }()
	execCommandCtx = fakeExecCommandCtx(`echo '{"result":"ok"}'`)

	result, err := RunSkill(context.Background(), skillManifest(), nil, 5*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Output == nil {
		t.Error("expected Output to be non-nil for JSON stdout")
	}
	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", result.ExitCode)
	}
}

func TestRunSkill_PlainStdout(t *testing.T) {
	orig := execCommandCtx
	defer func() { execCommandCtx = orig }()
	execCommandCtx = fakeExecCommandCtx(`echo "hello world"`)

	result, err := RunSkill(context.Background(), skillManifest(), nil, 5*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Output != nil {
		t.Error("expected Output to be nil for plain text stdout")
	}
	if !strings.Contains(result.Stdout, "hello world") {
		t.Errorf("expected stdout to contain 'hello world', got %q", result.Stdout)
	}
}

func TestRunSkill_NonZeroExit(t *testing.T) {
	orig := execCommandCtx
	defer func() { execCommandCtx = orig }()
	execCommandCtx = fakeExecCommandCtx(`echo "fail" >&2; exit 1`)

	result, err := RunSkill(context.Background(), skillManifest(), nil, 5*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Errorf("expected exit code 1, got %d", result.ExitCode)
	}
	if !strings.Contains(result.Stderr, "fail") {
		t.Errorf("expected stderr to contain 'fail', got %q", result.Stderr)
	}
}

func TestRunSkill_PermissionDenied(t *testing.T) {
	_, err := RunSkill(context.Background(), connectorManifestNoNetwork(), nil, 5*time.Second)
	if err == nil {
		t.Fatal("expected permission error")
	}
	if !strings.Contains(err.Error(), "permission") {
		t.Errorf("expected error to contain 'permission', got %q", err.Error())
	}
}

func TestRunSkill_DefaultTimeout(t *testing.T) {
	orig := execCommandCtx
	defer func() { execCommandCtx = orig }()
	execCommandCtx = fakeExecCommandCtx(`echo "ok"`)

	// timeout=0 should use DefaultTimeout (30s) but still succeed quickly
	result, err := RunSkill(context.Background(), skillManifest(), nil, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", result.ExitCode)
	}
}

func TestRunSkill_Timeout(t *testing.T) {
	orig := execCommandCtx
	defer func() { execCommandCtx = orig }()
	execCommandCtx = fakeExecCommandCtx(`sleep 30`)

	_, err := RunSkill(context.Background(), skillManifest(), nil, 50*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("expected error to contain 'timed out', got %q", err.Error())
	}
}

func TestRunSkill_CommandError(t *testing.T) {
	orig := execCommandCtx
	defer func() { execCommandCtx = orig }()
	// Return a command for a nonexistent binary
	execCommandCtx = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "/nonexistent/binary/xyz")
	}

	_, err := RunSkill(context.Background(), skillManifest(), nil, 5*time.Second)
	if err == nil {
		t.Fatal("expected error for nonexistent binary")
	}
	if !strings.Contains(err.Error(), "run skill") {
		t.Errorf("expected error to contain 'run skill', got %q", err.Error())
	}
}

func TestRunSkill_JSONArray(t *testing.T) {
	orig := execCommandCtx
	defer func() { execCommandCtx = orig }()
	execCommandCtx = fakeExecCommandCtx(`echo '[1,2,3]'`)

	result, err := RunSkill(context.Background(), skillManifest(), nil, 5*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Output == nil {
		t.Error("expected Output to be non-nil for JSON array stdout")
	}
}

func TestRunSkill_InvalidJSON(t *testing.T) {
	orig := execCommandCtx
	defer func() { execCommandCtx = orig }()
	execCommandCtx = fakeExecCommandCtx(`echo '{invalid json'`)

	result, err := RunSkill(context.Background(), skillManifest(), nil, 5*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Output != nil {
		t.Error("expected Output to be nil for invalid JSON")
	}
}
