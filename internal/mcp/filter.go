package mcp

import "github.com/user/claw2cli/internal/parser"

// FilterPlugins returns only the plugins that match the given filter criteria.
// If both skills and connectors are nil/empty, all plugins are returned.
func FilterPlugins(manifests []*parser.PluginManifest, skills, connectors []string) []*parser.PluginManifest {
	if len(skills) == 0 && len(connectors) == 0 {
		return manifests
	}

	allowed := make(map[string]bool)
	for _, s := range skills {
		allowed[s] = true
	}
	for _, c := range connectors {
		allowed[c] = true
	}

	var result []*parser.PluginManifest
	for _, m := range manifests {
		if allowed[m.Name] {
			result = append(result, m)
		}
	}
	return result
}
