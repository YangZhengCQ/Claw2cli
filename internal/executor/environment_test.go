package executor

import (
	"os"
	"strings"
	"testing"

	"github.com/YangZhengCQ/Claw2cli/internal/parser"
)

func TestBuildEnv_FiltersSensitiveVars(t *testing.T) {
	// Set sensitive environment variables
	sensitiveVars := []string{
		"AWS_SECRET_ACCESS_KEY=supersecret",
		"GITHUB_TOKEN=ghp_123456",
		"DATABASE_URL=postgres://secret@host/db",
		"PRIVATE_KEY=-----BEGIN RSA-----",
	}
	for _, v := range sensitiveVars {
		parts := strings.SplitN(v, "=", 2)
		os.Setenv(parts[0], parts[1])
		defer os.Unsetenv(parts[0])
	}

	m := &parser.PluginManifest{
		Name: "test-plugin",
		Type: parser.PluginTypeSkill,
	}

	env := BuildEnv(m)

	// Check that sensitive vars are NOT present
	for _, e := range env {
		for _, sv := range sensitiveVars {
			key := strings.SplitN(sv, "=", 2)[0]
			if strings.HasPrefix(e, key+"=") {
				t.Errorf("sensitive variable %q should have been filtered out, but found in env", key)
			}
		}
	}
}

func TestBuildEnv_AllowsSafeVars(t *testing.T) {
	m := &parser.PluginManifest{
		Name: "test-plugin",
		Type: parser.PluginTypeSkill,
	}

	env := BuildEnv(m)

	// C2C_ vars should be present
	foundName := false
	foundType := false
	for _, e := range env {
		if e == "C2C_PLUGIN_NAME=test-plugin" {
			foundName = true
		}
		if e == "C2C_PLUGIN_TYPE=skill" {
			foundType = true
		}
	}
	if !foundName {
		t.Error("C2C_PLUGIN_NAME should be in env")
	}
	if !foundType {
		t.Error("C2C_PLUGIN_TYPE should be in env")
	}
}

func TestIsSafeEnvVar(t *testing.T) {
	tests := []struct {
		input string
		safe  bool
	}{
		{"PATH=/usr/bin", true},
		{"HOME=/home/user", true},
		{"USER=joker", true},
		{"LANG=en_US.UTF-8", true},
		{"LC_ALL=en_US.UTF-8", true},
		{"TERM=xterm", true},
		{"SHELL=/bin/zsh", true},
		{"TMPDIR=/tmp", true},
		{"XDG_CONFIG_HOME=/home/user/.config", true},
		{"NODE_PATH=/usr/lib/node_modules", true},
		{"NPM_CONFIG_PREFIX=/usr/local", true},
		{"C2C_PLUGIN_NAME=test", true},
		{"AWS_SECRET_ACCESS_KEY=secret", false},
		{"GITHUB_TOKEN=ghp_123", false},
		{"DATABASE_URL=postgres://", false},
		{"OPENAI_API_KEY=sk-123", false},
		{"SECRET_KEY=abc", false},
	}

	for _, tt := range tests {
		got := isSafeEnvVar(tt.input)
		if got != tt.safe {
			t.Errorf("isSafeEnvVar(%q) = %v, want %v", tt.input, got, tt.safe)
		}
	}
}
