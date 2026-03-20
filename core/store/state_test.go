// Copyright (C) Eventuous HQ OÜ. All rights reserved
// Licensed under the Apache License, Version 2.0.

package store_test

import (
	"context"
	"errors"
	"testing"

	eventuous "github.com/eventuous/eventuous-go/core"
	"github.com/eventuous/eventuous-go/core/store"
	"github.com/eventuous/eventuous-go/core/test/memstore"
)

// Test domain types shared across state and aggregate tests.

type counter struct{ Count int }

type added struct{}

func counterFold(s counter, e any) counter {
	switch e.(type) {
	case added:
		return counter{Count: s.Count + 1}
	default:
		return s
	}
}

// seedEvents appends n added{} events to the given stream.
func seedEvents(t *testing.T, s *memstore.Store, stream eventuous.StreamName, n int) {
	t.Helper()
	events := make([]store.NewStreamEvent, n)
	for i := range events {
		events[i] = store.NewStreamEvent{Payload: added{}}
	}
	_, err := s.AppendEvents(context.Background(), stream, eventuous.VersionNoStream, events)
	if err != nil {
		t.Fatalf("seed: AppendEvents: %v", err)
	}
}

// --- LoadState tests ---

// TestLoadState_IsNew_EmptyStream: no stream, IsNew → zero state, nil events, VersionNoStream, no error.
func TestLoadState_IsNew_EmptyStream(t *testing.T) {
	s := memstore.New()
	stream := eventuous.StreamName("test-new-empty")

	state, events, version, err := store.LoadState(
		context.Background(), s, stream, counterFold, counter{}, eventuous.IsNew,
	)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if state.Count != 0 {
		t.Errorf("expected zero state (Count=0), got Count=%d", state.Count)
	}
	if events != nil {
		t.Errorf("expected nil events, got %v", events)
	}
	if version != eventuous.VersionNoStream {
		t.Errorf("expected VersionNoStream (%d), got %d", eventuous.VersionNoStream, version)
	}
}

// TestLoadState_IsNew_StreamExists: stream exists, IsNew → returns error.
func TestLoadState_IsNew_StreamExists(t *testing.T) {
	s := memstore.New()
	stream := eventuous.StreamName("test-new-exists")
	seedEvents(t, s, stream, 2)

	_, _, _, err := store.LoadState(
		context.Background(), s, stream, counterFold, counter{}, eventuous.IsNew,
	)

	if err == nil {
		t.Fatal("expected error for IsNew on existing stream, got nil")
	}
}

// TestLoadState_IsExisting_StreamExists: 3 events, IsExisting → count=3, version=2.
func TestLoadState_IsExisting_StreamExists(t *testing.T) {
	s := memstore.New()
	stream := eventuous.StreamName("test-existing-exists")
	seedEvents(t, s, stream, 3)

	state, events, version, err := store.LoadState(
		context.Background(), s, stream, counterFold, counter{}, eventuous.IsExisting,
	)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if state.Count != 3 {
		t.Errorf("expected Count=3, got Count=%d", state.Count)
	}
	if len(events) != 3 {
		t.Errorf("expected 3 events, got %d", len(events))
	}
	if version != eventuous.ExpectedVersion(2) {
		t.Errorf("expected version 2, got %d", version)
	}
}

// TestLoadState_IsExisting_NoStream: no stream, IsExisting → ErrStreamNotFound.
func TestLoadState_IsExisting_NoStream(t *testing.T) {
	s := memstore.New()
	stream := eventuous.StreamName("test-existing-nostream")

	_, _, _, err := store.LoadState(
		context.Background(), s, stream, counterFold, counter{}, eventuous.IsExisting,
	)

	if !errors.Is(err, eventuous.ErrStreamNotFound) {
		t.Errorf("expected ErrStreamNotFound, got %v", err)
	}
}

// TestLoadState_IsAny_EmptyStream: no stream, IsAny → zero state, VersionNoStream.
func TestLoadState_IsAny_EmptyStream(t *testing.T) {
	s := memstore.New()
	stream := eventuous.StreamName("test-any-empty")

	state, events, version, err := store.LoadState(
		context.Background(), s, stream, counterFold, counter{}, eventuous.IsAny,
	)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if state.Count != 0 {
		t.Errorf("expected zero state, got Count=%d", state.Count)
	}
	if len(events) != 0 {
		t.Errorf("expected 0 events, got %d", len(events))
	}
	if version != eventuous.VersionNoStream {
		t.Errorf("expected VersionNoStream, got %d", version)
	}
}

// TestLoadState_IsAny_StreamExists: stream with events, IsAny → folded state.
func TestLoadState_IsAny_StreamExists(t *testing.T) {
	s := memstore.New()
	stream := eventuous.StreamName("test-any-exists")
	seedEvents(t, s, stream, 4)

	state, events, version, err := store.LoadState(
		context.Background(), s, stream, counterFold, counter{}, eventuous.IsAny,
	)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if state.Count != 4 {
		t.Errorf("expected Count=4, got Count=%d", state.Count)
	}
	if len(events) != 4 {
		t.Errorf("expected 4 events, got %d", len(events))
	}
	if version != eventuous.ExpectedVersion(3) {
		t.Errorf("expected version 3, got %d", version)
	}
}
