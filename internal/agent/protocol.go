package agent

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"unicode/utf8"
)

const MaxRPCFrameBytes = 16 * 1024 * 1024

var (
	ErrFrameTooLarge  = errors.New("pi rpc frame exceeds the 16 MiB limit")
	ErrInvalidFrame   = errors.New("pi rpc emitted invalid JSON")
	ErrTruncatedFrame = errors.New("pi rpc stream ended with a truncated frame")
)

type rawEvent struct {
	Type string
	JSON json.RawMessage
}

type rpcEnvelope struct {
	ID      string          `json:"id"`
	Type    string          `json:"type"`
	Command string          `json:"command"`
	Success *bool           `json:"success"`
	Error   string          `json:"error"`
	Data    json.RawMessage `json:"data"`
}

// readLFFrame implements PI's byte-level LF-only JSONL framing. In particular,
// U+2028 and U+2029 remain ordinary UTF-8 bytes inside a JSON string.
func readLFFrame(reader *bufio.Reader, limit int) ([]byte, error) {
	if limit <= 0 {
		limit = MaxRPCFrameBytes
	}
	buffer := make([]byte, 0, 4096)
	for {
		chunk, err := reader.ReadSlice('\n')
		buffer = append(buffer, chunk...)
		if len(buffer) > limit+1 {
			return nil, ErrFrameTooLarge
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				if len(buffer) == 0 {
					return nil, io.EOF
				}
				return nil, ErrTruncatedFrame
			}
			return nil, err
		}

		buffer = buffer[:len(buffer)-1]
		if len(buffer) > 0 && buffer[len(buffer)-1] == '\r' {
			buffer = buffer[:len(buffer)-1]
		}
		if len(buffer) > limit {
			return nil, ErrFrameTooLarge
		}
		if len(bytes.Trim(buffer, " \t")) == 0 {
			return []byte{}, nil
		}
		if !utf8.Valid(buffer) || !json.Valid(buffer) {
			return nil, ErrInvalidFrame
		}
		return buffer, nil
	}
}

func decodeEnvelope(frame []byte) (rpcEnvelope, error) {
	var envelope rpcEnvelope
	if err := json.Unmarshal(frame, &envelope); err != nil {
		return rpcEnvelope{}, fmt.Errorf("%w: %v", ErrInvalidFrame, err)
	}
	if envelope.Type == "" {
		return rpcEnvelope{}, fmt.Errorf("%w: missing type", ErrInvalidFrame)
	}
	if envelope.Type == "response" {
		if envelope.ID == "" || envelope.Command == "" || envelope.Success == nil {
			return rpcEnvelope{}, fmt.Errorf(
				"%w: malformed response envelope",
				ErrInvalidFrame,
			)
		}
	}
	return envelope, nil
}
