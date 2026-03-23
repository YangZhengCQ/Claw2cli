package registry

import (
	"sync"

	"github.com/YangZhengCQ/Claw2cli/internal/protocol"
)

// toolRegistry caches discovered tool schemas per connector using type-safe access.
var (
	toolRegistry sync.Map // key: connector name (string), value: []protocol.ToolSchema
)

// Store caches tool schemas for a connector.
func Store(name string, tools []protocol.ToolSchema) {
	toolRegistry.Store(name, tools)
}

// Delete removes cached tools for a connector (on stop/crash).
func Delete(name string) {
	toolRegistry.Delete(name)
}

// Get returns the cached tool schemas for a connector, or nil.
func Get(name string) []protocol.ToolSchema {
	v, ok := toolRegistry.Load(name)
	if !ok {
		return nil
	}
	tools, ok := v.([]protocol.ToolSchema)
	if !ok {
		return nil
	}
	// Return a defensive copy to prevent callers from mutating registry state
	cp := make([]protocol.ToolSchema, len(tools))
	copy(cp, tools)
	return cp
}

// GetAll returns tools from all active connectors.
func GetAll() []protocol.ToolSchema {
	var all []protocol.ToolSchema
	toolRegistry.Range(func(key, value interface{}) bool {
		if tools, ok := value.([]protocol.ToolSchema); ok {
			all = append(all, tools...)
		}
		return true
	})
	return all
}
