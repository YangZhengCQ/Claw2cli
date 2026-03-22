package parser

// PluginType distinguishes skill plugins from connector plugins.
type PluginType string

const (
	PluginTypeSkill     PluginType = "skill"
	PluginTypeConnector PluginType = "connector"
)

// Permission represents a declared capability (e.g., "network", "fs:~/.c2c/storage/wechat").
type Permission string

// SkillMetadata is parsed from SKILL.md YAML frontmatter.
type SkillMetadata struct {
	Name         string   `yaml:"name"`
	Description  string   `yaml:"description"`
	Emoji        string   `yaml:"emoji,omitempty"`
	Requirements []string `yaml:"requirements,omitempty"`
}

// PluginManifest is parsed from c2c's own manifest.yaml.
type PluginManifest struct {
	Source      string       `yaml:"source"`
	Type        PluginType   `yaml:"type"`
	Permissions []Permission `yaml:"permissions"`
	Checksum    string       `yaml:"checksum"`

	// Populated at runtime, not serialized to YAML.
	Skill       *SkillMetadata `yaml:"-"`
	SkillBody   string         `yaml:"-"` // markdown body from SKILL.md
	Name        string         `yaml:"-"` // derived from directory name
	InstallPath string         `yaml:"-"` // absolute path to plugin dir
}
