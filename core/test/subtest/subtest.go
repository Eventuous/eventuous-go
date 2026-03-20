package subtest

import (
	"testing"

	eventuous "github.com/eventuous/eventuous-go/core"
	"github.com/eventuous/eventuous-go/core/codec"
	"github.com/eventuous/eventuous-go/core/store"
	"github.com/eventuous/eventuous-go/core/subscription"
)

// Config provides everything needed to test a subscription implementation.
type Config struct {
	// Store is the event store to write test events to.
	Store store.EventStore

	// NewStreamSub creates a catch-up subscription for a specific stream.
	// The subscription should use the provided handler and checkpoint store.
	NewStreamSub func(stream eventuous.StreamName, handler subscription.EventHandler, cs subscription.CheckpointStore) subscription.Subscription

	// NewAllSub creates a catch-up subscription for $all.
	NewAllSub func(handler subscription.EventHandler, cs subscription.CheckpointStore) subscription.Subscription
}

// NewCodec creates a JSON codec with the conformance test event type registered.
// External packages that wire the conformance suite must use this codec (or one
// that includes the same type) when creating their event store.
func NewCodec() codec.Codec {
	tm := codec.NewTypeMap()
	if err := codec.Register[testEvent](tm, "TestEvent"); err != nil {
		panic(err)
	}
	return codec.NewJSON(tm)
}

// RunAll runs the subscription conformance suite.
func RunAll(t *testing.T, cfg Config) {
	t.Run("ConsumeProducedEvents", func(t *testing.T) { TestConsumeProducedEvents(t, cfg) })
	t.Run("ResumeFromCheckpoint", func(t *testing.T) { TestResumeFromCheckpoint(t, cfg) })
}
