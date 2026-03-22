package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/user/claw2cli/internal/paths"
)

func TestLoadDefaults(t *testing.T) {
	dir := t.TempDir()
	paths.SetBaseDir(dir)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.DefaultTimeout != 30 {
		t.Errorf("expected default_timeout=30, got %d", cfg.DefaultTimeout)
	}
}

func TestLoad_MalformedYAML(t *testing.T) {
	dir := t.TempDir()
	paths.SetBaseDir(dir)

	content := []byte("{{{\n  bad: yaml: content\n")
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), content, 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	_, err := Load()
	if err == nil {
		t.Error("expected error for malformed YAML config")
	}
}

func TestLoad_UnmarshalError(t *testing.T) {
	dir := t.TempDir()
	paths.SetBaseDir(dir)

	// Use a YAML value that cannot be decoded into int (a nested map for an int field)
	content := []byte("default_timeout:\n  nested: value\n  another: thing\n")
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), content, 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	_, err := Load()
	if err == nil {
		t.Error("expected error when YAML value cannot be unmarshaled into config struct")
	}
}

func TestLoadFromFile(t *testing.T) {
	dir := t.TempDir()
	paths.SetBaseDir(dir)

	content := []byte("default_timeout: 60\n")
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), content, 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.DefaultTimeout != 60 {
		t.Errorf("expected default_timeout=60, got %d", cfg.DefaultTimeout)
	}
}
