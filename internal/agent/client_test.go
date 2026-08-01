package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

func TestRPCClientCorrelatesOutOfOrderResponsesAndEvents(t *testing.T) {
	stdoutReader, stdoutWriter := io.Pipe()
	stdinReader, stdinWriter := io.Pipe()
	client := newRPCClient(stdoutReader, stdinWriter, MaxRPCFrameBytes)
	t.Cleanup(func() {
		_ = stdoutWriter.Close()
		_ = stdinReader.Close()
	})

	requests := make(chan map[string]any, 2)
	go readTestRequests(t, stdinReader, requests, 2)
	type outcome struct {
		value string
		err   error
	}
	results := make(chan outcome, 2)
	for _, command := range []string{"get_state", "get_entries"} {
		command := command
		go func() {
			var response struct {
				Value string `json:"value"`
			}
			err := client.Call(context.Background(), command, nil, &response)
			results <- outcome{value: response.Value, err: err}
		}()
	}

	first := <-requests
	second := <-requests
	if _, err := stdoutWriter.Write([]byte(`{"type":"agent_start"}` + "\n")); err != nil {
		t.Fatalf("write event: %v", err)
	}
	for _, request := range []map[string]any{second, first} {
		line := `{"id":"` + request["id"].(string) + `","type":"response","command":"` +
			request["type"].(string) + `","success":true,"data":{"value":"` +
			request["type"].(string) + `"}}` + "\n"
		if _, err := stdoutWriter.Write([]byte(line)); err != nil {
			t.Fatalf("write response: %v", err)
		}
	}

	seen := map[string]bool{}
	for range 2 {
		result := <-results
		if result.err != nil {
			t.Fatalf("rpc call failed: %v", result.err)
		}
		seen[result.value] = true
	}
	if !seen["get_state"] || !seen["get_entries"] {
		t.Fatalf("unexpected correlated values %#v", seen)
	}
	event := <-client.Events()
	if event.Type != "agent_start" {
		t.Fatalf("unexpected event %#v", event)
	}
}

func TestRPCClientTimeoutDoesNotRejectLateResponse(t *testing.T) {
	stdoutReader, stdoutWriter := io.Pipe()
	stdinReader, stdinWriter := io.Pipe()
	client := newRPCClient(stdoutReader, stdinWriter, MaxRPCFrameBytes)
	t.Cleanup(func() {
		_ = stdoutWriter.Close()
		_ = stdinReader.Close()
	})

	requests := make(chan map[string]any, 2)
	go readTestRequests(t, stdinReader, requests, 2)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := client.Call(ctx, "get_state", nil, nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected timeout, got %v", err)
	}
	timedOut := <-requests
	if _, err := stdoutWriter.Write([]byte(`{"id":"` + timedOut["id"].(string) + `","type":"response","command":"get_state","success":true,"data":{}}` + "\n")); err != nil {
		t.Fatalf("write late response: %v", err)
	}

	result := make(chan error, 1)
	go func() { result <- client.Call(context.Background(), "get_entries", nil, nil) }()
	request := <-requests
	if _, err := stdoutWriter.Write([]byte(`{"id":"` + request["id"].(string) + `","type":"response","command":"get_entries","success":true,"data":{}}` + "\n")); err != nil {
		t.Fatalf("write response: %v", err)
	}
	if err := <-result; err != nil {
		t.Fatalf("connection did not survive late response: %v", err)
	}
}

func TestRPCClientFailsPendingCallsOnInvalidFrame(t *testing.T) {
	stdoutReader, stdoutWriter := io.Pipe()
	stdinReader, stdinWriter := io.Pipe()
	client := newRPCClient(stdoutReader, stdinWriter, MaxRPCFrameBytes)
	t.Cleanup(func() { _ = stdinReader.Close() })

	result := make(chan error, 1)
	go func() { result <- client.Call(context.Background(), "get_state", nil, nil) }()
	reader := bufio.NewReader(stdinReader)
	if _, err := reader.ReadString('\n'); err != nil {
		t.Fatalf("read request: %v", err)
	}
	if _, err := stdoutWriter.Write([]byte("terminal noise\n")); err != nil {
		t.Fatalf("write invalid frame: %v", err)
	}
	if err := <-result; !errors.Is(err, ErrInvalidFrame) {
		t.Fatalf("expected invalid frame, got %v", err)
	}
}

func TestRPCClientWriteFailureLeavesEventClosureToReader(t *testing.T) {
	stdoutReader, stdoutWriter := io.Pipe()
	client := newRPCClient(stdoutReader, failingWriter{}, MaxRPCFrameBytes)
	t.Cleanup(func() { _ = stdoutWriter.Close() })

	if err := client.Call(context.Background(), "get_state", nil, nil); err == nil {
		t.Fatal("expected write failure")
	}
	select {
	case <-client.Done():
	case <-time.After(time.Second):
		t.Fatal("client did not publish write failure")
	}
	select {
	case _, open := <-client.Events():
		if !open {
			t.Fatal("writer closed events while the reader was still active")
		}
	default:
	}

	if err := stdoutWriter.Close(); err != nil {
		t.Fatalf("close stdout: %v", err)
	}
	select {
	case _, open := <-client.Events():
		if open {
			t.Fatal("expected reader to close events")
		}
	case <-time.After(time.Second):
		t.Fatal("reader did not close events")
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}

func readTestRequests(
	t *testing.T,
	reader io.Reader,
	requests chan<- map[string]any,
	count int,
) {
	t.Helper()
	decoder := bufio.NewReader(reader)
	for range count {
		line, err := decoder.ReadString('\n')
		if err != nil {
			return
		}
		request := map[string]any{}
		if err := json.Unmarshal([]byte(strings.TrimSpace(line)), &request); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		requests <- request
	}
}
