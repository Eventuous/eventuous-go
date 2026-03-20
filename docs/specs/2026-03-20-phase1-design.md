# Eventuous Go — Phase 1 Design Spec

## Overview

Go port of Eventuous, a production-grade Event Sourcing library. Phase 1 covers the core framework and KurrentDB integration, enough to write and read aggregates, handle commands, and subscribe to events.

### Design principles

- **Functional-first**: Pure functions over OOP. The functional command service is the primary path; aggregate-based service is an optional layer.
- **Idiomatic Go**: Composition over inheritance. Explicit wiring over DI containers. Middleware chains over pipe/filter. `context.Context` + errors over async/await + exceptions.
- **Dependency efficiency**: Multi-module layout. Core has near-zero external deps. Each integration (KurrentDB, OTel) is a separate Go module.

### What's NOT in Phase 1

- Producers (BaseProducer, IProducer)
- Gateway (subscription-to-producer routing)
- Non-KurrentDB stores (PostgreSQL, SQL Server, MongoDB)
- Non-KurrentDB subscriptions (Kafka, RabbitMQ, Google Pub/Sub)
- ASP.NET-style HTTP command mapping (Go has its own HTTP patterns)
- Source generators (Go uses explicit registration)

---

## Module structure

```
eventuous-go/
├── core/                              # github.com/eventuous/eventuous-go/core
│   ├── go.mod                         # near-zero deps (uuid, slog)
│   ├── eventuous.go                   # StreamName, ExpectedVersion, Metadata, sentinel errors
│   ├── aggregate/                     # Domain concern
│   │   └── aggregate.go               # Aggregate[S]
│   ├── codec/                         # Serialization
│   │   ├── codec.go                   # Codec interface
│   │   ├── json.go                    # JSON implementation
│   │   └── typemap.go                 # TypeMap registry
│   ├── store/                         # Persistence
│   │   ├── interfaces.go              # EventReader, EventWriter, EventStore
│   │   ├── types.go                   # StreamEvent, NewStreamEvent, AppendResult
│   │   ├── state.go                   # LoadState (functional path)
│   │   └── aggregate.go              # LoadAggregate, StoreAggregate
│   ├── command/                       # Application
│   │   ├── service.go                 # Functional Service[S]
│   │   ├── handler.go                 # Handler[C,S], On registration
│   │   ├── result.go                  # Result[S]
│   │   └── aggservice.go             # AggregateService[S]
│   ├── subscription/                  # Subscription framework
│   │   ├── handler.go                 # EventHandler interface, HandlerFunc
│   │   ├── context.go                 # ConsumeContext
│   │   ├── subscription.go            # Subscription interface
│   │   ├── middleware.go              # Middleware type + built-ins
│   │   ├── checkpoint.go             # CheckpointStore, Checkpoint
│   │   └── committer.go              # Batched checkpoint committer
│   └── test/                          # Exported conformance tests
│       └── storetest/                 # Store implementation test suite
│
├── kurrentdb/                         # github.com/eventuous/eventuous-go/kurrentdb
│   ├── go.mod                         # depends on core + EventStore Go gRPC client
│   ├── store.go                       # EventStore implementation
│   ├── catchup.go                     # Catch-up subscription (stream + $all)
│   ├── persistent.go                  # Persistent subscription (stream + $all)
│   └── options.go                     # Functional options
│
└── otel/                              # github.com/eventuous/eventuous-go/otel
    ├── go.mod                         # depends on core + OTel SDK
    ├── command.go                     # Command service tracing/metrics
    └── subscription.go               # Subscription tracing middleware
```

Dependencies flow: `kurrentdb → core`, `otel → core`. Core depends on nothing heavy.

---

## Component specs

### 1. Shared primitives — `core/eventuous.go`

