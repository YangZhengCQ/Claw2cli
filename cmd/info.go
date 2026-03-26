package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/YangZhengCQ/Claw2cli/internal/executor"
	"github.com/YangZhengCQ/Claw2cli/internal/parser"
	"github.com/YangZhengCQ/Claw2cli/internal/protocol"
)

var infoCmd = &cobra.Command{
	Use:   "info <plugin>",
	Short: "Show details about an installed plugin",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		manifest, err := parser.LoadPlugin(name)
		if err != nil {
			return fmt.Errorf("plugin %q not found: %w", name, err)
		}

		fmt.Printf("Name:        %s\n", manifest.Name)
		fmt.Printf("Type:        %s\n", manifest.Type)
		fmt.Printf("Source:      %s\n", manifest.Source)
		fmt.Printf("Install:     %s\n", manifest.InstallPath)
		fmt.Printf("Checksum:    %s\n", manifest.Checksum)

		if len(manifest.Permissions) > 0 {
			fmt.Println("Permissions:")
			for _, p := range manifest.Permissions {
				fmt.Printf("  - %s\n", p)
			}
		}

		if manifest.Skill != nil {
			fmt.Println()
			if manifest.Skill.Emoji != "" {
				fmt.Printf("%s %s\n", manifest.Skill.Emoji, manifest.Skill.Name)
			}
			if manifest.Skill.Description != "" {
				fmt.Printf("Description: %s\n", manifest.Skill.Description)
			}
		}

		if manifest.SkillBody != "" {
			fmt.Println()
			fmt.Println("--- SKILL.md ---")
			fmt.Println(manifest.SkillBody)
		}

		// If connector is running, query discovered tools via UDS
		if manifest.Type == parser.PluginTypeConnector {
			tools := queryDiscoveredTools(name)
			if len(tools) > 0 {
				fmt.Printf("\nDiscovered Tools (%d):\n", len(tools))
				for _, t := range tools {
					fmt.Printf("  %-30s %s\n", t.Name, t.Description)
				}
			}
		}

		return nil
	},
}

func queryDiscoveredTools(name string) []protocol.ToolSchema {
	conn, err := executor.AttachConnector(name)
	if err != nil {
		return nil
	}
	defer conn.Close()

	reqID := fmt.Sprintf("info-%d", time.Now().UnixNano())
	msg := protocol.NewCommand("c2c-info", "list_tools", reqID, nil)
	data, err := json.Marshal(msg)
	if err != nil {
		return nil
	}
	if _, err := conn.Write(append(data, '\n')); err != nil {
		return nil
	}

	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	deadline := time.After(3 * time.Second)
	type discoverResult struct {
		tools []protocol.ToolSchema
		err   error
	}
	resultCh := make(chan discoverResult, 1)

	go func() {
		for scanner.Scan() {
			var resp protocol.Message
			if json.Unmarshal(scanner.Bytes(), &resp) != nil {
				continue
			}
			if resp.ID == reqID && resp.Type == protocol.TypeError {
				resultCh <- discoverResult{err: fmt.Errorf("[%s] %s", resp.Code, resp.MessageStr)}
				return
			}
			if resp.ID == reqID && resp.Type == protocol.TypeResponse {
				var dp protocol.DiscoveryPayload
				if json.Unmarshal(resp.Payload, &dp) == nil {
					resultCh <- discoverResult{tools: dp.Tools}
				}
				return
			}
		}
	}()

	select {
	case r := <-resultCh:
		if r.err != nil {
			fmt.Fprintf(os.Stderr, "warning: tool discovery failed: %v\n", r.err)
			return nil
		}
		return r.tools
	case <-deadline:
		return nil
	}
}
