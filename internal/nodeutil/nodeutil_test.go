package nodeutil

import (
	"errors"
	"os/exec"
	"strings"
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
	if err != nil && !strings.Contains(err.Error(), "mismatch") {
		t.Errorf("expected 'mismatch' in error, got: %v", err)
	}
}

func TestGetNpmChecksum_Integrity(t *testing.T) {
	orig := execCommandFn
	defer func() { execCommandFn = orig }()

	execCommandFn = func(name string, args ...string) *exec.Cmd {
		return exec.Command("echo", `{"dist":{"integrity":"sha512-abc123","shasum":"def456"}}`)
	}

	got, err := GetNpmChecksum("@scope/pkg@1.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "sha512-abc123" {
		t.Errorf("expected sha512-abc123, got %q", got)
	}
}

func TestGetNpmChecksum_FallbackToShasum(t *testing.T) {
	orig := execCommandFn
	defer func() { execCommandFn = orig }()

	execCommandFn = func(name string, args ...string) *exec.Cmd {
		return exec.Command("echo", `{"dist":{"shasum":"abc123"}}`)
	}

	got, err := GetNpmChecksum("simple-pkg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "sha1:abc123" {
		t.Errorf("expected sha1:abc123, got %q", got)
	}
}

func TestGetNpmChecksum_NpmError(t *testing.T) {
	orig := execCommandFn
	defer func() { execCommandFn = orig }()

	execCommandFn = func(name string, args ...string) *exec.Cmd {
		return exec.Command("false") // exits with code 1
	}

	_, err := GetNpmChecksum("nonexistent-pkg")
	if err == nil {
		t.Error("expected error from npm failure")
	}
}