```go
package eventuous

// StreamName is a typed string with "{Category}-{ID}" convention.
type StreamName string

func NewStreamName(category, id string) StreamName
func (s StreamName) Category() string    // everything before first "-"
func (s StreamName) ID() string          // everything after first "-"
func (s StreamName) String() string

// ExpectedVersion for optimistic concurrency on append.
type ExpectedVersion int64

const (
    VersionNoStream ExpectedVersion = -1  // stream must not exist
    VersionAny      ExpectedVersion = -2  // no concurrency check
)

// Metadata is event metadata (correlation ID, causation ID, custom headers).
type Metadata map[string]any

const (
    MetaCorrelationID = "eventuous.correlation-id"
    MetaCausationID   = "eventuous.causation-id"
    MetaMessageID     = "eventuous.message-id"
)

func (m Metadata) CorrelationID() string
func (m Metadata) WithCorrelationID(id string) Metadata
func (m Metadata) CausationID() string
func (m Metadata) WithCausationID(id string) Metadata

// ExpectedState controls how the command service loads state.
type ExpectedState int

const (
    IsNew      ExpectedState = iota  // stream must not exist
    IsExisting                        // stream must exist
    IsAny                             // either
)

// Sentinel errors.
var (
    ErrStreamNotFound        = errors.New("eventuous: stream not found")
    ErrOptimisticConcurrency = errors.New("eventuous: wrong expected version")
    ErrAggregateNotFound     = errors.New("eventuous: aggregate not found")
)
```

### 2. Aggregate — `core/aggregate/aggregate.go`

Domain concept. Not used by the functional command service.

```go
package aggregate

// Aggregate tracks state, pending changes, and version for optimistic concurrency.
// S is any state type — no interface constraint needed.
type Aggregate[S any] struct {
    state           S
    original        []any
    changes         []any
    originalVersion int64
    fold            func(S, any) S
}

func New[S any](fold func(S, any) S, zero S) *Aggregate[S]

// Read access
func (a *Aggregate[S]) State() S
func (a *Aggregate[S]) Changes() []any
func (a *Aggregate[S]) OriginalVersion() int64
func (a *Aggregate[S]) CurrentVersion() int64  // originalVersion + len(changes)

// Apply records a new event and folds it into state.
func (a *Aggregate[S]) Apply(event any)

// Load reconstructs state from persisted events (called by store.LoadAggregate).
func (a *Aggregate[S]) Load(version int64, events []any)

// ClearChanges resets pending changes after successful store.
func (a *Aggregate[S]) ClearChanges()

// Guards
func (a *Aggregate[S]) EnsureNew() error      // error if originalVersion >= 0
func (a *Aggregate[S]) EnsureExists() error    // error if originalVersion < 0
```

Users write domain logic as free functions, not methods on a custom aggregate subclass:

```go
func BookRoom(agg *aggregate.Aggregate[BookingState], roomID string) error {
    if err := agg.EnsureNew(); err != nil {
        return err
    }
    agg.Apply(RoomBooked{RoomID: roomID})
    return nil
}
```

### 3. Type map — `core/codec/typemap.go`

Bidirectional mapping between Go types and stable event type name strings. Required for serialization of stored events.

```go
package codec

// TypeMap holds bidirectional type ↔ name mappings.
type TypeMap struct {
    toName   map[reflect.Type]string
    fromName map[string]reflect.Type
}

func NewTypeMap() *TypeMap

// Register maps a Go type to a persistent event type name.
// The name is what gets stored — renaming the Go struct won't break existing events.
func Register[E any](tm *TypeMap, name string)

// TypeName returns the registered name for an event value.
func (tm *TypeMap) TypeName(event any) (string, error)

// NewInstance creates an empty pointer of the type registered under name.
// Used by Decode to unmarshal into the correct Go type.
func (tm *TypeMap) NewInstance(name string) (any, error)

// Convenience: reflect-based type map that uses struct name as the event type name.
// Useful for tests and prototyping. Not recommended for production (struct renames break stored events).
func ReflectTypeMap() *TypeMap
```

Registration pattern for users — one function per bounded context:

```go
func RegisterBookingEvents(tm *codec.TypeMap) {
    codec.Register[RoomBooked](tm, "RoomBooked")
    codec.Register[BookingConfirmed](tm, "BookingConfirmed")
    codec.Register[BookingCancelled](tm, "BookingCancelled")
}
```

