package subscription

import "context"

// Subscription consumes events from a source.
// Start blocks until ctx is cancelled or a fatal error occurs.
type Subscription interface {
	Start(ctx context.Context) error
}
