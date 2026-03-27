//go:build darwin

package sandbox

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/YangZhengCQ/Claw2cli/internal/parser"
)

func applyPlatform(cmd *exec.Cmd, manifest *parser.PluginManifest, paths SandboxPaths) error {
	// Check if sandbox-exec is available (deprecated on macOS, may be removed)
	if err := exec.Command("/usr/bin/sandbox-exec", "-n", "no-internet", "/usr/bin/true").Run(); err != nil {
		return fmt.Errorf("sandbox-exec not available: %w", err)
	}

	// Generate sandbox profile
	profile := generateProfile(manifest, paths)
	profilePath, err := writeTempProfile(profile)
	if err != nil {
		return fmt.Errorf("write sandbox profile: %w", err)
	}

	// Wrap command with sandbox-exec
	originalPath := cmd.Path
	originalArgs := cmd.Args
	cmd.Path = "/usr/bin/sandbox-exec"
	cmd.Args = append([]string{"sandbox-exec", "-f", profilePath}, originalArgs...)
	_ = originalPath // used implicitly via originalArgs[0]

	// Register cleanup for the temp profile after command exits
	cleanupFn = func() { os.Remove(profilePath) }

	return nil
}

// dangerousFsPaths are paths that must never be granted via fs: permissions.
var dangerousFsPaths = []string{"/", "/Users", "/System", "/Library", "/bin", "/sbin", "/etc"}

func isUnsafeFsPath(p string) bool {
	clean := filepath.Clean(p)
	for _, d := range dangerousFsPaths {
		if clean == d {
			return true
		}
	}
	return strings.Contains(clean, "..")
}

func generateProfile(manifest *parser.PluginManifest, spaths SandboxPaths) string {
	var sb strings.Builder

	// Use allow-default with selective deny.
	// macOS 15+ sandbox-exec rejects overly restrictive (deny default) custom profiles.
	sb.WriteString("(version 1)\n")
	sb.WriteString("(allow default)\n")

	// Network: deny if not declared in permissions
	hasNetwork := false
	for _, p := range manifest.Permissions {
		if string(p) == "network" {
			hasNetwork = true
		}
	}
	if !hasNetwork {
		sb.WriteString("(deny network*)\n")
	}

	return sb.String()
}

func writeTempProfile(content string) (string, error) {
	f, err := os.CreateTemp("", "c2c-sandbox-*.sb")
	if err != nil {
		return "", err
	}
	if _, err := f.WriteString(content); err != nil {
		f.Close()
		return "", err
	}
	f.Close()
	return f.Name(), nil
}
