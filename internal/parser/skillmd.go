package parser

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// ParseSkillMD reads a SKILL.md file and extracts the YAML frontmatter and markdown body.
func ParseSkillMD(path string) (*SkillMetadata, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("read skill.md: %w", err)
	}

	content := string(data)
	meta, body, err := splitFrontmatter(content)
	if err != nil {
		return nil, "", fmt.Errorf("parse skill.md frontmatter: %w", err)
	}

	skill := &SkillMetadata{}
	if err := yaml.Unmarshal([]byte(meta), skill); err != nil {
		return nil, "", fmt.Errorf("unmarshal skill.md frontmatter: %w", err)
	}

	return skill, body, nil
}

// splitFrontmatter splits a document with --- delimited YAML frontmatter.
// Returns the YAML string and the remaining markdown body.
func splitFrontmatter(content string) (string, string, error) {
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, "---") {
		return "", content, fmt.Errorf("no frontmatter found (must start with ---)")
	}

	// Find the closing ---
	rest := content[3:]
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return "", content, fmt.Errorf("no closing --- found for frontmatter")
	}

	frontmatter := strings.TrimSpace(rest[:idx])
	body := strings.TrimSpace(rest[idx+4:]) // skip \n---

	return frontmatter, body, nil
}
