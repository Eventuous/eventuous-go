// Copyright (C) Eventuous HQ OÜ. All rights reserved
// Licensed under the Apache License, Version 2.0.

package subscription

import "context"

// Subscription consumes events from a source.
// Start blocks until ctx is cancelled or a fatal error occurs.
type Subscription interface {
	Start(ctx context.Context) error
}
