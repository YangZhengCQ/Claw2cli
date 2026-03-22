package executor

import (
	"fmt"
	"strings"

	"github.com/user/claw2cli/internal/parser"
)

// CheckPermissions validates that a plugin has the required permissions.
// Returns an error if any declared permission is not recognized or if
// a connector lacks essential permissions.
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
