package parser

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/YangZhengCQ/Claw2cli/internal/paths"
	"gopkg.in/yaml.v3"
)

// ParseManifest reads a c2c manifest.yaml file.
func ParseManifest(path string) (*PluginManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}

	manifest := &PluginManifest{}
	if err := yaml.Unmarshal(data, manifest); err != nil {
		return nil, fmt.Errorf("unmarshal manifest: %w", err)
	}

	// Validate required fields
	name := manifest.Name
	if name == "" {
		name = path
	}
	if manifest.Source == "" {
		return nil, fmt.Errorf("manifest for %q: 'source' field is required", name)
	}
	if manifest.Type == "" {
		return nil, fmt.Errorf("manifest for %q: 'type' field is required", name)
	}
	if manifest.Type != PluginTypeSkill && manifest.Type != PluginTypeConnector {
		return nil, fmt.Errorf("manifest for %q: unknown type %q (must be 'skill' or 'connector')", name, manifest.Type)
	}

	return manifest, nil
}

// LoadPlugin loads a plugin by name from ~/.c2c/plugins/<name>/.
// It reads both manifest.yaml and SKILL.md (if present).
func LoadPlugin(name string) (*PluginManifest, error) {
	if err := paths.ValidateName(name); err != nil {
		return nil, err
	}
	pluginDir := paths.PluginDir(name)

	manifestPath := filepath.Join(pluginDir, "manifest.yaml")
	manifest, err := ParseManifest(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("load plugin %s: %w", name, err)
	}

	manifest.Name = name
	manifest.InstallPath = pluginDir

	// Try to load SKILL.md if it exists
	skillPath := filepath.Join(pluginDir, "SKILL.md")
	if _, statErr := os.Stat(skillPath); statErr == nil {
		skill, body, parseErr := ParseSkillMD(skillPath)
		if parseErr == nil {
			manifest.Skill = skill
			manifest.SkillBody = body
		} else {
			fmt.Fprintf(os.Stderr, "warning: could not parse SKILL.md for %q: %v\n", name, parseErr)
		}
	}

	return manifest, nil
}

// ListPlugins returns the names of all installed plugins.
func ListPlugins() ([]string, error) {
	entries, err := os.ReadDir(paths.PluginsDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		// Only include directories that have a manifest.yaml
		manifestPath := filepath.Join(paths.PluginDir(e.Name()), "manifest.yaml")
		if _, statErr := os.Stat(manifestPath); statErr == nil {
			names = append(names, e.Name())
		}
	}
	return names, nil
}
