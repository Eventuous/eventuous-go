package subscription_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/eventuous/eventuous-go/core/subscription"
)

// memCheckpointStore is an in-memory CheckpointStore for testing.
type memCheckpointStore struct {
	mu          sync.Mutex
	checkpoints map[string]subscription.Checkpoint
}

func newMemCheckpointStore() *memCheckpointStore {
	return &memCheckpointStore{
		checkpoints: make(map[string]subscription.Checkpoint),
	}
}

func (m *memCheckpointStore) GetCheckpoint(_ context.Context, id string) (subscription.Checkpoint, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if cp, ok := m.checkpoints[id]; ok {
		return cp, nil
	}
	return subscription.Checkpoint{ID: id}, nil
}

func (m *memCheckpointStore) StoreCheckpoint(_ context.Context, cp subscription.Checkpoint) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.checkpoints[cp.ID] = cp
	return nil
}

func (m *memCheckpointStore) getPosition(id string) *uint64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	if cp, ok := m.checkpoints[id]; ok {
		return cp.Position
	}
	return nil
}

// TestCommitter_CommitsAfterBatchSize verifies that after batchSize events the checkpoint is stored.
func TestCommitter_CommitsAfterBatchSize(t *testing.T) {
	store := newMemCheckpointStore()
	committer := subscription.NewCheckpointCommitter(store, "sub-1", 3, 0)
	ctx := context.Background()

	// positions are arbitrary; sequences are 1, 2, 3
	if err := committer.Commit(ctx, 100, 1); err != nil {
		t.Fatalf("Commit(100,1) error: %v", err)
	}
	if pos := store.getPosition("sub-1"); pos != nil {
		t.Errorf("after seq 1: expected no checkpoint, got %d", *pos)
	}

	if err := committer.Commit(ctx, 200, 2); err != nil {
		t.Fatalf("Commit(200,2) error: %v", err)
	}
	if pos := store.getPosition("sub-1"); pos != nil {
		t.Errorf("after seq 2: expected no checkpoint, got %d", *pos)
	}

	if err := committer.Commit(ctx, 300, 3); err != nil {
		t.Fatalf("Commit(300,3) error: %v", err)
	}
	pos := store.getPosition("sub-1")
	if pos == nil {
		t.Fatal("after seq 3: expected checkpoint, got nil")
	}
	if *pos != 300 {
		t.Errorf("after seq 3: expected position 300, got %d", *pos)
	}
}

// TestCommitter_CommitsOnInterval verifies that a time-based flush commits the checkpoint.
func TestCommitter_CommitsOnInterval(t *testing.T) {
	store := newMemCheckpointStore()
	// batchSize=0 means no batch limit; interval=50ms triggers commit by time
	committer := subscription.NewCheckpointCommitter(store, "sub-2", 0, 50*time.Millisecond)
	ctx := context.Background()

	if err := committer.Commit(ctx, 42, 1); err != nil {
		t.Fatalf("Commit(42,1) error: %v", err)
	}

	// No commit yet (interval not elapsed)
	if pos := store.getPosition("sub-2"); pos != nil {
		t.Errorf("immediate after commit: expected no checkpoint, got %d", *pos)
	}

	// Wait for interval to fire
	time.Sleep(100 * time.Millisecond)

	pos := store.getPosition("sub-2")
	if pos == nil {
		t.Fatal("after interval: expected checkpoint, got nil")
	}
	if *pos != 42 {
		t.Errorf("after interval: expected position 42, got %d", *pos)
	}
}

