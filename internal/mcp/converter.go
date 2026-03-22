package mcp

import (
	gomcp "github.com/mark3labs/mcp-go/mcp"
	"github.com/user/claw2cli/internal/parser"
)

// ManifestToTool converts a PluginManifest into an MCP Tool definition.
func ManifestToTool(manifest *parser.PluginManifest) gomcp.Tool {
	description := manifest.Source
	if manifest.Skill != nil && manifest.Skill.Description != "" {
		description = manifest.Skill.Description
	}

	opts := []gomcp.ToolOption{
		gomcp.WithDescription(description),
	}

	if manifest.Type == parser.PluginTypeSkill {
		// Skill tools accept arbitrary string arguments
		opts = append(opts,
			gomcp.WithString("args",
				gomcp.Description("Arguments to pass to the skill (space-separated)"),
			),
		)
	} else {
		// Connector tools accept an action and payload
		opts = append(opts,
			gomcp.WithString("action",
				gomcp.Description("Action to perform (e.g., send_message, get_status)"),
				gomcp.Required(),
			),
			gomcp.WithString("payload",
				gomcp.Description("JSON payload for the action"),
			),
		)
	}

	name := manifest.Name
	if manifest.Skill != nil && manifest.Skill.Name != "" {
		name = manifest.Skill.Name
	}

	return gomcp.NewTool(name, opts...)
}
