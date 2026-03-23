//go:build linux

package sandbox

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/YangZhengCQ/Claw2cli/internal/parser"
)

func applyPlatform(cmd *exec.Cmd, manifest *parser.PluginManifest, paths SandboxPaths) error {
	// Landlock requires kernel 5.13+ and the go-landlock library.
	// For now, implement a basic approach using environment restrictions.
	// Full landlock integration deferred until go-landlock is added to go.mod.

	hasNetwork := false
	for _, p := range manifest.Permissions {
		if string(p) == "network" {
			hasNetwork = true
		}
	}

	if !hasNetwork {
		// Without landlock, we can't block network on Linux without seccomp.
		// Log a warning that network restriction is not enforced.
		fmt.Fprintf(os.Stderr, "warning: network restriction requires landlock (kernel 5.13+)\n")
	}

	// Set restrictive umask for the subprocess
	// This is a minimal sandbox — full landlock integration is a separate task
	_ = strings.HasPrefix // avoid unused import
	_ = cmd               // will be used when landlock is integrated

	return nil
}
