package executor

import (
	"fmt"
	"strings"

	"github.com/YangZhengCQ/Claw2cli/internal/parser"
)

// CheckPermissions validates that a plugin declares recognized permissions.
// NOTE: Permissions are currently advisory only — they are syntax-checked
// but not enforced at the OS level. Plugins run with the full privileges
// of the current user. Real sandboxing (landlock, sandbox-exec) is planned.
func CheckPermissions(manifest *parser.PluginManifest) error {
	if manifest.Type == parser.PluginTypeConnector {
		// Connectors must declare network permission
		if !hasPermission(manifest.Permissions, "network") {
			return fmt.Errorf("connector %q must declare 'network' permission", manifest.Name)
		}
	}

	for _, perm := range manifest.Permissions {
		if !isValidPermission(perm) {
			return fmt.Errorf("unrecognized permission %q for plugin %q", perm, manifest.Name)
		}
	}

	return nil
}

// hasPermission checks if a specific permission is in the list.
func hasPermission(perms []parser.Permission, target string) bool {
	for _, p := range perms {
		if string(p) == target || strings.HasPrefix(string(p), target+":") {
			return true
		}
	}
	return false
}

// isValidPermission checks if a permission string is recognized.
func isValidPermission(perm parser.Permission) bool {
	p := string(perm)
	// Known permission prefixes
	known := []string{"network", "fs:", "credential:"}
	if p == "network" {
		return true
	}
	for _, prefix := range known {
		if strings.HasPrefix(p, prefix) {
			return true
		}
	}
	return false
}
