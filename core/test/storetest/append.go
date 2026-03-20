package storetest

import (
	"context"
	"errors"
	"testing"

	eventuous "github.com/eventuous/eventuous-go/core"
	"github.com/eventuous/eventuous-go/core/store"
)

// TestAppend verifies basic append behaviour.
func TestAppend(t *testing.T, s store.EventStore) {
	t.Helper()

	t.Run("ShouldAppendToNewStream", func(t *testing.T) {
		stream := uniqueStream()
		events := makeEvents(1)

		result, err := s.AppendEvents(context.Background(), stream, eventuous.VersionNoStream, events)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.NextExpectedVersion != 0 {
			t.Errorf("expected NextExpectedVersion=0, got %d", result.NextExpectedVersion)
		}
		// GlobalPosition is valid (no specific value required, just not negative-ish)
		_ = result.GlobalPosition
	})

	t.Run("ShouldAppendToExistingStream", func(t *testing.T) {
		stream := uniqueStream()
		events := makeEvents(1)

		result, err := s.AppendEvents(context.Background(), stream, eventuous.VersionNoStream, events)
		if err != nil {
			t.Fatalf("first append failed: %v", err)
		}
		if result.NextExpectedVersion != 0 {
			t.Errorf("expected NextExpectedVersion=0, got %d", result.NextExpectedVersion)
		}

		events2 := makeEvents(1)
		result2, err := s.AppendEvents(context.Background(), stream, eventuous.ExpectedVersion(result.NextExpectedVersion), events2)
		if err != nil {
			t.Fatalf("second append failed: %v", err)
		}
		if result2.NextExpectedVersion != 1 {
			t.Errorf("expected NextExpectedVersion=1, got %d", result2.NextExpectedVersion)
		}
	})

	t.Run("ShouldAppendMultipleEvents", func(t *testing.T) {
		stream := uniqueStream()
		events := makeEvents(3)

		_, err := s.AppendEvents(context.Background(), stream, eventuous.VersionNoStream, events)
		if err != nil {
			t.Fatalf("append failed: %v", err)
		}

		read, err := s.ReadEvents(context.Background(), stream, 0, 10)
		if err != nil {
			t.Fatalf("read failed: %v", err)
		}
		if len(read) != 3 {
			t.Fatalf("expected 3 events, got %d", len(read))
		}
		for i, e := range read {
			if e.Position != int64(i) {
				t.Errorf("event[%d]: expected Position=%d, got %d", i, i, e.Position)
			}
			payload, ok := e.Payload.(testEvent)
			if !ok {
				t.Errorf("event[%d]: unexpected payload type %T", i, e.Payload)
				continue
			}
			if payload.Index != i {
				t.Errorf("event[%d]: expected Index=%d, got %d", i, i, payload.Index)
			}
		}
	})

	t.Run("ShouldAppendWithVersionAny", func(t *testing.T) {
		stream := uniqueStream()
		events := makeEvents(1)

		// Append to a new stream with VersionAny.
		_, err := s.AppendEvents(context.Background(), stream, eventuous.VersionAny, events)
		if err != nil {
			t.Fatalf("first append with VersionAny failed: %v", err)
		}

		// Append again with VersionAny on an existing stream.
		events2 := makeEvents(1)
		_, err = s.AppendEvents(context.Background(), stream, eventuous.VersionAny, events2)
		if err != nil {
			t.Fatalf("second append with VersionAny failed: %v", err)
		}
	})
}

// TestOptimisticConcurrency verifies that the store enforces optimistic concurrency.
func TestOptimisticConcurrency(t *testing.T, s store.EventStore) {
	t.Helper()

	t.Run("ShouldFailOnWrongExpectedVersion", func(t *testing.T) {
		stream := uniqueStream()
		events := makeEvents(1)

		_, err := s.AppendEvents(context.Background(), stream, eventuous.VersionNoStream, events)
		if err != nil {
			t.Fatalf("initial append failed: %v", err)
		}

		// Use a wrong version (version 5 when stream is at 0).
		_, err = s.AppendEvents(context.Background(), stream, eventuous.ExpectedVersion(5), makeEvents(1))
		if !errors.Is(err, eventuous.ErrOptimisticConcurrency) {
			t.Errorf("expected ErrOptimisticConcurrency, got %v", err)
		}
	})

	t.Run("ShouldFailOnNoStreamWhenStreamExists", func(t *testing.T) {
		stream := uniqueStream()
		events := makeEvents(1)

		_, err := s.AppendEvents(context.Background(), stream, eventuous.VersionNoStream, events)
		if err != nil {
			t.Fatalf("initial append failed: %v", err)
		}

		// Try to append with VersionNoStream when stream already exists.
		_, err = s.AppendEvents(context.Background(), stream, eventuous.VersionNoStream, makeEvents(1))
		if !errors.Is(err, eventuous.ErrOptimisticConcurrency) {
			t.Errorf("expected ErrOptimisticConcurrency, got %v", err)
		}
	})
}
