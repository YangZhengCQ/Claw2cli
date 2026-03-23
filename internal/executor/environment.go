package executor

import (
	"os"
	"strings"

	"github.com/YangZhengCQ/Claw2cli/internal/parser"
	"github.com/YangZhengCQ/Claw2cli/internal/paths"
)

// safeEnvPrefixes lists environment variable prefixes that are safe to pass
// to plugin subprocesses. Everything else is filtered out to prevent
// leaking credentials (AWS_SECRET_ACCESS_KEY, GITHUB_TOKEN, etc.).
var safeEnvPrefixes = []string{
	"PATH=",
	"HOME=",
	"USER=",
	"LANG=",
	"LC_",
	"TERM=",
	"SHELL=",
	"TMPDIR=",
	"XDG_",
	"NODE_",
	"NPM_",
	"C2C_",
}

// BuildEnv constructs the environment variables for a plugin subprocess.
// Only safe variables are inherited; sensitive credentials are filtered out.
func BuildEnv(manifest *parser.PluginManifest) []string {
	var env []string
	for _, e := range os.Environ() {
		if isSafeEnvVar(e) {
			env = append(env, e)
		}
	}
	env = append(env,
		"C2C_PLUGIN_NAME="+manifest.Name,
		"C2C_PLUGIN_TYPE="+string(manifest.Type),
		"C2C_STORAGE_DIR="+paths.StorageDir(manifest.Name),
		"C2C_BASE_DIR="+paths.BaseDir(),
	)
	return env
}

// sensitiveEnvVars are specific variables that match safe prefixes but contain credentials.
var sensitiveEnvVars = []string{
	"NODE_AUTH_TOKEN=",
	"NPM_TOKEN=",
	"NPM_CONFIG__AUTHTOKEN=",
	"NODE_OPTIONS=",
}

func isSafeEnvVar(e string) bool {
	// Block known-sensitive vars even if they match a safe prefix
	for _, sv := range sensitiveEnvVars {
		if strings.HasPrefix(e, sv) {
			return false
		}
	}
	for _, prefix := range safeEnvPrefixes {
		if strings.HasPrefix(e, prefix) {
			return true
		}
	}
	return false
}
