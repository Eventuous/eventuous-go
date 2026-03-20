package kurrentdb_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	eventuous "github.com/eventuous/eventuous-go/core"
	"github.com/eventuous/eventuous-go/core/subscription"
	"github.com/eventuous/eventuous-go/core/test/testdomain"
	kdb "github.com/eventuous/eventuous-go/kurrentdb"
)

// persistentCollectingHandler collects ConsumeContext values and tracks retry counts.
type persistentCollectingHandler struct {
	mu     sync.Mutex
	events []*subscription.ConsumeContext
	done   chan struct{}
	want   int
}

func newPersistentCollectingHandler(want int) *persistentCollectingHandler {
	return &persistentCollectingHandler{
		events: make([]*subscription.ConsumeContext, 0, want),
		done:   make(chan struct{}),
		want:   want,
	}
}

func (h *persistentCollectingHandler) HandleEvent(_ context.Context, msg *subscription.ConsumeContext) error {
	h.mu.Lock()
	h.events = append(h.events, msg)
	if len(h.events) >= h.want {
		select {
		case <-h.done:
		default:
			close(h.done)
		}
	}
	h.mu.Unlock()
	return nil
}

func (h *persistentCollectingHandler) wait(timeout time.Duration) []*subscription.ConsumeContext {
	select {
	case <-h.done:
	case <-time.After(timeout):
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	cp := make([]*subscription.ConsumeContext, len(h.events))
	copy(cp, h.events)
	return cp
}

func TestPersistent_Stream_ConsumesEvents(t *testing.T) {
	client := setupClient(t)
	c := testdomain.NewCodec()
	s := kdb.NewStore(client, c)

	stream := eventuous.NewStreamName("PersistTest", uuid.New().String())
	appendTestEvents(t, s, stream, 3)

	groupName := "group-" + uuid.New().String()
	handler := newPersistentCollectingHandler(3)

	sub := kdb.NewPersistent(client, c, groupName,
		kdb.PersistentFromStream(stream),
		kdb.PersistentWithHandler(handler),
		kdb.PersistentWithBufferSize(10),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- sub.Start(ctx) }()

	events := handler.wait(15 * time.Second)
	cancel()

	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}

	for i, e := range events {
		if e.EventType != "RoomBooked" {
			t.Errorf("event %d: expected EventType RoomBooked, got %s", i, e.EventType)
		}
		if e.Payload == nil {
			t.Errorf("event %d: expected non-nil Payload", i)
		}
		if e.Stream != stream {
			t.Errorf("event %d: expected Stream %s, got %s", i, stream, e.Stream)
		}
		if e.SubscriptionID != groupName {
			t.Errorf("event %d: expected SubscriptionID %s, got %s", i, groupName, e.SubscriptionID)
		}
	}

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Start returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Error("Start did not return after context cancellation")
	}
}