### 4. Codec — `core/codec/codec.go`, `core/codec/json.go`

```go
package codec

// Codec encodes/decodes events for storage.
type Codec interface {
    Encode(event any) (data []byte, eventType string, contentType string, err error)
    Decode(data []byte, eventType string) (event any, err error)
}

// JSON codec using TypeMap for type resolution.
type JSONCodec struct {
    types *TypeMap
    opts  []json.EncoderOption  // optional json options
}

func NewJSON(types *TypeMap) *JSONCodec
```

Encode uses `TypeMap.TypeName()` to get the event type name, then `json.Marshal`.
Decode uses `TypeMap.NewInstance()` to get an empty typed pointer, then `json.Unmarshal`, then dereferences the pointer to return a value (not pointer).

### 5. Store interfaces — `core/store/`

```go
package store

type EventReader interface {
    ReadEvents(ctx context.Context, stream StreamName, start uint64, count int) ([]StreamEvent, error)
    ReadEventsBackwards(ctx context.Context, stream StreamName, start uint64, count int) ([]StreamEvent, error)
}

type EventWriter interface {
    AppendEvents(ctx context.Context, stream StreamName, expected ExpectedVersion, events []NewStreamEvent) (AppendResult, error)
}

type EventStore interface {
    EventReader
    EventWriter
    StreamExists(ctx context.Context, stream StreamName) (bool, error)
    DeleteStream(ctx context.Context, stream StreamName, expected ExpectedVersion) error
    TruncateStream(ctx context.Context, stream StreamName, position uint64, expected ExpectedVersion) error
}
```

Value types:

```go
type StreamEvent struct {
    ID          uuid.UUID
    EventType   string
    Payload     any        // deserialized event
    Metadata    Metadata
    ContentType string
    Position    int64      // position within the stream
    GlobalPosition uint64  // position in the global log
    Created     time.Time
}

type NewStreamEvent struct {
    ID       uuid.UUID
    Payload  any
    Metadata Metadata
}

type AppendResult struct {
    GlobalPosition      uint64
    NextExpectedVersion int64
}
```

Package-level functions for loading state and aggregates:

```go
// LoadState loads and folds events into state. Used by the functional command service.
// Returns the folded state, the events, and the expected version for the next append.
func LoadState[S any](
    ctx context.Context,
    reader EventReader,
    codec codec.Codec,
    stream StreamName,
    fold func(S, any) S,
    zero S,
    expected ExpectedState,
) (state S, events []any, version ExpectedVersion, err error)

// LoadAggregate loads an aggregate from a stream. Used by the aggregate command service.
func LoadAggregate[S any](
    ctx context.Context,
    reader EventReader,
    codec codec.Codec,
    stream StreamName,
    fold func(S, any) S,
    zero S,
) (*aggregate.Aggregate[S], error)

// StoreAggregate appends the aggregate's pending changes to the stream.
func StoreAggregate[S any](
    ctx context.Context,
    writer EventWriter,
    codec codec.Codec,
    stream StreamName,
    agg *aggregate.Aggregate[S],
) (AppendResult, error)
```

Error handling:

- `ReadEvents` on a non-existent stream returns `ErrStreamNotFound`.
- `AppendEvents` with a wrong expected version returns `ErrOptimisticConcurrency`.
- Both errors are sentinel values suitable for `errors.Is()`.

### 6. Functional command service — `core/command/`

Primary command handling path. No aggregates involved.

```go
package command

// Service handles commands by loading state, executing a handler, and storing new events.
type Service[S any] struct {
    reader   store.EventReader
    writer   store.EventWriter
    codec    codec.Codec
    fold     func(S, any) S
    zero     S
    handlers map[reflect.Type]untypedHandler[S]
}

func New[S any](
    reader store.EventReader,
    writer store.EventWriter,
    codec codec.Codec,
    fold func(S, any) S,
    zero S,
) *Service[S]

// Handle dispatches a command to its registered handler.
func (svc *Service[S]) Handle(ctx context.Context, command any) (*Result[S], error)

// Result of a handled command.
type Result[S any] struct {
    State          S
    NewEvents      []any
    GlobalPosition uint64
    StreamVersion  int64
}
```

