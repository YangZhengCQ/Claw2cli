package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	gomcp "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/user/claw2cli/internal/parser"
	"github.com/user/claw2cli/internal/protocol"
)

// Serve starts the MCP server over stdio, exposing the given plugins as tools.
func Serve(plugins []*parser.PluginManifest) error {
	mcpServer := server.NewMCPServer(
		"claw2cli",
		"0.1.0",
		server.WithToolCapabilities(true),
	)

	// Register each plugin as a static MCP tool
	for _, manifest := range plugins {
		tool := ManifestToTool(manifest)
		handler := makeHandler(manifest)
		mcpServer.AddTool(tool, handler)
	}

	// Register dynamic tools from running connectors
	registerDynamicTools(mcpServer, plugins)

	return server.ServeStdio(mcpServer)
}

// registerDynamicTools queries running connectors for discovered tools and registers them.
func registerDynamicTools(mcpServer *server.MCPServer, plugins []*parser.PluginManifest) {
	for _, manifest := range plugins {
		if manifest.Type != parser.PluginTypeConnector {
			continue
		}
		name := manifest.Name

		tools, err := DiscoverTools(name)
		if err != nil || len(tools) == 0 {
			continue
		}

		for _, ts := range tools {
			tool := toolSchemaToMCPTool(ts)
			connName := name
			toolName := ts.Name
			handler := func(ctx context.Context, request gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
				// Extract all arguments as JSON
				argsJSON, _ := json.Marshal(request.Params.Arguments)
				result, err := InvokeTool(connName, toolName, argsJSON)
				if err != nil {
					return &gomcp.CallToolResult{
						Content: []gomcp.Content{
							gomcp.TextContent{Type: "text", Text: fmt.Sprintf("Error: %v", err)},
						},
						IsError: true,
					}, nil
				}
				return &gomcp.CallToolResult{
					Content: []gomcp.Content{
						gomcp.TextContent{Type: "text", Text: string(result)},
					},
				}, nil
			}
			mcpServer.AddTool(tool, handler)
		}
	}
}

// toolSchemaToMCPTool converts a protocol.ToolSchema to an mcp-go Tool.
func toolSchemaToMCPTool(ts protocol.ToolSchema) gomcp.Tool {
	tool := gomcp.Tool{
		Name:        ts.Name,
		Description: ts.Description,
	}
	if len(ts.InputSchema) > 0 {
		var schema gomcp.ToolInputSchema
		if json.Unmarshal(ts.InputSchema, &schema) == nil {
			schema.Type = "object"
			tool.InputSchema = schema
		}
	}
	return tool
}

// makeHandler creates a tool handler function for a specific plugin.
func makeHandler(manifest *parser.PluginManifest) server.ToolHandlerFunc {
	return func(ctx context.Context, request gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		switch manifest.Type {
		case parser.PluginTypeSkill:
			return handleSkill(ctx, manifest, request)
		case parser.PluginTypeConnector:
			return handleConnector(manifest, request)
		default:
			return nil, fmt.Errorf("unknown plugin type: %s", manifest.Type)
		}
	}
}

func handleSkill(ctx context.Context, manifest *parser.PluginManifest, request gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
	argsStr := request.GetString("args", "")
	var args []string
	if argsStr != "" {
		args = strings.Fields(argsStr)
	}

	result, err := runSkillFn(ctx, manifest, args, 30*time.Second)
	if err != nil {
		return &gomcp.CallToolResult{
			Content: []gomcp.Content{
				gomcp.TextContent{Type: "text", Text: fmt.Sprintf("Error: %v", err)},
			},
			IsError: true,
		}, nil
	}

	// Return JSON output if available, otherwise raw stdout
	output := result.Stdout
	if result.Output != nil {
		formatted, _ := json.MarshalIndent(result.Output, "", "  ")
		output = string(formatted)
	}

	return &gomcp.CallToolResult{
		Content: []gomcp.Content{
			gomcp.TextContent{Type: "text", Text: output},
		},
	}, nil
}

func handleConnector(manifest *parser.PluginManifest, request gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
	action := request.GetString("action", "")
	if action == "" {
		return nil, fmt.Errorf("action is required for connector tools")
	}

	switch action {
	case "start":
		if err := startConnectorFn(manifest); err != nil {
			return &gomcp.CallToolResult{
				Content: []gomcp.Content{
					gomcp.TextContent{Type: "text", Text: fmt.Sprintf("Error starting: %v", err)},
				},
				IsError: true,
			}, nil
		}
		return &gomcp.CallToolResult{
			Content: []gomcp.Content{
				gomcp.TextContent{Type: "text", Text: fmt.Sprintf("Connector %q started", manifest.Name)},
			},
		}, nil

	case "stop":
		if err := stopConnectorFn(manifest.Name); err != nil {
			return &gomcp.CallToolResult{
				Content: []gomcp.Content{
					gomcp.TextContent{Type: "text", Text: fmt.Sprintf("Error stopping: %v", err)},
				},
				IsError: true,
			}, nil
		}
		return &gomcp.CallToolResult{
			Content: []gomcp.Content{
				gomcp.TextContent{Type: "text", Text: fmt.Sprintf("Connector %q stopped", manifest.Name)},
			},
		}, nil

	case "status":
		status, err := getConnectorStatusFn(manifest.Name)
		if err != nil {
			return &gomcp.CallToolResult{
				Content: []gomcp.Content{
					gomcp.TextContent{Type: "text", Text: fmt.Sprintf("Not running: %v", err)},
				},
			}, nil
		}
		data, _ := json.MarshalIndent(status, "", "  ")
		return &gomcp.CallToolResult{
			Content: []gomcp.Content{
				gomcp.TextContent{Type: "text", Text: string(data)},
			},
		}, nil

	default:
		// Forward arbitrary actions to the connector via UDS
		conn, err := attachConnectorFn(manifest.Name)
		if err != nil {
			return &gomcp.CallToolResult{
				Content: []gomcp.Content{
					gomcp.TextContent{Type: "text", Text: fmt.Sprintf("Cannot connect: %v", err)},
				},
				IsError: true,
			}, nil
		}
		defer conn.Close()

		payloadStr := request.GetString("payload", "{}")
		cmd := fmt.Sprintf(`{"type":"command","source":"%s","action":"%s","payload":%s,"id":"mcp-%d"}`,
			manifest.Name, action, payloadStr, time.Now().UnixNano())
		conn.Write([]byte(cmd + "\n"))

		// Read one response
		buf := make([]byte, 65536)
		n, err := conn.Read(buf)
		if err != nil {
			return &gomcp.CallToolResult{
				Content: []gomcp.Content{
					gomcp.TextContent{Type: "text", Text: "Command sent, no response received"},
				},
			}, nil
		}

		return &gomcp.CallToolResult{
			Content: []gomcp.Content{
				gomcp.TextContent{Type: "text", Text: string(buf[:n])},
			},
		}, nil
	}
}
