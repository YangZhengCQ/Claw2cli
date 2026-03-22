package registry

import (
	"sync"

	"github.com/user/claw2cli/internal/protocol"
)

// toolRegistry caches discovered tool schemas per connector.
var toolRegistry sync.Map // key: connector name (string), value: []protocol.ToolSchema

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
	return v.([]protocol.ToolSchema)
}

// GetAll returns tools from all active connectors.
func GetAll() []protocol.ToolSchema {
	var all []protocol.ToolSchema
	toolRegistry.Range(func(key, value interface{}) bool {
		all = append(all, value.([]protocol.ToolSchema)...)
		return true
	})
	return all
}