Handler registration:

```go
// Handler defines how to process a specific command type.
type Handler[C any, S any] struct {
    // Expected state: IsNew, IsExisting, or IsAny.
    Expected ExpectedState

    // Stream returns the stream name for this command.
    Stream func(C) StreamName

    // Act is a pure function: given current state and the command, return new events.
    Act func(ctx context.Context, state S, cmd C) ([]any, error)
}

// On registers a handler for command type C on the service.
func On[C any, S any](svc *Service[S], h Handler[C, S])
```

The `Handle` method internally:

1. Looks up the handler by `reflect.TypeOf(command)`.
2. Calls `handler.Stream(command)` to get the stream name.
3. Calls `store.LoadState(...)` with the handler's expected state.
4. Calls `handler.Act(ctx, state, command)` to get new events.
5. If no new events, returns current state (no-op).
6. Encodes events via codec and calls `store.AppendEvents(...)`.
7. Folds new events into state for the result.
8. Returns `Result[S]`.

Errors from `Act` are returned directly. `ErrOptimisticConcurrency` from the store is returned directly. No wrapping into a result type — Go convention is `(value, error)`.

### 7. Aggregate command service — `core/command/aggservice.go`

Optional layer for users who prefer the DDD aggregate pattern.

```go
// AggregateService handles commands using aggregates.
type AggregateService[S any] struct {
    reader   store.EventReader
    writer   store.EventWriter
    codec    codec.Codec
    fold     func(S, any) S
    zero     S
    handlers map[reflect.Type]untypedAggHandler[S]
}

func NewAggregateService[S any](
    reader store.EventReader,
    writer store.EventWriter,
    codec codec.Codec,
    fold func(S, any) S,
    zero S,
) *AggregateService[S]

func (svc *AggregateService[S]) Handle(ctx context.Context, command any) (*Result[S], error)

// AggregateHandler defines how to process a command using an aggregate.
type AggregateHandler[C any, S any] struct {
    Expected ExpectedState
    ID       func(C) string
    Act      func(ctx context.Context, agg *aggregate.Aggregate[S], cmd C) error
}

func OnAggregate[C any, S any](svc *AggregateService[S], h AggregateHandler[C, S])
```

Key difference from functional: `Act` receives `*aggregate.Aggregate[S]` and returns only `error`. New events are recorded inside the aggregate via `agg.Apply(...)`. The service reads `agg.Changes()` after `Act` returns.

Stream name is derived from `{TypeName}-{ID}` using the aggregate's state type name, not from a `Stream` function. The `ID` function extracts the entity ID from the command.

### 8. Subscription framework — `core/subscription/`

```go
package subscription

// Subscription consumes events from a source.
// Start blocks until ctx is cancelled or a fatal error occurs.
type Subscription interface {
    Start(ctx context.Context) error
}

// EventHandler processes a single event.
type EventHandler interface {
    HandleEvent(ctx context.Context, msg *ConsumeContext) error
}

// HandlerFunc adaptor — like http.HandlerFunc.
type HandlerFunc func(ctx context.Context, msg *ConsumeContext) error

func (f HandlerFunc) HandleEvent(ctx context.Context, msg *ConsumeContext) error {
    return f(ctx, msg)
}
```

Consume context:

```go
type ConsumeContext struct {
    EventID        uuid.UUID
    EventType      string
    Stream         StreamName
    Payload        any          // deserialized event, nil if unknown type
    Metadata       Metadata
    ContentType    string
    Position       uint64       // position in the source stream
    GlobalPosition uint64       // position in the global log ($all)
    Sequence       uint64       // local sequence number within this subscription
    Created        time.Time
    SubscriptionID string
}
```

Middleware pattern:

