package nodeutil

import (
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
)

// Package-level function pointers for dependency injection in tests.
var (
	execCommandFn = exec.Command
	lookPathFn    = exec.LookPath
)

// ResolvePluginPackage derives the actual plugin package name from the source.
// CLI wrapper packages like "@tencent-weixin/openclaw-weixin-cli" need the
// runtime package "@tencent-weixin/openclaw-weixin" to be installed instead.
func ResolvePluginPackage(source string) string {
	pkg := source
	if strings.HasPrefix(pkg, "@") {
		if idx := strings.LastIndex(pkg, "@"); idx > 0 {
			pkg = pkg[:idx]
		}
	} else if idx := strings.Index(pkg, "@"); idx > 0 {
		pkg = pkg[:idx]
	}
	pkg = strings.TrimSuffix(pkg, "-cli")
	return pkg
}

// ResolveNodeRunner returns "tsx" if available globally, otherwise "node".
// tsx is needed because OpenClaw plugins are ESM + TypeScript.
// If tsx is not found, it prompts the user for consent before installing globally.
func ResolveNodeRunner() string {
	if _, err := lookPathFn("tsx"); err == nil {
		return "tsx"
	}
	// Check if we're running interactively (stdin is a terminal)
	if stat, err := os.Stdin.Stat(); err != nil || (stat.Mode()&os.ModeCharDevice) == 0 {
		// Non-interactive mode (daemon, CI, pipe) — fail fast
		log.Printf("Warning: tsx not available and running non-interactively, falling back to node (TypeScript plugins may not load)")
		return "node"
	}
	// Prompt user for consent before modifying global npm state
	fmt.Fprintf(os.Stderr, "tsx (TypeScript executor) is required but not found.\n")
	fmt.Fprintf(os.Stderr, "Install it globally via 'npm install -g tsx'? [y/N] ")
	var answer string
	fmt.Scanln(&answer)
	if answer != "y" && answer != "Y" {
		log.Printf("Warning: tsx not available, falling back to node (TypeScript plugins may not load)")
		return "node"
	}
	install := execCommandFn("npm", "install", "-g", "tsx@4.19.4")
	install.Stdout = os.Stderr
	install.Stderr = os.Stderr
	if err := install.Run(); err == nil {
		if tsxPath, err := lookPathFn("tsx"); err == nil {
			return tsxPath
		}
	}
	log.Printf("Warning: tsx installation failed, falling back to node (TypeScript plugins may not load)")
	return "node"
}

// ResolveGlobalNodeModules finds the global npm node_modules directory.
func ResolveGlobalNodeModules() string {
	out, err := execCommandFn("npm", "root", "-g").Output()
	if err != nil {
		return ""
	}
	dir := strings.TrimRight(string(out), "\n")
	return dir
}

// GetNpmChecksum fetches the integrity hash of an npm package from the registry.
func GetNpmChecksum(source string) (string, error) {
	pkg := source
	if strings.HasPrefix(pkg, "@") {
		if idx := strings.LastIndex(pkg, "@"); idx > 0 {
			pkg = pkg[:idx]
		}
	} else if idx := strings.Index(pkg, "@"); idx > 0 {
		pkg = pkg[:idx]
	}

	out, err := exec.Command("npm", "info", pkg, "--json").Output()
	if err != nil {
		return "", err
	}

	var info struct {
		Dist struct {
			Shasum    string `json:"shasum"`
			Integrity string `json:"integrity"`
		} `json:"dist"`
	}
	if err := json.Unmarshal(out, &info); err != nil {
		return "", err
	}

	if info.Dist.Integrity != "" {
		return info.Dist.Integrity, nil
	}
	if info.Dist.Shasum != "" {
		return "sha1:" + info.Dist.Shasum, nil
	}

	h := sha512.Sum512(out)
	return "sha512:" + hex.EncodeToString(h[:]), nil
}

// VerifyChecksum compares the installed package's npm integrity hash
// against the expected checksum from the manifest. Returns nil if match
// or if no checksum was recorded (graceful degradation).
func VerifyChecksum(source, expected string) error {
	if expected == "" {
		return nil
	}
	current, err := GetNpmChecksum(source)
	if err != nil {
		return fmt.Errorf("checksum verification failed for %s: %w", source, err)
	}
	if current != expected {
		return fmt.Errorf("checksum mismatch for %s: expected %s, got %s (package may have been tampered with)", source, expected, current)
	}
	return nil
}

// EnsurePluginInstalled makes sure the npm plugin package is available globally.
// It installs both the source package (CLI wrapper) and the actual runtime plugin.
func EnsurePluginInstalled(source string) error {
	pkgs := []string{source}

	runtimePkg := ResolvePluginPackage(source)
	if runtimePkg != "" && runtimePkg != strings.TrimSuffix(source, "@latest") {
		pkgs = append(pkgs, runtimePkg)
	}

	for _, pkg := range pkgs {
		cmd := execCommandFn("npm", "list", "-g", pkg, "--depth=0")
		if err := cmd.Run(); err != nil {
			log.Printf("Installing plugin package: %s", pkg)
			install := execCommandFn("npm", "install", "-g", pkg)
			install.Stdout = os.Stderr
			install.Stderr = os.Stderr
			if err := install.Run(); err != nil {
				return fmt.Errorf("npm install -g %s: %w", pkg, err)
			}
		}
	}
	return nil
}
