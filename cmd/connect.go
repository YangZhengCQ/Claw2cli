package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/YangZhengCQ/Claw2cli/internal/executor"
	"github.com/YangZhengCQ/Claw2cli/internal/parser"
)

var foregroundMode bool

var connectCmd = &cobra.Command{
	Use:   "connect <connector>",
	Short: "Start a connector daemon",
	Long: `Launch a background daemon that maintains a long-lived connection (e.g., WeChat, Feishu).

By default runs as a background daemon. Use -f for foreground mode (debugging, QR login).`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		manifest, err := parser.LoadPlugin(name)
		if err != nil {
			return fmt.Errorf("load plugin: %w", err)
		}

		if manifest.Type != parser.PluginTypeConnector {
			return fmt.Errorf("%q is a %s, not a connector — use 'c2c run' instead", name, manifest.Type)
		}

		if foregroundMode {
			// Foreground mode (debugging): run _daemon directly so user sees QR codes etc.
			fmt.Printf("Starting %q in foreground (Ctrl+C to stop)...\n", name)
			isForeground = true
			return runDaemon(name)
		}

		// Background daemon mode (default)
		if err := executor.StartConnector(manifest); err != nil {
			return err
		}
		fmt.Printf("Connector %q started.\n", name)
		fmt.Printf("  Use 'c2c attach %s' to stream messages.\n", name)
		fmt.Printf("  Use 'c2c stop %s' to shut down.\n", name)
		return nil
	},
}

func init() {
	connectCmd.Flags().BoolVarP(&foregroundMode, "foreground", "f", false, "Run in foreground (for debugging and QR login)")
	connectCmd.Flags().BoolVar(&noSandbox, "no-sandbox", false, "Disable OS-level sandboxing")
}
