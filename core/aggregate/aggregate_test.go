// Copyright (C) Eventuous HQ OÜ. All rights reserved
// Licensed under the Apache License, Version 2.0.

package aggregate_test

import (
	"testing"

	"github.com/eventuous/eventuous-go/core/aggregate"
)

// Test domain types.

type counterState struct{ Count int }

type incremented struct{}
type decremented struct{}

func counterFold(s counterState, event any) counterState {
	switch event.(type) {
	case incremented:
		return counterState{Count: s.Count + 1}
	case decremented:
		return counterState{Count: s.Count - 1}
	default:
		return s
	}
}

func newCounter() *aggregate.Aggregate[counterState] {
	return aggregate.New(counterFold, counterState{})
}

// TestNew_IsEmpty verifies that a freshly created aggregate has no changes,
// version -1, and zero state.
func TestNew_IsEmpty(t *testing.T) {
	a := newCounter()

	if len(a.Changes()) != 0 {
		t.Errorf("expected 0 changes, got %d", len(a.Changes()))
	}
	if a.OriginalVersion() != -1 {
		t.Errorf("expected OriginalVersion -1, got %d", a.OriginalVersion())
	}
	if a.CurrentVersion() != -1 {
		t.Errorf("expected CurrentVersion -1, got %d", a.CurrentVersion())
	}
	if a.State().Count != 0 {
		t.Errorf("expected State.Count 0, got %d", a.State().Count)
	}
}

// TestApply_SingleEvent verifies that applying one event records it as a change
// and folds it into state.
func TestApply_SingleEvent(t *testing.T) {
	a := newCounter()
	a.Apply(incremented{})

	if len(a.Changes()) != 1 {
		t.Errorf("expected 1 change, got %d", len(a.Changes()))
	}
	if a.State().Count != 1 {
		t.Errorf("expected State.Count 1, got %d", a.State().Count)
	}
	if a.CurrentVersion() != 0 {
		t.Errorf("expected CurrentVersion 0, got %d", a.CurrentVersion())
	}
}

// TestApply_MultipleEvents verifies state and version after applying several events.
func TestApply_MultipleEvents(t *testing.T) {
	a := newCounter()
	a.Apply(incremented{})
	a.Apply(incremented{})
	a.Apply(incremented{})
	a.Apply(decremented{})

	if a.State().Count != 2 {
		t.Errorf("expected State.Count 2, got %d", a.State().Count)
	}
	if a.CurrentVersion() != 3 {
		t.Errorf("expected CurrentVersion 3, got %d", a.CurrentVersion())
	}
	if len(a.Changes()) != 4 {
		t.Errorf("expected 4 changes, got %d", len(a.Changes()))
	}
}

// TestLoad_SetsStateAndVersion verifies that Load reconstructs state and sets
// OriginalVersion, leaving no pending changes.
func TestLoad_SetsStateAndVersion(t *testing.T) {
	a := newCounter()
	a.Load(2, []any{incremented{}, incremented{}, incremented{}})

	if a.State().Count != 3 {
		t.Errorf("expected State.Count 3, got %d", a.State().Count)
	}
	if a.OriginalVersion() != 2 {
		t.Errorf("expected OriginalVersion 2, got %d", a.OriginalVersion())
	}
	if a.CurrentVersion() != 2 {
		t.Errorf("expected CurrentVersion 2, got %d", a.CurrentVersion())
	}
	if len(a.Changes()) != 0 {
		t.Errorf("expected 0 changes after Load, got %d", len(a.Changes()))
	}
}

// TestLoad_ThenApply verifies that applying events after loading correctly
// extends the version and changes list while preserving original version.
func TestLoad_ThenApply(t *testing.T) {
	a := newCounter()
	a.Load(2, []any{incremented{}, incremented{}})
	a.Apply(incremented{})

	if a.State().Count != 3 {
		t.Errorf("expected State.Count 3, got %d", a.State().Count)
	}
	if a.OriginalVersion() != 2 {
		t.Errorf("expected OriginalVersion 2, got %d", a.OriginalVersion())
	}
	if a.CurrentVersion() != 3 {
		t.Errorf("expected CurrentVersion 3, got %d", a.CurrentVersion())
	}
	if len(a.Changes()) != 1 {
		t.Errorf("expected 1 change, got %d", len(a.Changes()))
	}
}

// TestClearChanges verifies that ClearChanges empties pending events without
// touching the current state.
func TestClearChanges(t *testing.T) {
	a := newCounter()
	a.Apply(incremented{})
	a.Apply(incremented{})
	a.ClearChanges()

	if len(a.Changes()) != 0 {
		t.Errorf("expected 0 changes after ClearChanges, got %d", len(a.Changes()))
	}
	if a.State().Count != 2 {
		t.Errorf("expected State.Count 2 after ClearChanges, got %d", a.State().Count)
	}
}

// TestEnsureNew_OnNewAggregate verifies that EnsureNew returns nil for a fresh aggregate.
func TestEnsureNew_OnNewAggregate(t *testing.T) {
	a := newCounter()
	if err := a.EnsureNew(); err != nil {
		t.Errorf("expected nil error from EnsureNew on new aggregate, got %v", err)
	}
}

// TestEnsureNew_OnLoadedAggregate verifies that EnsureNew returns an error
// after the aggregate has been loaded from a stream.
func TestEnsureNew_OnLoadedAggregate(t *testing.T) {
	a := newCounter()
	a.Load(0, []any{incremented{}})
	if err := a.EnsureNew(); err == nil {
		t.Error("expected error from EnsureNew on loaded aggregate, got nil")
	}
}

// TestEnsureExists_OnNewAggregate verifies that EnsureExists returns an error
// for a fresh (never loaded) aggregate.
func TestEnsureExists_OnNewAggregate(t *testing.T) {
	a := newCounter()
	if err := a.EnsureExists(); err == nil {
		t.Error("expected error from EnsureExists on new aggregate, got nil")
	}
}

// TestEnsureExists_OnLoadedAggregate verifies that EnsureExists returns nil
// after the aggregate has been loaded.
func TestEnsureExists_OnLoadedAggregate(t *testing.T) {
	a := newCounter()
	a.Load(0, []any{incremented{}})
	if err := a.EnsureExists(); err != nil {
		t.Errorf("expected nil error from EnsureExists on loaded aggregate, got %v", err)
	}
}
