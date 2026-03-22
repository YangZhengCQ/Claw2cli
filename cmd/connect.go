package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/user/claw2cli/internal/executor"
	"github.com/user/claw2cli/internal/parser"
)

var connectCmd = &cobra.Command{
	Use:   "connect <connector>",
	Short: "Start a connector daemon",
	Long:  "Launch a background daemon that maintains a long-lived connection (e.g., WeChat, Feishu).",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		manifest, err := parser.LoadPlugin(name)
		if err != nil {
			return fmt.Errorf("load plugin: %w", err)
		}

		if manifest.Type != parser.PluginTypeConnector {
			return fmt.Errorf("%q is a %s, not a connector — use 'c2c run' instead", name, manifest.Type)
		}

		if err := executor.StartConnector(manifest); err != nil {
			return err
		}

		fmt.Printf("Connector %q started.\n", name)
		fmt.Printf("  Use 'c2c attach %s' to stream messages.\n", name)
		fmt.Printf("  Use 'c2c stop %s' to shut down.\n", name)
		return nil
	},
}
