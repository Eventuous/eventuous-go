# Booking Sample Application — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a single-service booking sample demonstrating Eventuous Go's functional command service, KurrentDB event store, catch-up subscription, and in-memory read model projections.

**Architecture:** A standalone Go module (`samples/booking/`) with four packages: `domain` (events, state, commands), `readmodel` (in-memory projections), `httpapi` (REST endpoints), and `main.go` (wiring). The command service writes to KurrentDB, a catch-up subscription projects events into two in-memory read models, and HTTP endpoints expose both commands and queries.

**Tech Stack:** Go 1.25+, `net/http` (stdlib), Eventuous Go core + kurrentdb modules, KurrentDB via testcontainers for tests.

**Spec:** `docs/superpowers/specs/2026-03-25-booking-sample-design.md`

---

## File Structure

```
samples/booking/
├── go.mod                      # Module definition + replace directives
├── go.sum                      # (generated)
├── main.go                     # App wiring: client, codec, service, read model, subscription, HTTP server
├── domain/
│   ├── events.go               # Event types (RoomBooked, PaymentRecorded, BookingCancelled) + NewTypeMap + NewCodec
│   ├── state.go                # BookingState + BookingFold
│   ├── state_test.go           # Table-driven fold tests
│   ├── commands.go             # Command types (BookRoom, RecordPayment, CancelBooking)
│   ├── handlers.go             # Command handler functions + BookingStream helper + validation errors
│   └── handlers_test.go        # Handler unit tests using memstore
├── readmodel/
│   ├── readmodel.go            # BookingReadModel with BookingDetails + MyBookings projections
│   └── readmodel_test.go       # Projection logic unit tests
├── httpapi/
│   ├── api.go                  # Route registration, HTTP handlers, JSON decode/encode, error mapping
│   └── api_test.go             # HTTP handler tests using httptest + memstore
└── integration_test.go         # End-to-end test with KurrentDB testcontainer (optional, Task 7)
```

---

### Task 1: Module scaffold and domain events

**Files:**
- Create: `samples/booking/go.mod`
- Create: `samples/booking/domain/events.go`

- [ ] **Step 1: Create go.mod**

```
samples/booking/go.mod
```

```
module github.com/eventuous/eventuous-go/samples/booking

go 1.25

replace (
	github.com/eventuous/eventuous-go/core => ../../core
	github.com/eventuous/eventuous-go/kurrentdb => ../../kurrentdb
)

require (
	github.com/eventuous/eventuous-go/core v0.0.0
	github.com/eventuous/eventuous-go/kurrentdb v0.0.0
)
```

Run: `cd samples/booking && go mod tidy`

- [ ] **Step 2: Create domain/events.go**

```go
// Copyright (C) Eventuous HQ OÜ. All rights reserved
// Licensed under the Apache License, Version 2.0.

package domain

import "github.com/eventuous/eventuous-go/core/codec"

// Events

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

// NewTypeMap creates a TypeMap with all booking events registered.
func NewTypeMap() *codec.TypeMap {
	tm := codec.NewTypeMap()
	mustRegister[RoomBooked](tm, "RoomBooked")
	mustRegister[PaymentRecorded](tm, "PaymentRecorded")
	mustRegister[BookingCancelled](tm, "BookingCancelled")
	return tm
}

// NewCodec creates a JSON codec with all booking events registered.
func NewCodec() codec.Codec {
	return codec.NewJSON(NewTypeMap())
}

func mustRegister[E any](tm *codec.TypeMap, name string) {
	if err := codec.Register[E](tm, name); err != nil {
		panic(err)
	}
}
```

- [ ] **Step 3: Verify it compiles**

Run: `cd samples/booking && go build ./domain/...`
Expected: clean build, no errors.

- [ ] **Step 4: Commit**

```bash
git add samples/booking/go.mod samples/booking/go.sum samples/booking/domain/events.go
git commit -m "feat(samples): scaffold booking module with domain events and codec"
```

---

### Task 2: Booking state and fold

**Files:**
- Create: `samples/booking/domain/state.go`
- Create: `samples/booking/domain/state_test.go`

- [ ] **Step 1: Write state_test.go with table-driven fold tests**

