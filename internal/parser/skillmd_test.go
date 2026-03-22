package parser

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseSkillMD(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "skills", "google-search", "SKILL.md")
	skill, body, err := ParseSkillMD(path)
	if err != nil {
		t.Fatalf("ParseSkillMD failed: %v", err)
	}

	if skill.Name != "google-search" {
		t.Errorf("name=%q, want google-search", skill.Name)
	}
	if skill.Description != "Search the web using Google" {
		t.Errorf("description=%q, want 'Search the web using Google'", skill.Description)
	}
	if skill.Emoji == "" {
		t.Error("emoji should not be empty")
	}
	if len(skill.Requirements) != 1 || skill.Requirements[0] != "node" {
		t.Errorf("requirements=%v, want [node]", skill.Requirements)
	}
	if body == "" {
		t.Error("body should not be empty")
	}
}

func TestParseSkillMD_NoFrontmatter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "SKILL.md")
	os.WriteFile(path, []byte("# Just markdown\nNo frontmatter here."), 0644)

	_, _, err := ParseSkillMD(path)
	if err == nil {
		t.Error("expected error for missing frontmatter")
	}
}

func TestParseSkillMD_EmptyBody(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "SKILL.md")
	content := "---\nname: test\ndescription: test skill\n---\n"
	os.WriteFile(path, []byte(content), 0644)

	skill, body, err := ParseSkillMD(path)
	if err != nil {
		t.Fatalf("ParseSkillMD failed: %v", err)
	}
	if skill.Name != "test" {
		t.Errorf("name=%q, want test", skill.Name)
	}
	if body != "" {
		t.Errorf("body=%q, want empty", body)
	}
}

func TestParseSkillMD_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "SKILL.md")
	content := "---\n[[[\n---\n# Body\n"
	os.WriteFile(path, []byte(content), 0644)

	_, _, err := ParseSkillMD(path)
	if err == nil {
		t.Error("expected error for invalid YAML frontmatter")
	}
}

func TestSplitFrontmatter_NoClosing(t *testing.T) {
	_, _, err := splitFrontmatter("---\nname: test\nno closing")
	if err == nil {
		t.Error("expected error for no closing ---")
	}
}
