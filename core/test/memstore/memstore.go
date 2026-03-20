// Copyright (C) Eventuous HQ OÜ. All rights reserved
// Licensed under the Apache License, Version 2.0.

package memstore

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"

	eventuous "github.com/eventuous/eventuous-go/core"
	"github.com/eventuous/eventuous-go/core/store"
)

// Compile-time interface check.
var _ store.EventStore = (*Store)(nil)

// Store is a thread-safe in-memory EventStore for testing.
type Store struct {
	mu             sync.Mutex
	streams        map[eventuous.StreamName][]store.StreamEvent
	globalPosition uint64
}

// New creates a new in-memory Store.
func New() *Store {
	return &Store{
		streams: make(map[eventuous.StreamName][]store.StreamEvent),
	}
}

// AppendEvents appends events to a stream, performing optimistic concurrency
// checks based on the expected version.
func (s *Store) AppendEvents(ctx context.Context, stream eventuous.StreamName, expected eventuous.ExpectedVersion, events []store.NewStreamEvent) (store.AppendResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, exists := s.streams[stream]
	currentLen := len(existing)

	switch expected {
	case eventuous.VersionNoStream:
		// Stream must NOT exist (or be empty).
		if exists && currentLen > 0 {
			return store.AppendResult{}, fmt.Errorf("%w: stream %q already exists", eventuous.ErrOptimisticConcurrency, stream)
		}
	case eventuous.VersionAny:
		// No version check; always succeeds.
	default:
		// expected >= 0: current stream length - 1 must equal expected.
		currentVersion := int64(currentLen) - 1
		if currentVersion != int64(expected) {
			return store.AppendResult{}, fmt.Errorf("%w: expected version %d but stream %q is at version %d", eventuous.ErrOptimisticConcurrency, expected, stream, currentVersion)
		}
	}

	now := time.Now()
	for i, e := range events {
		id := e.ID
		if id == uuid.Nil {
			id = uuid.New()
		}

		streamEvent := store.StreamEvent{
			ID:             id,
			EventType:      fmt.Sprintf("%T", e.Payload),
			Payload:        e.Payload,
			Metadata:       e.Metadata,
			Position:       int64(currentLen + i),
			GlobalPosition: s.globalPosition,
			Created:        now,
		}
		s.globalPosition++
		existing = append(existing, streamEvent)
	}

	s.streams[stream] = existing

	lastGlobalPos := uint64(0)
	if len(existing) > 0 {
		lastGlobalPos = existing[len(existing)-1].GlobalPosition
	}

	return store.AppendResult{
		GlobalPosition:      lastGlobalPos,
		NextExpectedVersion: int64(len(existing)) - 1,
	}, nil
}

// ReadEvents reads up to count events from the stream starting at the given
// stream position (0-based index).
func (s *Store) ReadEvents(ctx context.Context, stream eventuous.StreamName, start uint64, count int) ([]store.StreamEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	events, exists := s.streams[stream]
	if !exists || len(events) == 0 {
		return nil, fmt.Errorf("%w: %q", eventuous.ErrStreamNotFound, stream)
	}

	if int(start) >= len(events) {
		return []store.StreamEvent{}, nil
	}

	end := int(start) + count
	if end > len(events) {
		end = len(events)
	}

	result := make([]store.StreamEvent, end-int(start))
	copy(result, events[start:end])
	return result, nil
}

// ReadEventsBackwards reads up to count events from the stream in reverse
// order, starting from position start (inclusive). If start >= stream length,
// reading begins from the last event.
func (s *Store) ReadEventsBackwards(ctx context.Context, stream eventuous.StreamName, start uint64, count int) ([]store.StreamEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	events, exists := s.streams[stream]
	if !exists || len(events) == 0 {
		return nil, fmt.Errorf("%w: %q", eventuous.ErrStreamNotFound, stream)
	}

	// Guard against overflow when start is a sentinel "very large" value such
	// as ^uint64(0). Use the last event index in that case.
	var fromIdx int
	if start >= uint64(len(events)) {
		fromIdx = len(events) - 1
	} else {
		fromIdx = int(start)
	}

	// Collect up to count events going backwards from fromIdx.
	result := make([]store.StreamEvent, 0, count)
	for i := fromIdx; i >= 0 && len(result) < count; i-- {
		result = append(result, events[i])
	}
	return result, nil
}

// StreamExists returns true if the stream exists and contains at least one
// event.
func (s *Store) StreamExists(ctx context.Context, stream eventuous.StreamName) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	events, exists := s.streams[stream]
	return exists && len(events) > 0, nil
}

// DeleteStream removes a stream entirely. The expected version is validated
// using the same rules as AppendEvents.
func (s *Store) DeleteStream(ctx context.Context, stream eventuous.StreamName, expected eventuous.ExpectedVersion) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, exists := s.streams[stream]
	currentLen := len(existing)

	switch expected {
	case eventuous.VersionNoStream:
		if exists && currentLen > 0 {
			return fmt.Errorf("%w: stream %q already exists", eventuous.ErrOptimisticConcurrency, stream)
		}
	case eventuous.VersionAny:
		// No check.
	default:
		currentVersion := int64(currentLen) - 1
		if currentVersion != int64(expected) {
			return fmt.Errorf("%w: expected version %d but stream %q is at version %d", eventuous.ErrOptimisticConcurrency, expected, stream, currentVersion)
		}
	}

	delete(s.streams, stream)
	return nil
}

// TruncateStream removes all events with Position < truncatePosition from the
// stream. The expected version is validated before truncation.
func (s *Store) TruncateStream(ctx context.Context, stream eventuous.StreamName, position uint64, expected eventuous.ExpectedVersion) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, exists := s.streams[stream]
	currentLen := len(existing)

	switch expected {
	case eventuous.VersionNoStream:
		if exists && currentLen > 0 {
			return fmt.Errorf("%w: stream %q already exists", eventuous.ErrOptimisticConcurrency, stream)
		}
	case eventuous.VersionAny:
		// No check.
	default:
		currentVersion := int64(currentLen) - 1
		if currentVersion != int64(expected) {
			return fmt.Errorf("%w: expected version %d but stream %q is at version %d", eventuous.ErrOptimisticConcurrency, expected, stream, currentVersion)
		}
	}

	// Keep only events with Position >= truncatePosition.
	kept := existing[:0]
	for _, e := range existing {
		if uint64(e.Position) >= position {
			kept = append(kept, e)
		}
	}
	s.streams[stream] = kept
	return nil
}
