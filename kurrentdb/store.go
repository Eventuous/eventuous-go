// Copyright (C) Eventuous HQ OÜ. All rights reserved
// Licensed under the Apache License, Version 2.0.

package kurrentdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/google/uuid"
	"github.com/kurrent-io/KurrentDB-Client-Go/kurrentdb"

	eventuous "github.com/eventuous/eventuous-go/core"
	"github.com/eventuous/eventuous-go/core/codec"
	"github.com/eventuous/eventuous-go/core/store"
)

// Store is an EventStore implementation backed by KurrentDB.
type Store struct {
	client *kurrentdb.Client
	codec  codec.Codec
}

// Compile-time interface check.
var _ store.EventStore = (*Store)(nil)

// NewStore creates a new KurrentDB-backed event store.
func NewStore(client *kurrentdb.Client, codec codec.Codec) *Store {
	return &Store{
		client: client,
		codec:  codec,
	}
}

// AppendEvents appends events to a stream with optimistic concurrency control.
func (s *Store) AppendEvents(
	ctx context.Context,
	stream eventuous.StreamName,
	expected eventuous.ExpectedVersion,
	events []store.NewStreamEvent,
) (store.AppendResult, error) {
	proposed := make([]kurrentdb.EventData, len(events))

	for i, e := range events {
		data, eventType, _, err := s.codec.Encode(e.Payload)
		if err != nil {
			return store.AppendResult{}, fmt.Errorf("kurrentdb: encode event %d: %w", i, err)
		}

		var metaBytes []byte
		if e.Metadata != nil {
			metaBytes, err = json.Marshal(e.Metadata)
			if err != nil {
				return store.AppendResult{}, fmt.Errorf("kurrentdb: marshal metadata %d: %w", i, err)
			}
		}

		id := e.ID
		if id == uuid.Nil {
			id = uuid.New()
		}

		proposed[i] = kurrentdb.EventData{
			EventID:     id,
			EventType:   eventType,
			ContentType: kurrentdb.ContentTypeJson,
			Data:        data,
			Metadata:    metaBytes,
		}
	}

	opts := kurrentdb.AppendToStreamOptions{
		StreamState: mapExpectedVersion(expected),
	}

	result, err := s.client.AppendToStream(ctx, string(stream), opts, proposed...)
	if err != nil {
		if isWrongExpectedVersion(err) {
			return store.AppendResult{}, fmt.Errorf("%w: %s", eventuous.ErrOptimisticConcurrency, err)
		}
		return store.AppendResult{}, fmt.Errorf("kurrentdb: append to stream %q: %w", stream, err)
	}

	return store.AppendResult{
		GlobalPosition:      result.CommitPosition,
		NextExpectedVersion: int64(result.NextExpectedVersion),
	}, nil
}

// ReadEvents reads up to count events from the stream starting at the given
// stream position, in forward order.
func (s *Store) ReadEvents(
	ctx context.Context,
	stream eventuous.StreamName,
	start uint64,
	count int,
) ([]store.StreamEvent, error) {
	opts := kurrentdb.ReadStreamOptions{
		Direction: kurrentdb.Forwards,
		From:      kurrentdb.Revision(start),
	}

	return s.readStream(ctx, stream, opts, uint64(count))
}

// ReadEventsBackwards reads up to count events from the stream in reverse
// order, starting from the given position (inclusive).
func (s *Store) ReadEventsBackwards(
	ctx context.Context,
	stream eventuous.StreamName,
	start uint64,
	count int,
) ([]store.StreamEvent, error) {
	// When start is the sentinel max-uint64 value, read from the end.
	var from kurrentdb.StreamPosition
	if start == ^uint64(0) {
		from = kurrentdb.End{}
	} else {
		from = kurrentdb.Revision(start)
	}

	opts := kurrentdb.ReadStreamOptions{
		Direction: kurrentdb.Backwards,
		From:      from,
	}

	return s.readStream(ctx, stream, opts, uint64(count))
}

// readStream is the shared implementation for forward and backward reads.
func (s *Store) readStream(
	ctx context.Context,
	stream eventuous.StreamName,
	opts kurrentdb.ReadStreamOptions,
	count uint64,
) ([]store.StreamEvent, error) {
	rs, err := s.client.ReadStream(ctx, string(stream), opts, count)
	if err != nil {
		if isStreamNotFound(err) {
			return nil, fmt.Errorf("%w: %q", eventuous.ErrStreamNotFound, stream)
		}
		return nil, fmt.Errorf("kurrentdb: read stream %q: %w", stream, err)
	}
	defer rs.Close()

	var result []store.StreamEvent

	for {
		resolved, err := rs.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			if isStreamNotFound(err) {
				return nil, fmt.Errorf("%w: %q", eventuous.ErrStreamNotFound, stream)
			}
			return nil, fmt.Errorf("kurrentdb: recv from stream %q: %w", stream, err)
		}

		recorded := resolved.OriginalEvent()

		payload, err := s.codec.Decode(recorded.Data, recorded.EventType)
		if err != nil {
			return nil, fmt.Errorf("kurrentdb: decode event %q at position %d: %w", recorded.EventType, recorded.EventNumber, err)
		}

		var meta eventuous.Metadata
		if len(recorded.UserMetadata) > 0 {
			if err := json.Unmarshal(recorded.UserMetadata, &meta); err != nil {
				return nil, fmt.Errorf("kurrentdb: unmarshal metadata at position %d: %w", recorded.EventNumber, err)
			}
		}

		se := store.StreamEvent{
			ID:          recorded.EventID,
			EventType:   recorded.EventType,
			Payload:     payload,
			Metadata:    meta,
			ContentType: recorded.ContentType,
			Position:    int64(recorded.EventNumber),
			Created:     recorded.CreatedDate,
		}
		if resolved.Commit != nil {
			se.GlobalPosition = *resolved.Commit
		} else {
			se.GlobalPosition = recorded.Position.Commit
		}

		result = append(result, se)
	}

	return result, nil
}

