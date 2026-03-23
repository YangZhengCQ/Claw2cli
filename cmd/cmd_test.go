package cmd

import (
	"os"
	"testing"

	"github.com/YangZhengCQ/Claw2cli/internal/paths"
)

func TestInstallCmd_InvalidType(t *testing.T) {
	err := installPlugin("test-pkg", "invalid")
	if err == nil {
		t.Fatal("expected error for invalid type")
	}
	if err.Error() != `invalid plugin type "invalid": must be 'skill' or 'connector'` {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDerivePluginName(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"@tencent-weixin/openclaw-weixin-cli@latest", "wechat"},
		{"@larksuite/openclaw-lark@1.0.0", "feishu"},
		{"simple-package", "simple-package"},
		{"openclaw-test", "test"},
	}
	for _, tt := range tests {
		got := derivePluginName(tt.input)
		if got != tt.want {
			t.Errorf("derivePluginName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestConnectCmd_NotAConnector(t *testing.T) {
	dir := t.TempDir()
	paths.SetBaseDir(dir)
	paths.EnsureDirs()

	// Create a skill plugin
	pluginDir := paths.PluginDir("test-skill")
	os.MkdirAll(pluginDir, 0700)
	os.WriteFile(pluginDir+"/manifest.yaml", []byte("source: test\ntype: skill\n"), 0600)

	// Try to connect — should fail
	connectCmd.SetArgs([]string{"test-skill"})
	err := connectCmd.RunE(connectCmd, []string{"test-skill"})
	if err == nil {
		t.Fatal("expected error connecting to a skill")
	}
}
