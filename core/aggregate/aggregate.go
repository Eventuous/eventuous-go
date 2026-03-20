// Copyright (C) Eventuous HQ OÜ. All rights reserved
// Licensed under the Apache License, Version 2.0.

package aggregate

import "errors"

var (
	// ErrAggregateExists is returned when a command requires the aggregate to be
	// new, but it has already been persisted.
	ErrAggregateExists = errors.New("aggregate: already exists")
	// ErrAggregateNotExists is returned when a command requires the aggregate to
	// exist, but it has never been persisted.
	ErrAggregateNotExists = errors.New("aggregate: does not exist")
)

// Aggregate tracks state, pending changes, and version for optimistic concurrency.
// S is any state type — no interface constraint needed.
type Aggregate[S any] struct {
	state           S
	original        []any
	changes         []any
	originalVersion int64
	fold            func(S, any) S
}

// New creates a fresh aggregate with the given fold function and zero state.
// OriginalVersion is initialised to -1, indicating the aggregate has not yet
// been persisted.
func New[S any](fold func(S, any) S, zero S) *Aggregate[S] {
	return &Aggregate[S]{
		state:           zero,
		originalVersion: -1,
		fold:            fold,
	}
}

// State returns the current (folded) state of the aggregate.
func (a *Aggregate[S]) State() S {
	return a.state
}

// Changes returns the slice of pending (uncommitted) events that have been
// applied since the aggregate was loaded or created.
func (a *Aggregate[S]) Changes() []any {
	return a.changes
}

// OriginalVersion returns the stream version the aggregate was loaded at.
// -1 means the aggregate has never been persisted.
func (a *Aggregate[S]) OriginalVersion() int64 {
	return a.originalVersion
}

// CurrentVersion returns the version that the aggregate will be at after its
// pending changes are persisted.  For a new aggregate with no changes this is
// -1; each applied event increments the value by one.
func (a *Aggregate[S]) CurrentVersion() int64 {
	return a.originalVersion + int64(len(a.changes))
}

// Apply records a new domain event as a pending change and folds it into the
// current state.
func (a *Aggregate[S]) Apply(event any) {
	a.changes = append(a.changes, event)
	a.state = a.fold(a.state, event)
}

// Load reconstructs aggregate state from a slice of persisted events.  It is
// called by the event store after reading a stream.  version is the stream
// position of the last event in events.
func (a *Aggregate[S]) Load(version int64, events []any) {
	a.originalVersion = version
	a.original = make([]any, len(events))
	copy(a.original, events)
	a.changes = nil
	for _, e := range events {
		a.state = a.fold(a.state, e)
	}
}

// ClearChanges resets the pending changes slice.  It is called by the event
// store after the changes have been successfully persisted.
func (a *Aggregate[S]) ClearChanges() {
	a.changes = nil
}

// EnsureNew returns ErrAggregateExists when the aggregate has already been
// persisted (OriginalVersion >= 0).  Use this guard at the start of creation
// commands.
func (a *Aggregate[S]) EnsureNew() error {
	if a.originalVersion >= 0 {
		return ErrAggregateExists
	}
	return nil
}

// EnsureExists returns ErrAggregateNotExists when the aggregate has never been
// persisted (OriginalVersion < 0).  Use this guard at the start of mutation
// commands.
func (a *Aggregate[S]) EnsureExists() error {
	if a.originalVersion < 0 {
		return ErrAggregateNotExists
	}
	return nil
}
