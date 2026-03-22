package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/user/claw2cli/internal/executor"
	"github.com/user/claw2cli/internal/parser"
)

var runTimeout int

var runCmd = &cobra.Command{
	Use:   "run <skill> [-- args...]",
	Short: "Run a skill plugin",
	Long:  "Execute a skill plugin as a one-shot subprocess and return the result.",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		pluginArgs := args[1:]

		manifest, err := parser.LoadPlugin(name)
		if err != nil {
			return fmt.Errorf("load plugin: %w", err)
		}

		if manifest.Type != parser.PluginTypeSkill {
			return fmt.Errorf("%q is a %s, not a skill — use 'c2c connect' instead", name, manifest.Type)
		}

		timeout := time.Duration(runTimeout) * time.Second
		result, err := executor.RunSkill(context.Background(), manifest, pluginArgs, timeout)
		if err != nil {
			return err
		}

		if result.Output != nil {
			// Pretty-print JSON output
			var pretty json.RawMessage
			if json.Unmarshal(result.Output, &pretty) == nil {
				formatted, _ := json.MarshalIndent(pretty, "", "  ")
				fmt.Println(string(formatted))
			}
		} else if result.Stdout != "" {
			fmt.Print(result.Stdout)
		}

		if result.Stderr != "" {
			fmt.Fprint(cmd.ErrOrStderr(), result.Stderr)
		}

		if result.ExitCode != 0 {
			return fmt.Errorf("skill exited with code %d", result.ExitCode)
		}

		return nil
	},
}

func init() {
	runCmd.Flags().IntVar(&runTimeout, "timeout", 30, "Execution timeout in seconds")
}
