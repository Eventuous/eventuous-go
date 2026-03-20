// Copyright (C) Eventuous HQ OÜ. All rights reserved
// Licensed under the Apache License, Version 2.0.

package command

import (
	"context"
	"reflect"

	eventuous "github.com/eventuous/eventuous-go/core"
	"github.com/eventuous/eventuous-go/core/store"
	"github.com/google/uuid"
)

// CommandHandler is the interface for command handling, enabling decorators (e.g., OTel).
type CommandHandler[S any] interface {
	Handle(ctx context.Context, command any) (*Result[S], error)
}

// Service handles commands by loading state, executing a handler, and storing new events.
type Service[S any] struct {
	reader   store.EventReader
	writer   store.EventWriter
	fold     func(S, any) S
	zero     S
	handlers map[reflect.Type]untypedHandler[S]
}

// New creates a functional command service.
func New[S any](
	reader store.EventReader,
	writer store.EventWriter,
	fold func(S, any) S,
	zero S,
) *Service[S] {
	return &Service[S]{
		reader:   reader,
		writer:   writer,
		fold:     fold,
		zero:     zero,
		handlers: make(map[reflect.Type]untypedHandler[S]),
	}
}

// Handle dispatches a command to its registered handler.
// The pipeline:
//  1. Look up handler by command type
//  2. Get stream name from handler
//  3. LoadState from store (using handler's Expected state)
//  4. Call handler.Act(ctx, state, command) → new events
//  5. If no new events, return current state (no-op)
//  6. Append new events to store (each as NewStreamEvent with uuid.New())
//  7. Fold new events into state for the result
//  8. Return Result[S]
func (svc *Service[S]) Handle(ctx context.Context, command any) (*Result[S], error) {
	// Step 1: Look up handler by command type.
	cmdType := reflect.TypeOf(command)
	h, ok := svc.handlers[cmdType]
	if !ok {
		return nil, eventuous.ErrHandlerNotFound
	}

	// Step 2: Get stream name from handler.
	stream := h.stream(command)

	// Step 3: Load state from store.
	state, _, version, err := store.LoadState(ctx, svc.reader, stream, svc.fold, svc.zero, h.expected)
	if err != nil {
		return nil, err
	}

	// Step 4: Call handler.Act.
	newEvents, err := h.act(ctx, state, command)
	if err != nil {
		return nil, err
	}

	// Step 5: If no new events, return current state (no-op).
	if len(newEvents) == 0 {
		return &Result[S]{
			State:         state,
			NewEvents:     nil,
			StreamVersion: int64(version),
		}, nil
	}

	// Step 6: Append new events to store.
	streamEvents := make([]store.NewStreamEvent, len(newEvents))
	for i, e := range newEvents {
		streamEvents[i] = store.NewStreamEvent{
			ID:      uuid.New(),
			Payload: e,
		}
	}

	appendResult, err := svc.writer.AppendEvents(ctx, stream, version, streamEvents)
	if err != nil {
		return nil, err
	}

	// Step 7: Fold new events into state for the result.
	for _, e := range newEvents {
		state = svc.fold(state, e)
	}

	// Step 8: Return Result[S].
	return &Result[S]{
		State:          state,
		NewEvents:      newEvents,
		GlobalPosition: appendResult.GlobalPosition,
		StreamVersion:  appendResult.NextExpectedVersion,
	}, nil
}
