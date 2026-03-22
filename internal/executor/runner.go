package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/user/claw2cli/internal/parser"
)

// SkillResult holds the output from running a skill plugin.
type SkillResult struct {
	Stdout   string          `json:"stdout,omitempty"`
	Stderr   string          `json:"stderr,omitempty"`
	ExitCode int             `json:"exit_code"`
	Output   json.RawMessage `json:"output,omitempty"` // parsed JSON from stdout, if valid
}

// DefaultTimeout is the default skill execution timeout.
const DefaultTimeout = 30 * time.Second

// RunSkill executes a skill plugin as a subprocess via npx.
func RunSkill(ctx context.Context, manifest *parser.PluginManifest, args []string, timeout time.Duration) (*SkillResult, error) {
	if err := CheckPermissions(manifest); err != nil {
		return nil, fmt.Errorf("permission check: %w", err)
	}

	if timeout == 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Build the npx command
	cmdArgs := buildNpxArgs(manifest.Source, args)
	cmd := execCommandCtx(ctx, "npx", cmdArgs...)
	cmd.Env = BuildEnv(manifest)

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	result := &SkillResult{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return result, fmt.Errorf("skill %q timed out after %s", manifest.Name, timeout)
		} else if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			return result, fmt.Errorf("run skill %q: %w", manifest.Name, err)
		}
	}

	// Try to parse stdout as JSON
	trimmed := strings.TrimSpace(stdout.String())
	if len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[') {
		if json.Valid([]byte(trimmed)) {
			result.Output = json.RawMessage(trimmed)
		}
	}

	return result, nil
}

// buildNpxArgs constructs the arguments for npx invocation.
func buildNpxArgs(source string, args []string) []string {
	npxArgs := []string{"-y", source}
	npxArgs = append(npxArgs, args...)
	return npxArgs
}
