# Eventuous for Go

[![CI](https://github.com/Eventuous/eventuous-go/actions/workflows/ci.yml/badge.svg)](https://github.com/Eventuous/eventuous-go/actions/workflows/ci.yml)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)

Production-grade Event Sourcing library for Go, ported from [Eventuous](https://github.com/Eventuous/eventuous) (.NET).

- **Functional-first** — pure functions over OOP, type switch fold over handler registration
- **Idiomatic Go** — composition over inheritance, middleware chains, `context.Context` + errors
- **Multi-module** — import only what you need, no transitive dependency bloat

## Modules

| Module | Import | Description |
|--------|--------|-------------|
| **core** | `github.com/eventuous/eventuous-go/core` | Domain, persistence, command services, subscriptions — near-zero external deps |
| **kurrentdb** | `github.com/eventuous/eventuous-go/kurrentdb` | [KurrentDB](https://kurrent.io)/EventStoreDB store and subscriptions |
| **otel** | `github.com/eventuous/eventuous-go/otel` | OpenTelemetry tracing and metrics |

## Quick start

```bash
go get github.com/eventuous/eventuous-go/core
go get github.com/eventuous/eventuous-go/kurrentdb
```

### Define events and state

```go
type RoomBooked struct {
    BookingID string `json:"bookingId"`
    RoomID    string `json:"roomId"`
}

type BookingCancelled struct {
    BookingID string `json:"bookingId"`
    Reason    string `json:"reason"`
}

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
```

### Register event types

```go
types := codec.NewTypeMap()
codec.Register[RoomBooked](types, "RoomBooked")
codec.Register[BookingCancelled](types, "BookingCancelled")
jsonCodec := codec.NewJSON(types)
```

### Create a command service

```go
// Connect to KurrentDB
settings, _ := kurrentdb.ParseConnectionString("kurrentdb://localhost:2113?tls=false")
client, _ := kurrentdb.NewClient(settings)
store := kdb.NewStore(client, jsonCodec)

// Functional command service — state in, events out
svc := command.New[BookingState](store, store, bookingFold, BookingState{})

command.On(svc, command.Handler[BookRoom, BookingState]{
    Expected: eventuous.IsNew,
    Stream:   func(cmd BookRoom) eventuous.StreamName { return eventuous.NewStreamName("Booking", cmd.BookingID) },
    Act: func(ctx context.Context, state BookingState, cmd BookRoom) ([]any, error) {
        return []any{RoomBooked{BookingID: cmd.BookingID, RoomID: cmd.RoomID}}, nil
    },
})

// Handle a command
result, err := svc.Handle(ctx, BookRoom{BookingID: "123", RoomID: "room-42"})
```

### Subscribe to events

```go
sub := kdb.NewCatchUp(client, jsonCodec, "MyProjection",
    kdb.FromAll(),
    kdb.WithHandler(subscription.HandlerFunc(func(ctx context.Context, msg *subscription.ConsumeContext) error {
        fmt.Printf("Event: %s on stream %s\n", msg.EventType, msg.Stream)
        return nil
    })),
)

// Blocks until context is cancelled
sub.Start(ctx)
```

### Aggregate style (optional)

For teams that prefer the DDD aggregate pattern:

```go
func BookRoom(agg *aggregate.Aggregate[BookingState], roomID string) error {
    if err := agg.EnsureNew(); err != nil {
        return err
    }
    agg.Apply(RoomBooked{RoomID: roomID})
    return nil
}

svc := command.NewAggregateService[BookingState](store, store, bookingFold, BookingState{})

command.OnAggregate(svc, command.AggregateHandler[BookRoomCmd, BookingState]{
    Expected: eventuous.IsNew,
    ID:       func(cmd BookRoomCmd) string { return cmd.BookingID },
    Act: func(ctx context.Context, agg *aggregate.Aggregate[BookingState], cmd BookRoomCmd) error {
        return BookRoom(agg, cmd.RoomID)
    },
})
```

## Architecture

```
core/
├── eventuous.go          StreamName, Metadata, errors
├── aggregate/            Aggregate[S] — domain building block
├── codec/                TypeMap + JSON codec
├── store/                EventReader/Writer/Store interfaces, LoadState, LoadAggregate
├── command/              Functional Service[S] + AggregateService[S]
└── subscription/         EventHandler, middleware, checkpoint committer

kurrentdb/                KurrentDB store + catch-up & persistent subscriptions

otel/                     OpenTelemetry tracing + metrics
```

## Development

```bash
# Build all modules
(cd core && go build ./...)
(cd kurrentdb && go build ./...)
(cd otel && go build ./...)

# Test core (no infrastructure needed)
(cd core && go test ./...)

# Test kurrentdb (uses testcontainers — Docker required)
(cd kurrentdb && go test -timeout 300s ./...)

# Test otel
(cd otel && go test ./...)
```

## Community

- [Eventuous Slack](https://join.slack.com/t/eventuousworkspace/shared_invite/zt-tzrhtbxf-Tk7dSMuoVBvjkSf0Eq~Zpg)
- [DDD-CQRS-ES Discord](https://discord.gg/P4WcBp8r)
- [Eventuous documentation](https://eventuous.dev)

## License

Apache License 2.0 — see [LICENSE](LICENSE).
