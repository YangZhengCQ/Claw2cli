package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/user/claw2cli/internal/paths"
)

var rootCmd = &cobra.Command{
	Use:   "c2c",
	Short: "Claw2Cli — wrap OpenClaw plugins as standard CLI tools",
	Long: `c2c extracts high-quality plugins from the OpenClaw ecosystem
and makes them available as plain CLI commands or MCP tools.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		return paths.EnsureDirs()
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(connectCmd)
	rootCmd.AddCommand(stopCmd)
	rootCmd.AddCommand(attachCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(infoCmd)
	rootCmd.AddCommand(installCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(logsCmd)
	rootCmd.AddCommand(mcpCmd)
}
