package store

import (
	"context"
	"errors"
	"math"

	eventuous "github.com/eventuous/eventuous-go/core"
)

// LoadState loads events from a stream and folds them into state.
// Returns the folded state, the raw event payloads, and the expected version
// for the next append.
//
// Behavior based on ExpectedState:
//   - IsNew: stream must NOT exist. Returns zero state, nil events,
//     VersionNoStream. If stream exists, returns error.
//   - IsExisting: stream MUST exist. Returns folded state, events,
//     version = len(events)-1. If not found, returns ErrStreamNotFound.
//   - IsAny: if stream exists, returns folded state; if not, returns zero
//     state with VersionNoStream.
func LoadState[S any](
	ctx context.Context,
	reader EventReader,
	stream eventuous.StreamName,
	fold func(S, any) S,
	zero S,
	expected eventuous.ExpectedState,
) (state S, events []any, version eventuous.ExpectedVersion, err error) {
	// Read all events from the stream in a single large read.
	raw, readErr := reader.ReadEvents(ctx, stream, 0, math.MaxInt32)

	if readErr != nil {
		if !errors.Is(readErr, eventuous.ErrStreamNotFound) {
			// Unexpected error — propagate immediately.
			return zero, nil, eventuous.VersionNoStream, readErr
		}

		// Stream not found.
		switch expected {
		case eventuous.IsNew, eventuous.IsAny:
			// Expected — return zero state with VersionNoStream.
			return zero, nil, eventuous.VersionNoStream, nil
		case eventuous.IsExisting:
			return zero, nil, eventuous.VersionNoStream, readErr
		}
	}

	// Stream exists and events were read successfully.
	switch expected {
	case eventuous.IsNew:
		// Stream must NOT exist.
		return zero, nil, eventuous.VersionNoStream,
			errors.New("eventuous: stream already exists")
	}

	// Fold events into state.
	payloads := make([]any, 0, len(raw))
	state = zero
	for _, e := range raw {
		state = fold(state, e.Payload)
		payloads = append(payloads, e.Payload)
	}

	if len(payloads) == 0 {
		return state, nil, eventuous.VersionNoStream, nil
	}

	version = eventuous.ExpectedVersion(len(payloads) - 1)
	return state, payloads, version, nil
}
