// Copyright (C) Eventuous HQ OÜ. All rights reserved
// Licensed under the Apache License, Version 2.0.

package subscription

import (
	"time"

	"github.com/google/uuid"

	eventuous "github.com/eventuous/eventuous-go/core"
)

// ConsumeContext carries the event data through the subscription pipeline.
type ConsumeContext struct {
	EventID        uuid.UUID
	EventType      string
	Stream         eventuous.StreamName
	Payload        any // deserialized event, nil if unknown type
	Metadata       eventuous.Metadata
	ContentType    string
	Position       uint64 // position in the source stream
	GlobalPosition uint64 // position in the global log ($all)
	Sequence       uint64 // local sequence number within this subscription
	Created        time.Time
	SubscriptionID string
}
