# CLAUDE.md

This file provides guidance to Claude Code when working with code in this repository.

## What Is Eventuous Go

Go port of [Eventuous](https://github.com/Eventuous/eventuous), a production-grade Event Sourcing library. Implements DDD tactical patterns for Go: aggregates, command services, event stores, subscriptions, and projections. Functional-first design — the functional command service is the primary path; aggregate-based service is an optional layer.

## Build & Test Commands

```bash
# Build all modules
(cd core && go build ./...)
(cd kurrentdb && go build ./...)
(cd otel && go build ./...)

# Run all core tests (no infrastructure needed)
(cd core && go test ./...)

# Run all core tests with race detector
(cd core && go test -race ./...)

# Run kurrentdb integration tests (uses testcontainers — Docker required)
(cd kurrentdb && go test -race -timeout 300s ./...)

# Run otel tests
(cd otel && go test -race ./...)

# Run a single test by name
(cd core && go test ./aggregate/... -run TestClearChanges_AdvancesVersion -v)

# Check formatting
gofmt -l core/ kurrentdb/ otel/

# Vet all modules
(cd core && go vet ./...) && (cd kurrentdb && go vet ./...) && (cd otel && go vet ./...)
```

Integration tests use testcontainers-go to start KurrentDB automatically — no manual `docker compose up` needed. Docker must be running.

## Module Structure

This is a multi-module Go project for dependency efficiency. Each module has its own `go.mod`.

```
core/                              # github.com/eventuous/eventuous-go/core
├── eventuous.go                   # StreamName, ExpectedVersion, Metadata, sentinel errors
├── aggregate/                     # Domain: Aggregate[S] with fold, apply, guards
├── codec/                         # TypeMap (bidirectional type registry), Codec interface, JSON impl
├── store/                         # EventReader/Writer/Store interfaces, LoadState, LoadAggregate, StoreAggregate
├── command/                       # Functional Service[S], AggregateService[S], handler registration
├── subscription/                  # EventHandler, middleware chain, CheckpointCommitter with gap detection
└── test/                          # Exported test infrastructure
    ├── memstore/                  # In-memory EventStore for unit testing
    ├── storetest/                 # Store conformance test suite (run by any store impl)
    ├── subtest/                   # Subscription conformance test suite
    └── testdomain/                # Shared Booking domain (events, state, commands, codec setup)

kurrentdb/                         # github.com/eventuous/eventuous-go/kurrentdb
├── store.go                       # EventStore implementation for KurrentDB
├── catchup.go                     # Catch-up subscription (stream + $all)
├── persistent.go                  # Persistent subscription (stream + $all, ack/nack)
└── options.go                     # Functional options for subscriptions

otel/                              # github.com/eventuous/eventuous-go/otel
├── command.go                     # TracedCommandHandler decorator (tracing + metrics)
└── subscription.go                # TracingMiddleware for subscriptions
```

Dependencies flow: `kurrentdb → core`, `otel → core`. Core depends on nothing heavy.

## Architecture

### Domain Model

`Aggregate[S]` tracks state, pending changes, and version. State is any struct — no interface needed. State reconstruction uses a **fold function** (`func(S, any) S`) with a type switch, not handler registration.

### Command Services (Two Approaches)

**Functional** (primary): `command.Service[S]` — loads state via fold, handler is `func(ctx, state, cmd) ([]any, error)`, no aggregate involved. Registered via `command.On(svc, handler)`.

**Aggregate** (optional): `command.AggregateService[S]` — loads aggregate, handler calls `agg.Apply()`, framework reads `agg.Changes()`. Registered via `command.OnAggregate(svc, handler)`.

### Persistence

`store.EventReader`, `store.EventWriter`, `store.EventStore` interfaces. Package-level generic functions `LoadState`, `LoadAggregate`, `StoreAggregate` handle the load/store cycle.

### Subscriptions

`subscription.EventHandler` interface with `HandlerFunc` adaptor. Middleware chain pattern (like `net/http`): `WithConcurrency`, `WithPartitioning`, `WithLogging`. `CheckpointCommitter` handles batched commits with gap detection.

### Serialization

`codec.TypeMap` for bidirectional type name mapping (explicit registration, no reflection magic). `codec.Codec` interface with `JSONCodec` implementation.

## Key Conventions

- **Package name**: the root package is `eventuous` (import as `eventuous "github.com/eventuous/eventuous-go/core"`)
- **Stream naming**: `{Category}-{ID}` via `eventuous.NewStreamName(category, id)`
- **Type mapping**: events must be explicitly registered in `codec.TypeMap` — `codec.Register[MyEvent](tm, "MyEvent")`
- **Errors**: sentinel errors with `errors.Is()` — `ErrStreamNotFound`, `ErrOptimisticConcurrency`, `ErrHandlerNotFound`
- **Context**: all I/O functions take `context.Context` as first parameter
- **No DI container**: explicit wiring, functional options for configuration

## Code Style

- Go 1.25+ (matches module requirements in go.mod)
- Use generics where they reduce boilerplate, not for everything
- Errors over panics. Sentinel errors with `errors.Is()`
- `context.Context` as first parameter on all I/O functions
- `slog` for structured logging
- Functional options for configuration (subscription options, etc.)
- Table-driven tests
- All `.go` files must have the license header:
  ```go
  // Copyright (C) Eventuous HQ OÜ. All rights reserved
  // Licensed under the Apache License, Version 2.0.
  ```
- Run `gofmt` before committing — CI enforces formatting
- Run `go vet` — CI enforces vet

## Test Patterns

### Conformance test suites

Store implementations run the shared conformance suite:
```go
func TestMyStore(t *testing.T) {
    s := mystore.New()
    storetest.RunAll(t, s)
}
```

### Shared test domain

All command service and subscription tests use the Booking domain from `core/test/testdomain/`:
- Events: `RoomBooked`, `BookingImported`, `PaymentRecorded`, `BookingCancelled`
- State: `BookingState` with `BookingFold`
- Commands: `BookRoom`, `ImportBooking`, `RecordPayment`, `CancelBooking`
- Helpers: `testdomain.NewCodec()`, `testdomain.BookingStream(id)`

### Integration tests

KurrentDB tests use testcontainers-go — `setupClient(t)` in `kurrentdb/testutil_test.go` starts a container automatically.

## Design Specs

- Design spec: `docs/specs/2026-03-20-phase1-design.md`
- Implementation plan: `docs/specs/2026-03-20-phase1-plan.md`
