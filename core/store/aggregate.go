package store

import (
	"context"

	"github.com/google/uuid"

	eventuous "github.com/eventuous/eventuous-go/core"
	"github.com/eventuous/eventuous-go/core/aggregate"
)

// LoadAggregate loads an aggregate from a stream by reading all events and
// calling agg.Load(). Returns a new Aggregate with state reconstructed from
// the stream. If the stream doesn't exist, returns a fresh aggregate
// (version -1).
func LoadAggregate[S any](
	ctx context.Context,
	reader EventReader,
	stream eventuous.StreamName,
	fold func(S, any) S,
	zero S,
) (*aggregate.Aggregate[S], error) {
	agg := aggregate.New(fold, zero)

	state, events, version, err := LoadState(ctx, reader, stream, fold, zero, eventuous.IsAny)
	if err != nil {
		return nil, err
	}

	if len(events) > 0 {
		// version is the position of the last event (0-based).
		agg.Load(int64(version), events)
	}
	// Suppress the "declared and not used" warning — state is intentionally
	// discarded here because Load re-folds from events.
	_ = state

	return agg, nil
}

// StoreAggregate appends the aggregate's pending changes to the stream.
// Uses the aggregate's OriginalVersion for optimistic concurrency.
// If there are no changes, returns a no-op AppendResult.
func StoreAggregate[S any](
	ctx context.Context,
	writer EventWriter,
	stream eventuous.StreamName,
	agg *aggregate.Aggregate[S],
) (AppendResult, error) {
	changes := agg.Changes()
	if len(changes) == 0 {
		return AppendResult{}, nil
	}

	events := make([]NewStreamEvent, len(changes))
	for i, c := range changes {
		events[i] = NewStreamEvent{
			ID:      uuid.New(),
			Payload: c,
		}
	}

	var expected eventuous.ExpectedVersion
	if agg.OriginalVersion() == -1 {
		expected = eventuous.VersionNoStream
	} else {
		expected = eventuous.ExpectedVersion(agg.OriginalVersion())
	}

	return writer.AppendEvents(ctx, stream, expected, events)
}