func TestPersistent_Stream_AcksOnSuccess(t *testing.T) {
	client := setupClient(t)
	c := testdomain.NewCodec()
	s := kdb.NewStore(client, c)

	stream := eventuous.NewStreamName("PersistAck", uuid.New().String())
	appendTestEvents(t, s, stream, 3)

	groupName := "group-" + uuid.New().String()

	handler := newPersistentCollectingHandler(3)

	sub := kdb.NewPersistent(client, c, groupName,
		kdb.PersistentFromStream(stream),
		kdb.PersistentWithHandler(handler),
		kdb.PersistentWithBufferSize(10),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- sub.Start(ctx) }()

	events := handler.wait(15 * time.Second)

	if len(events) != 3 {
		cancel()
		t.Fatalf("expected 3 events, got %d", len(events))
	}

	// All 3 events were received and acked (handler returned nil).
	// Give the server a moment to checkpoint the acks before closing.
	time.Sleep(1 * time.Second)
	cancel()

	// Wait for the first subscription to cleanly shut down.
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Start returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Error("Start did not return after context cancellation")
	}

	// Reconnect to the same persistent subscription group and see if there are
	// any events left (there shouldn't be because they were acked).
	handler2 := newPersistentCollectingHandler(1) // want 1 but expect 0
	sub2 := kdb.NewPersistent(client, c, groupName,
		kdb.PersistentFromStream(stream),
		kdb.PersistentWithHandler(handler2),
		kdb.PersistentWithBufferSize(10),
	)

	ctx2, cancel2 := context.WithCancel(context.Background())
	errCh2 := make(chan error, 1)
	go func() { errCh2 <- sub2.Start(ctx2) }()

	// Wait a short time to see if any events are re-delivered.
	events2 := handler2.wait(3 * time.Second)
	cancel2()

	if len(events2) > 0 {
		t.Errorf("expected 0 events on reconnect (all should be acked), got %d", len(events2))
	}

	select {
	case err := <-errCh2:
		if err != nil {
			t.Errorf("second Start returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Error("second Start did not return after context cancellation")
	}
}

func TestPersistent_Stream_NacksOnError(t *testing.T) {
	client := setupClient(t)
	c := testdomain.NewCodec()
	s := kdb.NewStore(client, c)

	stream := eventuous.NewStreamName("PersistNack", uuid.New().String())
	appendTestEvents(t, s, stream, 1)

	groupName := "group-" + uuid.New().String()

	// Handler that fails on the first attempt, succeeds on subsequent ones.
	var callCount atomic.Int32
	var mu sync.Mutex
	var received []*subscription.ConsumeContext
	done := make(chan struct{})

	failOnceHandler := subscription.HandlerFunc(func(_ context.Context, msg *subscription.ConsumeContext) error {
		count := callCount.Add(1)
		mu.Lock()
		received = append(received, msg)
		total := len(received)
		mu.Unlock()

		if count == 1 {
			return errors.New("simulated failure")
		}

		// On retry (count >= 2), succeed and signal done.
		if total >= 2 {
			select {
			case <-done:
			default:
				close(done)
			}
		}
		return nil
	})

	sub := kdb.NewPersistent(client, c, groupName,
		kdb.PersistentFromStream(stream),
		kdb.PersistentWithHandler(failOnceHandler),
		kdb.PersistentWithBufferSize(10),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- sub.Start(ctx) }()

	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("timed out waiting for retry")
	}

	cancel()

	mu.Lock()
	receivedCount := len(received)
	mu.Unlock()

	if receivedCount < 2 {
		t.Fatalf("expected at least 2 deliveries (1 fail + 1 success), got %d", receivedCount)
	}

	// The event should have arrived at least twice (same EventType).
	mu.Lock()
	firstType := received[0].EventType
	mu.Unlock()
	if firstType != "RoomBooked" {
		t.Errorf("expected EventType RoomBooked, got %s", firstType)
	}

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Start returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Error("Start did not return after context cancellation")
	}
}

func TestPersistent_AutoCreatesGroup(t *testing.T) {
	client := setupClient(t)
	c := testdomain.NewCodec()
	s := kdb.NewStore(client, c)

	stream := eventuous.NewStreamName("PersistAuto", uuid.New().String())
	appendTestEvents(t, s, stream, 2)

	// Use a unique group name that hasn't been pre-created.
	groupName := fmt.Sprintf("auto-group-%s", uuid.New().String())
	handler := newPersistentCollectingHandler(2)

	sub := kdb.NewPersistent(client, c, groupName,
		kdb.PersistentFromStream(stream),
		kdb.PersistentWithHandler(handler),
		kdb.PersistentWithBufferSize(10),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- sub.Start(ctx) }()

	events := handler.wait(15 * time.Second)
	cancel()

	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}

	for i, e := range events {
		if e.EventType != "RoomBooked" {
			t.Errorf("event %d: expected EventType RoomBooked, got %s", i, e.EventType)
		}
	}

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Start returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Error("Start did not return after context cancellation")
	}
}
