// Copyright (C) Eventuous HQ OÜ. All rights reserved
// Licensed under the Apache License, Version 2.0.

package subtest

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	eventuous "github.com/eventuous/eventuous-go/core"
	"github.com/eventuous/eventuous-go/core/store"
	"github.com/eventuous/eventuous-go/core/subscription"
)

// testEvent is the internal type used in subscription conformance tests.
type testEvent struct {
	Index int    `json:"index"`
	Data  string `json:"data"`
}

// TestEvent is the exported alias so external packages can build a matching codec.
type TestEvent = testEvent

// runID is generated once per process to ensure stream names are unique
// across test runs, even against persistent stores like KurrentDB.
var runID = uuid.New().String()[:8]

// streamCounter generates unique stream names across parallel tests.
var streamCounter atomic.Uint64

// uniqueStream returns a unique stream name for test isolation.
func uniqueStream() eventuous.StreamName {
	n := streamCounter.Add(1)
	return eventuous.StreamName(fmt.Sprintf("subtest-%s-%d", runID, n))
}

// makeEvents creates n NewStreamEvent instances with sequential indices.
func makeEvents(n int) []store.NewStreamEvent {
	events := make([]store.NewStreamEvent, n)
	for i := range events {
		events[i] = store.NewStreamEvent{
			ID:      uuid.New(),
			Payload: testEvent{Index: i, Data: fmt.Sprintf("event-%d", i)},
		}
	}
	return events
}

// collectingHandler collects events for assertion.
type collectingHandler struct {
	events chan *subscription.ConsumeContext
}

func newCollectingHandler(bufSize int) *collectingHandler {
	return &collectingHandler{
		events: make(chan *subscription.ConsumeContext, bufSize),
	}
}

func (h *collectingHandler) HandleEvent(ctx context.Context, msg *subscription.ConsumeContext) error {
	select {
	case h.events <- msg:
	case <-ctx.Done():
	}
	return nil
}

// waitForEvents waits for n events with a timeout.
func (h *collectingHandler) waitForEvents(n int, timeout time.Duration) ([]*subscription.ConsumeContext, error) {
	collected := make([]*subscription.ConsumeContext, 0, n)
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()

	for len(collected) < n {
		select {
		case msg := <-h.events:
			collected = append(collected, msg)
		case <-deadline.C:
			return collected, fmt.Errorf("timeout waiting for events: got %d of %d", len(collected), n)
		}
	}
	return collected, nil
}

// memCheckpointStore is a simple in-memory checkpoint store for testing.
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
	return subscription.Checkpoint{ID: id, Position: nil}, nil
}

func (m *memCheckpointStore) StoreCheckpoint(_ context.Context, checkpoint subscription.Checkpoint) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.checkpoints[checkpoint.ID] = checkpoint
	return nil
}

// TestConsumeProducedEvents verifies that a subscription delivers all appended
// events to the handler in position order with correct payloads.
//
//  1. Create a unique stream name
//  2. Append 5 events to the stream via cfg.Store
//  3. Create a collecting handler that records received events into a channel
//  4. Create a subscription via cfg.NewStreamSub
//  5. Start the subscription in a goroutine (with a cancellable context)
//  6. Wait for the handler to receive all 5 events (with a timeout of 5 seconds)
//  7. Cancel the context to stop the subscription
//  8. Verify: received 5 events, in correct order (by position), with correct payloads
func TestConsumeProducedEvents(t *testing.T, cfg Config) {
	t.Helper()

	const eventCount = 5

	stream := uniqueStream()
	events := makeEvents(eventCount)

	_, err := cfg.Store.AppendEvents(context.Background(), stream, eventuous.VersionNoStream, events)
	if err != nil {
		t.Fatalf("failed to append events: %v", err)
	}

	handler := newCollectingHandler(eventCount)
	cs := newMemCheckpointStore()
	sub := cfg.NewStreamSub(stream, handler, cs)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		if err := sub.Start(ctx); err != nil && ctx.Err() == nil {
			t.Errorf("subscription error: %v", err)
		}
	}()

	received, err := handler.waitForEvents(eventCount, 5*time.Second)
	cancel()

	if err != nil {
		t.Fatalf("did not receive all events: %v", err)
	}

	if len(received) != eventCount {
		t.Fatalf("expected %d events, got %d", eventCount, len(received))
	}

	for i, msg := range received {
		if msg.Position != uint64(i) {
			t.Errorf("event[%d]: expected Position=%d, got %d", i, i, msg.Position)
		}
		payload, ok := msg.Payload.(testEvent)
		if !ok {
			t.Errorf("event[%d]: unexpected payload type %T", i, msg.Payload)
			continue
		}
		if payload.Index != i {
			t.Errorf("event[%d]: expected Index=%d, got %d", i, i, payload.Index)
		}
	}
}

// TestResumeFromCheckpoint verifies that a subscription honours a pre-existing
// checkpoint and only delivers events after the checkpointed position.
//
//  1. Create a unique stream and append 5 events
//  2. Create an in-memory CheckpointStore, pre-set checkpoint to position 2
//  3. Create a collecting handler
//  4. Start subscription with the checkpoint store
//  5. Wait for events (should receive only events at positions 3 and 4 — after checkpoint)
//  6. Verify: received 2 events (not 5), starting from position 3
func TestResumeFromCheckpoint(t *testing.T, cfg Config) {
	t.Helper()

	const eventCount = 5
	const checkpointPosition = uint64(2)
	const expectedCount = 2 // positions 3 and 4

	stream := uniqueStream()
	events := makeEvents(eventCount)

	_, err := cfg.Store.AppendEvents(context.Background(), stream, eventuous.VersionNoStream, events)
	if err != nil {
		t.Fatalf("failed to append events: %v", err)
	}

	cs := newMemCheckpointStore()
	// Pre-set checkpoint to position 2 so the subscription resumes from position 3.
	pos := checkpointPosition
	if err := cs.StoreCheckpoint(context.Background(), subscription.Checkpoint{
		ID:       stream.String(),
		Position: &pos,
	}); err != nil {
		t.Fatalf("failed to pre-set checkpoint: %v", err)
	}

	handler := newCollectingHandler(eventCount)
	sub := cfg.NewStreamSub(stream, handler, cs)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		if err := sub.Start(ctx); err != nil && ctx.Err() == nil {
			t.Errorf("subscription error: %v", err)
		}
	}()

	received, err := handler.waitForEvents(expectedCount, 5*time.Second)
	cancel()

	if err != nil {
		t.Fatalf("did not receive expected events: %v", err)
	}

	if len(received) != expectedCount {
		t.Fatalf("expected %d events after checkpoint, got %d", expectedCount, len(received))
	}

	// First event after checkpoint should be at position 3.
	if received[0].Position != checkpointPosition+1 {
		t.Errorf("first resumed event: expected Position=%d, got %d", checkpointPosition+1, received[0].Position)
	}
}
