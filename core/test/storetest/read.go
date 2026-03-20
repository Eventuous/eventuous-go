package storetest

import (
	"context"
	"errors"
	"testing"

	eventuous "github.com/eventuous/eventuous-go/core"
	"github.com/eventuous/eventuous-go/core/store"
)

// TestRead verifies forward read behaviour.
func TestRead(t *testing.T, s store.EventStore) {
	t.Helper()

	t.Run("ShouldReadOne", func(t *testing.T) {
		stream := uniqueStream()
		_, err := s.AppendEvents(context.Background(), stream, eventuous.VersionNoStream, makeEvents(1))
		if err != nil {
			t.Fatalf("append failed: %v", err)
		}

		read, err := s.ReadEvents(context.Background(), stream, 0, 1)
		if err != nil {
			t.Fatalf("read failed: %v", err)
		}
		if len(read) != 1 {
			t.Fatalf("expected 1 event, got %d", len(read))
		}
		e := read[0]
		if e.Payload == nil {
			t.Error("Payload must not be nil")
		}
		if e.EventType == "" {
			t.Error("EventType must not be empty")
		}
		if e.Created.IsZero() {
			t.Error("Created must not be zero")
		}
		if e.Position != 0 {
			t.Errorf("expected Position=0, got %d", e.Position)
		}
	})

	t.Run("ShouldReadMany", func(t *testing.T) {
		stream := uniqueStream()
		_, err := s.AppendEvents(context.Background(), stream, eventuous.VersionNoStream, makeEvents(5))
		if err != nil {
			t.Fatalf("append failed: %v", err)
		}

		read, err := s.ReadEvents(context.Background(), stream, 0, 10)
		if err != nil {
			t.Fatalf("read failed: %v", err)
		}
		if len(read) != 5 {
			t.Fatalf("expected 5 events, got %d", len(read))
		}
		for i, e := range read {
			if e.Position != int64(i) {
				t.Errorf("event[%d]: expected Position=%d, got %d", i, i, e.Position)
			}
		}
	})

	t.Run("ShouldReadFromPosition", func(t *testing.T) {
		stream := uniqueStream()
		_, err := s.AppendEvents(context.Background(), stream, eventuous.VersionNoStream, makeEvents(5))
		if err != nil {
			t.Fatalf("append failed: %v", err)
		}

		read, err := s.ReadEvents(context.Background(), stream, 2, 10)
		if err != nil {
			t.Fatalf("read failed: %v", err)
		}
		if len(read) != 3 {
			t.Fatalf("expected 3 events (positions 2,3,4), got %d", len(read))
		}
		for i, e := range read {
			expectedPos := int64(2 + i)
			if e.Position != expectedPos {
				t.Errorf("event[%d]: expected Position=%d, got %d", i, expectedPos, e.Position)
			}
		}
	})

	t.Run("ShouldReadWithCount", func(t *testing.T) {
		stream := uniqueStream()
		_, err := s.AppendEvents(context.Background(), stream, eventuous.VersionNoStream, makeEvents(5))
		if err != nil {
			t.Fatalf("append failed: %v", err)
		}

		read, err := s.ReadEvents(context.Background(), stream, 0, 2)
		if err != nil {
			t.Fatalf("read failed: %v", err)
		}
		if len(read) != 2 {
			t.Fatalf("expected 2 events, got %d", len(read))
		}
	})

	t.Run("ShouldReturnErrorForNonExistentStream", func(t *testing.T) {
		stream := uniqueStream()

		_, err := s.ReadEvents(context.Background(), stream, 0, 10)
		if !errors.Is(err, eventuous.ErrStreamNotFound) {
			t.Errorf("expected ErrStreamNotFound, got %v", err)
		}
	})
}

// TestReadBackwards verifies reverse read behaviour.
func TestReadBackwards(t *testing.T, s store.EventStore) {
	t.Helper()

	t.Run("ShouldReadBackwardsFromEnd", func(t *testing.T) {
		stream := uniqueStream()
		_, err := s.AppendEvents(context.Background(), stream, eventuous.VersionNoStream, makeEvents(5))
		if err != nil {
			t.Fatalf("append failed: %v", err)
		}

		// Use a very large start to indicate "from the end".
		read, err := s.ReadEventsBackwards(context.Background(), stream, ^uint64(0), 5)
		if err != nil {
			t.Fatalf("read backwards failed: %v", err)
		}
		if len(read) != 5 {
			t.Fatalf("expected 5 events, got %d", len(read))
		}
		// Verify reverse order: positions 4,3,2,1,0.
		for i, e := range read {
			expectedPos := int64(4 - i)
			if e.Position != expectedPos {
				t.Errorf("event[%d]: expected Position=%d, got %d", i, expectedPos, e.Position)
			}
		}
	})

	t.Run("ShouldReadBackwardsFromPosition", func(t *testing.T) {
		stream := uniqueStream()
		_, err := s.AppendEvents(context.Background(), stream, eventuous.VersionNoStream, makeEvents(5))
		if err != nil {
			t.Fatalf("append failed: %v", err)
		}

		// Read 2 events backwards starting from position 3 → positions 3,2.
		read, err := s.ReadEventsBackwards(context.Background(), stream, 3, 2)
		if err != nil {
			t.Fatalf("read backwards failed: %v", err)
		}
		if len(read) != 2 {
			t.Fatalf("expected 2 events, got %d", len(read))
		}
		if read[0].Position != 3 {
			t.Errorf("event[0]: expected Position=3, got %d", read[0].Position)
		}
		if read[1].Position != 2 {
			t.Errorf("event[1]: expected Position=2, got %d", read[1].Position)
		}
	})

	t.Run("ShouldReturnErrorForNonExistentStream", func(t *testing.T) {
		stream := uniqueStream()

		_, err := s.ReadEventsBackwards(context.Background(), stream, 0, 10)
		if !errors.Is(err, eventuous.ErrStreamNotFound) {
			t.Errorf("expected ErrStreamNotFound, got %v", err)
		}
	})
}