```go
// Copyright (C) Eventuous HQ OÜ. All rights reserved
// Licensed under the Apache License, Version 2.0.

package domain_test

import (
	"testing"

	"github.com/eventuous/eventuous-go/samples/booking/domain"
)

func TestBookingFold(t *testing.T) {
	tests := []struct {
		name   string
		state  domain.BookingState
		event  any
		expect domain.BookingState
	}{
		{
			name:  "RoomBooked initializes state",
			state: domain.BookingState{},
			event: domain.RoomBooked{
				BookingID: "b1", GuestID: "g1", RoomID: "r1",
				CheckIn: "2026-04-01", CheckOut: "2026-04-05",
				Price: 500, Currency: "USD",
			},
			expect: domain.BookingState{
				ID: "b1", GuestID: "g1", RoomID: "r1",
				CheckIn: "2026-04-01", CheckOut: "2026-04-05",
				Price: 500, Outstanding: 500, Currency: "USD",
			},
		},
		{
			name: "PaymentRecorded decrements outstanding",
			state: domain.BookingState{
				ID: "b1", Price: 500, Outstanding: 500, Currency: "USD",
			},
			event: domain.PaymentRecorded{BookingID: "b1", Amount: 200, Currency: "USD", PaymentID: "p1"},
			expect: domain.BookingState{
				ID: "b1", Price: 500, Outstanding: 300, Currency: "USD",
			},
		},
		{
			name: "PaymentRecorded sets Paid when outstanding reaches zero",
			state: domain.BookingState{
				ID: "b1", Price: 500, Outstanding: 200, Currency: "USD",
			},
			event: domain.PaymentRecorded{BookingID: "b1", Amount: 200, Currency: "USD", PaymentID: "p2"},
			expect: domain.BookingState{
				ID: "b1", Price: 500, Outstanding: 0, Currency: "USD", Paid: true,
			},
		},
		{
			name: "BookingCancelled sets Cancelled",
			state: domain.BookingState{
				ID: "b1", Price: 500, Outstanding: 500, Currency: "USD",
			},
			event:  domain.BookingCancelled{BookingID: "b1", Reason: "changed plans"},
			expect: domain.BookingState{
				ID: "b1", Price: 500, Outstanding: 500, Currency: "USD", Cancelled: true,
			},
		},
		{
			name:   "unknown event is ignored",
			state:  domain.BookingState{ID: "b1"},
			event:  "unknown",
			expect: domain.BookingState{ID: "b1"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := domain.BookingFold(tt.state, tt.event)
			if got != tt.expect {
				t.Errorf("got %+v, want %+v", got, tt.expect)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd samples/booking && go test ./domain/... -run TestBookingFold -v`
Expected: FAIL — `BookingState` and `BookingFold` not defined.

- [ ] **Step 3: Write state.go**

```go
// Copyright (C) Eventuous HQ OÜ. All rights reserved
// Licensed under the Apache License, Version 2.0.

package domain

// BookingState is the write-side state reconstructed by folding events.
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

// BookingFold is the fold function used by the command service to reconstruct state.
func BookingFold(state BookingState, event any) BookingState {
	switch e := event.(type) {
	case RoomBooked:
		return BookingState{
			ID:          e.BookingID,
			GuestID:     e.GuestID,
			RoomID:      e.RoomID,
			CheckIn:     e.CheckIn,
			CheckOut:    e.CheckOut,
			Price:       e.Price,
			Outstanding: e.Price,
			Currency:    e.Currency,
		}
	case PaymentRecorded:
		state.Outstanding -= e.Amount
		if state.Outstanding <= 0 {
			state.Paid = true
		}
		return state
	case BookingCancelled:
		state.Cancelled = true
		return state
	default:
		return state
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd samples/booking && go test ./domain/... -run TestBookingFold -v`
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add samples/booking/domain/state.go samples/booking/domain/state_test.go
git commit -m "feat(samples): add BookingState and fold function with tests"
```

---

### Task 3: Commands and handler functions

**Files:**
- Create: `samples/booking/domain/commands.go`
- Create: `samples/booking/domain/handlers.go`
- Create: `samples/booking/domain/handlers_test.go`

- [ ] **Step 1: Create commands.go (types needed by tests)**

```go
// Copyright (C) Eventuous HQ OÜ. All rights reserved
// Licensed under the Apache License, Version 2.0.

package domain

// BookRoom creates a new booking.
type BookRoom struct {
	BookingID string
	GuestID   string
	RoomID    string
	CheckIn   string
	CheckOut  string
	Price     float64
	Currency  string
}

// RecordPayment records a payment against an existing booking.
type RecordPayment struct {
	BookingID string
	Amount    float64
	Currency  string
	PaymentID string
}

// CancelBooking cancels an existing booking.
type CancelBooking struct {
	BookingID string
	Reason    string
}
```

- [ ] **Step 2: Write handlers_test.go**

```go
// Copyright (C) Eventuous HQ OÜ. All rights reserved
// Licensed under the Apache License, Version 2.0.

package domain_test

import (
	"context"
	"errors"
	"testing"

	"github.com/eventuous/eventuous-go/samples/booking/domain"
)

