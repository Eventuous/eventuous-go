package storetest

import (
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"

	eventuous "github.com/eventuous/eventuous-go/core"
	"github.com/eventuous/eventuous-go/core/store"
)

// RunAll runs the full conformance suite against the given store.
func RunAll(t *testing.T, s store.EventStore) {
	t.Helper()
	t.Run("Append", func(t *testing.T) { TestAppend(t, s) })
	t.Run("Read", func(t *testing.T) { TestRead(t, s) })
	t.Run("ReadBackwards", func(t *testing.T) { TestReadBackwards(t, s) })
	t.Run("OptimisticConcurrency", func(t *testing.T) { TestOptimisticConcurrency(t, s) })
	t.Run("StreamExists", func(t *testing.T) { TestStreamExists(t, s) })
	t.Run("DeleteStream", func(t *testing.T) { TestDeleteStream(t, s) })
	t.Run("TruncateStream", func(t *testing.T) { TestTruncateStream(t, s) })
}

// testEvent is a simple payload for conformance tests.
type testEvent struct {
	Index int
	Data  string
}

// makeEvents creates n NewStreamEvent instances with sequential indices.
func makeEvents(n int) []store.NewStreamEvent {
	events := make([]store.NewStreamEvent, n)
	for i := range events {
		events[i] = store.NewStreamEvent{
			ID:      uuid.New(),
			Payload: testEvent{Index: i, Data: fmt.Sprintf("event-%d", i)},
		}
	}
	return events
}

// streamCounter is used to generate unique stream names.
var streamCounter atomic.Uint64

// uniqueStream returns a unique stream name for test isolation.
func uniqueStream() eventuous.StreamName {
	n := streamCounter.Add(1)
	return eventuous.StreamName(fmt.Sprintf("test-stream-%d", n))
}