func TestGetNpmChecksum_StripsVersionFromScopedPackage(t *testing.T) {
	orig := execCommandFn
	defer func() { execCommandFn = orig }()

	var capturedArgs []string
	execCommandFn = func(name string, args ...string) *exec.Cmd {
		capturedArgs = append([]string{name}, args...)
		return exec.Command("echo", `{"dist":{"integrity":"sha512-test"}}`)
	}

	_, err := GetNpmChecksum("@scope/pkg@1.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should strip version: "@scope/pkg@1.0.0" -> "@scope/pkg"
	pkgArg := capturedArgs[2] // npm info <pkg> --json
	if pkgArg != "@scope/pkg" {
		t.Errorf("expected package arg %q, got %q", "@scope/pkg", pkgArg)
	}
}

func TestGetNpmChecksum_UsesExecCommandFn(t *testing.T) {
	orig := execCommandFn
	defer func() { execCommandFn = orig }()

	called := false
	execCommandFn = func(name string, args ...string) *exec.Cmd {
		called = true
		return exec.Command("echo", `{"dist":{"integrity":"sha512-x"}}`)
	}

	if _, err := GetNpmChecksum("pkg"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("GetNpmChecksum should use execCommandFn, not exec.Command directly")
	}
}

// --- ResolveNodeRunner tests ---

func TestResolveNodeRunner_TsxFound(t *testing.T) {
	origLook := lookPathFn
	defer func() { lookPathFn = origLook }()

	lookPathFn = func(file string) (string, error) {
		if file == "tsx" {
			return "/usr/local/bin/tsx", nil
		}
		return "", errors.New("not found")
	}

	got := ResolveNodeRunner()
	if got != "tsx" {
		t.Errorf("expected 'tsx' when tsx is in PATH, got %q", got)
	}
}

func TestResolveNodeRunner_NonInteractiveFallsBackToNode(t *testing.T) {
	origLook := lookPathFn
	defer func() { lookPathFn = origLook }()

	lookPathFn = func(file string) (string, error) {
		return "", errors.New("not found")
	}

	// In test context, stdin is a pipe (non-interactive), so it should fall back to "node"
	got := ResolveNodeRunner()
	if got != "node" {
		t.Errorf("expected 'node' in non-interactive mode, got %q", got)
	}
}

// --- EnsurePluginInstalled tests ---

func TestEnsurePluginInstalled_AlreadyInstalled(t *testing.T) {
	orig := execCommandFn
	defer func() { execCommandFn = orig }()

	// npm list succeeds for all packages (already installed)
	execCommandFn = func(name string, args ...string) *exec.Cmd {
		return exec.Command("true")
	}

	err := EnsurePluginInstalled("@scope/pkg-cli@latest")
	if err != nil {
		t.Errorf("expected nil when already installed, got: %v", err)
	}
}

func TestEnsurePluginInstalled_InstallsOnMissing(t *testing.T) {
	orig := execCommandFn
	defer func() { execCommandFn = orig }()

	var installedPkgs []string
	execCommandFn = func(name string, args ...string) *exec.Cmd {
		if len(args) > 0 && args[0] == "list" {
			return exec.Command("false") // not installed
		}
		if len(args) > 0 && args[0] == "install" {
			// Capture which package is being installed
			for _, a := range args {
				if !strings.HasPrefix(a, "-") && a != "npm" && a != "install" {
					installedPkgs = append(installedPkgs, a)
				}
			}
			return exec.Command("true")
		}
		return exec.Command("true")
	}

	err := EnsurePluginInstalled("@scope/pkg-cli@latest")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should install both the source and the runtime package
	if len(installedPkgs) < 2 {
		t.Errorf("expected at least 2 packages installed (source + runtime), got: %v", installedPkgs)
	}
}

func TestEnsurePluginInstalled_InstallFailure(t *testing.T) {
	orig := execCommandFn
	defer func() { execCommandFn = orig }()

	execCommandFn = func(name string, args ...string) *exec.Cmd {
		if len(args) > 0 && args[0] == "list" {
			return exec.Command("false") // not installed
		}
		if len(args) > 0 && args[0] == "install" {
			return exec.Command("false") // install fails
		}
		return exec.Command("true")
	}

	err := EnsurePluginInstalled("@scope/pkg")
	if err == nil {
		t.Fatal("expected error when npm install fails")
	}
	if !strings.Contains(err.Error(), "npm install") {
		t.Errorf("expected 'npm install' in error, got: %v", err)
	}
}

func TestEnsurePluginInstalled_UsesIgnoreScripts(t *testing.T) {
	orig := execCommandFn
	defer func() { execCommandFn = orig }()

	var installArgs [][]string
	execCommandFn = func(name string, args ...string) *exec.Cmd {
		if len(args) > 0 && args[0] == "list" {
			return exec.Command("false") // not installed
		}
		if len(args) > 0 && args[0] == "install" {
			installArgs = append(installArgs, append([]string{name}, args...))
			return exec.Command("true")
		}
		return exec.Command("true")
	}

	err := EnsurePluginInstalled("@scope/pkg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, args := range installArgs {
		found := false
		for _, a := range args {
			if a == "--ignore-scripts" {
				found = true
			}
		}
		if !found {
			t.Errorf("npm install should include --ignore-scripts to prevent supply-chain attacks, got: %v", args)
		}
	}
}

func TestEnsurePluginInstalled_SameSourceAndRuntime(t *testing.T) {
	orig := execCommandFn
	defer func() { execCommandFn = orig }()

	var listChecks []string
	execCommandFn = func(name string, args ...string) *exec.Cmd {
		if len(args) > 0 && args[0] == "list" {
			// Capture the package being checked
			for i, a := range args {
				if a == "-g" && i+1 < len(args) {
					listChecks = append(listChecks, args[i+1])
				}
			}
			return exec.Command("true") // already installed
		}
		return exec.Command("true")
	}

	// "simple-pkg" has no -cli suffix, so source == runtime
	err := EnsurePluginInstalled("simple-pkg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should only check one package (no duplicate)
	if len(listChecks) != 1 {
		t.Errorf("expected 1 npm list check for same source/runtime, got %d: %v", len(listChecks), listChecks)
	}
}
