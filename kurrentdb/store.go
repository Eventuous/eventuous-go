package kurrentdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"

	"github.com/google/uuid"
	"github.com/kurrent-io/KurrentDB-Client-Go/kurrentdb"

	eventuous "github.com/eventuous/eventuous-go/core"
	"github.com/eventuous/eventuous-go/core/codec"
	"github.com/eventuous/eventuous-go/core/store"
)

// Store implements store.EventStore using KurrentDB (EventStoreDB) as the
// backing persistence layer.
type Store struct {
	client *kurrentdb.Client
	codec  codec.Codec
}

// Compile-time interface check.
var _ store.EventStore = (*Store)(nil)

// NewStore creates a new KurrentDB event store.
func NewStore(client *kurrentdb.Client, codec codec.Codec) *Store {
	return &Store{
		client: client,
		codec:  codec,
	}
}

// AppendEvents appends events to a stream with optimistic concurrency control.
func (s *Store) AppendEvents(ctx context.Context, stream eventuous.StreamName, expected eventuous.ExpectedVersion, events []store.NewStreamEvent) (store.AppendResult, error) {
	eventData := make([]kurrentdb.EventData, len(events))
	for i, e := range events {
		data, eventType, contentType, err := s.codec.Encode(e.Payload)
		if err != nil {
			return store.AppendResult{}, fmt.Errorf("encoding event %d: %w", i, err)
		}

		id := e.ID
		if id == uuid.Nil {
			id = uuid.New()
		}

		var ct kurrentdb.ContentType
		if contentType == "application/json" {
			ct = kurrentdb.ContentTypeJson
		} else {
			ct = kurrentdb.ContentTypeBinary
		}

		var metaBytes []byte
		if e.Metadata != nil {
			metaBytes, err = json.Marshal(e.Metadata)
			if err != nil {
				return store.AppendResult{}, fmt.Errorf("encoding metadata for event %d: %w", i, err)
			}
		}

		eventData[i] = kurrentdb.EventData{
			EventID:     id,
			EventType:   eventType,
			ContentType: ct,
			Data:        data,
			Metadata:    metaBytes,
		}
	}

	opts := kurrentdb.AppendToStreamOptions{
		StreamState: toStreamState(expected),
	}

	result, err := s.client.AppendToStream(ctx, string(stream), opts, eventData...)
	if err != nil {
		return store.AppendResult{}, mapError(err)
	}

	return store.AppendResult{
		GlobalPosition:      result.CommitPosition,
		NextExpectedVersion: int64(result.NextExpectedVersion),
	}, nil
}

// ReadEvents reads up to count events from the stream starting at position
// start, in forward (chronological) order.
func (s *Store) ReadEvents(ctx context.Context, stream eventuous.StreamName, start uint64, count int) ([]store.StreamEvent, error) {
	opts := kurrentdb.ReadStreamOptions{
		From:      kurrentdb.Revision(start),
		Direction: kurrentdb.Forwards,
	}

	readStream, err := s.client.ReadStream(ctx, string(stream), opts, uint64(count))
	if err != nil {
		return nil, mapError(err)
	}
	defer readStream.Close()

	return s.collectEvents(readStream)
}

// ReadEventsBackwards reads up to count events from the stream in reverse
// order. If start is the max uint64 sentinel, reading begins from the end.
func (s *Store) ReadEventsBackwards(ctx context.Context, stream eventuous.StreamName, start uint64, count int) ([]store.StreamEvent, error) {
	var from kurrentdb.StreamPosition
	if start == math.MaxUint64 {
		from = kurrentdb.End{}
	} else {
		from = kurrentdb.Revision(start)
	}

	opts := kurrentdb.ReadStreamOptions{
		From:      from,
		Direction: kurrentdb.Backwards,
	}

	readStream, err := s.client.ReadStream(ctx, string(stream), opts, uint64(count))
	if err != nil {
		return nil, mapError(err)
	}
	defer readStream.Close()

	return s.collectEvents(readStream)
}

// StreamExists returns true if the stream exists and contains at least one event.
func (s *Store) StreamExists(ctx context.Context, stream eventuous.StreamName) (bool, error) {
	opts := kurrentdb.ReadStreamOptions{
		From:      kurrentdb.Start{},
		Direction: kurrentdb.Forwards,
	}

	readStream, err := s.client.ReadStream(ctx, string(stream), opts, 1)
	if err != nil {
		return false, mapError(err)
	}
	defer readStream.Close()

	_, err = readStream.Recv()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return false, nil
		}
		kdbErr, ok := kurrentdb.FromError(err)
		if !ok && kdbErr.Code() == kurrentdb.ErrorCodeResourceNotFound {
			return false, nil
		}
		return false, mapError(err)
	}

	return true, nil
}

// DeleteStream soft-deletes a stream with optimistic concurrency control.
func (s *Store) DeleteStream(ctx context.Context, stream eventuous.StreamName, expected eventuous.ExpectedVersion) error {
	opts := kurrentdb.DeleteStreamOptions{
		StreamState: toStreamState(expected),
	}

	_, err := s.client.DeleteStream(ctx, string(stream), opts)
	if err != nil {
		return mapError(err)
	}
	return nil
}

