package cmd

import (
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/user/claw2cli/internal/executor"
	"github.com/user/claw2cli/internal/parser"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List installed plugins and running connectors",
	RunE: func(cmd *cobra.Command, args []string) error {
		plugins, err := parser.ListPlugins()
		if err != nil {
			return err
		}

		if len(plugins) == 0 {
			fmt.Println("No plugins installed. Use 'c2c install <package>' to add one.")
			return nil
		}

		connectors, _ := executor.ListConnectors()
		runningSet := map[string]int{}
		for _, c := range connectors {
			if c.Running {
				runningSet[c.Name] = c.PID
			}
		}

		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "NAME\tTYPE\tSTATUS\tSOURCE")
		for _, name := range plugins {
			manifest, err := parser.LoadPlugin(name)
			if err != nil {
				continue
			}
			status := "-"
			if manifest.Type == parser.PluginTypeConnector {
				if pid, ok := runningSet[name]; ok {
					status = fmt.Sprintf("running (PID %d)", pid)
				} else {
					status = "stopped"
				}
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", name, manifest.Type, status, manifest.Source)
		}
		w.Flush()
		return nil
	},
}
