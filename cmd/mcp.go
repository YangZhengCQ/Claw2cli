package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	c2cmcp "github.com/user/claw2cli/internal/mcp"
	"github.com/user/claw2cli/internal/parser"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "MCP server commands",
}

var mcpSkills []string
var mcpConnectors []string

var mcpServeCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the MCP server over stdio",
	Long:  "Run as a Model Context Protocol server, exposing installed plugins as MCP tools.",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Load all installed plugins
		names, err := parser.ListPlugins()
		if err != nil {
			return fmt.Errorf("list plugins: %w", err)
		}

		if len(names) == 0 {
			return fmt.Errorf("no plugins installed — use 'c2c install <package>' first")
		}

		var manifests []*parser.PluginManifest
		for _, name := range names {
			m, err := parser.LoadPlugin(name)
			if err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "Warning: skip %s: %v\n", name, err)
				continue
			}
			manifests = append(manifests, m)
		}

		// Apply filters
		manifests = c2cmcp.FilterPlugins(manifests, mcpSkills, mcpConnectors)

		if len(manifests) == 0 {
			return fmt.Errorf("no plugins match the given filters")
		}

		return c2cmcp.Serve(manifests)
	},
}

func init() {
	mcpServeCmd.Flags().StringSliceVar(&mcpSkills, "skills", nil, "Only expose these skills")
	mcpServeCmd.Flags().StringSliceVar(&mcpConnectors, "connectors", nil, "Only expose these connectors")
	mcpCmd.AddCommand(mcpServeCmd)
}
