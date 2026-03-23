package protocol

import (
	"bytes"
	"testing"
)

func FuzzNDJSONDecode(f *testing.F) {
	f.Add([]byte(`{"type":"event","source":"test"}`))
	f.Add([]byte(`{}`))
	f.Add([]byte(`invalid json`))
	f.Add([]byte(`{"type":"command","action":"invoke_tool","payload":null}`))
	f.Add([]byte(``))

	f.Fuzz(func(t *testing.T, data []byte) {
		dec := NewDecoder(bytes.NewReader(data))
		dec.Decode() // must not panic
	})
}
