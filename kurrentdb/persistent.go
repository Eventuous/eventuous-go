package kurrentdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync/atomic"

	"github.com/kurrent-io/KurrentDB-Client-Go/kurrentdb"

	eventuous "github.com/eventuous/eventuous-go/core"
	"github.com/eventuous/eventuous-go/core/codec"
	"github.com/eventuous/eventuous-go/core/subscription"
)

// Compile-time interface check.
var _ subscription.Subscription = (*Persistent)(nil)

// Persistent is a persistent subscription that reads events from KurrentDB
// either from a single stream or from $all. The server manages checkpoints
// and supports ack/nack for at-least-once delivery.
type Persistent struct {
	client *kurrentdb.Client
	codec  codec.Codec
	id     string // used as the group name
	config persistentConfig
	logger *slog.Logger
}

// NewPersistent creates a new persistent subscription.
func NewPersistent(client *kurrentdb.Client, codec codec.Codec, id string, opts ...PersistentOption) *Persistent {
	cfg := persistentConfig{}
	for _, o := range opts {
		o(&cfg)
	}
	return &Persistent{
		client: client,
		codec:  codec,
		id:     id,
		config: cfg,
		logger: slog.Default().With("subscription", id),
	}
}

// Start begins consuming events and blocks until the context is cancelled
// or a fatal error occurs.
func (s *Persistent) Start(ctx context.Context) error {
	if s.config.handler == nil {
		return fmt.Errorf("kurrentdb: persistent subscription %q: handler is required", s.id)
	}

	// Build handler chain with middleware.
	handler := subscription.Chain(s.config.handler, s.config.middleware...)

	// Try to create the persistent subscription (auto-create if not exists).
	if err := s.ensureSubscription(ctx); err != nil {
		return fmt.Errorf("kurrentdb: persistent subscription %q: create: %w", s.id, err)
	}

	// Connect to the persistent subscription.
	sub, err := s.connect(ctx)
	if err != nil {
		return fmt.Errorf("kurrentdb: persistent subscription %q: connect: %w", s.id, err)
	}
	defer sub.Close()

	s.logger.Info("persistent subscription started",
		"stream", s.streamName(),
		"group", s.id,
	)

	// Event loop.
	var sequence atomic.Uint64
	for {
		event := sub.Recv()

		if event.SubscriptionDropped != nil {
			// If context was cancelled, treat as clean shutdown.
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("kurrentdb: persistent subscription %q: dropped: %w", s.id, event.SubscriptionDropped.Error)
		}

		if event.EventAppeared == nil {
			// Ignore CheckPointReached.
			continue
		}

		resolved := event.EventAppeared.Event
		recorded := resolved.OriginalEvent()

		// Skip system events (event types starting with $).
		if strings.HasPrefix(recorded.EventType, "$") {
			if err := sub.Ack(resolved); err != nil {
				s.logger.Debug("failed to ack system event",
					"eventType", recorded.EventType,
					"error", err,
				)
			}
			continue
		}

		// Decode the event payload.
		payload, decodeErr := s.codec.Decode(recorded.Data, recorded.EventType)
		if decodeErr != nil {
			// Unknown type: set payload to nil, don't skip the event.
			s.logger.Debug("failed to decode event, setting payload to nil",
				"eventType", recorded.EventType,
				"error", decodeErr,
			)
			payload = nil
		}

		// Parse user metadata.
		var meta eventuous.Metadata
		if len(recorded.UserMetadata) > 0 {
			if err := json.Unmarshal(recorded.UserMetadata, &meta); err != nil {
				s.logger.Debug("failed to unmarshal metadata",
					"eventType", recorded.EventType,
					"error", err,
				)
			}
		}

		// Determine position and global position.
		var position, globalPosition uint64
		if s.config.stream != nil {
			position = recorded.EventNumber
			if resolved.Commit != nil {
				globalPosition = *resolved.Commit
			} else {
				globalPosition = recorded.Position.Commit
			}
		} else {
			position = recorded.EventNumber
			globalPosition = recorded.Position.Commit
		}

		seq := sequence.Add(1)

		consumeCtx := &subscription.ConsumeContext{
			EventID:        recorded.EventID,
			EventType:      recorded.EventType,
			Stream:         eventuous.StreamName(recorded.StreamID),
			Payload:        payload,
			Metadata:       meta,
			ContentType:    recorded.ContentType,
			Position:       position,
			GlobalPosition: globalPosition,
			Sequence:       seq,
			Created:        recorded.CreatedDate,
			SubscriptionID: s.id,
		}

		if err := handler.HandleEvent(ctx, consumeCtx); err != nil {
			// Handler failed: nack with retry.
			s.logger.Debug("handler error, nacking event",
				"eventType", recorded.EventType,
				"position", position,
				"error", err,
			)
			if nackErr := sub.Nack(err.Error(), kurrentdb.NackActionRetry, resolved); nackErr != nil {
				s.logger.Debug("failed to nack event",
					"eventType", recorded.EventType,
					"error", nackErr,
				)
			}
		} else {
			// Handler succeeded: ack the event.
			if ackErr := sub.Ack(resolved); ackErr != nil {
				s.logger.Debug("failed to ack event",
					"eventType", recorded.EventType,
					"error", ackErr,
				)
			}
		}
	}
}

// ensureSubscription creates the persistent subscription group if it does not already exist.
func (s *Persistent) ensureSubscription(ctx context.Context) error {
	if s.config.stream != nil {
		err := s.client.CreatePersistentSubscription(ctx, string(*s.config.stream), s.id,
			kurrentdb.PersistentStreamSubscriptionOptions{
				StartFrom: kurrentdb.Start{},
			})
		if err != nil && !isAlreadyExists(err) {
			return err
		}
		return nil
	}

	// $all subscription.
	opts := kurrentdb.PersistentAllSubscriptionOptions{
		StartFrom: kurrentdb.Start{},
		Filter:    s.config.filter,
	}
	err := s.client.CreatePersistentSubscriptionToAll(ctx, s.id, opts)
	if err != nil && !isAlreadyExists(err) {
		return err
	}
	return nil
}

// connect subscribes to the persistent subscription group.
func (s *Persistent) connect(ctx context.Context) (*kurrentdb.PersistentSubscription, error) {
	opts := kurrentdb.SubscribeToPersistentSubscriptionOptions{
		BufferSize: s.config.bufferSize,
	}

	if s.config.stream != nil {
		return s.client.SubscribeToPersistentSubscription(ctx, string(*s.config.stream), s.id, opts)
	}
	return s.client.SubscribeToPersistentSubscriptionToAll(ctx, s.id, opts)
}

// streamName returns a display name for logging purposes.
func (s *Persistent) streamName() string {
	if s.config.stream != nil {
		return s.config.stream.String()
	}
	return "$all"
}

// isAlreadyExists checks whether an error indicates the persistent subscription group already exists.
func isAlreadyExists(err error) bool {
	var kdbErr *kurrentdb.Error
	if errors.As(err, &kdbErr) {
		return kdbErr.IsErrorCode(kurrentdb.ErrorCodeResourceAlreadyExists)
	}
	return false
}
