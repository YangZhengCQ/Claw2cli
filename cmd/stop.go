package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/user/claw2cli/internal/executor"
)

var stopCmd = &cobra.Command{
	Use:   "stop <connector>",
	Short: "Stop a running connector daemon",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		if err := executor.StopConnector(name); err != nil {
			return err
		}
		fmt.Printf("Connector %q stopped.\n", name)
		return nil
	},
}
