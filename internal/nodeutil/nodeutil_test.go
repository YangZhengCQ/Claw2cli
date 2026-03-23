package nodeutil

import (
	"testing"
)

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

func TestVerifyChecksum_EmptyExpected(t *testing.T) {
	// Empty expected = no verification = always pass
	if err := VerifyChecksum("any-package", ""); err != nil {
		t.Errorf("expected nil for empty checksum, got: %v", err)
	}
}

func TestVerifyChecksum_Mismatch(t *testing.T) {
	// This will fail because the expected checksum is fake
	err := VerifyChecksum("@tencent-weixin/openclaw-weixin-cli", "sha512:fakechecksum")
	if err == nil {
		t.Error("expected checksum mismatch error")
	}
	if err != nil && !contains(err.Error(), "mismatch") {
		t.Errorf("expected 'mismatch' in error, got: %v", err)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
