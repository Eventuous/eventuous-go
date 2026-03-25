# Booking Sample Application — Design Spec

A single-service sample application demonstrating Eventuous Go's core capabilities: functional command service, KurrentDB event store, catch-up subscription, and in-memory read model projections.

## Goals

- Show users how to build a complete event-sourced application with Eventuous Go
- Demonstrate the full write → store → subscribe → project cycle
- Use only the functional command service (the primary path)
- Keep it simple enough to read top-to-bottom, rich enough to be useful as a reference

## Module & File Layout

```
samples/booking/
├── go.mod                  # github.com/eventuous/eventuous-go/samples/booking
├── main.go                 # Wiring: client, codec, service, read model, subscription, HTTP
├── domain/
│   ├── events.go           # Event types + codec registration
│   ├── state.go            # BookingState + BookingFold
│   └── commands.go         # Command types + handler functions
├── readmodel/
│   └── readmodel.go        # In-memory projections + query methods
└── httpapi/
    └── api.go              # Route registration, handlers, error mapping
```

The module depends on `core` and `kurrentdb`. It is a standalone `go.mod` following the existing multi-module pattern.

## Domain Model

This sample defines its own domain types in `domain/`. It does not import `core/test/testdomain`, which is reserved for framework conformance tests.

### Events

```go
type RoomBooked struct {
    BookingID string  `json:"bookingId"`
    GuestID   string  `json:"guestId"`
    RoomID    string  `json:"roomId"`
    CheckIn   string  `json:"checkIn"`
    CheckOut  string  `json:"checkOut"`
    Price     float64 `json:"price"`
    Currency  string  `json:"currency"`
}

type PaymentRecorded struct {
    BookingID string  `json:"bookingId"`
    Amount    float64 `json:"amount"`
    Currency  string  `json:"currency"`
    PaymentID string  `json:"paymentId"`
}

type BookingCancelled struct {
    BookingID string `json:"bookingId"`
    Reason    string `json:"reason"`
}
```

All events registered in a `codec.TypeMap` via a `NewTypeMap()` function in `events.go`. A `NewCodec()` helper constructs the full `codec.Codec` (`codec.NewJSON(NewTypeMap())`), following the same pattern as `testdomain.NewCodec()`.

### State

```go
type BookingState struct {
    ID          string
    GuestID     string
    RoomID      string
    CheckIn     string
    CheckOut    string
    Price       float64
    Outstanding float64
    Currency    string
    Paid        bool
    Cancelled   bool
}
```

`BookingFold(state BookingState, event any) BookingState` — type switch over events:
- `RoomBooked`: initializes all fields, `Outstanding = Price`
- `PaymentRecorded`: decrements outstanding, sets `Paid = Outstanding <= 0`
- `BookingCancelled`: sets `Cancelled = true`

### Commands

```go
type BookRoom struct {
    BookingID string
    GuestID   string
    RoomID    string
    CheckIn   string
    CheckOut  string
    Price     float64
    Currency  string
}

type RecordPayment struct {
    BookingID string
    Amount    float64
    Currency  string
    PaymentID string
}

type CancelBooking struct {
    BookingID string
    Reason    string
}
```

## Command Service

Single `command.Service[BookingState]` with three registered handlers:

### BookRoom (ExpectedState: IsNew)
- Stream: `Booking-{BookingID}`
- Emits: `RoomBooked` with `Outstanding = Price`

### RecordPayment (ExpectedState: IsExisting)
- Stream: `Booking-{BookingID}`
- Validates: booking not cancelled, not already fully paid
- Emits: `PaymentRecorded`

### CancelBooking (ExpectedState: IsExisting)
- Stream: `Booking-{BookingID}`
- Validates: not already cancelled
- Emits: `BookingCancelled`

All handlers are pure functions: `func(ctx, BookingState, Command) ([]any, error)`.

The HTTP handlers for `RecordPayment` (and others) read updated fields from `result.State` — the write-side state after fold — not from the read model. This avoids eventual consistency issues in the response.

## Read Model

An in-memory `BookingReadModel` struct with `sync.RWMutex` and two projections:

### BookingDetails — `map[string]BookingDocument`

Keyed by booking ID. Full booking view:

