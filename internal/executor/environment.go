package executor

import (
	"os"

	"github.com/user/claw2cli/internal/parser"
	"github.com/user/claw2cli/internal/paths"
)

// BuildEnv constructs the environment variables for a plugin subprocess.
// It inherits the current process environment and adds c2c-specific variables.
func BuildEnv(manifest *parser.PluginManifest) []string {
	env := os.Environ()
	env = append(env,
		"C2C_PLUGIN_NAME="+manifest.Name,
		"C2C_PLUGIN_TYPE="+string(manifest.Type),
		"C2C_STORAGE_DIR="+paths.StorageDir(manifest.Name),
		"C2C_BASE_DIR="+paths.BaseDir(),
	)
	return env
}
