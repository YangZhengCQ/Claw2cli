package paths

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSetBaseDir(t *testing.T) {
	original := baseDir
	defer SetBaseDir(original)

	SetBaseDir("/tmp/test-c2c")
	if BaseDir() != "/tmp/test-c2c" {
		t.Errorf("expected /tmp/test-c2c, got %s", BaseDir())
	}
}

func TestPathFunctions(t *testing.T) {
	SetBaseDir("/tmp/test-c2c")
	defer SetBaseDir(baseDir)

	tests := []struct {
		name     string
		got      string
		expected string
	}{
		{"PluginsDir", PluginsDir(), "/tmp/test-c2c/plugins"},
		{"PluginDir", PluginDir("wechat"), "/tmp/test-c2c/plugins/wechat"},
		{"StorageDir", StorageDir("wechat"), "/tmp/test-c2c/storage/wechat"},
		{"SocketPath", SocketPath("wechat"), "/tmp/test-c2c/sockets/wechat.sock"},
		{"PIDPath", PIDPath("wechat"), "/tmp/test-c2c/pids/wechat.pid"},
		{"ConfigPath", ConfigPath(), "/tmp/test-c2c/config.yaml"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, tt.got)
			}
		})
	}
}

func TestEnsureDirs(t *testing.T) {
	dir := t.TempDir()
	SetBaseDir(dir)
	defer SetBaseDir(baseDir)

	if err := EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs failed: %v", err)
	}

	for _, sub := range []string{"plugins", "storage", "sockets", "pids"} {
		p := filepath.Join(dir, sub)
		info, err := os.Stat(p)
		if err != nil {
			t.Errorf("directory %s not created: %v", sub, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("%s is not a directory", sub)
		}
	}
}

func TestEnsureDirs_PermissionError(t *testing.T) {
	original := baseDir
	defer SetBaseDir(original)

	SetBaseDir("/dev/null/impossible")
	err := EnsureDirs()
	if err == nil {
		t.Error("expected error when creating dirs under /dev/null")
	}
}

func TestEnsureStorageDir(t *testing.T) {
	dir := t.TempDir()
	SetBaseDir(dir)
	defer SetBaseDir(baseDir)

	if err := EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs failed: %v", err)
	}

	if err := EnsureStorageDir("wechat"); err != nil {
		t.Fatalf("EnsureStorageDir failed: %v", err)
	}

	info, err := os.Stat(StorageDir("wechat"))
	if err != nil {
		t.Fatalf("storage dir not created: %v", err)
	}
	if info.Mode().Perm() != 0700 {
		t.Errorf("expected 0700 permissions, got %o", info.Mode().Perm())
	}
}
