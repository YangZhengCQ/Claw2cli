package store

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/YangZhengCQ/Claw2cli/internal/parser"
	"github.com/YangZhengCQ/Claw2cli/internal/paths"
)

// execCommandFn is swappable for testing.
var execCommandFn = exec.Command

// Store manages the local package directory for a plugin.
type Store struct {
	pluginDir string
	name      string
}

// New creates a Store for the given plugin name.
func New(name string) *Store {
	return &Store{
		pluginDir: paths.PluginDir(name),
		name:      name,
	}
}

// NodeModulesPath returns the path to the local node_modules directory.
func (s *Store) NodeModulesPath() string {
	return filepath.Join(s.pluginDir, "node_modules")
}

// IsInstalled checks whether node_modules exists and is non-empty.
func (s *Store) IsInstalled() bool {
	entries, err := os.ReadDir(s.NodeModulesPath())
	return err == nil && len(entries) > 0
}

// Install resolves the exact version, installs locally, and records integrity.
// Returns the resolved version and integrity hash.
func (s *Store) Install(source string) (resolvedVersion, integrity string, err error) {
	// Resolve exact version
	resolvedVersion, err = resolveVersion(source)
	if err != nil {
		return "", "", fmt.Errorf("resolve version: %w", err)
	}

	// Construct exact spec
	exactSpec := source
	if resolvedVersion != "" {
		// Strip existing version from source, append resolved
		base := stripVersion(source)
		exactSpec = base + "@" + resolvedVersion
	}

	// Install locally
	cmd := execCommandFn("npm", "install", "--prefix", s.pluginDir, exactSpec)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", "", fmt.Errorf("npm install --prefix %s %s: %w", s.pluginDir, exactSpec, err)
	}

	// Also install runtime package if different (strip -cli suffix)
	runtimePkg := stripCLISuffix(stripVersion(source))
	basePkg := stripVersion(source)
	if runtimePkg != basePkg {
		runtimeSpec := runtimePkg
		if resolvedVersion != "" {
			runtimeSpec = runtimePkg + "@" + resolvedVersion
		}
		cmd2 := execCommandFn("npm", "install", "--prefix", s.pluginDir, runtimeSpec)
		cmd2.Stdout = os.Stderr
		cmd2.Stderr = os.Stderr
		if err := cmd2.Run(); err != nil {
			return "", "", fmt.Errorf("npm install runtime package: %w", err)
		}
	}

	// Get integrity hash
	integrity, _ = getIntegrity(source)

	return resolvedVersion, integrity, nil
}

// Verify checks that installed packages match manifest integrity hashes.
func (s *Store) Verify(manifest *parser.PluginManifest) error {
	if !s.IsInstalled() {
		return fmt.Errorf("packages not installed for %q — run 'c2c install %s' first", s.name, s.name)
	}

	expectedHash := manifest.Integrity
	if expectedHash == "" {
		expectedHash = manifest.Checksum // backwards compat
	}
	if expectedHash == "" {
		return nil // no integrity to check
	}

	currentHash, err := getIntegrity(manifest.Source)
	if err != nil {
		return fmt.Errorf("verify integrity: %w", err)
	}

	if currentHash != expectedHash {
		return fmt.Errorf("integrity mismatch for %q: expected %s, got %s", s.name, expectedHash, currentHash)
	}
	return nil
}

// resolveVersion queries npm for the exact version of a package.
func resolveVersion(source string) (string, error) {
	out, err := execCommandFn("npm", "view", source, "version", "--json").Output()
	if err != nil {
		return "", err
	}
	var version string
	if err := json.Unmarshal(out, &version); err != nil {
		return strings.TrimSpace(string(out)), nil
	}
	return version, nil
}

// getIntegrity queries npm for the package's integrity hash.
func getIntegrity(source string) (string, error) {
	out, err := execCommandFn("npm", "view", source, "dist.integrity", "--json").Output()
	if err != nil {
		return "", err
	}
	var hash string
	if err := json.Unmarshal(out, &hash); err != nil {
		return strings.TrimSpace(string(out)), nil
	}
	return hash, nil
}

// stripVersion removes version suffix from a package spec.
func stripVersion(source string) string {
	if strings.HasPrefix(source, "@") {
		if idx := strings.LastIndex(source, "@"); idx > 0 {
			return source[:idx]
		}
	} else if idx := strings.Index(source, "@"); idx > 0 {
		return source[:idx]
	}
	return source
}

// stripCLISuffix removes -cli suffix from package name.
func stripCLISuffix(pkg string) string {
	return strings.TrimSuffix(pkg, "-cli")
}
