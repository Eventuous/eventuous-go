// Copyright (C) Eventuous HQ OÜ. All rights reserved
// Licensed under the Apache License, Version 2.0.

package storetest

import (
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"

	eventuous "github.com/eventuous/eventuous-go/core"
	"github.com/eventuous/eventuous-go/core/codec"
	"github.com/eventuous/eventuous-go/core/store"
)

// runID is generated once per process to ensure stream names are unique
// across test runs, even against persistent stores like KurrentDB.
var runID = uuid.New().String()[:8]

// RunAll runs the full conformance suite against the given store.
func RunAll(t *testing.T, s store.EventStore) {
	t.Helper()
	t.Run("Append", func(t *testing.T) { TestAppend(t, s) })
	t.Run("Read", func(t *testing.T) { TestRead(t, s) })
	t.Run("ReadBackwards", func(t *testing.T) { TestReadBackwards(t, s) })
	t.Run("OptimisticConcurrency", func(t *testing.T) { TestOptimisticConcurrency(t, s) })
	t.Run("StreamExists", func(t *testing.T) { TestStreamExists(t, s) })
	t.Run("DeleteStream", func(t *testing.T) { TestDeleteStream(t, s) })
	t.Run("TruncateStream", func(t *testing.T) { TestTruncateStream(t, s) })
}

// TestEvent is a simple payload for conformance tests.
type TestEvent = testEvent

// testEvent is the internal type used in conformance tests.
type testEvent struct {
	Index int    `json:"index"`
	Data  string `json:"data"`
}

// NewCodec creates a JSON codec with the conformance test event type registered.
func NewCodec() codec.Codec {
	tm := codec.NewTypeMap()
	if err := codec.Register[testEvent](tm, "TestEvent"); err != nil {
		panic(err)
	}
	return codec.NewJSON(tm)
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

// streamCounter is used to generate unique stream names.
var streamCounter atomic.Uint64

// uniqueStream returns a unique stream name for test isolation.
// Names include a per-process UUID prefix so tests work against persistent
// stores (like KurrentDB) that retain data across runs.
func uniqueStream() eventuous.StreamName {
	n := streamCounter.Add(1)
	return eventuous.StreamName(fmt.Sprintf("test-%s-%d", runID, n))
}
