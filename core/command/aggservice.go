package command

import (
	"context"
	"reflect"

	eventuous "github.com/eventuous/eventuous-go/core"
	"github.com/eventuous/eventuous-go/core/aggregate"
	"github.com/eventuous/eventuous-go/core/store"
)

// AggregateService handles commands using aggregates.
// S is the state type held by the aggregate.
type AggregateService[S any] struct {
	reader   store.EventReader
	writer   store.EventWriter
	fold     func(S, any) S
	zero     S
	handlers map[reflect.Type]untypedAggHandler[S]
}

// untypedAggHandler erases the command type for storage in the handlers map.
type untypedAggHandler[S any] struct {
	expected eventuous.ExpectedState
	id       func(any) string
	act      func(ctx context.Context, agg *aggregate.Aggregate[S], cmd any) error
}

// NewAggregateService creates an aggregate-based command service.
func NewAggregateService[S any](
	reader store.EventReader,
	writer store.EventWriter,
	fold func(S, any) S,
	zero S,
) *AggregateService[S] {
	return &AggregateService[S]{
		reader:   reader,
		writer:   writer,
		fold:     fold,
		zero:     zero,
		handlers: make(map[reflect.Type]untypedAggHandler[S]),
	}
}

// Handle dispatches a command to its registered handler.
// Pipeline:
//  1. Look up handler by reflect.TypeOf(command)
//  2. Get entity ID from handler.id(command)
//  3. Build stream name: "{StateTypeName}-{id}"
//  4. Load aggregate from store using store.LoadAggregate
//  5. For IsNew: call agg.EnsureNew(). For IsExisting: call agg.EnsureExists().
//  6. Call handler.act(ctx, agg, command)
//  7. If no changes on aggregate, return current state (no-op)
//  8. Store aggregate using store.StoreAggregate
//  9. Return Result[S] with updated state
func (svc *AggregateService[S]) Handle(ctx context.Context, command any) (*Result[S], error) {
	// Step 1: Look up handler by command type.
	cmdType := reflect.TypeOf(command)
	h, ok := svc.handlers[cmdType]
	if !ok {
		return nil, eventuous.ErrHandlerNotFound
	}

	// Step 2: Get entity ID.
	id := h.id(command)

	// Step 3: Build stream name from state type name + id.
	var zero S
	typeName := reflect.TypeOf(zero).Name()
	stream := eventuous.NewStreamName(typeName, id)

	// Step 4: Load aggregate from store.
	agg, err := store.LoadAggregate(ctx, svc.reader, stream, svc.fold, svc.zero)
	if err != nil {
		return nil, err
	}

	// Step 5: Enforce expected state.
	switch h.expected {
	case eventuous.IsNew:
		if err := agg.EnsureNew(); err != nil {
			return nil, err
		}
	case eventuous.IsExisting:
		if err := agg.EnsureExists(); err != nil {
			return nil, err
		}
	// IsAny: no guard needed.
	}

	// Step 6: Execute domain logic.
	if err := h.act(ctx, agg, command); err != nil {
		return nil, err
	}

	// Step 7: If no changes, return current state (no-op).
	changes := agg.Changes()
	if len(changes) == 0 {
		return &Result[S]{
			State:         agg.State(),
			NewEvents:     nil,
			StreamVersion: agg.OriginalVersion(),
		}, nil
	}

	// Step 8: Persist changes.
	appendResult, err := store.StoreAggregate(ctx, svc.writer, stream, agg)
	if err != nil {
		return nil, err
	}

	// Step 9: Return result.
	return &Result[S]{
		State:          agg.State(),
		NewEvents:      changes,
		GlobalPosition: appendResult.GlobalPosition,
		StreamVersion:  appendResult.NextExpectedVersion,
	}, nil
}

// AggregateHandler defines how to process a command using an aggregate.
type AggregateHandler[C any, S any] struct {
	// Expected state: IsNew, IsExisting, or IsAny.
	Expected eventuous.ExpectedState

	// ID returns the entity identifier from the command.
	ID func(C) string

	// Act applies domain logic to the aggregate. Events are recorded via agg.Apply().
	Act func(ctx context.Context, agg *aggregate.Aggregate[S], cmd C) error
}

// OnAggregate registers an aggregate handler on the service.
func OnAggregate[C any, S any](svc *AggregateService[S], h AggregateHandler[C, S]) {
	var zero C
	cmdType := reflect.TypeOf(zero)
	svc.handlers[cmdType] = untypedAggHandler[S]{
		expected: h.Expected,
		id: func(cmd any) string {
			return h.ID(cmd.(C))
		},
		act: func(ctx context.Context, agg *aggregate.Aggregate[S], cmd any) error {
			return h.Act(ctx, agg, cmd.(C))
		},
	}
}