// TestCommitter_GapDetection verifies that gaps in sequence prevent premature checkpointing.
func TestCommitter_GapDetection(t *testing.T) {
	store := newMemCheckpointStore()
	// batchSize=1 so every advance triggers a commit attempt
	committer := subscription.NewCheckpointCommitter(store, "sub-3", 1, 0)
	ctx := context.Background()

	// seq 1 → position 10
	if err := committer.Commit(ctx, 10, 1); err != nil {
		t.Fatalf("Commit(10,1) error: %v", err)
	}
	pos := store.getPosition("sub-3")
	if pos == nil || *pos != 10 {
		t.Errorf("after seq 1: expected position 10, got %v", pos)
	}

	// seq 2 → position 20
	if err := committer.Commit(ctx, 20, 2); err != nil {
		t.Fatalf("Commit(20,2) error: %v", err)
	}
	pos = store.getPosition("sub-3")
	if pos == nil || *pos != 20 {
		t.Errorf("after seq 2: expected position 20, got %v", pos)
	}

	// seq 4 → position 40 (gap at seq 3)
	if err := committer.Commit(ctx, 40, 4); err != nil {
		t.Fatalf("Commit(40,4) error: %v", err)
	}
	pos = store.getPosition("sub-3")
	// Checkpoint should still be at 20 because seq 3 hasn't arrived yet
	if pos == nil || *pos != 20 {
		t.Errorf("after seq 4 (gap at 3): expected position still 20, got %v", pos)
	}

	// seq 3 → position 30: fills the gap, so we can advance to seq 4's position
	if err := committer.Commit(ctx, 30, 3); err != nil {
		t.Fatalf("Commit(30,3) error: %v", err)
	}
	pos = store.getPosition("sub-3")
	if pos == nil || *pos != 40 {
		t.Errorf("after seq 3 fills gap: expected position 40 (seq 4's position), got %v", pos)
	}
}

// TestCommitter_Flush verifies that Flush() commits immediately regardless of batch/interval.
func TestCommitter_Flush(t *testing.T) {
	store := newMemCheckpointStore()
	// Large batchSize and no interval — normally would never commit
	committer := subscription.NewCheckpointCommitter(store, "sub-4", 1000, 0)
	ctx := context.Background()

	if err := committer.Commit(ctx, 55, 1); err != nil {
		t.Fatalf("Commit(55,1) error: %v", err)
	}
	if err := committer.Commit(ctx, 66, 2); err != nil {
		t.Fatalf("Commit(66,2) error: %v", err)
	}

	// Not yet committed (batch not full)
	if pos := store.getPosition("sub-4"); pos != nil {
		t.Errorf("before Flush: expected no checkpoint, got %d", *pos)
	}

	if err := committer.Flush(ctx); err != nil {
		t.Fatalf("Flush error: %v", err)
	}

	pos := store.getPosition("sub-4")
	if pos == nil {
		t.Fatal("after Flush: expected checkpoint, got nil")
	}
	if *pos != 66 {
		t.Errorf("after Flush: expected position 66, got %d", *pos)
	}
}

// TestCommitter_OutOfOrder verifies that out-of-order sequences are handled correctly.
func TestCommitter_OutOfOrder(t *testing.T) {
	store := newMemCheckpointStore()
	// batchSize=1 so every advance triggers a commit
	committer := subscription.NewCheckpointCommitter(store, "sub-5", 1, 0)
	ctx := context.Background()

	// seq 3 arrives first
	if err := committer.Commit(ctx, 300, 3); err != nil {
		t.Fatalf("Commit(300,3) error: %v", err)
	}
	// No commit: gap at 1 and 2
	if pos := store.getPosition("sub-5"); pos != nil {
		t.Errorf("after seq 3 (out of order): expected no checkpoint, got %d", *pos)
	}

	// seq 1 arrives
	if err := committer.Commit(ctx, 100, 1); err != nil {
		t.Fatalf("Commit(100,1) error: %v", err)
	}
	// Can now commit seq 1's position (seq 2 still missing)
	pos := store.getPosition("sub-5")
	if pos == nil || *pos != 100 {
		t.Errorf("after seq 1: expected position 100, got %v", pos)
	}

	// seq 2 arrives — now 1, 2, 3 are all present
	if err := committer.Commit(ctx, 200, 2); err != nil {
		t.Fatalf("Commit(200,2) error: %v", err)
	}
	// Should advance to seq 3's position
	pos = store.getPosition("sub-5")
	if pos == nil || *pos != 300 {
		t.Errorf("after seq 2 fills gap: expected position 300, got %v", pos)
	}
}
