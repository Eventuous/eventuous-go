package subscription

import (
	"context"
	"sync"
	"time"
)

// CheckpointCommitter batches checkpoint writes with gap detection.
// It ensures we never commit position N+1 if position N hasn't been processed yet.
// This prevents data loss when events are processed concurrently and out of order.
type CheckpointCommitter struct {
	store          CheckpointStore
	subscriptionID string
	batchSize      int
	interval       time.Duration

	mu                   sync.Mutex
	pending              map[uint64]uint64 // sequence → position
	lastCommittedSeq     uint64            // highest committed sequence (0 = nothing committed yet)
	lastCommittedPos     uint64            // position associated with lastCommittedSeq
	hasCommittedPos      bool              // true once we have a real position to commit
	uncommittedSeq       uint64            // frontier of contiguous sequences we've seen but not yet stored
	uncommittedPos       uint64            // position at the uncommitted frontier
	hasUncommittedPos    bool              // true if we've advanced but haven't stored yet
	count                int               // events accumulated since last actual store
	lastCommitTime       time.Time
	timer                *time.Timer
}

// NewCheckpointCommitter creates a committer.
// batchSize: commit after this many events (0 = no batch limit).
// interval: commit at least this often (0 = no time limit).
func NewCheckpointCommitter(
	store CheckpointStore,
	subscriptionID string,
	batchSize int,
	interval time.Duration,
) *CheckpointCommitter {
	c := &CheckpointCommitter{
		store:          store,
		subscriptionID: subscriptionID,
		batchSize:      batchSize,
		interval:       interval,
		pending:        make(map[uint64]uint64),
		lastCommitTime: time.Now(),
	}

	if interval > 0 {
		c.timer = time.AfterFunc(interval, func() {
			_ = c.Flush(context.Background())
		})
	}

	return c
}

// Commit records that the event at the given position with the given sequence has been processed.
// The committer tracks these and commits the highest position where all prior sequences
// have also been processed (gap detection).
//
// Example: if events arrive as sequence 1,2,4 (skipping 3):
//   - After sequence 1: commit position of seq 1
//   - After sequence 2: commit position of seq 2
//   - After sequence 4: DON'T commit (seq 3 is missing, gap detected)
//   - When sequence 3 arrives: commit position of seq 4 (all gaps filled)
func (c *CheckpointCommitter) Commit(ctx context.Context, position uint64, sequence uint64) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Record this sequence → position in pending.
	c.pending[sequence] = position

	// Walk forward from the current frontier (lastCommittedSeq + 1 if nothing pending,
	// or from uncommittedSeq + 1 if we have an uncommitted frontier).
	startSeq := c.lastCommittedSeq
	if c.hasUncommittedPos {
		startSeq = c.uncommittedSeq
	}

	advanced := false
	for {
		next := startSeq + 1
		pos, ok := c.pending[next]
		if !ok {
			break
		}
		delete(c.pending, next)
		startSeq = next
		c.uncommittedSeq = next
		c.uncommittedPos = pos
		c.hasUncommittedPos = true
		advanced = true
	}

	if !advanced {
		// Gap detected or nothing new to advance.
		return nil
	}

	// We advanced — decide whether to actually write to the store.
	c.count++

	shouldCommit := false
	if c.batchSize > 0 && c.count >= c.batchSize {
		shouldCommit = true
	}
	if c.interval > 0 && time.Since(c.lastCommitTime) >= c.interval {
		shouldCommit = true
	}
	// If neither limit is set, always commit immediately.
	if c.batchSize == 0 && c.interval == 0 {
		shouldCommit = true
	}

	if !shouldCommit {
		return nil
	}

	return c.storeUncommittedLocked(ctx)
}

// storeUncommittedLocked writes the current uncommitted frontier to the store and
// updates the committed frontier. Must be called with mu held.
func (c *CheckpointCommitter) storeUncommittedLocked(ctx context.Context) error {
	if !c.hasUncommittedPos {
		return nil
	}

	pos := c.uncommittedPos
	cp := Checkpoint{
		ID:       c.subscriptionID,
		Position: &pos,
	}
	if err := c.store.StoreCheckpoint(ctx, cp); err != nil {
		return err
	}

	c.lastCommittedSeq = c.uncommittedSeq
	c.lastCommittedPos = c.uncommittedPos
	c.hasCommittedPos = true
	c.hasUncommittedPos = false
	c.count = 0
	c.lastCommitTime = time.Now()

	// Reset the timer if interval-based flushing is configured.
	if c.interval > 0 && c.timer != nil {
		c.timer.Reset(c.interval)
	}
	return nil
}

// Flush forces an immediate commit of the highest contiguous position.
func (c *CheckpointCommitter) Flush(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.storeUncommittedLocked(ctx)
}

// Close stops the timer and flushes.
func (c *CheckpointCommitter) Close(ctx context.Context) error {
	c.mu.Lock()
	if c.timer != nil {
		c.timer.Stop()
	}
	c.mu.Unlock()
	return c.Flush(ctx)
}
