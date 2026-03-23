package store

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/YangZhengCQ/Claw2cli/internal/paths"
)

const tsxVersion = "4.19.4"

// TsxPath returns the path to the shared tsx binary.
func TsxPath() string {
	return filepath.Join(paths.BaseDir(), "bin", "tsx")
}

// EnsureTsx installs tsx to ~/.c2c/bin/ if not present.
func EnsureTsx() (string, error) {
	tsxPath := TsxPath()

	// Check if already installed
	if _, err := os.Stat(tsxPath); err == nil {
		return tsxPath, nil
	}

	// Install tsx locally
	binDir := filepath.Join(paths.BaseDir(), "bin")
	if err := os.MkdirAll(binDir, 0700); err != nil {
		return "", fmt.Errorf("create bin directory: %w", err)
	}

	// Install tsx to a temp prefix, then symlink the binary
	tmpDir := filepath.Join(paths.BaseDir(), "bin", ".tsx-install")
	cmd := execCommandFn("npm", "install", "--prefix", tmpDir, fmt.Sprintf("tsx@%s", tsxVersion))
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("install tsx@%s: %w", tsxVersion, err)
	}

	// Create wrapper script that invokes the installed tsx
	tsxBin := filepath.Join(tmpDir, "node_modules", ".bin", "tsx")
	wrapper := fmt.Sprintf("#!/bin/sh\nexec %q \"$@\"\n", tsxBin)
	if err := os.WriteFile(tsxPath, []byte(wrapper), 0755); err != nil {
		return "", fmt.Errorf("write tsx wrapper: %w", err)
	}

	return tsxPath, nil
}

// ResolveTsx returns the tsx path, falling back to global "tsx" or "node".
func ResolveTsx() string {
	// Prefer local tsx
	if p := TsxPath(); fileExists(p) {
		return p
	}
	// Fallback: global tsx
	if p, err := exec.LookPath("tsx"); err == nil {
		return p
	}
	// Last resort: node (TypeScript plugins may not load)
	return "node"
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