// TruncateStream truncates a stream by setting the $tb (truncate before)
// metadata on the stream. Events with positions less than the given position
// will be removed during the next scavenge.
//
// Note: The expected version parameter refers to the data stream's version
// and is validated by reading the stream first. The metadata stream write
// uses Any{} because its version is independent from the data stream.
func (s *Store) TruncateStream(ctx context.Context, stream eventuous.StreamName, position uint64, expected eventuous.ExpectedVersion) error {
	// Validate the expected version against the data stream first.
	if err := s.validateStreamVersion(ctx, stream, expected); err != nil {
		return err
	}

	// KurrentDB uses stream metadata with TruncateBefore to truncate streams.
	// We set the $tb property via the stream metadata API.
	meta := kurrentdb.StreamMetadata{}
	meta.SetTruncateBefore(position)

	// The metadata stream ($$<stream>) has its own version, independent of
	// the data stream. We use Any{} to avoid version conflicts.
	opts := kurrentdb.AppendToStreamOptions{
		StreamState: kurrentdb.Any{},
	}

	_, err := s.client.SetStreamMetadata(ctx, string(stream), opts, meta)
	if err != nil {
		return mapError(err)
	}
	return nil
}

// validateStreamVersion checks the current stream version against the expected
// version. This is used when the actual KurrentDB operation targets a different
// stream (e.g. the metadata stream).
func (s *Store) validateStreamVersion(ctx context.Context, stream eventuous.StreamName, expected eventuous.ExpectedVersion) error {
	switch expected {
	case eventuous.VersionAny:
		return nil
	case eventuous.VersionNoStream:
		exists, err := s.StreamExists(ctx, stream)
		if err != nil {
			return err
		}
		if exists {
			return fmt.Errorf("%w: stream %q already exists", eventuous.ErrOptimisticConcurrency, stream)
		}
		return nil
	default:
		// Read 1 event backwards from end to get current version.
		readStream, err := s.client.ReadStream(ctx, string(stream), kurrentdb.ReadStreamOptions{
			From:      kurrentdb.End{},
			Direction: kurrentdb.Backwards,
		}, 1)
		if err != nil {
			return mapError(err)
		}
		defer readStream.Close()

		resolved, err := readStream.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				if expected != eventuous.VersionNoStream {
					return fmt.Errorf("%w: stream %q is empty but expected version %d", eventuous.ErrOptimisticConcurrency, stream, expected)
				}
				return nil
			}
			return mapError(err)
		}

		currentVersion := int64(resolved.Event.EventNumber)
		if currentVersion != int64(expected) {
			return fmt.Errorf("%w: expected version %d but stream %q is at version %d", eventuous.ErrOptimisticConcurrency, expected, stream, currentVersion)
		}
		return nil
	}
}

// collectEvents reads all events from a KurrentDB read stream and decodes them.
func (s *Store) collectEvents(readStream *kurrentdb.ReadStream) ([]store.StreamEvent, error) {
	var events []store.StreamEvent

	for {
		resolved, err := readStream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, mapError(err)
		}

		recorded := resolved.Event

		payload, err := s.codec.Decode(recorded.Data, recorded.EventType)
		if err != nil {
			return nil, fmt.Errorf("decoding event %s: %w", recorded.EventType, err)
		}

		var meta eventuous.Metadata
		if len(recorded.UserMetadata) > 0 {
			if err := json.Unmarshal(recorded.UserMetadata, &meta); err != nil {
				// If metadata doesn't unmarshal as a map, just leave it nil.
				meta = nil
			}
		}

		events = append(events, store.StreamEvent{
			ID:             recorded.EventID,
			EventType:      recorded.EventType,
			Payload:        payload,
			Metadata:       meta,
			ContentType:    recorded.ContentType,
			Position:       int64(recorded.EventNumber),
			GlobalPosition: recorded.Position.Commit,
			Created:        recorded.CreatedDate,
		})
	}

	if len(events) == 0 {
		return events, nil
	}
	return events, nil
}

// toStreamState maps an eventuous ExpectedVersion to a KurrentDB StreamState.
func toStreamState(v eventuous.ExpectedVersion) kurrentdb.StreamState {
	switch v {
	case eventuous.VersionNoStream:
		return kurrentdb.NoStream{}
	case eventuous.VersionAny:
		return kurrentdb.Any{}
	default:
		return kurrentdb.Revision(uint64(v))
	}
}

// mapError translates KurrentDB errors to eventuous sentinel errors.
func mapError(err error) error {
	if err == nil {
		return nil
	}

	kdbErr, ok := kurrentdb.FromError(err)
	// In the KurrentDB Go client, ok == false means there IS an error.
	if !ok {
		switch kdbErr.Code() {
		case kurrentdb.ErrorCodeResourceNotFound:
			return fmt.Errorf("%w: %s", eventuous.ErrStreamNotFound, kdbErr.Error())
		case kurrentdb.ErrorCodeWrongExpectedVersion:
			return fmt.Errorf("%w: %s", eventuous.ErrOptimisticConcurrency, kdbErr.Error())
		default:
			// Some operations (e.g. DeleteStream) return ErrorCodeUnknown but
			// the underlying gRPC error message contains WrongExpectedVersion.
			if strings.Contains(kdbErr.Error(), "WrongExpectedVersion") {
				return fmt.Errorf("%w: %s", eventuous.ErrOptimisticConcurrency, kdbErr.Error())
			}
		}
		return fmt.Errorf("kurrentdb: %w", kdbErr)
	}

	return err
}

