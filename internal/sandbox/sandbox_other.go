//go:build !linux && !darwin

package sandbox

import (
	"fmt"
	"os/exec"

	"github.com/YangZhengCQ/Claw2cli/internal/parser"
)

func applyPlatform(cmd *exec.Cmd, manifest *parser.PluginManifest, paths SandboxPaths) error {
	return fmt.Errorf("sandbox not available on this platform")
}
