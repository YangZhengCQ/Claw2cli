package protocol

import (
	"encoding/json"
	"time"
)

// MessageType identifies the kind of NDJSON message.
type MessageType string

const (
	TypeEvent     MessageType = "event"
	TypeCommand   MessageType = "command"
	TypeResponse  MessageType = "response"
	TypeError     MessageType = "error"
	TypeLog       MessageType = "log"
	TypeDiscovery MessageType = "discovery"
	TypePing      MessageType = "ping"
	TypePong      MessageType = "pong"
	TypeShutdown  MessageType = "shutdown"
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
		Ts:      time.Now().Unix(),
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
		Ts:      time.Now().Unix(),
	}
}

// NewResponse creates a response message correlated to a command.
func NewResponse(source, id string, payload json.RawMessage) *Message {
	return &Message{
		Type:    TypeResponse,
		Source:  source,
		ID:      id,
		Payload: payload,
		Ts:      time.Now().Unix(),
	}
}

// NewError creates an error message.
func NewError(source, code, message string) *Message {
	return &Message{
		Type:       TypeError,
		Source:     source,
		Code:       code,
		MessageStr: message,
		Ts:         time.Now().Unix(),
	}
}

// NewLog creates a log message.
func NewLog(source, level, message string) *Message {
	return &Message{
		Type:       TypeLog,
		Source:     source,
		Level:      level,
		MessageStr: message,
		Ts:         time.Now().Unix(),
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
// Returns nil if the tools cannot be marshaled (should not happen in practice).
func NewDiscovery(source string, tools []ToolSchema) *Message {
	payload, err := json.Marshal(DiscoveryPayload{Tools: tools})
	if err != nil {
		return nil
	}
	return &Message{
		Type:    TypeDiscovery,
		Source:  source,
		Payload: payload,
		Ts:      time.Now().Unix(),
	}
}

// NewPing creates a ping message for readiness checks.
func NewPing(source, id string) *Message {
	return &Message{
		Type:   TypePing,
		Source: source,
		ID:     id,
		Ts:     time.Now().Unix(),
	}
}

// NewPong creates a pong response to a ping.
func NewPong(source, id string) *Message {
	return &Message{
		Type:   TypePong,
		Source: source,
		ID:     id,
		Ts:     time.Now().Unix(),
	}
}
