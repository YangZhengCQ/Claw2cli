package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/YangZhengCQ/Claw2cli/internal/executor"
	"github.com/YangZhengCQ/Claw2cli/internal/nodeutil"
	"github.com/YangZhengCQ/Claw2cli/internal/parser"
	"github.com/YangZhengCQ/Claw2cli/internal/paths"
	"gopkg.in/yaml.v3"
)

var checkOnly bool

var updateCmd = &cobra.Command{
	Use:   "update [plugin]",
	Short: "Update installed plugins to the latest version",
	Long: `Check for and install updates for installed plugins.

Examples:
  c2c update              # update all installed plugins
  c2c update wechat       # update specific plugin
  c2c update --check      # check for updates without installing`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 1 {
			return updatePlugin(args[0])
		}
		return updateAll()
	},
}

func init() {
	updateCmd.Flags().BoolVar(&checkOnly, "check", false, "Check for updates without installing")
}

func updateAll() error {
	names, err := parser.ListPlugins()
	if err != nil {
		return fmt.Errorf("list plugins: %w", err)
	}
	if len(names) == 0 {
		fmt.Println("No plugins installed.")
		return nil
	}

	updated := 0
	for _, name := range names {
		manifest, err := parser.LoadPlugin(name)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: %s: %v\n", name, err)
			continue
		}
		changed, err := updateSinglePlugin(manifest)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: %s: %v\n", name, err)
			continue
		}
		if changed {
			updated++
		}
	}

	if updated == 0 {
		fmt.Println("All plugins are up to date.")
	} else {
		fmt.Printf("\nUpdated %d plugin(s).\n", updated)
	}
	return nil
}

func updatePlugin(name string) error {
	manifest, err := parser.LoadPlugin(name)
	if err != nil {
		return fmt.Errorf("plugin %q not found: %w", name, err)
	}
	changed, err := updateSinglePlugin(manifest)
	if err != nil {
		return err
	}
	if !changed {
		fmt.Printf("%s is up to date.\n", name)
	}
	return nil
}

func updateSinglePlugin(manifest *parser.PluginManifest) (bool, error) {
	fmt.Printf("Checking %s (%s)...\n", manifest.Name, manifest.Source)

	newChecksum, err := nodeutil.GetNpmChecksum(manifest.Source)
	if err != nil {
		return false, fmt.Errorf("fetch checksum: %w", err)
	}

	if newChecksum == manifest.Checksum {
		return false, nil
	}

	fmt.Printf("  Update available: checksum changed\n")
	fmt.Printf("    old: %s\n", truncate(manifest.Checksum, 40))
	fmt.Printf("    new: %s\n", truncate(newChecksum, 40))

	if checkOnly {
		return true, nil
	}

	// Install the updated package
	fmt.Printf("  Installing update...\n")
	if err := nodeutil.EnsurePluginInstalled(manifest.Source); err != nil {
		return false, fmt.Errorf("install: %w", err)
	}

	// Update manifest with new checksum
	manifest.Checksum = newChecksum
	manifestData, err := yaml.Marshal(struct {
		Source      string            `yaml:"source"`
		Type        parser.PluginType `yaml:"type"`
		Permissions []parser.Permission `yaml:"permissions"`
		Checksum    string            `yaml:"checksum"`
	}{
		Source:      manifest.Source,
		Type:        manifest.Type,
		Permissions: manifest.Permissions,
		Checksum:    newChecksum,
	})
	if err != nil {
		return false, fmt.Errorf("marshal manifest: %w", err)
	}

	manifestPath := filepath.Join(paths.PluginDir(manifest.Name), "manifest.yaml")
	if err := os.WriteFile(manifestPath, manifestData, 0644); err != nil {
		return false, fmt.Errorf("write manifest: %w", err)
	}

	fmt.Printf("  Updated %s.\n", manifest.Name)

	// Warn if connector is running
	if manifest.Type == parser.PluginTypeConnector {
		status, err := executor.GetConnectorStatus(manifest.Name)
		if err == nil && status.Running {
			fmt.Printf("  Note: %s is running (PID %d). Restart with: c2c stop %s && c2c connect %s\n",
				manifest.Name, status.PID, manifest.Name, manifest.Name)
		}
	}

	return true, nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
