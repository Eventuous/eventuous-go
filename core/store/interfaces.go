package store

import (
	"context"

	eventuous "github.com/eventuous/eventuous-go/core"
)

// EventReader reads events from streams.
type EventReader interface {
	ReadEvents(ctx context.Context, stream eventuous.StreamName, start uint64, count int) ([]StreamEvent, error)
	ReadEventsBackwards(ctx context.Context, stream eventuous.StreamName, start uint64, count int) ([]StreamEvent, error)
}

// EventWriter appends events to streams.
type EventWriter interface {
	AppendEvents(ctx context.Context, stream eventuous.StreamName, expected eventuous.ExpectedVersion, events []NewStreamEvent) (AppendResult, error)
}

// EventStore combines reading, writing, and stream management.
type EventStore interface {
	EventReader
	EventWriter
	StreamExists(ctx context.Context, stream eventuous.StreamName) (bool, error)
	DeleteStream(ctx context.Context, stream eventuous.StreamName, expected eventuous.ExpectedVersion) error
	TruncateStream(ctx context.Context, stream eventuous.StreamName, position uint64, expected eventuous.ExpectedVersion) error
}
