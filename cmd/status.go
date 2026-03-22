package cmd

import (
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/user/claw2cli/internal/executor"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show status of running connectors",
	RunE: func(cmd *cobra.Command, args []string) error {
		connectors, err := executor.ListConnectors()
		if err != nil {
			return err
		}

		if len(connectors) == 0 {
			fmt.Println("No connectors running.")
			return nil
		}

		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "NAME\tPID\tSTATUS\tUPTIME")
		for _, c := range connectors {
			status := "dead"
			uptime := "-"
			if c.Running {
				status = "running"
				if !c.StartedAt.IsZero() {
					uptime = time.Since(c.StartedAt).Truncate(time.Second).String()
				}
			}
			fmt.Fprintf(w, "%s\t%d\t%s\t%s\n", c.Name, c.PID, status, uptime)
		}
		w.Flush()
		return nil
	},
}
