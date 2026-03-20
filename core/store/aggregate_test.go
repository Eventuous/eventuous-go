// Copyright (C) Eventuous HQ OÜ. All rights reserved
// Licensed under the Apache License, Version 2.0.

package store_test

import (
	"context"
	"errors"
	"testing"

	eventuous "github.com/eventuous/eventuous-go/core"
	"github.com/eventuous/eventuous-go/core/aggregate"
	"github.com/eventuous/eventuous-go/core/store"
	"github.com/eventuous/eventuous-go/core/test/memstore"
)

func newCounterAggregate() *aggregate.Aggregate[counter] {
	return aggregate.New(counterFold, counter{})
}

// TestLoadAggregate_NonExistentStream: stream doesn't exist → new aggregate, version -1.
func TestLoadAggregate_NonExistentStream(t *testing.T) {
	s := memstore.New()
	stream := eventuous.StreamName("counter-nonexistent")

	agg, err := store.LoadAggregate(
		context.Background(), s, stream, counterFold, counter{},
	)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if agg.OriginalVersion() != -1 {
		t.Errorf("expected OriginalVersion -1, got %d", agg.OriginalVersion())
	}
	if agg.State().Count != 0 {
		t.Errorf("expected Count=0, got Count=%d", agg.State().Count)
	}
}

// TestStoreAggregate_AppendsChanges: Apply events, store, then ReadEvents to verify.
func TestStoreAggregate_AppendsChanges(t *testing.T) {
	s := memstore.New()
	stream := eventuous.StreamName("counter-store")

	agg := newCounterAggregate()
	agg.Apply(added{})
	agg.Apply(added{})

	result, err := store.StoreAggregate(context.Background(), s, stream, agg)
	if err != nil {
		t.Fatalf("StoreAggregate: %v", err)
	}
	if result.NextExpectedVersion != 1 {
		t.Errorf("expected NextExpectedVersion 1, got %d", result.NextExpectedVersion)
	}

	// Verify events were actually written.
	events, err := s.ReadEvents(context.Background(), stream, 0, 100)
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	if len(events) != 2 {
		t.Errorf("expected 2 events in stream, got %d", len(events))
	}
}

// TestLoadAggregate_AfterStore: store then load → state matches, version correct.
func TestLoadAggregate_AfterStore(t *testing.T) {
	s := memstore.New()
	stream := eventuous.StreamName("counter-roundtrip")

	agg := newCounterAggregate()
	agg.Apply(added{})
	agg.Apply(added{})
	agg.Apply(added{})

	_, err := store.StoreAggregate(context.Background(), s, stream, agg)
	if err != nil {
		t.Fatalf("StoreAggregate: %v", err)
	}

	loaded, err := store.LoadAggregate(
		context.Background(), s, stream, counterFold, counter{},
	)
	if err != nil {
		t.Fatalf("LoadAggregate: %v", err)
	}
	if loaded.State().Count != 3 {
		t.Errorf("expected Count=3, got Count=%d", loaded.State().Count)
	}
	if loaded.OriginalVersion() != 2 {
		t.Errorf("expected OriginalVersion=2, got %d", loaded.OriginalVersion())
	}
	if len(loaded.Changes()) != 0 {
		t.Errorf("expected 0 changes after load, got %d", len(loaded.Changes()))
	}
}

// TestStoreAggregate_NoChanges: no-op when there are no pending changes.
func TestStoreAggregate_NoChanges(t *testing.T) {
	s := memstore.New()
	stream := eventuous.StreamName("counter-nochanges")

	agg := newCounterAggregate()
	// No Apply calls.

	result, err := store.StoreAggregate(context.Background(), s, stream, agg)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	// result should be zero value
	if result.NextExpectedVersion != 0 || result.GlobalPosition != 0 {
		t.Errorf("expected zero AppendResult, got %+v", result)
	}

	// Stream should not exist.
	exists, err := s.StreamExists(context.Background(), stream)
	if err != nil {
		t.Fatalf("StreamExists: %v", err)
	}
	if exists {
		t.Error("expected stream to not exist after no-op store")
	}
}

// TestStoreAggregate_OptimisticConcurrency: load, store, then try to store
// the same (now stale) aggregate again → ErrOptimisticConcurrency.
func TestStoreAggregate_OptimisticConcurrency(t *testing.T) {
	s := memstore.New()
	stream := eventuous.StreamName("counter-occ")

	// First: store initial state.
	agg1 := newCounterAggregate()
	agg1.Apply(added{})
	_, err := store.StoreAggregate(context.Background(), s, stream, agg1)
	if err != nil {
		t.Fatalf("first StoreAggregate: %v", err)
	}

	// Load the aggregate (version=0).
	agg2, err := store.LoadAggregate(
		context.Background(), s, stream, counterFold, counter{},
	)
	if err != nil {
		t.Fatalf("LoadAggregate: %v", err)
	}
	agg2.Apply(added{})

	// Concurrently store another event via a third actor.
	agg3 := newCounterAggregate()
	agg3.Apply(added{})
	// Manually set up stale scenario: Load agg3 at version 0, then apply.
	agg3.Load(0, []any{added{}})
	agg3.Apply(added{})

	// Store agg2 successfully.
	_, err = store.StoreAggregate(context.Background(), s, stream, agg2)
	if err != nil {
		t.Fatalf("second StoreAggregate: %v", err)
	}

	// Now try to store agg3 which is still at original version 0 — should conflict.
	_, err = store.StoreAggregate(context.Background(), s, stream, agg3)
	if !errors.Is(err, eventuous.ErrOptimisticConcurrency) {
		t.Errorf("expected ErrOptimisticConcurrency, got %v", err)
	}
}
