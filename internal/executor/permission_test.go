package executor

import (
	"testing"

	"github.com/user/claw2cli/internal/parser"
)

func TestCheckPermissions_ValidSkill(t *testing.T) {
	m := &parser.PluginManifest{
		Name:        "test",
		Type:        parser.PluginTypeSkill,
		Permissions: []parser.Permission{"network"},
	}
	if err := CheckPermissions(m); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCheckPermissions_ConnectorMissingNetwork(t *testing.T) {
	m := &parser.PluginManifest{
		Name:        "test",
		Type:        parser.PluginTypeConnector,
		Permissions: []parser.Permission{"fs:~/.c2c/storage/test"},
	}
	err := CheckPermissions(m)
	if err == nil {
		t.Error("expected error for connector without network permission")
	}
}

func TestCheckPermissions_ConnectorWithNetwork(t *testing.T) {
	m := &parser.PluginManifest{
		Name: "test",
		Type: parser.PluginTypeConnector,
		Permissions: []parser.Permission{
			"network",
			"fs:~/.c2c/storage/test",
			"credential:keychain",
		},
	}
	if err := CheckPermissions(m); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCheckPermissions_UnrecognizedPermission(t *testing.T) {
	m := &parser.PluginManifest{
		Name:        "test",
		Type:        parser.PluginTypeSkill,
		Permissions: []parser.Permission{"execute-arbitrary-code"},
	}
	err := CheckPermissions(m)
	if err == nil {
		t.Error("expected error for unrecognized permission")
	}
}

func TestCheckPermissions_EmptyPermissions(t *testing.T) {
	m := &parser.PluginManifest{
		Name:        "test",
		Type:        parser.PluginTypeSkill,
		Permissions: nil,
	}
	if err := CheckPermissions(m); err != nil {
		t.Errorf("skill with no permissions should be ok: %v", err)
	}
}
