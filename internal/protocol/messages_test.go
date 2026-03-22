package protocol

import (
	"bytes"
	"encoding/json"
	"io"
	"testing"
)

func TestEncoderDecoder_Roundtrip(t *testing.T) {
	messages := []*Message{
		NewEvent("wechat", "message.received", json.RawMessage(`{"text":"hello"}`)),
		NewCommand("wechat", "send_message", "req-001", json.RawMessage(`{"to":"user1","text":"hi"}`)),
		NewResponse("wechat", "req-001", json.RawMessage(`{"ok":true}`)),
		NewError("feishu", "AUTH_FAILED", "token expired"),
		NewLog("wechat", "info", "heartbeat ok"),
	}

	var buf bytes.Buffer
	enc := NewEncoder(&buf)
	for _, msg := range messages {
		if err := enc.Encode(msg); err != nil {
			t.Fatalf("encode failed: %v", err)
		}
	}

	dec := NewDecoder(&buf)
	for i, expected := range messages {
		got, err := dec.Decode()
		if err != nil {
			t.Fatalf("decode message %d failed: %v", i, err)
		}
		if got.Type != expected.Type {
			t.Errorf("message %d: type=%s, want %s", i, got.Type, expected.Type)
		}
		if got.Source != expected.Source {
			t.Errorf("message %d: source=%s, want %s", i, got.Source, expected.Source)
		}
		if got.ID != expected.ID {
			t.Errorf("message %d: id=%s, want %s", i, got.ID, expected.ID)
		}
	}

	// EOF after all messages consumed
	_, err := dec.Decode()
	if err != io.EOF {
		t.Errorf("expected io.EOF, got %v", err)
	}
}

func TestDecoder_InvalidJSON(t *testing.T) {
	r := bytes.NewBufferString("not valid json\n")
	dec := NewDecoder(r)
	_, err := dec.Decode()
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

// errWriter is a writer that always returns an error.
type errWriter struct{}

func (errWriter) Write([]byte) (int, error) {
	return 0, io.ErrClosedPipe
}

func TestEncode_WriteError(t *testing.T) {
	enc := NewEncoder(errWriter{})
	msg := NewEvent("test", "topic", json.RawMessage(`{}`))
	err := enc.Encode(msg)
	if err == nil {
		t.Fatal("expected error from writer, got nil")
	}
}

func TestDecode_ScannerError(t *testing.T) {
	// errReader returns an error immediately; the scanner will propagate it
	r := &errReader{err: io.ErrUnexpectedEOF}
	dec := NewDecoder(r)
	_, err := dec.Decode()
	if err == nil {
		t.Fatal("expected error from scanner, got nil")
	}
	if err == io.EOF {
		t.Fatal("expected a non-EOF error, got io.EOF")
	}
}

// errReader always returns an error.
type errReader struct {
	err error
}

func (r *errReader) Read(p []byte) (int, error) {
	return 0, r.err
}

func TestEncoder_MessageFields(t *testing.T) {
	var buf bytes.Buffer
	enc := NewEncoder(&buf)

	msg := NewEvent("wechat", "message.received", json.RawMessage(`{"text":"hi"}`))
	msg.Ts = 1711100000

	if err := enc.Encode(msg); err != nil {
		t.Fatalf("encode failed: %v", err)
	}

	// Verify JSON contains expected fields
	var parsed map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if parsed["type"] != "event" {
		t.Errorf("type=%v, want event", parsed["type"])
	}
	if parsed["source"] != "wechat" {
		t.Errorf("source=%v, want wechat", parsed["source"])
	}
	if parsed["topic"] != "message.received" {
		t.Errorf("topic=%v, want message.received", parsed["topic"])
	}
	if parsed["ts"].(float64) != 1711100000 {
		t.Errorf("ts=%v, want 1711100000", parsed["ts"])
	}
}
