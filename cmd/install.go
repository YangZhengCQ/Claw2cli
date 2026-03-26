package cmd

import (
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/YangZhengCQ/Claw2cli/internal/parser"
	"github.com/YangZhengCQ/Claw2cli/internal/paths"
	"github.com/YangZhengCQ/Claw2cli/internal/store"
	"gopkg.in/yaml.v3"
)

var pluginType string
var skipVerify bool

var installCmd = &cobra.Command{
	Use:   "install <package>",
	Short: "Install an OpenClaw plugin",
	Long:  "Install an npm-based OpenClaw plugin and generate a c2c manifest.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		source := args[0]
		return installPlugin(source, pluginType)
	},
}

func init() {
	installCmd.Flags().StringVar(&pluginType, "type", "skill", "Plugin type: skill or connector")
	installCmd.Flags().BoolVar(&skipVerify, "skip-verify", false, "Skip checksum verification (not recommended)")
}

func installPlugin(source, pType string) error {
	if pType != "skill" && pType != "connector" {
		return fmt.Errorf("invalid plugin type %q: must be 'skill' or 'connector'", pType)
	}

	// Pre-flight: check that node and npm are available
	if err := checkNodeNpm(); err != nil {
		return err
	}

	// Pre-flight: verify the shim files are bundled correctly
	if err := checkShimFiles(); err != nil {
		return err
	}

	// Ensure all base directories exist (~/.c2c/plugins, sockets, pids, etc.)
	if err := paths.EnsureDirs(); err != nil {
		return fmt.Errorf("create base dirs: %w", err)
	}

	// Derive plugin name from source
	name := derivePluginName(source)
	if err := paths.ValidateName(name); err != nil {
		return fmt.Errorf("invalid plugin name derived from %q: %w", source, err)
	}

	// Check for existing plugin with the same name
	pluginDir := paths.PluginDir(name)
	if _, err := os.Stat(pluginDir); err == nil {
		return fmt.Errorf("plugin %q already exists — uninstall it first or choose a different name", name)
	}

	fmt.Printf("Installing %q as %q (%s)...\n", source, name, pType)

	// Get package info and checksum from npm
	checksum, err := getNpmChecksum(source)
	if err != nil {
		if !skipVerify {
			return fmt.Errorf("could not verify package integrity: %w\n  To install without verification, use: c2c install --skip-verify %s", err, source)
		}
		fmt.Fprintf(os.Stderr, "Warning: skipping integrity verification: %v\n", err)
		checksum = ""
	}

	// Create plugin directory
	if err := os.MkdirAll(pluginDir, 0700); err != nil {
		return fmt.Errorf("create plugin dir: %w", err)
	}

	// Build default permissions
	perms := []parser.Permission{"network"}
	if pType == "connector" {
		perms = append(perms,
			parser.Permission(fmt.Sprintf("fs:%s", paths.StorageDir(name))),
		)
	}

	// Generate manifest.yaml
	manifest := parser.PluginManifest{
		Source:      source,
		Type:        parser.PluginType(pType),
		Permissions: perms,
		Checksum:    checksum,
	}

	manifestData, err := yaml.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}

	manifestPath := filepath.Join(pluginDir, "manifest.yaml")
	if err := os.WriteFile(manifestPath, manifestData, 0600); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}

	// Create storage directory with 0700
	if err := paths.EnsureStorageDir(name); err != nil {
		return fmt.Errorf("create storage dir: %w", err)
	}

	// Install packages locally
	s := store.New(name)
	resolvedVersion, integrity, err := s.Install(source)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: local install failed (will retry on connect): %v\n", err)
	} else {
		// Update manifest with resolved version and integrity
		manifest.ResolvedVersion = resolvedVersion
		manifest.Integrity = integrity
		manifestData, _ = yaml.Marshal(manifest)
		os.WriteFile(manifestPath, manifestData, 0600)
	}

	// Ensure tsx is available
	if _, err := store.EnsureTsx(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not install tsx: %v\n", err)
	}

	fmt.Printf("Installed %q.\n", name)
	fmt.Printf("  Manifest: %s\n", manifestPath)
	fmt.Printf("  Storage:  %s\n", paths.StorageDir(name))
	if pType == "connector" {
		fmt.Printf("\nTo start: c2c connect %s\n", name)
	} else {
		fmt.Printf("\nTo run:   c2c run %s [args...]\n", name)
	}
	return nil
}

// checkNodeNpm verifies that node and npm are available on PATH.
func checkNodeNpm() error {
	if _, err := exec.LookPath("node"); err != nil {
		return fmt.Errorf("node not found in PATH — install Node.js first: https://nodejs.org")
	}
	if _, err := exec.LookPath("npm"); err != nil {
		return fmt.Errorf("npm not found in PATH — install Node.js first: https://nodejs.org")
	}
	return nil
}

// checkShimFiles verifies the shim entry point and fake SDK exist.
func checkShimFiles() error {
	shim := shimDir()
	entry := filepath.Join(shim, "c2c-shim.js")
	if _, err := os.Stat(entry); os.IsNotExist(err) {
		return fmt.Errorf("shim not found at %s — is c2c installed correctly?", entry)
	}
	fakeSdk := filepath.Join(shim, "node_modules", "@openclaw", "plugin-sdk", "index.js")
	if _, err := os.Stat(fakeSdk); os.IsNotExist(err) {
		return fmt.Errorf("fake plugin-sdk not found at %s — is c2c installed correctly?", fakeSdk)
	}
	return nil
}

// derivePluginName extracts a short name from an npm package specifier.
// "@tencent-weixin/openclaw-weixin-cli@latest" -> "wechat"
// "@scope/name@version" -> "name"
// "simple-package" -> "simple-package"
func derivePluginName(source string) string {
	// Strip version
	s := source
	if idx := strings.LastIndex(s, "@"); idx > 0 {
		s = s[:idx]
	}
	// Strip scope
	if strings.HasPrefix(s, "@") {
		parts := strings.SplitN(s, "/", 2)
		if len(parts) == 2 {
			s = parts[1]
		}
	}
	// Apply known aliases
	aliases := map[string]string{
		"openclaw-weixin-cli": "wechat",
		"openclaw-feishu-cli": "feishu",
		"openclaw-lark":       "feishu",
	}
	if alias, ok := aliases[s]; ok {
		return alias
	}
	// Strip "openclaw-" prefix if present
	s = strings.TrimPrefix(s, "openclaw-")
	return s
}

// getNpmChecksum runs `npm info` to get the package's shasum.
func getNpmChecksum(source string) (string, error) {
	// Query npm info for the exact package spec (including version if specified)
	out, err := exec.Command("npm", "info", source, "--json").Output()
	if err != nil {
		return "", err
	}

	var info struct {
		Dist struct {
			Shasum   string `json:"shasum"`
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

	// Fallback: compute our own hash from the npm info output
	h := sha512.Sum512(out)
	return "sha512:" + hex.EncodeToString(h[:]), nil
}
