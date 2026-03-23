package store

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/YangZhengCQ/Claw2cli/internal/paths"
)

func TestStripVersion(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"@scope/pkg@1.0.0", "@scope/pkg"},
		{"@scope/pkg@latest", "@scope/pkg"},
		{"@scope/pkg", "@scope/pkg"},
		{"simple-pkg@1.0.0", "simple-pkg"},
		{"simple-pkg", "simple-pkg"},
	}
	for _, tt := range tests {
		got := stripVersion(tt.input)
		if got != tt.want {
			t.Errorf("stripVersion(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestStripCLISuffix(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"@scope/openclaw-weixin-cli", "@scope/openclaw-weixin"},
		{"@scope/openclaw-weixin", "@scope/openclaw-weixin"},
		{"simple-cli", "simple"},
	}
	for _, tt := range tests {
		got := stripCLISuffix(tt.input)
		if got != tt.want {
			t.Errorf("stripCLISuffix(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestIsInstalled(t *testing.T) {
	dir := t.TempDir()
	paths.SetBaseDir(dir)
	s := &Store{pluginDir: filepath.Join(dir, "plugins", "test"), name: "test"}

	if s.IsInstalled() {
		t.Error("should not be installed yet")
	}

	// Create node_modules with a file
	nm := s.NodeModulesPath()
	os.MkdirAll(nm, 0700)
	os.WriteFile(filepath.Join(nm, ".package-lock.json"), []byte("{}"), 0600)

	if !s.IsInstalled() {
		t.Error("should be installed after creating node_modules")
	}
}

func TestNodeModulesPath(t *testing.T) {
	s := &Store{pluginDir: "/tmp/test-plugin", name: "test"}
	want := "/tmp/test-plugin/node_modules"
	if got := s.NodeModulesPath(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveTsx(t *testing.T) {
	// Should return something (tsx, node, or local path)
	result := ResolveTsx()
	if result == "" {
		t.Error("ResolveTsx should return non-empty string")
	}
}
