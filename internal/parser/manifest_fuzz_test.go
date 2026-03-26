package parser

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/YangZhengCQ/Claw2cli/internal/paths"
)

func FuzzParseManifest(f *testing.F) {
	f.Add([]byte("source: test\ntype: skill\n"))
	f.Add([]byte("source: \"\"\ntype: connector\n"))
	f.Add([]byte("invalid yaml: [[["))
	f.Add([]byte(""))

	f.Fuzz(func(t *testing.T, data []byte) {
		dir := t.TempDir()
		paths.SetBaseDir(dir)
		pluginDir := filepath.Join(dir, "plugins", "fuzz")
		os.MkdirAll(pluginDir, 0700)
		os.WriteFile(filepath.Join(pluginDir, "manifest.yaml"), data, 0644)
		LoadPlugin("fuzz") // must not panic
	})
}
