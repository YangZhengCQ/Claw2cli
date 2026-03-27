package paths

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// baseDir is the root of all c2c data. Override with SetBaseDir for testing.
var baseDir string

func init() {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: $HOME not set — c2c requires a home directory for data storage\n")
		os.Exit(1)
	}
	baseDir = filepath.Join(home, ".c2c")
}

// initBaseDir sets baseDir from the given home directory. Returns error if home is empty.
// Extracted from init() for testability.
func initBaseDir(home string) error {
	if home == "" {
		return fmt.Errorf("home directory is empty — c2c requires a home directory for data storage")
	}
	baseDir = filepath.Join(home, ".c2c")
	return nil
}

// ShimDir returns the path to the shim directory bundled with the c2c binary.
// It checks: next to binary, ../libexec/shim (Homebrew), and CWD fallback.
func ShimDir() string {
	self, err := os.Executable()
	if err != nil {
		return "shim"
	}
	binDir := filepath.Dir(self)
	// Check if shim/ exists next to the binary
	dir := filepath.Join(binDir, "shim")
	if _, err := os.Stat(dir); err == nil {
		return dir
	}
	// Homebrew: shim is in ../libexec/shim relative to bin/
	libexecDir := filepath.Join(binDir, "..", "libexec", "shim")
	if _, err := os.Stat(libexecDir); err == nil {
		return libexecDir
	}
	// Note: CWD fallback removed for security — running from an untrusted directory
	// could load attacker-controlled shim code. Use explicit install locations only.
	return dir
}

// SetBaseDir overrides the base directory (for testing).
func SetBaseDir(dir string) {
	baseDir = dir
}

// BaseDir returns the root c2c data directory (~/.c2c).
func BaseDir() string {
	return baseDir
}

// ValidateName rejects plugin names that could cause path traversal.
func ValidateName(name string) error {
	if name == "" {
		return fmt.Errorf("plugin name cannot be empty")
	}
	if strings.Contains(name, "..") || strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return fmt.Errorf("invalid plugin name %q: must not contain '..', '/' or '\\'", name)
	}
	return nil
}

// PluginsDir returns ~/.c2c/plugins.
func PluginsDir() string {
	return filepath.Join(baseDir, "plugins")
}

// PluginDir returns ~/.c2c/plugins/<name>.
func PluginDir(name string) string {
	return filepath.Join(baseDir, "plugins", name)
}

// StorageDir returns ~/.c2c/storage/<name>.
func StorageDir(name string) string {
	return filepath.Join(baseDir, "storage", name)
}

// SocketPath returns ~/.c2c/sockets/<name>.sock.
func SocketPath(name string) string {
	return filepath.Join(baseDir, "sockets", name+".sock")
}

// PIDPath returns ~/.c2c/pids/<name>.pid.
func PIDPath(name string) string {
	return filepath.Join(baseDir, "pids", name+".pid")
}

// ConfigPath returns ~/.c2c/config.yaml.
func ConfigPath() string {
	return filepath.Join(baseDir, "config.yaml")
}

// EnsureDirs creates all required c2c directories.
func EnsureDirs() error {
	dirs := []string{
		baseDir,
		filepath.Join(baseDir, "plugins"),
		filepath.Join(baseDir, "storage"),
		filepath.Join(baseDir, "sockets"),
		filepath.Join(baseDir, "pids"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0700); err != nil {
			return err
		}
	}
	return nil
}

// EnsureStorageDir creates the storage directory for a specific plugin with 0700 permissions.
func EnsureStorageDir(name string) error {
	return os.MkdirAll(StorageDir(name), 0700)
}
