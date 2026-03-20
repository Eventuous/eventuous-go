// Copyright (C) Eventuous HQ OÜ. All rights reserved
// Licensed under the Apache License, Version 2.0.

package command

import (
	"context"
	"reflect"

	eventuous "github.com/eventuous/eventuous-go/core"
)

// Handler defines how to process a specific command type.
type Handler[C any, S any] struct {
	// Expected state: IsNew, IsExisting, or IsAny.
	Expected eventuous.ExpectedState

	// Stream returns the stream name for this command.
	Stream func(C) eventuous.StreamName

	// Act is a pure function: given current state and the command, return new events.
	Act func(ctx context.Context, state S, cmd C) ([]any, error)
}

// untypedHandler erases the command type for storage in the handlers map.
type untypedHandler[S any] struct {
	expected eventuous.ExpectedState
	stream   func(any) eventuous.StreamName
	act      func(ctx context.Context, state S, cmd any) ([]any, error)
}

// On registers a command handler on the service.
func On[C any, S any](svc *Service[S], h Handler[C, S]) {
	var zero C
	cmdType := reflect.TypeOf(zero)
	svc.handlers[cmdType] = untypedHandler[S]{
		expected: h.Expected,
		stream: func(cmd any) eventuous.StreamName {
			return h.Stream(cmd.(C))
		},
		act: func(ctx context.Context, state S, cmd any) ([]any, error) {
			return h.Act(ctx, state, cmd.(C))
		},
	}
}