// StreamExists returns true if the stream exists and has at least one event.
func (s *Store) StreamExists(ctx context.Context, stream eventuous.StreamName) (bool, error) {
	opts := kurrentdb.ReadStreamOptions{
		Direction: kurrentdb.Forwards,
		From:      kurrentdb.Start{},
	}

	rs, err := s.client.ReadStream(ctx, string(stream), opts, 1)
	if err != nil {
		if isStreamNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("kurrentdb: check stream %q: %w", stream, err)
	}
	defer rs.Close()

	_, err = rs.Recv()
	if errors.Is(err, io.EOF) {
		return false, nil
	}
	if err != nil {
		if isStreamNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("kurrentdb: check stream %q: %w", stream, err)
	}

	return true, nil
}

// DeleteStream soft-deletes a stream with optimistic concurrency control.
func (s *Store) DeleteStream(ctx context.Context, stream eventuous.StreamName, expected eventuous.ExpectedVersion) error {
	opts := kurrentdb.DeleteStreamOptions{
		StreamState: mapExpectedVersion(expected),
	}

	_, err := s.client.DeleteStream(ctx, string(stream), opts)
	if err != nil {
		if isWrongExpectedVersion(err) {
			return fmt.Errorf("%w: %s", eventuous.ErrOptimisticConcurrency, err)
		}
		return fmt.Errorf("kurrentdb: delete stream %q: %w", stream, err)
	}

	return nil
}

// TruncateStream sets the TruncateBefore metadata on the stream so that events
// before the given position are no longer returned. KurrentDB does not have a
// dedicated truncate API, so we use stream metadata with $tb.
//
// The expected version is validated against the data stream first to honor
// optimistic concurrency — if the stream version doesn't match, the truncation
// is rejected before touching metadata.
func (s *Store) TruncateStream(
	ctx context.Context,
	stream eventuous.StreamName,
	position uint64,
	expected eventuous.ExpectedVersion,
) error {
	// Validate expected version against the data stream by attempting a
	// no-op read. We read 1 event backwards to get the current stream
	// version and compare.
	if expected != eventuous.VersionAny {
		rs, err := s.client.ReadStream(ctx, string(stream), kurrentdb.ReadStreamOptions{
			From:      kurrentdb.End{},
			Direction: kurrentdb.Backwards,
		}, 1)
		if err != nil {
			if isStreamNotFound(err) {
				if expected != eventuous.VersionNoStream {
					return fmt.Errorf("kurrentdb: truncate stream %q: %w", stream, eventuous.ErrOptimisticConcurrency)
				}
			} else {
				return fmt.Errorf("kurrentdb: truncate stream %q: %w", stream, err)
			}
		} else {
			defer rs.Close()
			event, err := rs.Recv()
			if err != nil && !errors.Is(err, io.EOF) {
				if isStreamNotFound(err) && expected != eventuous.VersionNoStream {
					return fmt.Errorf("kurrentdb: truncate stream %q: %w", stream, eventuous.ErrOptimisticConcurrency)
				}
			}
			if event != nil {
				currentVersion := int64(event.Event.EventNumber)
				if int64(expected) != currentVersion {
					return fmt.Errorf("kurrentdb: truncate stream %q: %w", stream, eventuous.ErrOptimisticConcurrency)
				}
			}
		}
	}

	meta := &kurrentdb.StreamMetadata{}
	meta.SetTruncateBefore(position)

	opts := kurrentdb.AppendToStreamOptions{
		StreamState: kurrentdb.Any{},
	}

	_, err := s.client.SetStreamMetadata(ctx, string(stream), opts, *meta)
	if err != nil {
		return fmt.Errorf("kurrentdb: truncate stream %q: %w", stream, err)
	}

	return nil
}

// mapExpectedVersion converts an Eventuous ExpectedVersion to a KurrentDB StreamState.
func mapExpectedVersion(v eventuous.ExpectedVersion) kurrentdb.StreamState {
	switch v {
	case eventuous.VersionNoStream:
		return kurrentdb.NoStream{}
	case eventuous.VersionAny:
		return kurrentdb.Any{}
	default:
		return kurrentdb.StreamRevision{Value: uint64(v)}
	}
}

// isWrongExpectedVersion checks whether an error is a KurrentDB wrong-expected-version error.
func isWrongExpectedVersion(err error) bool {
	var kdbErr *kurrentdb.Error
	if errors.As(err, &kdbErr) {
		if kdbErr.IsErrorCode(kurrentdb.ErrorCodeWrongExpectedVersion) {
			return true
		}
	}
	// Fallback: the delete endpoint wraps the error with an ErrorCode of 0
	// (Unknown), so the typed check above misses it. Match the gRPC error
	// description instead.
	return strings.Contains(err.Error(), "WrongExpectedVersion")
}

// isStreamNotFound checks whether an error indicates that the stream was not found.
func isStreamNotFound(err error) bool {
	var kdbErr *kurrentdb.Error
	if errors.As(err, &kdbErr) {
		return kdbErr.IsErrorCode(kurrentdb.ErrorCodeResourceNotFound)
	}
	// Fallback: errors from rs.Recv() may not unwrap into *kurrentdb.Error.
	// Match the error code string that appears in the message.
	return strings.Contains(err.Error(), "ErrorCodeResourceNotFound") ||
		strings.Contains(err.Error(), "a remote resource was not found")
}
