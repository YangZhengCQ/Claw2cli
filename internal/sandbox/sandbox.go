package sandbox

import (
	"os/exec"

	"github.com/YangZhengCQ/Claw2cli/internal/parser"
)

// SandboxPaths contains paths the sandbox must allow access to.
type SandboxPaths struct {
	ShimDir     string // path to shim/ directory (read-only)
	NodeModules string // path to plugin's node_modules (read-only)
	NodeRunner  string // path to tsx/node binary (read-only)
	StorageDir  string // path to plugin's storage dir (read-write)
}

// Apply configures OS-level sandboxing on the given command based on
// the plugin's declared permissions. This is a platform-specific operation.
// Returns nil on success, or an error if the sandbox cannot be applied.
// Callers should treat errors as non-fatal (fail-open with warning).
func Apply(cmd *exec.Cmd, manifest *parser.PluginManifest, paths SandboxPaths) error {
	return applyPlatform(cmd, manifest, paths)
}
