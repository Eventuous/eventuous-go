package kurrentdb_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	eventuous "github.com/eventuous/eventuous-go/core"
	"github.com/eventuous/eventuous-go/core/store"
	"github.com/eventuous/eventuous-go/core/subscription"
	"github.com/eventuous/eventuous-go/core/test/testdomain"
	kdb "github.com/eventuous/eventuous-go/kurrentdb"
)

// collectingHandler collects ConsumeContext values until the desired count is reached.
type collectingHandler struct {
	mu     sync.Mutex
	events []*subscription.ConsumeContext
	done   chan struct{}
	want   int
}

func newCollectingHandler(want int) *collectingHandler {
	return &collectingHandler{
		events: make([]*subscription.ConsumeContext, 0, want),
		done:   make(chan struct{}),
		want:   want,
	}
}

func (h *collectingHandler) HandleEvent(_ context.Context, msg *subscription.ConsumeContext) error {
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

func (h *collectingHandler) wait(timeout time.Duration) []*subscription.ConsumeContext {
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

// memCheckpointStore is an in-memory CheckpointStore for testing.
type memCheckpointStore struct {
	mu          sync.Mutex
	checkpoints map[string]subscription.Checkpoint
}

func newMemCheckpointStore() *memCheckpointStore {
	return &memCheckpointStore{
		checkpoints: make(map[string]subscription.Checkpoint),
	}
}

func (m *memCheckpointStore) GetCheckpoint(_ context.Context, id string) (subscription.Checkpoint, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if cp, ok := m.checkpoints[id]; ok {
		return cp, nil
	}
	return subscription.Checkpoint{ID: id}, nil
}

func (m *memCheckpointStore) StoreCheckpoint(_ context.Context, cp subscription.Checkpoint) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.checkpoints[cp.ID] = cp
	return nil
}

// appendTestEvents appends n RoomBooked events to the given stream.
func appendTestEvents(t *testing.T, s *kdb.Store, stream eventuous.StreamName, n int) {
	t.Helper()
	events := make([]store.NewStreamEvent, n)
	for i := range events {
		events[i] = store.NewStreamEvent{
			ID:      uuid.New(),
			Payload: testdomain.RoomBooked{BookingID: stream.ID(), RoomID: fmt.Sprintf("room-%d", i), Price: 100},
		}
	}
	_, err := s.AppendEvents(context.Background(), stream, eventuous.VersionNoStream, events)
	if err != nil {
		t.Fatal(err)
	}
}

func TestCatchUp_Stream_ConsumesEvents(t *testing.T) {
	client := setupClient(t)
	c := testdomain.NewCodec()
	s := kdb.NewStore(client, c)

	stream := eventuous.NewStreamName("CatchUpTest", uuid.New().String())
	appendTestEvents(t, s, stream, 5)

	handler := newCollectingHandler(5)
	sub := kdb.NewCatchUp(client, c, "test-stream-sub",
		kdb.FromStream(stream),
		kdb.WithHandler(handler),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- sub.Start(ctx) }()

	events := handler.wait(10 * time.Second)
	cancel()

	if len(events) != 5 {
		t.Fatalf("expected 5 events, got %d", len(events))
	}

	// Verify order: Position should be 0, 1, 2, 3, 4.
	for i, e := range events {
		if e.Position != uint64(i) {
			t.Errorf("event %d: expected Position %d, got %d", i, i, e.Position)
		}
		if e.EventType != "RoomBooked" {
			t.Errorf("event %d: expected EventType RoomBooked, got %s", i, e.EventType)
		}
		if e.Payload == nil {
			t.Errorf("event %d: expected non-nil Payload", i)
		}
		if e.Stream != stream {
			t.Errorf("event %d: expected Stream %s, got %s", i, stream, e.Stream)
		}
		if e.SubscriptionID != "test-stream-sub" {
			t.Errorf("event %d: expected SubscriptionID test-stream-sub, got %s", i, e.SubscriptionID)
		}
		if e.Sequence != uint64(i+1) {
			t.Errorf("event %d: expected Sequence %d, got %d", i, i+1, e.Sequence)
		}
	}

	// Ensure Start returns cleanly after cancellation.
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Start returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Error("Start did not return after context cancellation")
	}
}

func TestCatchUp_All_ConsumesEvents(t *testing.T) {
	client := setupClient(t)
	c := testdomain.NewCodec()
	s := kdb.NewStore(client, c)

	stream1 := eventuous.NewStreamName("CatchUpAll", uuid.New().String())
	stream2 := eventuous.NewStreamName("CatchUpAll", uuid.New().String())
	appendTestEvents(t, s, stream1, 3)
	appendTestEvents(t, s, stream2, 2)

	// We want at least 5 events (the ones we just appended). There may be more from $all.
	handler := newCollectingHandler(5)
	sub := kdb.NewCatchUp(client, c, "test-all-sub",
		kdb.FromAll(),
		kdb.WithHandler(handler),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- sub.Start(ctx) }()

	events := handler.wait(10 * time.Second)
	cancel()

	if len(events) < 5 {
		t.Fatalf("expected at least 5 events, got %d", len(events))
	}

	// Verify that events from both streams are present.
	foundStream1 := false
	foundStream2 := false
	for _, e := range events {
		if e.Stream == stream1 {
			foundStream1 = true
		}
		if e.Stream == stream2 {
			foundStream2 = true
		}
	}
	if !foundStream1 {
		t.Errorf("no events found from stream %s", stream1)
	}
	if !foundStream2 {
		t.Errorf("no events found from stream %s", stream2)
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

func TestCatchUp_Stream_ResumesFromCheckpoint(t *testing.T) {
	client := setupClient(t)
	c := testdomain.NewCodec()
	s := kdb.NewStore(client, c)

	stream := eventuous.NewStreamName("CatchUpResume", uuid.New().String())
	appendTestEvents(t, s, stream, 5)

	// Pre-set checkpoint at position 2 (meaning events 0, 1, 2 have been processed).
	// The subscription should start AFTER position 2, so we expect events at positions 3 and 4.
	cpStore := newMemCheckpointStore()
	pos := uint64(2)
	if err := cpStore.StoreCheckpoint(context.Background(), subscription.Checkpoint{
		ID:       "test-resume-sub",
		Position: &pos,
	}); err != nil {
		t.Fatal(err)
	}

	handler := newCollectingHandler(2)
	sub := kdb.NewCatchUp(client, c, "test-resume-sub",
		kdb.FromStream(stream),
		kdb.WithHandler(handler),
		kdb.WithCheckpointStore(cpStore),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- sub.Start(ctx) }()

	events := handler.wait(10 * time.Second)
	cancel()

	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}

	// Verify positions are 3 and 4.
	if events[0].Position != 3 {
		t.Errorf("first event: expected Position 3, got %d", events[0].Position)
	}
	if events[1].Position != 4 {
		t.Errorf("second event: expected Position 4, got %d", events[1].Position)
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
