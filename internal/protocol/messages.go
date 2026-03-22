package protocol

import "encoding/json"

// MessageType identifies the kind of NDJSON message.
type MessageType string

const (
	TypeEvent     MessageType = "event"
	TypeCommand   MessageType = "command"
	TypeResponse  MessageType = "response"
	TypeError     MessageType = "error"
	TypeLog       MessageType = "log"
	TypeDiscovery MessageType = "discovery"
)

// Message is the NDJSON envelope for all c2c IPC communication.
// All messages carry a Source field to identify the originating plugin.
type Message struct {
	Type   MessageType     `json:"type"`
	Source string          `json:"source"`

	// Event fields
	Topic string `json:"topic,omitempty"`

	// Command fields
	Action string `json:"action,omitempty"`

	// Command/Response correlation
	ID string `json:"id,omitempty"`

	// Error fields
	Code       string `json:"code,omitempty"`
	MessageStr string `json:"message,omitempty"`

	// Log fields
	Level string `json:"level,omitempty"`

	// Payload carries the actual data (deferred parsing).
	Payload json.RawMessage `json:"payload,omitempty"`

	// Timestamp as unix epoch seconds.
	Ts int64 `json:"ts,omitempty"`
}

// NewEvent creates an event message.
func NewEvent(source, topic string, payload json.RawMessage) *Message {
	return &Message{
		Type:    TypeEvent,
		Source:  source,
		Topic:   topic,
		Payload: payload,
	}
}

// NewCommand creates a command message with a correlation ID.
func NewCommand(source, action, id string, payload json.RawMessage) *Message {
	return &Message{
		Type:    TypeCommand,
		Source:  source,
		Action:  action,
		ID:      id,
		Payload: payload,
	}
}

// NewResponse creates a response message correlated to a command.
func NewResponse(source, id string, payload json.RawMessage) *Message {
	return &Message{
		Type:    TypeResponse,
		Source:  source,
		ID:      id,
		Payload: payload,
	}
}

// NewError creates an error message.
func NewError(source, code, message string) *Message {
	return &Message{
		Type:       TypeError,
		Source:     source,
		Code:       code,
		MessageStr: message,
	}
}

// NewLog creates a log message.
func NewLog(source, level, message string) *Message {
	return &Message{
		Type:       TypeLog,
		Source:     source,
		Level:      level,
		MessageStr: message,
	}
}

// ToolSchema describes a single tool in MCP Tool Schema format.
type ToolSchema struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// DiscoveryPayload is the payload of a discovery message.
type DiscoveryPayload struct {
	Tools []ToolSchema `json:"tools"`
}

// NewDiscovery creates a discovery message carrying tool schemas.
func NewDiscovery(source string, tools []ToolSchema) *Message {
	payload, _ := json.Marshal(DiscoveryPayload{Tools: tools})
	return &Message{
		Type:    TypeDiscovery,
		Source:  source,
		Payload: payload,
	}
}
