package store

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/YangZhengCQ/Claw2cli/internal/parser"
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

func TestCleanupReplacedPackages(t *testing.T) {
	dir := t.TempDir()
	s := &Store{pluginDir: dir, name: "test"}
	nm := s.NodeModulesPath()

	// Create fake packages that our shim replaces
	for _, pkg := range []string{"openclaw", "clawdbot", "@mariozechner/pi-ai"} {
		os.MkdirAll(filepath.Join(nm, pkg), 0700)
		os.WriteFile(filepath.Join(nm, pkg, "index.js"), []byte("module.exports = {}"), 0600)
	}

	// Verify they exist
	if _, err := os.Stat(filepath.Join(nm, "openclaw")); err != nil {
		t.Fatal("openclaw should exist before cleanup")
	}

	// Run cleanup
	s.CleanupReplacedPackages()

	// Verify they're gone
	for _, pkg := range []string{"openclaw", "clawdbot", "@mariozechner"} {
		if _, err := os.Stat(filepath.Join(nm, pkg)); err == nil {
			t.Errorf("%s should be removed after cleanup", pkg)
		}
	}
}

func TestInstall_PassesIgnoreScripts(t *testing.T) {
	orig := execCommandFn
	defer func() { execCommandFn = orig }()

	var capturedArgs [][]string
	execCommandFn = func(name string, args ...string) *exec.Cmd {
		capturedArgs = append(capturedArgs, append([]string{name}, args...))
		return exec.Command("echo", "ok") // no-op
	}

	dir := t.TempDir()
	s := &Store{pluginDir: dir, name: "test"}

	s.Install("@test/pkg@1.0.0")

	// Verify --ignore-scripts is in the npm install command
	found := false
	for _, args := range capturedArgs {
		if args[0] == "npm" && len(args) > 1 && args[1] == "install" {
			for _, a := range args {
				if a == "--ignore-scripts" {
					found = true
				}
			}
		}
	}
	if !found {
		t.Errorf("npm install should include --ignore-scripts, got commands: %v", capturedArgs)
	}
}

func TestInstall_LogsIntegrityError(t *testing.T) {
	origExec := execCommandFn
	origIntegrity := getIntegrityFn
	defer func() { execCommandFn = origExec; getIntegrityFn = origIntegrity }()

	execCommandFn = func(name string, args ...string) *exec.Cmd {
		if len(args) > 0 && args[0] == "view" && len(args) > 2 && args[2] == "version" {
			return exec.Command("echo", `"1.0.0"`)
		}
		return exec.Command("echo", "ok")
	}
	getIntegrityFn = func(source string) (string, error) {
		return "", fmt.Errorf("npm registry unreachable")
	}

	dir := t.TempDir()
	s := &Store{pluginDir: dir, name: "test"}

	// Capture log output
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	_, integrity, err := s.Install("@test/pkg@1.0.0")
	if err != nil {
		t.Fatalf("Install should not fail when only integrity check fails, got: %v", err)
	}
	if integrity != "" {
		t.Errorf("expected empty integrity on error, got %q", integrity)
	}
	if !strings.Contains(buf.String(), "integrity") {
		t.Errorf("expected warning log about integrity, got: %q", buf.String())
	}
}

// --- store.Verify tests ---

func setupInstalledStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s := &Store{pluginDir: dir, name: "test-plugin"}
	nm := s.NodeModulesPath()
	os.MkdirAll(nm, 0700)
	os.WriteFile(filepath.Join(nm, ".package-lock.json"), []byte("{}"), 0600)
	return s
}

func TestVerify_NotInstalled(t *testing.T) {
	dir := t.TempDir()
	s := &Store{pluginDir: dir, name: "test-plugin"}
	manifest := &parser.PluginManifest{Source: "pkg", Integrity: "sha512-abc"}

	err := s.Verify(manifest)
	if err == nil {
		t.Fatal("expected error for uninstalled plugin")
	}
	if !strings.Contains(err.Error(), "not installed") {
		t.Errorf("expected 'not installed' error, got: %v", err)
	}
}

func TestVerify_NoIntegrity_NoChecksum_Passes(t *testing.T) {
	s := setupInstalledStore(t)
	manifest := &parser.PluginManifest{Source: "pkg"}

	err := s.Verify(manifest)
	if err != nil {
		t.Errorf("expected nil when no integrity to check, got: %v", err)
	}
}

func TestVerify_FallsBackToChecksum(t *testing.T) {
	orig := getIntegrityFn
	defer func() { getIntegrityFn = orig }()

	getIntegrityFn = func(source string) (string, error) {
		return "sha512-matching", nil
	}

	s := setupInstalledStore(t)
	manifest := &parser.PluginManifest{
		Source:   "pkg",
		Checksum: "sha512-matching", // old field, Integrity is empty
	}

	err := s.Verify(manifest)
	if err != nil {
		t.Errorf("expected nil for matching checksum fallback, got: %v", err)
	}
}

func TestVerify_IntegrityMatch(t *testing.T) {
	orig := getIntegrityFn
	defer func() { getIntegrityFn = orig }()

	getIntegrityFn = func(source string) (string, error) {
		return "sha512-goodhash", nil
	}

	s := setupInstalledStore(t)
	manifest := &parser.PluginManifest{
		Source:    "pkg",
		Integrity: "sha512-goodhash",
	}

	err := s.Verify(manifest)
	if err != nil {
		t.Errorf("expected nil for matching integrity, got: %v", err)
	}
}

func TestVerify_IntegrityMismatch(t *testing.T) {
	orig := getIntegrityFn
	defer func() { getIntegrityFn = orig }()

	getIntegrityFn = func(source string) (string, error) {
		return "sha512-different", nil
	}

	s := setupInstalledStore(t)
	manifest := &parser.PluginManifest{
		Source:    "pkg",
		Integrity: "sha512-expected",
	}

	err := s.Verify(manifest)
	if err == nil {
		t.Fatal("expected mismatch error")
	}
	if !strings.Contains(err.Error(), "mismatch") {
		t.Errorf("expected 'mismatch' in error, got: %v", err)
	}
}

func TestVerify_IntegrityFetchError(t *testing.T) {
	orig := getIntegrityFn
	defer func() { getIntegrityFn = orig }()

	getIntegrityFn = func(source string) (string, error) {
		return "", fmt.Errorf("npm failed")
	}

	s := setupInstalledStore(t)
	manifest := &parser.PluginManifest{
		Source:    "pkg",
		Integrity: "sha512-something",
	}

	err := s.Verify(manifest)
	if err == nil {
		t.Fatal("expected error when npm fails")
	}
	if !strings.Contains(err.Error(), "verify integrity") {
		t.Errorf("expected 'verify integrity' in error, got: %v", err)
	}
}

func TestVerify_IntegrityTakesPrecedenceOverChecksum(t *testing.T) {
	orig := getIntegrityFn
	defer func() { getIntegrityFn = orig }()

	getIntegrityFn = func(source string) (string, error) {
		return "sha512-integrity-value", nil
	}

	s := setupInstalledStore(t)
	manifest := &parser.PluginManifest{
		Source:    "pkg",
		Integrity: "sha512-integrity-value",
		Checksum:  "sha512-old-checksum",
	}

	err := s.Verify(manifest)
	if err != nil {
		t.Errorf("should match Integrity (not Checksum), got: %v", err)
	}
}

func TestResolveTsx(t *testing.T) {
	// Should return something (tsx, node, or local path)
	result := ResolveTsx()
	if result == "" {
		t.Error("ResolveTsx should return non-empty string")
	}
}
