//go:build darwin

package sandbox

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/YangZhengCQ/Claw2cli/internal/parser"
)

func TestGenerateProfile_NetworkDeniedByDefault(t *testing.T) {
	manifest := &parser.PluginManifest{
		Permissions: []parser.Permission{},
	}
	paths := SandboxPaths{
		ShimDir:     "/shim",
		NodeModules: "/nm",
		NodeRunner:  "/tsx",
		StorageDir:  "/storage",
	}
	profile := generateProfile(manifest, paths)
	if !strings.Contains(profile, "(deny network") {
		t.Error("profile should deny network when no network permission declared")
	}
}

func TestGenerateProfile_NetworkAllowedWhenDeclared(t *testing.T) {
	manifest := &parser.PluginManifest{
		Permissions: []parser.Permission{"network"},
	}
	paths := SandboxPaths{
		ShimDir:     "/shim",
		NodeModules: "/nm",
		NodeRunner:  "/tsx",
		StorageDir:  "/storage",
	}
	profile := generateProfile(manifest, paths)
	if strings.Contains(profile, "(deny network") {
		t.Error("profile should NOT deny network when network permission declared")
	}
}

func TestIsUnsafeFsPath(t *testing.T) {
	unsafe := []string{"/", "/Users", "/System", "/Library", "/bin", "/etc", "../escape"}
	for _, p := range unsafe {
		if !isUnsafeFsPath(p) {
			t.Errorf("isUnsafeFsPath(%q) should be true (dangerous path)", p)
		}
	}

	safe := []string{"/data/plugin", "/home/user/.c2c/storage/wechat", "/tmp/test"}
	for _, p := range safe {
		if isUnsafeFsPath(p) {
			t.Errorf("isUnsafeFsPath(%q) should be false (safe path)", p)
		}
	}
}

func TestGenerateProfile_ValidSBPL(t *testing.T) {
	manifest := &parser.PluginManifest{
		Permissions: []parser.Permission{"network"},
	}
	spaths := SandboxPaths{
		ShimDir:     "/tmp/shim",
		NodeModules: "/tmp/nm",
		NodeRunner:  "/usr/bin/node",
		StorageDir:  "/tmp/storage",
	}
	profile := generateProfile(manifest, spaths)

	// Write and validate with sandbox-exec
	profilePath, err := writeTempProfile(profile)
	if err != nil {
		t.Fatalf("write profile: %v", err)
	}

	// sandbox-exec should accept the profile (run /usr/bin/true)
	cmd := exec.Command("/usr/bin/sandbox-exec", "-f", profilePath, "/usr/bin/true")
	if err := cmd.Run(); err != nil {
		t.Errorf("sandbox-exec rejected profile: %v\nProfile:\n%s", err, profile)
	}
}