func TestHandleBookRoom(t *testing.T) {
	cmd := domain.BookRoom{
		BookingID: "b1", GuestID: "g1", RoomID: "r1",
		CheckIn: "2026-04-01", CheckOut: "2026-04-05",
		Price: 500, Currency: "USD",
	}
	events, err := domain.HandleBookRoom(context.Background(), domain.BookingState{}, cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	e, ok := events[0].(domain.RoomBooked)
	if !ok {
		t.Fatalf("expected RoomBooked, got %T", events[0])
	}
	if e.BookingID != "b1" || e.Price != 500 {
		t.Errorf("unexpected event: %+v", e)
	}
}

func TestHandleRecordPayment(t *testing.T) {
	state := domain.BookingState{ID: "b1", Price: 500, Outstanding: 500, Currency: "USD"}
	cmd := domain.RecordPayment{BookingID: "b1", Amount: 200, Currency: "USD", PaymentID: "p1"}

	events, err := domain.HandleRecordPayment(context.Background(), state, cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	e, ok := events[0].(domain.PaymentRecorded)
	if !ok {
		t.Fatalf("expected PaymentRecorded, got %T", events[0])
	}
	if e.Amount != 200 {
		t.Errorf("expected amount 200, got %f", e.Amount)
	}
}

func TestHandleRecordPayment_RejectsCancelledBooking(t *testing.T) {
	state := domain.BookingState{Cancelled: true}
	cmd := domain.RecordPayment{BookingID: "b1", Amount: 200, Currency: "USD", PaymentID: "p1"}

	_, err := domain.HandleRecordPayment(context.Background(), state, cmd)
	if !errors.Is(err, domain.ErrBookingCancelled) {
		t.Errorf("expected ErrBookingCancelled, got %v", err)
	}
}

func TestHandleRecordPayment_RejectsPaidBooking(t *testing.T) {
	state := domain.BookingState{Paid: true}
	cmd := domain.RecordPayment{BookingID: "b1", Amount: 200, Currency: "USD", PaymentID: "p1"}

	_, err := domain.HandleRecordPayment(context.Background(), state, cmd)
	if !errors.Is(err, domain.ErrBookingAlreadyPaid) {
		t.Errorf("expected ErrBookingAlreadyPaid, got %v", err)
	}
}

func TestHandleCancelBooking(t *testing.T) {
	state := domain.BookingState{ID: "b1", Price: 500, Outstanding: 500}
	cmd := domain.CancelBooking{BookingID: "b1", Reason: "changed plans"}

	events, err := domain.HandleCancelBooking(context.Background(), state, cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if _, ok := events[0].(domain.BookingCancelled); !ok {
		t.Fatalf("expected BookingCancelled, got %T", events[0])
	}
}

func TestHandleCancelBooking_RejectsAlreadyCancelled(t *testing.T) {
	state := domain.BookingState{Cancelled: true}
	cmd := domain.CancelBooking{BookingID: "b1", Reason: "again"}

	_, err := domain.HandleCancelBooking(context.Background(), state, cmd)
	if !errors.Is(err, domain.ErrAlreadyCancelled) {
		t.Errorf("expected ErrAlreadyCancelled, got %v", err)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `cd samples/booking && go test ./domain/... -run TestHandle -v`
Expected: FAIL — `HandleBookRoom`, `HandleRecordPayment`, `HandleCancelBooking` not defined.

- [ ] **Step 4: Write handlers.go**

```go
// Copyright (C) Eventuous HQ OÜ. All rights reserved
// Licensed under the Apache License, Version 2.0.

package domain

import (
	"context"
	"errors"

	eventuous "github.com/eventuous/eventuous-go/core"
)

// Validation errors returned by command handlers.
var (
	ErrBookingCancelled   = errors.New("booking is cancelled")
	ErrBookingAlreadyPaid = errors.New("booking is already paid")
	ErrAlreadyCancelled   = errors.New("booking is already cancelled")
)

// BookingStream returns the stream name for a booking.
func BookingStream(id string) eventuous.StreamName {
	return eventuous.NewStreamName("Booking", id)
}

// HandleBookRoom handles the BookRoom command.
func HandleBookRoom(_ context.Context, _ BookingState, cmd BookRoom) ([]any, error) {
	return []any{
		RoomBooked{
			BookingID: cmd.BookingID,
			GuestID:   cmd.GuestID,
			RoomID:    cmd.RoomID,
			CheckIn:   cmd.CheckIn,
			CheckOut:  cmd.CheckOut,
			Price:     cmd.Price,
			Currency:  cmd.Currency,
		},
	}, nil
}

// HandleRecordPayment handles the RecordPayment command.
func HandleRecordPayment(_ context.Context, state BookingState, cmd RecordPayment) ([]any, error) {
	if state.Cancelled {
		return nil, ErrBookingCancelled
	}
	if state.Paid {
		return nil, ErrBookingAlreadyPaid
	}
	return []any{
		PaymentRecorded{
			BookingID: cmd.BookingID,
			Amount:    cmd.Amount,
			Currency:  cmd.Currency,
			PaymentID: cmd.PaymentID,
		},
	}, nil
}

// HandleCancelBooking handles the CancelBooking command.
func HandleCancelBooking(_ context.Context, state BookingState, cmd CancelBooking) ([]any, error) {
	if state.Cancelled {
		return nil, ErrAlreadyCancelled
	}
	return []any{
		BookingCancelled{
			BookingID: cmd.BookingID,
			Reason:    cmd.Reason,
		},
	}, nil
}
```

- [ ] **Step 5: Run tests**

Run: `cd samples/booking && go test ./domain/... -v`
Expected: all PASS.

- [ ] **Step 6: Commit**

```bash
git add samples/booking/domain/commands.go samples/booking/domain/handlers.go samples/booking/domain/handlers_test.go
git commit -m "feat(samples): add booking commands and handler functions with tests"
```

---

### Task 4: In-memory read model

**Files:**
- Create: `samples/booking/readmodel/readmodel.go`
- Create: `samples/booking/readmodel/readmodel_test.go`

- [ ] **Step 1: Write readmodel_test.go**

```go
// Copyright (C) Eventuous HQ OÜ. All rights reserved
// Licensed under the Apache License, Version 2.0.

package readmodel_test

import (
	"context"
	"testing"

	"github.com/eventuous/eventuous-go/core/subscription"
	"github.com/eventuous/eventuous-go/samples/booking/domain"
	"github.com/eventuous/eventuous-go/samples/booking/readmodel"
)

func msg(payload any) *subscription.ConsumeContext {
	return &subscription.ConsumeContext{
		Payload: payload,
	}
}

func TestBookingDetails_RoomBooked(t *testing.T) {
	rm := readmodel.New()
	err := rm.HandleEvent(context.Background(), msg(domain.RoomBooked{
		BookingID: "b1", GuestID: "g1", RoomID: "r1",
		CheckIn: "2026-04-01", CheckOut: "2026-04-05",
		Price: 500, Currency: "USD",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	doc, ok := rm.GetBooking("b1")
	if !ok {
		t.Fatal("expected booking to exist")
	}
	if doc.GuestID != "g1" || doc.Price != 500 || doc.Outstanding != 500 {
		t.Errorf("unexpected doc: %+v", doc)
	}
}

func TestBookingDetails_PaymentUpdatesOutstanding(t *testing.T) {
	rm := readmodel.New()
	_ = rm.HandleEvent(context.Background(), msg(domain.RoomBooked{
		BookingID: "b1", GuestID: "g1", RoomID: "r1",
		CheckIn: "2026-04-01", CheckOut: "2026-04-05",
		Price: 500, Currency: "USD",
	}))

	_ = rm.HandleEvent(context.Background(), msg(domain.PaymentRecorded{
		BookingID: "b1", Amount: 200, Currency: "USD", PaymentID: "p1",
	}))

	doc, _ := rm.GetBooking("b1")
	if doc.Outstanding != 300 || doc.Paid {
		t.Errorf("expected outstanding=300 paid=false, got outstanding=%f paid=%v", doc.Outstanding, doc.Paid)
	}
}

func TestBookingDetails_FullPaymentSetsPaid(t *testing.T) {
	rm := readmodel.New()
	_ = rm.HandleEvent(context.Background(), msg(domain.RoomBooked{
		BookingID: "b1", GuestID: "g1", RoomID: "r1",
		CheckIn: "2026-04-01", CheckOut: "2026-04-05",
		Price: 500, Currency: "USD",
	}))

	_ = rm.HandleEvent(context.Background(), msg(domain.PaymentRecorded{
		BookingID: "b1", Amount: 500, Currency: "USD", PaymentID: "p1",
	}))

	doc, _ := rm.GetBooking("b1")
	if !doc.Paid {
		t.Error("expected booking to be marked as paid")
	}
}

func TestBookingDetails_Cancelled(t *testing.T) {
	rm := readmodel.New()
	_ = rm.HandleEvent(context.Background(), msg(domain.RoomBooked{
		BookingID: "b1", GuestID: "g1", RoomID: "r1",
		CheckIn: "2026-04-01", CheckOut: "2026-04-05",
		Price: 500, Currency: "USD",
	}))

	_ = rm.HandleEvent(context.Background(), msg(domain.BookingCancelled{
		BookingID: "b1", Reason: "changed plans",
	}))

	doc, _ := rm.GetBooking("b1")
	if !doc.Cancelled {
		t.Error("expected booking to be cancelled")
	}
}

func TestMyBookings_Populated(t *testing.T) {
	rm := readmodel.New()
	_ = rm.HandleEvent(context.Background(), msg(domain.RoomBooked{
		BookingID: "b1", GuestID: "g1", RoomID: "r1",
		CheckIn: "2026-04-01", CheckOut: "2026-04-05",
		Price: 500, Currency: "USD",
	}))

	bookings := rm.GetGuestBookings("g1")
	if len(bookings) != 1 {
		t.Fatalf("expected 1 booking, got %d", len(bookings))
	}
	if bookings[0].BookingID != "b1" {
		t.Errorf("unexpected booking: %+v", bookings[0])
	}
}

func TestMyBookings_CancelledMarked(t *testing.T) {
	rm := readmodel.New()
	_ = rm.HandleEvent(context.Background(), msg(domain.RoomBooked{
		BookingID: "b1", GuestID: "g1", RoomID: "r1",
		CheckIn: "2026-04-01", CheckOut: "2026-04-05",
		Price: 500, Currency: "USD",
	}))

	_ = rm.HandleEvent(context.Background(), msg(domain.BookingCancelled{
		BookingID: "b1", Reason: "changed plans",
	}))

	bookings := rm.GetGuestBookings("g1")
	if len(bookings) != 1 || !bookings[0].Cancelled {
		t.Errorf("expected cancelled booking, got %+v", bookings)
	}
}

func TestNilPayloadIgnored(t *testing.T) {
	rm := readmodel.New()
	err := rm.HandleEvent(context.Background(), &subscription.ConsumeContext{Payload: nil})
	if err != nil {
		t.Fatalf("nil payload should be ignored, got error: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd samples/booking && go test ./readmodel/... -v`
Expected: FAIL — package does not exist.

- [ ] **Step 3: Write readmodel.go**

```go
// Copyright (C) Eventuous HQ OÜ. All rights reserved
// Licensed under the Apache License, Version 2.0.

package readmodel

import (
	"context"
	"sync"

	"github.com/eventuous/eventuous-go/core/subscription"
	"github.com/eventuous/eventuous-go/samples/booking/domain"
)

// BookingDocument is the full booking view used by the BookingDetails projection.
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

// BookingSummary is the compact view used by the MyBookings projection.
type BookingSummary struct {
	BookingID string  `json:"bookingId"`
	RoomID    string  `json:"roomId"`
	CheckIn   string  `json:"checkIn"`
	CheckOut  string  `json:"checkOut"`
	Price     float64 `json:"price"`
	Currency  string  `json:"currency"`
	Cancelled bool    `json:"cancelled"`
}

// BookingReadModel holds two in-memory projections and implements subscription.EventHandler.
type BookingReadModel struct {
	mu       sync.RWMutex
	bookings map[string]BookingDocument  // keyed by booking ID
	guests   map[string][]BookingSummary // keyed by guest ID
}

// Compile-time interface check.
var _ subscription.EventHandler = (*BookingReadModel)(nil)

// New creates an empty BookingReadModel.
func New() *BookingReadModel {
	return &BookingReadModel{
		bookings: make(map[string]BookingDocument),
		guests:   make(map[string][]BookingSummary),
	}
}

// HandleEvent dispatches an event to both projections.
func (rm *BookingReadModel) HandleEvent(_ context.Context, msg *subscription.ConsumeContext) error {
	if msg.Payload == nil {
		return nil
	}

	rm.mu.Lock()
	defer rm.mu.Unlock()

	switch e := msg.Payload.(type) {
	case domain.RoomBooked:
		rm.bookings[e.BookingID] = BookingDocument{
			BookingID:   e.BookingID,
			GuestID:     e.GuestID,
			RoomID:      e.RoomID,
			CheckIn:     e.CheckIn,
			CheckOut:    e.CheckOut,
			Price:       e.Price,
			Outstanding: e.Price,
			Currency:    e.Currency,
		}
		rm.guests[e.GuestID] = append(rm.guests[e.GuestID], BookingSummary{
			BookingID: e.BookingID,
			RoomID:    e.RoomID,
			CheckIn:   e.CheckIn,
			CheckOut:  e.CheckOut,
			Price:     e.Price,
			Currency:  e.Currency,
		})

	case domain.PaymentRecorded:
		if doc, ok := rm.bookings[e.BookingID]; ok {
			doc.Outstanding -= e.Amount
			if doc.Outstanding <= 0 {
				doc.Paid = true
			}
			rm.bookings[e.BookingID] = doc
		}

	case domain.BookingCancelled:
		if doc, ok := rm.bookings[e.BookingID]; ok {
			doc.Cancelled = true
			rm.bookings[e.BookingID] = doc

			// Update MyBookings using guest ID from BookingDetails.
			guestID := doc.GuestID
			for i, s := range rm.guests[guestID] {
				if s.BookingID == e.BookingID {
					rm.guests[guestID][i].Cancelled = true
					break
				}
			}
		}
	}

	return nil
}

// GetBooking returns the booking document for the given ID.
func (rm *BookingReadModel) GetBooking(id string) (BookingDocument, bool) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	doc, ok := rm.bookings[id]
	return doc, ok
}

// GetGuestBookings returns the list of bookings for a guest.
func (rm *BookingReadModel) GetGuestBookings(guestID string) []BookingSummary {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return rm.guests[guestID]
}
```

- [ ] **Step 4: Run tests**

Run: `cd samples/booking && go test ./readmodel/... -v`
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add samples/booking/readmodel/
git commit -m "feat(samples): add in-memory read model with BookingDetails and MyBookings projections"
```

---

### Task 5: HTTP API

**Files:**
- Create: `samples/booking/httpapi/api.go`
- Create: `samples/booking/httpapi/api_test.go`

- [ ] **Step 1: Write api_test.go**

Tests use `httptest`, `memstore` for the command service, and the real read model. This validates the HTTP layer end-to-end without KurrentDB.

```go
// Copyright (C) Eventuous HQ OÜ. All rights reserved
// Licensed under the Apache License, Version 2.0.

package httpapi_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	eventuous "github.com/eventuous/eventuous-go/core"
	"github.com/eventuous/eventuous-go/core/command"
	"github.com/eventuous/eventuous-go/core/test/memstore"
	"github.com/eventuous/eventuous-go/samples/booking/domain"
	"github.com/eventuous/eventuous-go/samples/booking/httpapi"
	"github.com/eventuous/eventuous-go/samples/booking/readmodel"
)

func setupRouter(t *testing.T) (*http.ServeMux, *readmodel.BookingReadModel) {
	t.Helper()
	store := memstore.New()
	svc := command.New[domain.BookingState](store, store, domain.BookingFold, domain.BookingState{})
	command.On(svc, command.Handler[domain.BookRoom, domain.BookingState]{
		Expected: eventuous.IsNew,
		Stream:   func(cmd domain.BookRoom) eventuous.StreamName { return domain.BookingStream(cmd.BookingID) },
		Act:      domain.HandleBookRoom,
	})
	command.On(svc, command.Handler[domain.RecordPayment, domain.BookingState]{
		Expected: eventuous.IsExisting,
		Stream:   func(cmd domain.RecordPayment) eventuous.StreamName { return domain.BookingStream(cmd.BookingID) },
		Act:      domain.HandleRecordPayment,
	})
	command.On(svc, command.Handler[domain.CancelBooking, domain.BookingState]{
		Expected: eventuous.IsExisting,
		Stream:   func(cmd domain.CancelBooking) eventuous.StreamName { return domain.BookingStream(cmd.BookingID) },
		Act:      domain.HandleCancelBooking,
	})

	rm := readmodel.New()
	mux := http.NewServeMux()
	httpapi.Register(mux, svc, rm)
	return mux, rm
}

func TestBookRoom(t *testing.T) {
	mux, _ := setupRouter(t)

	body := `{"guestId":"g1","roomId":"r1","checkIn":"2026-04-01","checkOut":"2026-04-05","price":500,"currency":"USD"}`
	req := httptest.NewRequest(http.MethodPost, "/bookings", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	if resp["bookingId"] == nil || resp["bookingId"] == "" {
		t.Error("expected bookingId in response")
	}
}

func TestRecordPayment(t *testing.T) {
	mux, _ := setupRouter(t)

	// First book a room.
	body := `{"guestId":"g1","roomId":"r1","checkIn":"2026-04-01","checkOut":"2026-04-05","price":500,"currency":"USD"}`
	req := httptest.NewRequest(http.MethodPost, "/bookings", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	var bookResp map[string]any
	json.Unmarshal(w.Body.Bytes(), &bookResp)
	bookingID := bookResp["bookingId"].(string)

	// Record payment.
	payBody := `{"amount":200,"currency":"USD","paymentId":"p1"}`
	req = httptest.NewRequest(http.MethodPost, "/bookings/"+bookingID+"/payments", bytes.NewBufferString(payBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var payResp map[string]any
	json.Unmarshal(w.Body.Bytes(), &payResp)
	if payResp["outstanding"].(float64) != 300 {
		t.Errorf("expected outstanding=300, got %v", payResp["outstanding"])
	}
}

func TestGetBooking_NotFound(t *testing.T) {
	mux, _ := setupRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/bookings/nonexistent", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestGetGuestBookings_Empty(t *testing.T) {
	mux, _ := setupRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/guests/nobody/bookings", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp []any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp) != 0 {
		t.Errorf("expected empty array, got %v", resp)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd samples/booking && go test ./httpapi/... -v`
Expected: FAIL — `httpapi` package doesn't exist.

- [ ] **Step 3: Write api.go**

```go
// Copyright (C) Eventuous HQ OÜ. All rights reserved
// Licensed under the Apache License, Version 2.0.

package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"

	eventuous "github.com/eventuous/eventuous-go/core"
	"github.com/eventuous/eventuous-go/core/command"
	"github.com/eventuous/eventuous-go/samples/booking/domain"
	"github.com/eventuous/eventuous-go/samples/booking/readmodel"
)

// Register adds all booking HTTP routes to the mux.
func Register(
	mux *http.ServeMux,
	svc command.CommandHandler[domain.BookingState],
	rm *readmodel.BookingReadModel,
) {
	mux.HandleFunc("POST /bookings", handleBookRoom(svc))
	mux.HandleFunc("POST /bookings/{id}/payments", handleRecordPayment(svc))
	mux.HandleFunc("POST /bookings/{id}/cancel", handleCancelBooking(svc))
	mux.HandleFunc("GET /bookings/{id}", handleGetBooking(rm))
	mux.HandleFunc("GET /guests/{id}/bookings", handleGetGuestBookings(rm))
}

func handleBookRoom(svc command.CommandHandler[domain.BookingState]) http.HandlerFunc {
	type request struct {
		GuestID  string  `json:"guestId"`
		RoomID   string  `json:"roomId"`
		CheckIn  string  `json:"checkIn"`
		CheckOut string  `json:"checkOut"`
		Price    float64 `json:"price"`
		Currency string  `json:"currency"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		bookingID := uuid.New().String()
		result, err := svc.Handle(r.Context(), domain.BookRoom{
			BookingID: bookingID,
			GuestID:   req.GuestID,
			RoomID:    req.RoomID,
			CheckIn:   req.CheckIn,
			CheckOut:  req.CheckOut,
			Price:     req.Price,
			Currency:  req.Currency,
		})
		if err != nil {
			writeCommandError(w, err)
			return
		}

		writeJSON(w, http.StatusCreated, map[string]any{
			"bookingId":     bookingID,
			"streamVersion": result.StreamVersion,
		})
	}
}

func handleRecordPayment(svc command.CommandHandler[domain.BookingState]) http.HandlerFunc {
	type request struct {
		Amount    float64 `json:"amount"`
		Currency  string  `json:"currency"`
		PaymentID string  `json:"paymentId"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		bookingID := r.PathValue("id")

		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		result, err := svc.Handle(r.Context(), domain.RecordPayment{
			BookingID: bookingID,
			Amount:    req.Amount,
			Currency:  req.Currency,
			PaymentID: req.PaymentID,
		})
		if err != nil {
			writeCommandError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"bookingId":   bookingID,
			"outstanding": result.State.Outstanding,
			"paid":        result.State.Paid,
		})
	}
}

func handleCancelBooking(svc command.CommandHandler[domain.BookingState]) http.HandlerFunc {
	type request struct {
		Reason string `json:"reason"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		bookingID := r.PathValue("id")

		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		_, err := svc.Handle(r.Context(), domain.CancelBooking{
			BookingID: bookingID,
			Reason:    req.Reason,
		})
		if err != nil {
			writeCommandError(w, err)
			return
		}

		w.WriteHeader(http.StatusOK)
	}
}

func handleGetBooking(rm *readmodel.BookingReadModel) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		doc, ok := rm.GetBooking(id)
		if !ok {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, http.StatusOK, doc)
	}
}

func handleGetGuestBookings(rm *readmodel.BookingReadModel) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		guestID := r.PathValue("id")
		bookings := rm.GetGuestBookings(guestID)
		if bookings == nil {
			bookings = []readmodel.BookingSummary{}
		}
		writeJSON(w, http.StatusOK, bookings)
	}
}

// writeJSON encodes v as JSON and writes it with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// writeError writes a JSON error response.
func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

// writeCommandError maps domain/framework errors to HTTP status codes.
func writeCommandError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, eventuous.ErrStreamNotFound):
		writeError(w, http.StatusNotFound, err)
	case errors.Is(err, eventuous.ErrOptimisticConcurrency):
		writeError(w, http.StatusConflict, err)
	case errors.Is(err, eventuous.ErrHandlerNotFound):
		writeError(w, http.StatusBadRequest, err)
	case errors.Is(err, domain.ErrBookingCancelled),
		errors.Is(err, domain.ErrBookingAlreadyPaid),
		errors.Is(err, domain.ErrAlreadyCancelled):
		writeError(w, http.StatusUnprocessableEntity, err)
	default:
		writeError(w, http.StatusInternalServerError, err)
	}
}
```

- [ ] **Step 4: Run tests**

Run: `cd samples/booking && go test ./httpapi/... -v`
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add samples/booking/httpapi/
git commit -m "feat(samples): add HTTP API with command and query endpoints"
```

---

### Task 6: main.go — app wiring

**Files:**
- Create: `samples/booking/main.go`

- [ ] **Step 1: Write main.go**

```go
// Copyright (C) Eventuous HQ OÜ. All rights reserved
// Licensed under the Apache License, Version 2.0.

package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/kurrent-io/KurrentDB-Client-Go/kurrentdb"

	eventuous "github.com/eventuous/eventuous-go/core"
	"github.com/eventuous/eventuous-go/core/command"
	"github.com/eventuous/eventuous-go/core/subscription"
	kdb "github.com/eventuous/eventuous-go/kurrentdb"
	"github.com/eventuous/eventuous-go/samples/booking/domain"
	"github.com/eventuous/eventuous-go/samples/booking/httpapi"
	"github.com/eventuous/eventuous-go/samples/booking/readmodel"
)

func main() {
	// 1. Configuration.
	kurrentURL := os.Getenv("KURRENTDB_URL")
	if kurrentURL == "" {
		kurrentURL = "esdb://localhost:2113?tls=false"
	}

	// 2. KurrentDB client.
	settings, err := kurrentdb.ParseConnectionString(kurrentURL)
	if err != nil {
		log.Fatalf("invalid KurrentDB connection string: %v", err)
	}
	client, err := kurrentdb.NewClient(settings)
	if err != nil {
		log.Fatalf("failed to create KurrentDB client: %v", err)
	}

	// 3. Codec.
	codec := domain.NewCodec()

	// 4. Event store.
	store := kdb.NewStore(client, codec)

	// 5. Command service.
	svc := command.New[domain.BookingState](store, store, domain.BookingFold, domain.BookingState{})
	command.On(svc, command.Handler[domain.BookRoom, domain.BookingState]{
		Expected: eventuous.IsNew,
		Stream:   func(cmd domain.BookRoom) eventuous.StreamName { return domain.BookingStream(cmd.BookingID) },
		Act:      domain.HandleBookRoom,
	})
	command.On(svc, command.Handler[domain.RecordPayment, domain.BookingState]{
		Expected: eventuous.IsExisting,
		Stream:   func(cmd domain.RecordPayment) eventuous.StreamName { return domain.BookingStream(cmd.BookingID) },
		Act:      domain.HandleRecordPayment,
	})
	command.On(svc, command.Handler[domain.CancelBooking, domain.BookingState]{
		Expected: eventuous.IsExisting,
		Stream:   func(cmd domain.CancelBooking) eventuous.StreamName { return domain.BookingStream(cmd.BookingID) },
		Act:      domain.HandleCancelBooking,
	})

	// 6. Read model.
	rm := readmodel.New()

	// 7. Graceful shutdown context.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// 8. Catch-up subscription.
	sub := kdb.NewCatchUp(client, codec, "booking-projections",
		kdb.WithHandler(rm),
		kdb.WithMiddleware(subscription.WithLogging(slog.Default())),
	)
	go func() {
		if err := sub.Start(ctx); err != nil {
			log.Printf("subscription stopped: %v", err)
		}
	}()

	// 9. HTTP server.
	mux := http.NewServeMux()
	httpapi.Register(mux, svc, rm)

	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	go func() {
		<-ctx.Done()
		slog.Info("shutting down HTTP server")
		server.Shutdown(context.Background())
	}()

	slog.Info("starting booking sample", "addr", ":8080", "kurrentdb", kurrentURL)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("HTTP server error: %v", err)
	}
}
```

- [ ] **Step 2: Run go mod tidy and verify it compiles**

Run: `cd samples/booking && go mod tidy && go build .`
Expected: clean build.

- [ ] **Step 3: Commit**

```bash
git add samples/booking/main.go samples/booking/go.mod samples/booking/go.sum
git commit -m "feat(samples): add main.go with full app wiring"
```

---

### Task 7: Run all tests and verify

- [ ] **Step 1: Run all sample tests**

Run: `cd samples/booking && go test -race ./...`
Expected: all PASS.

- [ ] **Step 2: Run gofmt check**

Run: `gofmt -l samples/booking/`
Expected: no output (all files formatted).

- [ ] **Step 3: Run go vet**

Run: `cd samples/booking && go vet ./...`
Expected: no issues.

- [ ] **Step 4: Verify existing project tests still pass**

Run: `cd core && go test ./...`
Expected: all PASS (no regressions).

- [ ] **Step 5: Commit any fixes if needed, then final commit**

```bash
git add -A samples/booking/
git commit -m "feat(samples): booking sample — complete with domain, read model, HTTP API"
```