```go
// Middleware wraps a handler with additional behavior.
type Middleware func(EventHandler) EventHandler

// Chain applies middleware in order: first middleware is outermost.
func Chain(handler EventHandler, mw ...Middleware) EventHandler

// Built-in middleware:

// WithConcurrency processes events concurrently up to the given limit.
func WithConcurrency(limit int) Middleware

// WithPartitioning distributes events across N goroutines by partition key.
// keyFunc extracts the partition key; nil defaults to stream name.
func WithPartitioning(count int, keyFunc func(*ConsumeContext) string) Middleware

// WithLogging logs event processing.
func WithLogging(logger *slog.Logger) Middleware
```

Checkpoint management:

```go
type Checkpoint struct {
    ID       string
    Position *uint64   // nil means no checkpoint stored yet
}

type CheckpointStore interface {
    GetCheckpoint(ctx context.Context, id string) (Checkpoint, error)
    StoreCheckpoint(ctx context.Context, checkpoint Checkpoint) error
}

// CheckpointCommitter batches checkpoint writes with gap detection.
// Prevents committing position N+1 if position N hasn't been processed yet.
type CheckpointCommitter struct { /* internal */ }

func NewCheckpointCommitter(
    store CheckpointStore,
    subscriptionID string,
    batchSize int,           // commit after this many events (default: 100)
    interval time.Duration,  // commit at least this often (default: 5s)
) *CheckpointCommitter

func (c *CheckpointCommitter) Commit(ctx context.Context, position uint64, sequence uint64) error
func (c *CheckpointCommitter) Flush(ctx context.Context) error
func (c *CheckpointCommitter) Close() error
```

Gap detection: the committer tracks the highest contiguous sequence number. If events arrive out of order (from concurrent processing), it only commits up to the highest position where all prior positions are also processed. This prevents data loss on restart.

### 9. KurrentDB store — `kurrentdb/store.go`

Implements `store.EventStore` using the EventStoreDB/KurrentDB Go gRPC client.

```go
package kurrentdb

type Store struct {
    client *esdb.Client
    codec  codec.Codec
}

func NewStore(client *esdb.Client, codec codec.Codec) *Store

// Implements store.EventReader
func (s *Store) ReadEvents(ctx context.Context, stream StreamName, start uint64, count int) ([]StreamEvent, error)
func (s *Store) ReadEventsBackwards(ctx context.Context, stream StreamName, start uint64, count int) ([]StreamEvent, error)

// Implements store.EventWriter
func (s *Store) AppendEvents(ctx context.Context, stream StreamName, expected ExpectedVersion, events []NewStreamEvent) (AppendResult, error)

// Implements store.EventStore
func (s *Store) StreamExists(ctx context.Context, stream StreamName) (bool, error)
func (s *Store) DeleteStream(ctx context.Context, stream StreamName, expected ExpectedVersion) error
func (s *Store) TruncateStream(ctx context.Context, stream StreamName, position uint64, expected ExpectedVersion) error
```

Version mapping:
- `VersionNoStream` → `esdb.NoStream`
- `VersionAny` → `esdb.Any`
- Positive version → `esdb.StreamRevision(uint64(version))`

Error mapping:
- `WrongExpectedStreamRevision` → `ErrOptimisticConcurrency`
- `StreamNotFound` → `ErrStreamNotFound`

### 10. KurrentDB subscriptions — `kurrentdb/catchup.go`, `kurrentdb/persistent.go`

Functional options pattern:

```go
// Catch-up subscription — stream or $all based on options.
type CatchUp struct { /* internal */ }

func NewCatchUp(client *esdb.Client, codec codec.Codec, id string, opts ...CatchUpOption) *CatchUp

// Implements subscription.Subscription
func (s *CatchUp) Start(ctx context.Context) error

// Options
type CatchUpOption func(*catchUpConfig)

func FromStream(name StreamName) CatchUpOption       // subscribe to a specific stream
func FromAll() CatchUpOption                          // subscribe to $all
func WithCheckpointStore(cs CheckpointStore) CatchUpOption
func WithHandler(h EventHandler) CatchUpOption
func WithMiddleware(mw ...Middleware) CatchUpOption
func WithResolveLinkTos(resolve bool) CatchUpOption
func WithCredentials(user, pass string) CatchUpOption
func WithFilter(filter SubscriptionFilter) CatchUpOption  // server-side filter for $all
```

