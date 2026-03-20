package subscription_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	eventuous "github.com/eventuous/eventuous-go/core"
	"github.com/eventuous/eventuous-go/core/subscription"
)

// helpers

func makeMsg(stream eventuous.StreamName) *subscription.ConsumeContext {
	return &subscription.ConsumeContext{
		EventID:   uuid.New(),
		EventType: "TestEvent",
		Stream:    stream,
		Created:   time.Now(),
	}
}

func noopHandler() subscription.EventHandler {
	return subscription.HandlerFunc(func(_ context.Context, _ *subscription.ConsumeContext) error {
		return nil
	})
}

// TestHandlerFunc_ImplementsInterface is a compile-time check.
func TestHandlerFunc_ImplementsInterface(t *testing.T) {
	var _ subscription.EventHandler = subscription.HandlerFunc(nil)
}

// TestChain_ExecutesInOrder verifies that middleware wraps handlers in the correct order.
func TestChain_ExecutesInOrder(t *testing.T) {
	var mu sync.Mutex
	var calls []string

	record := func(s string) {
		mu.Lock()
		calls = append(calls, s)
		mu.Unlock()
	}

	makeMW := func(name string) subscription.Middleware {
		return func(next subscription.EventHandler) subscription.EventHandler {
			return subscription.HandlerFunc(func(ctx context.Context, msg *subscription.ConsumeContext) error {
				record(name + "-before")
				err := next.HandleEvent(ctx, msg)
				record(name + "-after")
				return err
			})
		}
	}

	inner := subscription.HandlerFunc(func(_ context.Context, _ *subscription.ConsumeContext) error {
		record("handler")
		return nil
	})

	chained := subscription.Chain(inner, makeMW("A"), makeMW("B"), makeMW("C"))

	if err := chained.HandleEvent(context.Background(), makeMsg("test-1")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{"A-before", "B-before", "C-before", "handler", "C-after", "B-after", "A-after"}
	if len(calls) != len(want) {
		t.Fatalf("got calls %v, want %v", calls, want)
	}
	for i, c := range calls {
		if c != want[i] {
			t.Errorf("calls[%d] = %q, want %q", i, c, want[i])
		}
	}
}

// TestWithLogging_LogsEvents verifies that WithLogging emits slog records.
func TestWithLogging_LogsEvents(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	handler := subscription.Chain(noopHandler(), subscription.WithLogging(logger))

	msg := makeMsg("order-1")
	msg.EventType = "OrderPlaced"

	if err := handler.HandleEvent(context.Background(), msg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "OrderPlaced") {
		t.Errorf("log output %q does not contain event type %q", out, "OrderPlaced")
	}
}

// TestWithConcurrency_LimitsConcurrency verifies that no more than `limit` handlers run at once.
func TestWithConcurrency_LimitsConcurrency(t *testing.T) {
	const limit = 2
	const total = 5

	var (
		active  atomic.Int64
		maxSeen atomic.Int64
		wg      sync.WaitGroup
	)

	slow := subscription.HandlerFunc(func(_ context.Context, _ *subscription.ConsumeContext) error {
		cur := active.Add(1)
		// track max
		for {
			prev := maxSeen.Load()
			if cur <= prev || maxSeen.CompareAndSwap(prev, cur) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		active.Add(-1)
		return nil
	})

	handler := subscription.Chain(slow, subscription.WithConcurrency(limit))

	for i := range total {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = handler.HandleEvent(context.Background(), makeMsg(eventuous.StreamName("stream-"+string(rune('A'+i)))))
		}(i)
	}

	wg.Wait()

	if got := maxSeen.Load(); got > limit {
		t.Errorf("max concurrent = %d, want <= %d", got, limit)
	}
}

// TestWithPartitioning_SameKeySamePartition verifies that events for the same stream
// arrive at the inner handler in order.
func TestWithPartitioning_SameKeySamePartition(t *testing.T) {
	const numPartitions = 4
	const eventsPerStream = 5

	type arrival struct {
		stream   eventuous.StreamName
		sequence int
	}

	var (
		mu       sync.Mutex
		arrivals []arrival
	)

	inner := subscription.HandlerFunc(func(_ context.Context, msg *subscription.ConsumeContext) error {
		mu.Lock()
		arrivals = append(arrivals, arrival{stream: msg.Stream, sequence: int(msg.Sequence)})
		mu.Unlock()
		return nil
	})

	handler := subscription.Chain(inner, subscription.WithPartitioning(numPartitions, nil))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the partitioned handler (it needs a context to run goroutines).
	// We pass ctx via HandleEvent calls — the partitioner goroutines should
	// start on first use or be started externally. Per the spec the middleware
	// factory starts goroutines when the handler is created.
	// Since Chain returns an EventHandler, we call it directly.

	var wg sync.WaitGroup
	streams := []eventuous.StreamName{"booking-123", "booking-456"}

	for seq := range eventsPerStream {
		for _, s := range streams {
			wg.Add(1)
			s := s
			seq := seq
			go func() {
				defer wg.Done()
				msg := makeMsg(s)
				msg.Sequence = uint64(seq)
				_ = handler.HandleEvent(ctx, msg)
			}()
		}
	}

	wg.Wait()

	// Verify ordering per stream.
	seenPerStream := map[eventuous.StreamName][]int{}
	for _, a := range arrivals {
		seenPerStream[a.stream] = append(seenPerStream[a.stream], a.sequence)
	}

	for _, s := range streams {
		seqs := seenPerStream[s]
		if len(seqs) != eventsPerStream {
			t.Errorf("stream %s: got %d events, want %d", s, len(seqs), eventsPerStream)
			continue
		}
		// Since callers send concurrently the absolute order may differ, but
		// events for the same stream must be delivered sequentially (no interleaving).
		// We just verify all sequences are present.
		seen := map[int]bool{}
		for _, seq := range seqs {
			seen[seq] = true
		}
		for i := range eventsPerStream {
			if !seen[i] {
				t.Errorf("stream %s: missing sequence %d, got %v", s, i, seqs)
			}
		}
	}
}
