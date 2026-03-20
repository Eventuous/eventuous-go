package kurrentdb

import (
	"github.com/kurrent-io/KurrentDB-Client-Go/kurrentdb"

	eventuous "github.com/eventuous/eventuous-go/core"
	"github.com/eventuous/eventuous-go/core/subscription"
)

type catchUpConfig struct {
	stream          *eventuous.StreamName // nil = $all
	handler         subscription.EventHandler
	checkpointStore subscription.CheckpointStore
	middleware      []subscription.Middleware
	resolveLinkTos  bool
	filter          *kurrentdb.SubscriptionFilter
}

// CatchUpOption configures a CatchUp subscription.
type CatchUpOption func(*catchUpConfig)

// FromStream configures the subscription to read from a single stream.
func FromStream(name eventuous.StreamName) CatchUpOption {
	return func(c *catchUpConfig) {
		c.stream = &name
	}
}

// FromAll configures the subscription to read from the $all stream.
func FromAll() CatchUpOption {
	return func(c *catchUpConfig) {
		c.stream = nil
	}
}

// WithHandler sets the event handler for the subscription.
func WithHandler(h subscription.EventHandler) CatchUpOption {
	return func(c *catchUpConfig) {
		c.handler = h
	}
}

// WithCheckpointStore sets the checkpoint store used to persist and resume positions.
func WithCheckpointStore(cs subscription.CheckpointStore) CatchUpOption {
	return func(c *catchUpConfig) {
		c.checkpointStore = cs
	}
}

// WithMiddleware adds middleware to the subscription handler chain.
func WithMiddleware(mw ...subscription.Middleware) CatchUpOption {
	return func(c *catchUpConfig) {
		c.middleware = append(c.middleware, mw...)
	}
}

// WithResolveLinkTos controls whether link events are resolved to their targets.
func WithResolveLinkTos(resolve bool) CatchUpOption {
	return func(c *catchUpConfig) {
		c.resolveLinkTos = resolve
	}
}

// WithFilter sets a server-side filter for $all subscriptions.
func WithFilter(filter *kurrentdb.SubscriptionFilter) CatchUpOption {
	return func(c *catchUpConfig) {
		c.filter = filter
	}
}
