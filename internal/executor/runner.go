package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/YangZhengCQ/Claw2cli/internal/parser"
	"github.com/YangZhengCQ/Claw2cli/internal/store"
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

// maxOutputSize is the maximum captured output size from skill execution (10 MB).
const maxOutputSize = 10 * 1024 * 1024

// limitedWriter caps writes at a byte limit to prevent OOM from malicious plugins.
type limitedWriter struct {
	buf      strings.Builder
	limit    int
	exceeded bool
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	remaining := w.limit - w.buf.Len()
	if remaining <= 0 {
		w.exceeded = true
		return len(p), nil // accept but discard
	}
	if len(p) > remaining {
		w.buf.Write(p[:remaining])
		w.exceeded = true
		return len(p), nil
	}
	return w.buf.Write(p)
}

func (w *limitedWriter) String() string { return w.buf.String() }
func (w *limitedWriter) Len() int       { return w.buf.Len() }

// RunSkill executes a skill plugin as a subprocess using the local store.
func RunSkill(ctx context.Context, manifest *parser.PluginManifest, args []string, timeout time.Duration) (*SkillResult, error) {
	if err := CheckPermissions(manifest); err != nil {
		return nil, fmt.Errorf("permission check: %w", err)
	}

	if timeout == 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Build the command using local store
	s := store.New(manifest.Name)
	if !s.IsInstalled() {
		return nil, fmt.Errorf("skill %q not installed — run 'c2c install %s' first", manifest.Name, manifest.Name)
	}
	tsxPath := store.ResolveTsx()
	// Execute the skill's main entry point from local node_modules
	cmd := execCommandCtx(ctx, tsxPath, filepath.Join(s.NodeModulesPath(), ".bin", manifest.Name))
	cmd.Env = append(BuildEnv(manifest), "NODE_PATH="+s.NodeModulesPath())

	stdout := &limitedWriter{limit: maxOutputSize}
	stderr := &limitedWriter{limit: maxOutputSize}
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	err := cmd.Run()

	if stdout.exceeded || stderr.exceeded {
		return nil, fmt.Errorf("skill %q output exceeded %d bytes limit", manifest.Name, maxOutputSize)
	}

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

