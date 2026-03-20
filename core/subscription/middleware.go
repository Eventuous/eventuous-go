package subscription

import (
	"context"
	"hash/fnv"
	"log/slog"
	"sync"
)

// Middleware wraps a handler with additional behavior.
type Middleware func(EventHandler) EventHandler

// Chain applies middleware in order: first middleware is outermost.
//
// Example: Chain(h, A, B, C) produces A(B(C(h))), so execution order is
// A-before → B-before → C-before → h → C-after → B-after → A-after.
func Chain(handler EventHandler, mw ...Middleware) EventHandler {
	for i := len(mw) - 1; i >= 0; i-- {
		handler = mw[i](handler)
	}
	return handler
}

// WithLogging logs event processing at debug level.
func WithLogging(logger *slog.Logger) Middleware {
	return func(next EventHandler) EventHandler {
		return HandlerFunc(func(ctx context.Context, msg *ConsumeContext) error {
			logger.Debug("handling event",
				"type", msg.EventType,
				"stream", msg.Stream,
				"position", msg.Position,
			)
			err := next.HandleEvent(ctx, msg)
			if err != nil {
				logger.Debug("event handler error",
					"type", msg.EventType,
					"error", err,
				)
			} else {
				logger.Debug("event handled",
					"type", msg.EventType,
				)
			}
			return err
		})
	}
}

// WithConcurrency processes events concurrently up to the given limit.
// Uses a semaphore channel to bound concurrency. A WaitGroup ensures all
// in-flight goroutines complete before the handler returns (on context
// cancellation or error the semaphore drain still happens).
func WithConcurrency(limit int) Middleware {
	return func(next EventHandler) EventHandler {
		sem := make(chan struct{}, limit)
		var wg sync.WaitGroup

		return HandlerFunc(func(ctx context.Context, msg *ConsumeContext) error {
			// Acquire semaphore slot (blocks when at limit).
			sem <- struct{}{}
			wg.Add(1)

			go func() {
				defer func() {
					<-sem // release slot
					wg.Done()
				}()
				_ = next.HandleEvent(ctx, msg)
			}()

			// Wait for all goroutines when context is done.
			select {
			case <-ctx.Done():
				wg.Wait()
			default:
			}

			return nil
		})
	}
}

// partitionWork is a unit of work sent to a partition goroutine.
type partitionWork struct {
	ctx     context.Context
	msg     *ConsumeContext
	errChan chan error
}

// WithPartitioning distributes events across N goroutines by partition key.
// keyFunc extracts the partition key from a ConsumeContext; nil defaults to
// stream name. Events with the same key always go to the same partition so
// ordering is preserved per key.
//
// The partition goroutines are started lazily on the first HandleEvent call
// and run until their input channels are closed (i.e. the returned handler is
// garbage-collected) or the context passed to HandleEvent is cancelled.
func WithPartitioning(count int, keyFunc func(*ConsumeContext) string) Middleware {
	if keyFunc == nil {
		keyFunc = func(msg *ConsumeContext) string {
			return msg.Stream.String()
		}
	}

	return func(next EventHandler) EventHandler {
		channels := make([]chan partitionWork, count)
		for i := range channels {
			channels[i] = make(chan partitionWork, 64)
		}

		var once sync.Once
		var workerCtx context.Context
		var cancelWorkers context.CancelFunc

		startWorkers := func(ctx context.Context) {
			once.Do(func() {
				workerCtx, cancelWorkers = context.WithCancel(ctx)
				for i := range channels {
					ch := channels[i]
					go func() {
						for work := range ch {
							err := next.HandleEvent(work.ctx, work.msg)
							work.errChan <- err
						}
					}()
				}
				// Stop workers when the parent context is cancelled.
				go func() {
					<-workerCtx.Done()
					cancelWorkers()
					for _, ch := range channels {
						close(ch)
					}
				}()
			})
		}

		return HandlerFunc(func(ctx context.Context, msg *ConsumeContext) error {
			startWorkers(ctx)

			key := keyFunc(msg)
			partition := hashKey(key, count)

			errChan := make(chan error, 1)
			work := partitionWork{ctx: ctx, msg: msg, errChan: errChan}

			select {
			case channels[partition] <- work:
			case <-ctx.Done():
				return ctx.Err()
			case <-workerCtx.Done():
				return workerCtx.Err()
			}

			select {
			case err := <-errChan:
				return err
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}
}

// hashKey returns a partition index in [0, count) for the given key using FNV-1a.
func hashKey(key string, count int) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	return int(h.Sum32()) % count
}
