package agent

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestReadLFFramePreservesUnicodeLineSeparatorsAndSplitUTF8(t *testing.T) {
	input := []byte("{\"value\":\"甲\u2028乙\u2029丙\"}\n")
	reader := bufio.NewReaderSize(&oneByteReader{data: input}, 2)
	frame, err := readLFFrame(reader, MaxRPCFrameBytes)
	if err != nil {
		t.Fatalf("read strict frame: %v", err)
	}
	if string(frame) != strings.TrimSuffix(string(input), "\n") {
		t.Fatalf("unicode separators changed: %q", frame)
	}
	if _, err := readLFFrame(reader, MaxRPCFrameBytes); !errors.Is(err, io.EOF) {
		t.Fatalf("expected EOF after the frame, got %v", err)
	}
}

func TestReadLFFrameAcceptsCRLFAndSkipsNoBytes(t *testing.T) {
	reader := bufio.NewReader(bytes.NewBufferString("{\"type\":\"one\"}\r\n{\"type\":\"two\"}\n"))
	first, err := readLFFrame(reader, MaxRPCFrameBytes)
	if err != nil || string(first) != "{\"type\":\"one\"}" {
		t.Fatalf("first frame %q, error %v", first, err)
	}
	second, err := readLFFrame(reader, MaxRPCFrameBytes)
	if err != nil || string(second) != "{\"type\":\"two\"}" {
		t.Fatalf("second frame %q, error %v", second, err)
	}
}

func TestReadLFFrameDoesNotTreatUnicodeSeparatorsAsBlankLines(t *testing.T) {
	_, err := readLFFrame(
		bufio.NewReader(strings.NewReader("\u2028\n")),
		MaxRPCFrameBytes,
	)
	if !errors.Is(err, ErrInvalidFrame) {
		t.Fatalf("expected raw U+2028 to remain an invalid JSON frame, got %v", err)
	}
}

func TestReadLFFrameRejectsInvalidTruncatedAndOversizeFrames(t *testing.T) {
	tests := []struct {
		name  string
		input string
		limit int
		want  error
	}{
		{name: "invalid", input: "not-json\n", limit: 32, want: ErrInvalidFrame},
		{name: "truncated", input: "{\"type\":\"event\"}", limit: 32, want: ErrTruncatedFrame},
		{name: "oversize", input: "{\"value\":\"123456789\"}\n", limit: 8, want: ErrFrameTooLarge},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := readLFFrame(
				bufio.NewReaderSize(strings.NewReader(test.input), 4),
				test.limit,
			)
			if !errors.Is(err, test.want) {
				t.Fatalf("expected %v, got %v", test.want, err)
			}
		})
	}
}

func TestDecodeEnvelopeRequiresCorrelatedResponseShape(t *testing.T) {
	valid, err := decodeEnvelope([]byte(
		`{"id":"req-1","type":"response","command":"get_state","success":true,"data":{}}`,
	))
	if err != nil || valid.ID != "req-1" {
		t.Fatalf("decode valid response: %#v, %v", valid, err)
	}
	for _, frame := range []string{
		`{"type":"response","command":"get_state","success":true}`,
		`{"id":"req-1","type":"response","success":true}`,
		`{"id":"req-1","type":"response","command":"get_state"}`,
	} {
		if _, err := decodeEnvelope([]byte(frame)); !errors.Is(err, ErrInvalidFrame) {
			t.Fatalf("expected malformed response to fail: %s, %v", frame, err)
		}
	}
}

type oneByteReader struct {
	data []byte
}

func (reader *oneByteReader) Read(target []byte) (int, error) {
	if len(reader.data) == 0 {
		return 0, io.EOF
	}
	target[0] = reader.data[0]
	reader.data = reader.data[1:]
	return 1, nil
}
