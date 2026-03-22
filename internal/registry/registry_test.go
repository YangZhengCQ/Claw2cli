package registry

import (
	"encoding/json"
	"testing"

	"github.com/user/claw2cli/internal/protocol"
)

func TestStoreAndGet(t *testing.T) {
	defer Delete("test")

	tools := []protocol.ToolSchema{
		{Name: "test_send_text", Description: "Send text", InputSchema: json.RawMessage(`{}`)},
	}
	Store("test", tools)

	got := Get("test")
	if len(got) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(got))
	}
	if got[0].Name != "test_send_text" {
		t.Errorf("expected test_send_text, got %s", got[0].Name)
	}
}

func TestGetMissing(t *testing.T) {
	got := Get("nonexistent")
	if got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestDelete(t *testing.T) {
	Store("del-test", []protocol.ToolSchema{{Name: "x"}})
	Delete("del-test")
	if Get("del-test") != nil {
		t.Error("expected nil after delete")
	}
}

func TestGetAll(t *testing.T) {
	defer Delete("a")
	defer Delete("b")

	Store("a", []protocol.ToolSchema{{Name: "a_tool"}})
	Store("b", []protocol.ToolSchema{{Name: "b_tool1"}, {Name: "b_tool2"}})

	all := GetAll()
	if len(all) < 3 {
		t.Errorf("expected at least 3 tools, got %d", len(all))
	}
}
