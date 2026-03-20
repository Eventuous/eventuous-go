// Copyright (C) Eventuous HQ OÜ. All rights reserved
// Licensed under the Apache License, Version 2.0.

package storetest

import (
	"context"
	"errors"
	"testing"

	eventuous "github.com/eventuous/eventuous-go/core"
	"github.com/eventuous/eventuous-go/core/store"
)

// TestStreamExists verifies StreamExists behaviour.
func TestStreamExists(t *testing.T, s store.EventStore) {
	t.Helper()

	t.Run("ShouldReturnFalseForNonExistentStream", func(t *testing.T) {
		stream := uniqueStream()

		exists, err := s.StreamExists(context.Background(), stream)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if exists {
			t.Error("expected stream to not exist")
		}
	})

	t.Run("ShouldReturnTrueAfterAppend", func(t *testing.T) {
		stream := uniqueStream()

		_, err := s.AppendEvents(context.Background(), stream, eventuous.VersionNoStream, makeEvents(1))
		if err != nil {
			t.Fatalf("append failed: %v", err)
		}

		exists, err := s.StreamExists(context.Background(), stream)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !exists {
			t.Error("expected stream to exist after append")
		}
	})
}

// TestDeleteStream verifies delete behaviour.
func TestDeleteStream(t *testing.T, s store.EventStore) {
	t.Helper()

	t.Run("ShouldDeleteStream", func(t *testing.T) {
		stream := uniqueStream()

		result, err := s.AppendEvents(context.Background(), stream, eventuous.VersionNoStream, makeEvents(1))
		if err != nil {
			t.Fatalf("append failed: %v", err)
		}

		err = s.DeleteStream(context.Background(), stream, eventuous.ExpectedVersion(result.NextExpectedVersion))
		if err != nil {
			t.Fatalf("delete failed: %v", err)
		}

		exists, err := s.StreamExists(context.Background(), stream)
		if err != nil {
			t.Fatalf("StreamExists failed: %v", err)
		}
		if exists {
			t.Error("expected stream to not exist after deletion")
		}
	})

	t.Run("ShouldFailDeleteOnWrongVersion", func(t *testing.T) {
		stream := uniqueStream()

		_, err := s.AppendEvents(context.Background(), stream, eventuous.VersionNoStream, makeEvents(1))
		if err != nil {
			t.Fatalf("append failed: %v", err)
		}

		// Delete with a wrong expected version.
		err = s.DeleteStream(context.Background(), stream, eventuous.ExpectedVersion(99))
		if !errors.Is(err, eventuous.ErrOptimisticConcurrency) {
			t.Errorf("expected ErrOptimisticConcurrency, got %v", err)
		}
	})
}

// TestTruncateStream verifies truncation behaviour.
func TestTruncateStream(t *testing.T, s store.EventStore) {
	t.Helper()

	t.Run("ShouldTruncateStream", func(t *testing.T) {
		stream := uniqueStream()

		result, err := s.AppendEvents(context.Background(), stream, eventuous.VersionNoStream, makeEvents(5))
		if err != nil {
			t.Fatalf("append failed: %v", err)
		}

		// Truncate at position 2 — events at 0 and 1 should be removed.
		err = s.TruncateStream(context.Background(), stream, 2, eventuous.ExpectedVersion(result.NextExpectedVersion))
		if err != nil {
			t.Fatalf("truncate failed: %v", err)
		}

		read, err := s.ReadEvents(context.Background(), stream, 0, 10)
		if err != nil {
			t.Fatalf("read after truncate failed: %v", err)
		}
		if len(read) != 3 {
			t.Fatalf("expected 3 events after truncation, got %d", len(read))
		}
		for i, e := range read {
			expectedPos := int64(2 + i)
			if e.Position != expectedPos {
				t.Errorf("event[%d]: expected Position=%d, got %d", i, expectedPos, e.Position)
			}
		}
	})
}
