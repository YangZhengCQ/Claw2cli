package parser

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/YangZhengCQ/Claw2cli/internal/paths"
)

func TestParseManifest(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "manifests", "wechat", "manifest.yaml")
	manifest, err := ParseManifest(path)
	if err != nil {
		t.Fatalf("ParseManifest failed: %v", err)
	}

	if manifest.Source != "@tencent-weixin/openclaw-weixin-cli@latest" {
		t.Errorf("source=%q", manifest.Source)
	}
	if manifest.Type != PluginTypeConnector {
		t.Errorf("type=%q, want connector", manifest.Type)
	}
	if len(manifest.Permissions) != 3 {
		t.Errorf("permissions count=%d, want 3", len(manifest.Permissions))
	}
	if manifest.Checksum == "" {
		t.Error("checksum should not be empty")
	}
}

func TestLoadPlugin(t *testing.T) {
	dir := t.TempDir()
	paths.SetBaseDir(dir)

	// Create plugin directory with manifest and SKILL.md
	pluginDir := filepath.Join(dir, "plugins", "test-skill")
	os.MkdirAll(pluginDir, 0755)

	manifest := `source: "@test/skill@1.0.0"
type: skill
permissions:
  - network
checksum: "sha512:abc123"
`
	os.WriteFile(filepath.Join(pluginDir, "manifest.yaml"), []byte(manifest), 0644)

	skillmd := `---
name: test-skill
description: A test skill
---

# Test Skill

Does things.
`
	os.WriteFile(filepath.Join(pluginDir, "SKILL.md"), []byte(skillmd), 0644)

	plugin, err := LoadPlugin("test-skill")
	if err != nil {
		t.Fatalf("LoadPlugin failed: %v", err)
	}

	if plugin.Name != "test-skill" {
		t.Errorf("name=%q, want test-skill", plugin.Name)
	}
	if plugin.Type != PluginTypeSkill {
		t.Errorf("type=%q, want skill", plugin.Type)
	}
	if plugin.Skill == nil {
		t.Fatal("skill metadata should be loaded")
	}
	if plugin.Skill.Name != "test-skill" {
		t.Errorf("skill.name=%q, want test-skill", plugin.Skill.Name)
	}
	if plugin.SkillBody == "" {
		t.Error("skill body should not be empty")
	}
}

func TestParseManifest_FileNotFound(t *testing.T) {
	_, err := ParseManifest("/nonexistent/path/manifest.yaml")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestParseManifest_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.yaml")
	os.WriteFile(path, []byte("{{{"), 0644)

	_, err := ParseManifest(path)
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

func TestLoadPlugin_NoSkillMD(t *testing.T) {
	dir := t.TempDir()
	paths.SetBaseDir(dir)

	pluginDir := filepath.Join(dir, "plugins", "no-skill")
	os.MkdirAll(pluginDir, 0755)

	manifest := `source: "@test/no-skill@1.0.0"
type: connector
permissions:
  - network
checksum: "sha512:def456"
`
	os.WriteFile(filepath.Join(pluginDir, "manifest.yaml"), []byte(manifest), 0644)
	// Intentionally no SKILL.md

	plugin, err := LoadPlugin("no-skill")
	if err != nil {
		t.Fatalf("LoadPlugin failed: %v", err)
	}
	if plugin.Skill != nil {
		t.Error("expected Skill to be nil when no SKILL.md exists")
	}
}

func TestLoadPlugin_ManifestNotFound(t *testing.T) {
	dir := t.TempDir()
	paths.SetBaseDir(dir)

	// Create plugin dir but no manifest.yaml
	pluginDir := filepath.Join(dir, "plugins", "broken")
	os.MkdirAll(pluginDir, 0755)

	_, err := LoadPlugin("broken")
	if err == nil {
		t.Error("expected error when manifest.yaml is missing")
	}
}

func TestListPlugins_NonDirEntry(t *testing.T) {
	dir := t.TempDir()
	paths.SetBaseDir(dir)

	pluginsDir := filepath.Join(dir, "plugins")
	os.MkdirAll(pluginsDir, 0755)

	// Create a regular file inside plugins dir (not a directory)
	os.WriteFile(filepath.Join(pluginsDir, "not-a-dir"), []byte("hello"), 0644)

	// Also create a valid plugin dir
	os.MkdirAll(filepath.Join(pluginsDir, "valid"), 0755)
	os.WriteFile(filepath.Join(pluginsDir, "valid", "manifest.yaml"), []byte("source: test\ntype: skill\n"), 0644)

	names, err := ListPlugins()
	if err != nil {
		t.Fatalf("ListPlugins failed: %v", err)
	}
	if len(names) != 1 || names[0] != "valid" {
		t.Errorf("names=%v, want [valid]", names)
	}
}

func TestListPlugins_ReadDirError(t *testing.T) {
	dir := t.TempDir()
	paths.SetBaseDir(dir)

	// Create a file where the plugins directory should be, so ReadDir fails
	pluginsPath := filepath.Join(dir, "plugins")
	os.WriteFile(pluginsPath, []byte("not a dir"), 0644)

	_, err := ListPlugins()
	if err == nil {
		t.Error("expected error when plugins path is a file, not a directory")
	}
}

func TestParseSkillMD_FileNotFound(t *testing.T) {
	_, _, err := ParseSkillMD("/nonexistent/path/SKILL.md")
	if err == nil {
		t.Error("expected error for nonexistent SKILL.md file")
	}
}

func TestListPlugins(t *testing.T) {
	dir := t.TempDir()
	paths.SetBaseDir(dir)

	// Create two plugin dirs, one with manifest, one without
	os.MkdirAll(filepath.Join(dir, "plugins", "valid"), 0755)
	os.WriteFile(filepath.Join(dir, "plugins", "valid", "manifest.yaml"), []byte("source: test\ntype: skill\n"), 0644)

	os.MkdirAll(filepath.Join(dir, "plugins", "invalid"), 0755)
	// No manifest.yaml

	names, err := ListPlugins()
	if err != nil {
		t.Fatalf("ListPlugins failed: %v", err)
	}

	if len(names) != 1 || names[0] != "valid" {
		t.Errorf("names=%v, want [valid]", names)
	}
}

func TestListPlugins_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	paths.SetBaseDir(dir)

	names, err := ListPlugins()
	if err != nil {
		t.Fatalf("ListPlugins failed: %v", err)
	}
	if len(names) != 0 {
		t.Errorf("expected empty list, got %v", names)
	}
}
