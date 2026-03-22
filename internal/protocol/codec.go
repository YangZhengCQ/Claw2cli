package protocol

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
)

// Encoder writes NDJSON messages to a writer.
type Encoder struct {
	w io.Writer
}

// NewEncoder creates a new NDJSON encoder.
func NewEncoder(w io.Writer) *Encoder {
	return &Encoder{w: w}
}

// Encode writes a single message as a JSON line followed by newline.
func (e *Encoder) Encode(msg *Message) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}
	data = append(data, '\n')
	_, err = e.w.Write(data)
	return err
}

// Decoder reads NDJSON messages from a reader.
type Decoder struct {
	scanner *bufio.Scanner
}

// NewDecoder creates a new NDJSON decoder.
func NewDecoder(r io.Reader) *Decoder {
	return &Decoder{scanner: bufio.NewScanner(r)}
}

// Decode reads the next message. Returns io.EOF when no more messages.
func (d *Decoder) Decode() (*Message, error) {
	if !d.scanner.Scan() {
		if err := d.scanner.Err(); err != nil {
			return nil, err
		}
		return nil, io.EOF
	}
	line := d.scanner.Bytes()
	msg := &Message{}
	if err := json.Unmarshal(line, msg); err != nil {
		return nil, fmt.Errorf("unmarshal message: %w", err)
	}
	return msg, nil
}
