package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/user/claw2cli/internal/parser"
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

		return nil
	},
}
