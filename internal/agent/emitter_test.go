package agent

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestEventDeliveryQueueIsBoundedAndDoesNotCallEmitterInline(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	delivery := newEventDeliveryQueue(func(Event) {
		select {
		case <-started:
		default:
			close(started)
		}
		<-release
	})
	if err := delivery.enqueue(Event{Version: 1, EventID: "first", Payload: map[string]any{}}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("event emitter worker did not start")
	}
	for index := 0; index < eventDeliveryCapacity; index++ {
		if err := delivery.enqueue(Event{
			Version: 1, EventID: newID("event"), Payload: map[string]any{},
		}); err != nil {
			t.Fatalf("fill delivery queue at %d: %v", index, err)
		}
	}
	if err := delivery.enqueue(Event{
		Version: 1, EventID: "overflow", Payload: map[string]any{},
	}); !errors.Is(err, errEventDeliveryBackpressure) {
		t.Fatalf("expected bounded queue backpressure, got %v", err)
	}
	close(release)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := delivery.close(ctx); err != nil {
		t.Fatal(err)
	}
}