```go
// Persistent subscription — stream or $all based on options.
type Persistent struct { /* internal */ }

func NewPersistent(client *esdb.Client, codec codec.Codec, id string, opts ...PersistentOption) *Persistent

func (s *Persistent) Start(ctx context.Context) error

type PersistentOption func(*persistentConfig)

func PersistentFromStream(name StreamName) PersistentOption
func PersistentFromAll() PersistentOption
func PersistentWithHandler(h EventHandler) PersistentOption
func PersistentWithMiddleware(mw ...Middleware) PersistentOption
func PersistentWithBufferSize(size int) PersistentOption
func PersistentWithCredentials(user, pass string) PersistentOption
```

Lifecycle:
- `Start(ctx)` blocks. Cancelling `ctx` triggers graceful shutdown.
- Catch-up subscriptions auto-reconnect on transient errors with backoff.
- Persistent subscriptions auto-create the consumer group if it doesn't exist on first connect.
- Ack/Nack: returning `nil` from the handler = ack. Returning `error` = nack (persistent) or log + continue (catch-up).

### 11. OpenTelemetry — `otel/`

Thin module providing tracing and metrics integration.

```go
package otel

// CommandMiddleware wraps a command service with tracing and metrics.
func TraceCommands[S any](
    svc *command.Service[S],
    tracer trace.Tracer,
    meter metric.Meter,
) *command.Service[S]

// Same for aggregate service.
func TraceAggregateCommands[S any](
    svc *command.AggregateService[S],
    tracer trace.Tracer,
    meter metric.Meter,
) *command.AggregateService[S]

// Subscription middleware for tracing event handling.
func TracingMiddleware(tracer trace.Tracer) subscription.Middleware
```

Command tracing creates a span per `Handle()` call with command type as the span name. Subscription tracing creates a span per event with event type as the span name.

### 12. Conformance tests — `core/test/storetest/`

Exported test suite that any `store.EventStore` implementation can run against:

```go
package storetest

// RunAll runs the full conformance suite against the given store.
func RunAll(t *testing.T, store store.EventStore)

// Individual test groups (can run selectively):
func TestAppend(t *testing.T, store store.EventStore)
func TestRead(t *testing.T, store store.EventStore)
func TestReadBackwards(t *testing.T, store store.EventStore)
func TestOptimisticConcurrency(t *testing.T, store store.EventStore)
func TestStreamExists(t *testing.T, store store.EventStore)
func TestDeleteStream(t *testing.T, store store.EventStore)
```

This mirrors Eventuous C#'s `StoreAppendTests<T>`, `StoreReadTests<T>` pattern — write once, verify every implementation.

---

## Estimated size

| Component | Go LOC | Notes |
|-----------|--------|-------|
| Shared primitives | ~60 | StreamName, Metadata, errors, constants |
| Aggregate | ~80 | Aggregate[S] struct + methods |
| Codec + TypeMap | ~150 | JSON codec, bidirectional type registry |
| Store interfaces + functions | ~250 | Interfaces, LoadState, LoadAggregate, StoreAggregate |
| Functional command service | ~200 | Service[S], Handler[C,S], Handle pipeline |
| Aggregate command service | ~150 | AggregateService[S], AggregateHandler[C,S] |
| Subscription framework | ~600 | Interfaces, middleware, checkpoint, committer |
| KurrentDB store | ~300 | EventStore impl, version/error mapping |
| KurrentDB subscriptions | ~400 | Catch-up + persistent, options, reconnection |
| OTel module | ~150 | Command wrapper, subscription middleware |
| **Total source** | **~2,350** | |
| Conformance tests | ~400 | Store test suite |
| KurrentDB integration tests | ~800 | Store + subscription tests |
| Unit tests | ~1,000 | Domain, command service, codec, checkpoint |
| **Total tests** | **~2,200** | |
| **Grand total** | **~4,550** | |

