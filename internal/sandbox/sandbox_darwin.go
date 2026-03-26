//go:build darwin

package sandbox

import (
	"fmt"
	"os"
	"os/exec"
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

	return nil
}

func generateProfile(manifest *parser.PluginManifest, spaths SandboxPaths) string {
	var sb strings.Builder
	sb.WriteString("(version 1)\n")
	sb.WriteString("(deny default)\n")
	sb.WriteString("(allow process-exec)\n")
	sb.WriteString("(allow process-fork)\n")
	sb.WriteString("(allow sysctl-read)\n")
	sb.WriteString("(allow mach-lookup)\n")

	// Allow reading shim and node_modules
	sb.WriteString(fmt.Sprintf("(allow file-read* (subpath %q))\n", spaths.ShimDir))
	sb.WriteString(fmt.Sprintf("(allow file-read* (subpath %q))\n", spaths.NodeModules))
	sb.WriteString(fmt.Sprintf("(allow file-read* (literal %q))\n", spaths.NodeRunner))

	// Allow read-write to storage dir
	if spaths.StorageDir != "" {
		sb.WriteString(fmt.Sprintf("(allow file-read* file-write* (subpath %q))\n", spaths.StorageDir))
	}

	// Allow tmp
	sb.WriteString("(allow file-read* file-write* (subpath \"/tmp\"))\n")
	sb.WriteString("(allow file-read* file-write* (subpath \"/private/tmp\"))\n")

	// Allow reading system libraries
	sb.WriteString("(allow file-read* (subpath \"/usr\"))\n")
	sb.WriteString("(allow file-read* (subpath \"/Library\"))\n")
	sb.WriteString("(allow file-read* (subpath \"/System\"))\n")

	// Network: only if declared
	hasNetwork := false
	for _, p := range manifest.Permissions {
		if string(p) == "network" {
			hasNetwork = true
		}
		if strings.HasPrefix(string(p), "fs:") {
			path := strings.TrimPrefix(string(p), "fs:")
			sb.WriteString(fmt.Sprintf("(allow file-read* file-write* (subpath %q))\n", path))
		}
	}
	if hasNetwork {
		sb.WriteString("(allow network*)\n")
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
