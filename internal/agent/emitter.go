package agent

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
)

const (
	eventDeliveryCapacity = 1024
	maxEventDeliveryBytes = 8 * 1024 * 1024
)

var errEventDeliveryBackpressure = errors.New("agent event delivery queue is full")

type queuedEvent struct {
	event Event
	size  int64
}

type eventDeliveryQueue struct {
	emit Emitter

	mutex     sync.Mutex
	closed    bool
	queue     chan queuedEvent
	done      chan struct{}
	closeOnce sync.Once
	bytes     atomic.Int64
}

func newEventDeliveryQueue(emit Emitter) *eventDeliveryQueue {
	if emit == nil {
		return nil
	}
	delivery := &eventDeliveryQueue{
		emit: emit, queue: make(chan queuedEvent, eventDeliveryCapacity),
		done: make(chan struct{}),
	}
	go delivery.run()
	return delivery
}

func (delivery *eventDeliveryQueue) enqueue(event Event) error {
	if delivery == nil {
		return nil
	}
	encoded, err := json.Marshal(event)
	if err != nil {
		return err
	}
	size := int64(len(encoded))
	delivery.mutex.Lock()
	defer delivery.mutex.Unlock()
	if delivery.closed {
		return errors.New("agent event delivery queue is closed")
	}
	if delivery.bytes.Load()+size > maxEventDeliveryBytes {
		return errEventDeliveryBackpressure
	}
	delivery.bytes.Add(size)
	select {
	case delivery.queue <- queuedEvent{event: event, size: size}:
		return nil
	default:
		delivery.bytes.Add(-size)
		return errEventDeliveryBackpressure
	}
}

func (delivery *eventDeliveryQueue) close(ctx context.Context) error {
	if delivery == nil {
		return nil
	}
	delivery.closeOnce.Do(func() {
		delivery.mutex.Lock()
		delivery.closed = true
		close(delivery.queue)
		delivery.mutex.Unlock()
	})
	select {
	case <-delivery.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (delivery *eventDeliveryQueue) run() {
	defer close(delivery.done)
	for queued := range delivery.queue {
		delivery.emit(queued.event)
		delivery.bytes.Add(-queued.size)
	}
}
