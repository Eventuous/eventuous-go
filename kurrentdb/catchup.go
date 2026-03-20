// Copyright (C) Eventuous HQ OÜ. All rights reserved
// Licensed under the Apache License, Version 2.0.

package kurrentdb

import (
	"context"
	"encoding/json"
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
var _ subscription.Subscription = (*CatchUp)(nil)

// CatchUp is a catch-up subscription that reads events from KurrentDB
// either from a single stream or from $all.
type CatchUp struct {
	client *kurrentdb.Client
	codec  codec.Codec
	id     string
	config catchUpConfig
	logger *slog.Logger
}

// NewCatchUp creates a new catch-up subscription.
func NewCatchUp(client *kurrentdb.Client, codec codec.Codec, id string, opts ...CatchUpOption) *CatchUp {
	cfg := catchUpConfig{}
	for _, o := range opts {
		o(&cfg)
	}
	return &CatchUp{
		client: client,
		codec:  codec,
		id:     id,
		config: cfg,
		logger: slog.Default().With("subscription", id),
	}
}

// Start begins consuming events and blocks until the context is cancelled
// or a fatal error occurs.
func (s *CatchUp) Start(ctx context.Context) error {
	if s.config.handler == nil {
		return fmt.Errorf("kurrentdb: catch-up subscription %q: handler is required", s.id)
	}

	// Build handler chain with middleware.
	handler := subscription.Chain(s.config.handler, s.config.middleware...)

	// Load checkpoint.
	var checkpointPos *uint64
	if s.config.checkpointStore != nil {
		cp, err := s.config.checkpointStore.GetCheckpoint(ctx, s.id)
		if err != nil {
			return fmt.Errorf("kurrentdb: catch-up subscription %q: load checkpoint: %w", s.id, err)
		}
		checkpointPos = cp.Position
	}

	// Subscribe to KurrentDB.
	var sub *kurrentdb.Subscription
	var err error

	if s.config.stream != nil {
		sub, err = s.subscribeToStream(ctx, *s.config.stream, checkpointPos)
	} else {
		sub, err = s.subscribeToAll(ctx, checkpointPos)
	}
	if err != nil {
		return fmt.Errorf("kurrentdb: catch-up subscription %q: subscribe: %w", s.id, err)
	}
	defer sub.Close()

	s.logger.Info("subscription started",
		"stream", s.streamName(),
		"checkpoint", checkpointPos,
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
			return fmt.Errorf("kurrentdb: catch-up subscription %q: dropped: %w", s.id, event.SubscriptionDropped.Error)
		}

		if event.EventAppeared == nil {
			// Ignore CheckPointReached, CaughtUp, FellBehind.
			continue
		}

		resolved := event.EventAppeared
		recorded := resolved.OriginalEvent()

		// Skip system events (event types starting with $).
		if strings.HasPrefix(recorded.EventType, "$") {
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
			return fmt.Errorf("kurrentdb: catch-up subscription %q: handler error at position %d: %w", s.id, position, err)
		}

		// Store checkpoint.
		if s.config.checkpointStore != nil {
			var cpPosition uint64
			if s.config.stream != nil {
				cpPosition = recorded.EventNumber
			} else {
				cpPosition = recorded.Position.Commit
			}
			cp := subscription.Checkpoint{
				ID:       s.id,
				Position: &cpPosition,
			}
			if err := s.config.checkpointStore.StoreCheckpoint(ctx, cp); err != nil {
				return fmt.Errorf("kurrentdb: catch-up subscription %q: store checkpoint: %w", s.id, err)
			}
		}
	}
}

// subscribeToStream creates a stream subscription starting after the checkpoint position.
// The KurrentDB subscription API's From field means "start after this position",
// so we pass the checkpoint position directly (not checkpoint+1).
func (s *CatchUp) subscribeToStream(ctx context.Context, stream eventuous.StreamName, checkpoint *uint64) (*kurrentdb.Subscription, error) {
	opts := kurrentdb.SubscribeToStreamOptions{
		ResolveLinkTos: s.config.resolveLinkTos,
	}

	if checkpoint != nil {
		opts.From = kurrentdb.Revision(*checkpoint)
	} else {
		opts.From = kurrentdb.Start{}
	}

	return s.client.SubscribeToStream(ctx, string(stream), opts)
}

// subscribeToAll creates an $all subscription starting after the checkpoint position.
// The KurrentDB subscription API's From field means "start after this position",
// so we pass the checkpoint position directly (not checkpoint+1).
func (s *CatchUp) subscribeToAll(ctx context.Context, checkpoint *uint64) (*kurrentdb.Subscription, error) {
	opts := kurrentdb.SubscribeToAllOptions{
		ResolveLinkTos: s.config.resolveLinkTos,
		Filter:         s.config.filter,
	}

	if checkpoint != nil {
		opts.From = kurrentdb.Position{
			Commit:  *checkpoint,
			Prepare: *checkpoint,
		}
	} else {
		opts.From = kurrentdb.Start{}
	}

	return s.client.SubscribeToAll(ctx, opts)
}

// streamName returns a display name for logging purposes.
func (s *CatchUp) streamName() string {
	if s.config.stream != nil {
		return s.config.stream.String()
	}
	return "$all"
}