---

## End-to-end usage example

```go
package main

import (
    "context"
    "log"
    "log/slog"

    "github.com/EventStore/EventStore-Client-Go/v4/esdb"
    "github.com/eventuous/eventuous-go/core"
    "github.com/eventuous/eventuous-go/core/codec"
    "github.com/eventuous/eventuous-go/core/command"
    "github.com/eventuous/eventuous-go/core/subscription"
    "github.com/eventuous/eventuous-go/kurrentdb"
)

// --- Events ---

type RoomBooked struct {
    BookingID string
    RoomID    string
}

type BookingCancelled struct {
    BookingID string
    Reason    string
}

// --- State ---

type BookingState struct {
    ID     string
    RoomID string
    Active bool
}

func bookingFold(state BookingState, event any) BookingState {
    switch e := event.(type) {
    case RoomBooked:
        return BookingState{ID: e.BookingID, RoomID: e.RoomID, Active: true}
    case BookingCancelled:
        state.Active = false
        return state
    default:
        return state
    }
}

// --- Commands ---

type BookRoom struct {
    BookingID string
    RoomID    string
}

type CancelBooking struct {
    BookingID string
    Reason    string
}

// --- Setup ---

func main() {
    ctx := context.Background()

    // Type registration
    types := codec.NewTypeMap()
    codec.Register[RoomBooked](types, "RoomBooked")
    codec.Register[BookingCancelled](types, "BookingCancelled")
    jsonCodec := codec.NewJSON(types)

    // KurrentDB client
    settings, _ := esdb.ParseConnectionString("esdb://localhost:2113?tls=false")
    client, _ := esdb.NewClient(settings)

    // Event store
    store := kurrentdb.NewStore(client, jsonCodec)

    // Command service (functional)
    svc := command.New(store, store, jsonCodec, bookingFold, BookingState{})

    command.On(svc, command.Handler[BookRoom, BookingState]{
        Expected: eventuous.IsNew,
        Stream:   func(cmd BookRoom) eventuous.StreamName { return eventuous.NewStreamName("Booking", cmd.BookingID) },
        Act: func(ctx context.Context, state BookingState, cmd BookRoom) ([]any, error) {
            return []any{RoomBooked{BookingID: cmd.BookingID, RoomID: cmd.RoomID}}, nil
        },
    })

    command.On(svc, command.Handler[CancelBooking, BookingState]{
        Expected: eventuous.IsExisting,
        Stream:   func(cmd CancelBooking) eventuous.StreamName { return eventuous.NewStreamName("Booking", cmd.BookingID) },
        Act: func(ctx context.Context, state BookingState, cmd CancelBooking) ([]any, error) {
            if !state.Active {
                return nil, nil // already cancelled, no-op
            }
            return []any{BookingCancelled{BookingID: cmd.BookingID, Reason: cmd.Reason}}, nil
        },
    })

    // Handle a command
    result, err := svc.Handle(ctx, BookRoom{BookingID: "booking-123", RoomID: "room-42"})
    if err != nil {
        log.Fatal(err)
    }
    slog.Info("booked", "state", result.State, "version", result.StreamVersion)

    // Subscription
    sub := kurrentdb.NewCatchUp(client, jsonCodec, "BookingProjections",
        kurrentdb.FromAll(),
        kurrentdb.WithHandler(subscription.HandlerFunc(func(ctx context.Context, msg *subscription.ConsumeContext) error {
            slog.Info("event received", "type", msg.EventType, "stream", msg.Stream)
            return nil
        })),
        kurrentdb.WithMiddleware(
            subscription.WithLogging(slog.Default()),
        ),
    )

    // Start subscription (blocks until context cancelled)
    if err := sub.Start(ctx); err != nil {
        log.Fatal(err)
    }
}
```
