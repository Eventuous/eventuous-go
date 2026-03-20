// Copyright (C) Eventuous HQ OÜ. All rights reserved
// Licensed under the Apache License, Version 2.0.

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

// --- Persistent subscription options ---

type persistentConfig struct {
	stream     *eventuous.StreamName // nil = $all
	handler    subscription.EventHandler
	middleware []subscription.Middleware
	bufferSize uint32
	filter     *kurrentdb.SubscriptionFilter // for $all only
}

// PersistentOption configures a Persistent subscription.
type PersistentOption func(*persistentConfig)

// PersistentFromStream configures the persistent subscription to read from a single stream.
func PersistentFromStream(name eventuous.StreamName) PersistentOption {
	return func(c *persistentConfig) {
		c.stream = &name
	}
}

// PersistentFromAll configures the persistent subscription to read from the $all stream.
func PersistentFromAll() PersistentOption {
	return func(c *persistentConfig) {
		c.stream = nil
	}
}

// PersistentWithHandler sets the event handler for the persistent subscription.
func PersistentWithHandler(h subscription.EventHandler) PersistentOption {
	return func(c *persistentConfig) {
		c.handler = h
	}
}

// PersistentWithMiddleware adds middleware to the persistent subscription handler chain.
func PersistentWithMiddleware(mw ...subscription.Middleware) PersistentOption {
	return func(c *persistentConfig) {
		c.middleware = append(c.middleware, mw...)
	}
}

// PersistentWithBufferSize sets the buffer size for the persistent subscription connection.
func PersistentWithBufferSize(size int) PersistentOption {
	return func(c *persistentConfig) {
		c.bufferSize = uint32(size)
	}
}

// PersistentWithFilter sets a server-side filter for $all persistent subscriptions.
func PersistentWithFilter(filter *kurrentdb.SubscriptionFilter) PersistentOption {
	return func(c *persistentConfig) {
		c.filter = filter
	}
}
