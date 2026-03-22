package nodeutil

import "testing"

func TestResolvePluginPackage(t *testing.T) {
	tests := []struct {
		source   string
		expected string
	}{
		{"@tencent-weixin/openclaw-weixin-cli@latest", "@tencent-weixin/openclaw-weixin"},
		{"@tencent-weixin/openclaw-weixin-cli", "@tencent-weixin/openclaw-weixin"},
		{"@larksuite/openclaw-lark@1.0.0", "@larksuite/openclaw-lark"},
		{"@larksuite/openclaw-lark", "@larksuite/openclaw-lark"},
		{"simple-plugin@2.0.0", "simple-plugin"},
		{"simple-plugin-cli", "simple-plugin"},
		{"simple-plugin", "simple-plugin"},
		{"@scope/name", "@scope/name"},
	}

	for _, tt := range tests {
		t.Run(tt.source, func(t *testing.T) {
			got := ResolvePluginPackage(tt.source)
			if got != tt.expected {
				t.Errorf("ResolvePluginPackage(%q) = %q, want %q", tt.source, got, tt.expected)
			}
		})
	}
}
