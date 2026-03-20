package kurrentdb_test

import (
	"testing"

	eventuous "github.com/eventuous/eventuous-go/core"
	"github.com/eventuous/eventuous-go/core/subscription"
	"github.com/eventuous/eventuous-go/core/test/subtest"
	kdb "github.com/eventuous/eventuous-go/kurrentdb"
)

func TestKurrentDB_SubscriptionConformance(t *testing.T) {
	client := setupClient(t)
	c := subtest.NewCodec()
	s := kdb.NewStore(client, c)

	subtest.RunAll(t, subtest.Config{
		Store: s,
		NewStreamSub: func(stream eventuous.StreamName, h subscription.EventHandler, cs subscription.CheckpointStore) subscription.Subscription {
			// Use stream.String() as the subscription ID so the checkpoint ID
			// matches what the conformance test sets in the checkpoint store.
			return kdb.NewCatchUp(client, c, stream.String(),
				kdb.FromStream(stream),
				kdb.WithHandler(h),
				kdb.WithCheckpointStore(cs),
			)
		},
		NewAllSub: func(h subscription.EventHandler, cs subscription.CheckpointStore) subscription.Subscription {
			return kdb.NewCatchUp(client, c, "conformance-all",
				kdb.FromAll(),
				kdb.WithHandler(h),
				kdb.WithCheckpointStore(cs),
			)
		},
	})
}
