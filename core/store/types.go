// Copyright (C) Eventuous HQ OÜ. All rights reserved
// Licensed under the Apache License, Version 2.0.

package store

import (
	"time"

	"github.com/google/uuid"

	eventuous "github.com/eventuous/eventuous-go/core"
)

// StreamEvent is an event read from a stream.
type StreamEvent struct {
	ID             uuid.UUID
	EventType      string
	Payload        any
	Metadata       eventuous.Metadata
	ContentType    string
	Position       int64  // position within the stream
	GlobalPosition uint64 // position in the global log
	Created        time.Time
}

// NewStreamEvent is an event to be appended to a stream.
type NewStreamEvent struct {
	ID       uuid.UUID
	Payload  any
	Metadata eventuous.Metadata
}

// AppendResult is returned after a successful append.
type AppendResult struct {
	GlobalPosition      uint64
	NextExpectedVersion int64
}
