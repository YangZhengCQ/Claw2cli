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
	"github.com/user/claw2cli/internal/parser"
	"github.com/user/claw2cli/internal/paths"
	"gopkg.in/yaml.v3"
)

var pluginType string

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
}

func installPlugin(source, pType string) error {
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

	fmt.Printf("Installing %q as %q (%s)...\n", source, name, pType)

	// Get package info and checksum from npm
	checksum, err := getNpmChecksum(source)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not get checksum: %v\n", err)
		checksum = ""
	}

	// Create plugin directory
	pluginDir := paths.PluginDir(name)
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
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
	if err := os.WriteFile(manifestPath, manifestData, 0644); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}

	// Create storage directory with 0700
	if err := paths.EnsureStorageDir(name); err != nil {
		return fmt.Errorf("create storage dir: %w", err)
	}

	// For connectors, pre-install the npm package globally so `c2c connect` starts faster
	if pType == "connector" {
		fmt.Printf("Pre-installing npm package globally...\n")
		if err := preInstallPackage(source); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: pre-install failed (will retry on connect): %v\n", err)
		}
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

// preInstallPackage runs `npm install -g` to cache both the CLI wrapper
// and the actual runtime plugin package ahead of time.
func preInstallPackage(source string) error {
	// Install the source package (CLI wrapper)
	cmd := exec.Command("npm", "install", "-g", source)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}

	// Also install the runtime plugin if different (e.g. strip -cli suffix)
	runtimePkg := resolvePluginPackage(source)
	stripVersion := source
	if strings.HasPrefix(stripVersion, "@") {
		if idx := strings.LastIndex(stripVersion, "@"); idx > 0 {
			stripVersion = stripVersion[:idx]
		}
	}
	if runtimePkg != "" && runtimePkg != stripVersion {
		fmt.Printf("Pre-installing runtime package: %s\n", runtimePkg)
		cmd2 := exec.Command("npm", "install", "-g", runtimePkg)
		cmd2.Stdout = os.Stderr
		cmd2.Stderr = os.Stderr
		if err := cmd2.Run(); err != nil {
			return err
		}
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
	// Strip version for npm info
	pkg := source
	if idx := strings.LastIndex(pkg, "@"); idx > 0 {
		pkg = pkg[:idx]
	}

	out, err := exec.Command("npm", "info", pkg, "--json").Output()
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