```go
type BookingDocument struct {
    BookingID   string  `json:"bookingId"`
    GuestID     string  `json:"guestId"`
    RoomID      string  `json:"roomId"`
    CheckIn     string  `json:"checkIn"`
    CheckOut    string  `json:"checkOut"`
    Price       float64 `json:"price"`
    Outstanding float64 `json:"outstanding"`
    Currency    string  `json:"currency"`
    Paid        bool    `json:"paid"`
    Cancelled   bool    `json:"cancelled"`
}
```

- `RoomBooked` → insert document
- `PaymentRecorded` → update outstanding, set paid if `<= 0`
- `BookingCancelled` → set cancelled flag

Query: `GetBooking(id string) (BookingDocument, bool)`

### MyBookings — `map[string][]BookingSummary`

Keyed by guest ID. List of a guest's bookings:

```go
type BookingSummary struct {
    BookingID string  `json:"bookingId"`
    RoomID    string  `json:"roomId"`
    CheckIn   string  `json:"checkIn"`
    CheckOut  string  `json:"checkOut"`
    Price     float64 `json:"price"`
    Currency  string  `json:"currency"`
    Cancelled bool    `json:"cancelled"`
}
```

- `RoomBooked` → append summary to guest's list
- `BookingCancelled` → looks up `BookingDetails` to find the guest ID, then marks the matching entry as cancelled in the guest's list

Query: `GetGuestBookings(guestID string) []BookingSummary`

### Subscription

KurrentDB catch-up subscription on `$all`. The read model implements `subscription.EventHandler`. The `HandleEvent` implementation uses a type switch on `msg.Payload` and silently ignores `nil` payloads and unknown event types (the `default` case returns `nil`).

Uses `subscription.WithLogging(slog.Default())` middleware, passed via `kurrentdb.WithMiddleware(...)`. No server-side filter is applied — the read model ignores events it doesn't recognize. For production, consider `kurrentdb.WithFilter()` to reduce traffic.

Starts on app boot and tails from the beginning.

## HTTP API

Standard library `net/http` with Go 1.22+ route patterns.

### Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/bookings` | Book a room. Generates UUID for booking ID. Returns `201` with `{bookingId, streamVersion}` |
| `POST` | `/bookings/{id}/payments` | Record a payment. Returns `200` with `{bookingId, outstanding, paid}` |
| `POST` | `/bookings/{id}/cancel` | Cancel a booking. Returns `200` |
| `GET`  | `/bookings/{id}` | Get booking details from read model. Returns `200` or `404` |
| `GET`  | `/guests/{id}/bookings` | Get guest's bookings from read model. Returns `200` (empty array if none) |

### Error Mapping

| Error | HTTP Status |
|-------|-------------|
| `ErrStreamNotFound` | `404` |
| `ErrOptimisticConcurrency` | `409` |
| `ErrHandlerNotFound` | `400` |
| Validation errors | `422` |
| Everything else | `500` |

A small helper function, not a framework.

### Request/Response Examples

**Book a room:**
```
POST /bookings
{"guestId": "guest-1", "roomId": "room-42", "checkIn": "2026-04-01", "checkOut": "2026-04-05", "price": 500, "currency": "USD"}

201 Created
{"bookingId": "generated-uuid", "streamVersion": 0}
```

**Record payment:**
```
POST /bookings/{id}/payments
{"amount": 200, "currency": "USD", "paymentId": "pay-1"}

200 OK
{"bookingId": "...", "outstanding": 300, "paid": false}
```

## App Wiring (main.go)

1. Read `KURRENTDB_URL` env var (default: `esdb://localhost:2113?tls=false`)
2. Create KurrentDB client
3. Create codec via `domain.NewCodec()`
4. Create KurrentDB store — the same store instance is passed as both reader and writer to `command.New`
5. Create `command.Service[BookingState]`, register all three handlers
6. Create `BookingReadModel`
7. Use `signal.NotifyContext` for graceful shutdown (SIGINT/SIGTERM)
8. Start KurrentDB catch-up subscription in a goroutine with the cancellable context, read model as handler + `WithMiddleware(subscription.WithLogging(slog.Default()))`
9. Register HTTP routes
10. Start `net/http` server on `:8080` — on context cancellation, call `Server.Shutdown` for graceful drain

No config files, no DI container. Explicit wiring.

## What This Does NOT Include

- Aggregate-based command service (optional layer, not the primary path)
- MongoDB or any external read model store
- Multi-service setup (gateway, integration events)
- OpenTelemetry instrumentation (exists in `otel/` module but kept out of the sample for clarity)
- Authentication, middleware, or production concerns
