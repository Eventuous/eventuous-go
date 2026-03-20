package subscription

import "context"

// Checkpoint represents the last processed position for a subscription.
type Checkpoint struct {
	ID       string
	Position *uint64 // nil means no checkpoint stored yet
}

// CheckpointStore persists and retrieves checkpoints.
type CheckpointStore interface {
	GetCheckpoint(ctx context.Context, id string) (Checkpoint, error)
	StoreCheckpoint(ctx context.Context, checkpoint Checkpoint) error
}
