package paths

import (
	"os"
	"path/filepath"
)

// baseDir is the root of all c2c data. Override with SetBaseDir for testing.
var baseDir string

func init() {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	baseDir = filepath.Join(home, ".c2c")
}

// SetBaseDir overrides the base directory (for testing).
func SetBaseDir(dir string) {
	baseDir = dir
}

// BaseDir returns the root c2c data directory (~/.c2c).
func BaseDir() string {
	return baseDir
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
		if err := os.MkdirAll(d, 0755); err != nil {
			return err
		}
	}
	return nil
}

// EnsureStorageDir creates the storage directory for a specific plugin with 0700 permissions.
func EnsureStorageDir(name string) error {
	return os.MkdirAll(StorageDir(name), 0700)
}
