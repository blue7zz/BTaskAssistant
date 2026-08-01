package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

var ErrRPCClosed = errors.New("pi rpc connection is closed")

type rpcCallResult struct {
	envelope rpcEnvelope
	err      error
}

type pendingCall struct {
	command string
	result  chan rpcCallResult
}

type rpcClient struct {
	reader *bufio.Reader
	writer io.Writer
	limit  int

	writeMutex sync.Mutex
	mutex      sync.Mutex
	pending    map[string]pendingCall
	closedErr  error
	closeOnce  sync.Once
	done       chan struct{}
	events     chan rawEvent
	nextID     atomic.Uint64
}

func newRPCClient(reader io.Reader, writer io.Writer, limit int) *rpcClient {
	if limit <= 0 {
		limit = MaxRPCFrameBytes
	}
	client := &rpcClient{
		reader:  bufio.NewReaderSize(reader, 64*1024),
		writer:  writer,
		limit:   limit,
		pending: make(map[string]pendingCall),
		done:    make(chan struct{}),
		events:  make(chan rawEvent, 512),
	}
	go client.readLoop()
	return client
}

func (client *rpcClient) Call(
	ctx context.Context,
	command string,
	fields map[string]any,
	result any,
) error {
	if command == "" {
		return errors.New("pi rpc command is required")
	}
	id := "btask-" + strconv.FormatUint(client.nextID.Add(1), 10)
	request := make(map[string]any, len(fields)+2)
	request["id"] = id
	request["type"] = command
	for key, value := range fields {
		if key == "id" || key == "type" {
			return fmt.Errorf("pi rpc field %q is reserved", key)
		}
		request[key] = value
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("encode pi rpc %s request: %w", command, err)
	}
	if len(encoded)+1 > client.limit {
		return ErrFrameTooLarge
	}
	encoded = append(encoded, '\n')

	pending := pendingCall{command: command, result: make(chan rpcCallResult, 1)}
	client.mutex.Lock()
	if client.closedErr != nil {
		err := client.closedErr
		client.mutex.Unlock()
		return err
	}
	client.pending[id] = pending
	client.mutex.Unlock()

	client.writeMutex.Lock()
	_, writeErr := client.writer.Write(encoded)
	client.writeMutex.Unlock()
	if writeErr != nil {
		client.removePending(id)
		client.fail(fmt.Errorf("write pi rpc %s request: %w", command, writeErr))
		return writeErr
	}

	select {
	case received := <-pending.result:
		if received.err != nil {
			return received.err
		}
		envelope := received.envelope
		if envelope.Command != command {
			err := fmt.Errorf(
				"pi rpc response command mismatch: requested %s, received %s",
				command,
				envelope.Command,
			)
			client.fail(err)
			return err
		}
		if envelope.Success == nil || !*envelope.Success {
			message := envelope.Error
			if message == "" {
				message = "unknown rejection"
			}
			return fmt.Errorf("pi rpc %s rejected: %s", command, message)
		}
		if result != nil && len(envelope.Data) > 0 && string(envelope.Data) != "null" {
			if err := json.Unmarshal(envelope.Data, result); err != nil {
				return fmt.Errorf("decode pi rpc %s response: %w", command, err)
			}
		}
		return nil
	case <-ctx.Done():
		client.removePending(id)
		return ctx.Err()
	case <-client.done:
		client.mutex.Lock()
		err := client.closedErr
		client.mutex.Unlock()
		if err == nil {
			err = ErrRPCClosed
		}
		return err
	}
}

func (client *rpcClient) Send(ctx context.Context, fields map[string]any) error {
	requestType, _ := fields["type"].(string)
	if strings.TrimSpace(requestType) == "" {
		return errors.New("pi rpc notification type is required")
	}
	encoded, err := json.Marshal(fields)
	if err != nil {
		return fmt.Errorf("encode pi rpc %s notification: %w", requestType, err)
	}
	if len(encoded)+1 > client.limit {
		return ErrFrameTooLarge
	}
	encoded = append(encoded, '\n')
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-client.done:
		return ErrRPCClosed
	default:
	}
	client.writeMutex.Lock()
	_, writeErr := client.writer.Write(encoded)
	client.writeMutex.Unlock()
	if writeErr != nil {
		client.fail(fmt.Errorf("write pi rpc %s notification: %w", requestType, writeErr))
	}
	return writeErr
}

func (client *rpcClient) Events() <-chan rawEvent {
	return client.events
}

func (client *rpcClient) Done() <-chan struct{} {
	return client.done
}

func (client *rpcClient) Err() error {
	client.mutex.Lock()
	defer client.mutex.Unlock()
	return client.closedErr
}

func (client *rpcClient) readLoop() {
	// Only the reader owns the events channel. A concurrent write failure may
	// close done while this goroutine is selecting a send; closing events from
	// that writer path would race with the send and can panic.
	defer close(client.events)
	for {
		frame, err := readLFFrame(client.reader, client.limit)
		if err != nil {
			client.fail(err)
			return
		}
		if len(frame) == 0 {
			continue
		}
		envelope, err := decodeEnvelope(frame)
		if err != nil {
			client.fail(err)
			return
		}
		if envelope.Type == "response" {
			client.resolve(envelope)
			continue
		}
		select {
		case client.events <- rawEvent{Type: envelope.Type, JSON: append(json.RawMessage(nil), frame...)}:
		case <-client.done:
			return
		}
	}
}

func (client *rpcClient) resolve(envelope rpcEnvelope) {
	client.mutex.Lock()
	pending, exists := client.pending[envelope.ID]
	if exists {
		delete(client.pending, envelope.ID)
	}
	client.mutex.Unlock()
	// A response may arrive after its caller timed out. It is still a valid PI
	// frame, so discard it instead of poisoning the long-lived session.
	if !exists {
		return
	}
	pending.result <- rpcCallResult{envelope: envelope}
}

func (client *rpcClient) removePending(id string) {
	client.mutex.Lock()
	delete(client.pending, id)
	client.mutex.Unlock()
}

func (client *rpcClient) fail(err error) {
	if err == nil {
		err = ErrRPCClosed
	}
	client.closeOnce.Do(func() {
		client.mutex.Lock()
		client.closedErr = err
		pending := client.pending
		client.pending = make(map[string]pendingCall)
		client.mutex.Unlock()
		for _, call := range pending {
			call.result <- rpcCallResult{err: err}
		}
		close(client.done)
	})
}
