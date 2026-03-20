package kurrentdb_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	eventuous "github.com/eventuous/eventuous-go/core"
	"github.com/eventuous/eventuous-go/core/command"
	"github.com/eventuous/eventuous-go/core/subscription"
	"github.com/eventuous/eventuous-go/core/test/testdomain"
	kdb "github.com/eventuous/eventuous-go/kurrentdb"
)

// TestEndToEnd validates the entire eventuous-go stack end-to-end with a real
// KurrentDB instance: type registration, codec, event store, functional command
// service (BookRoom + RecordPayment), and catch-up subscription.
func TestEndToEnd(t *testing.T) {
	// 1. Setup: KurrentDB client, codec, store.
	client := setupClient(t)
	jsonCodec := testdomain.NewCodec()
	store := kdb.NewStore(client, jsonCodec)
	bookingID := "e2e-booking-" + uuid.New().String()[:8]

	// 2. Create functional command service with the booking fold.
	svc := command.New[testdomain.BookingState](store, store, testdomain.BookingFold, testdomain.BookingState{})

	// Register BookRoom handler (stream must be new).
	command.On(svc, command.Handler[testdomain.BookRoom, testdomain.BookingState]{
		Expected: eventuous.IsNew,
		Stream: func(cmd testdomain.BookRoom) eventuous.StreamName {
			return testdomain.BookingStream(cmd.BookingID)
		},
		Act: func(ctx context.Context, state testdomain.BookingState, cmd testdomain.BookRoom) ([]any, error) {
			return []any{testdomain.RoomBooked{
				BookingID: cmd.BookingID,
				RoomID:    cmd.RoomID,
				Price:     cmd.Price,
			}}, nil
		},
	})

	// Register RecordPayment handler (stream must exist).
	command.On(svc, command.Handler[testdomain.RecordPayment, testdomain.BookingState]{
		Expected: eventuous.IsExisting,
		Stream: func(cmd testdomain.RecordPayment) eventuous.StreamName {
			return testdomain.BookingStream(cmd.BookingID)
		},
		Act: func(ctx context.Context, state testdomain.BookingState, cmd testdomain.RecordPayment) ([]any, error) {
			return []any{testdomain.PaymentRecorded{
				BookingID: cmd.BookingID,
				Amount:    cmd.Amount,
			}}, nil
		},
	})

	// 3. Handle BookRoom command.
	ctx := context.Background()
	result, err := svc.Handle(ctx, testdomain.BookRoom{
		BookingID: bookingID,
		RoomID:    "room-101",
		Price:     150.00,
	})
	if err != nil {
		t.Fatalf("BookRoom failed: %v", err)
	}

	// Verify state after BookRoom.
	if result.State.ID != bookingID {
		t.Errorf("expected ID %q, got %q", bookingID, result.State.ID)
	}
	if result.State.RoomID != "room-101" {
		t.Errorf("expected RoomID %q, got %q", "room-101", result.State.RoomID)
	}
	if result.State.Price != 150.00 {
		t.Errorf("expected Price 150, got %f", result.State.Price)
	}
	if !result.State.Active {
		t.Error("expected Active=true after BookRoom")
	}
	if len(result.NewEvents) != 1 {
		t.Errorf("expected 1 new event from BookRoom, got %d", len(result.NewEvents))
	}

	// 4. Handle RecordPayment command.
	result2, err := svc.Handle(ctx, testdomain.RecordPayment{
		BookingID: bookingID,
		Amount:    50.00,
	})
	if err != nil {
		t.Fatalf("RecordPayment failed: %v", err)
	}

	// Verify state after RecordPayment.
	if result2.State.AmountPaid != 50.00 {
		t.Errorf("expected AmountPaid 50, got %f", result2.State.AmountPaid)
	}

	// 5. Subscribe to the booking stream and verify events are received.
	var received atomic.Int32
	bookingStream := testdomain.BookingStream(bookingID)

	sub := kdb.NewCatchUp(client, jsonCodec, "e2e-test-"+bookingID,
		kdb.FromStream(bookingStream),
		kdb.WithHandler(subscription.HandlerFunc(func(ctx context.Context, msg *subscription.ConsumeContext) error {
			received.Add(1)
			return nil
		})),
	)

	subCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Start subscription in background.
	done := make(chan error, 1)
	go func() { done <- sub.Start(subCtx) }()

	// Wait for 2 events (RoomBooked + RecordPayment).
	deadline := time.After(10 * time.Second)
	for received.Load() < 2 {
		select {
		case <-deadline:
			t.Fatalf("timeout waiting for events, received %d", received.Load())
		case <-time.After(100 * time.Millisecond):
		}
	}

	cancel() // stop subscription

	// Wait for the subscription goroutine to finish.
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("subscription Start returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Error("subscription Start did not return after context cancellation")
	}

	if received.Load() != 2 {
		t.Errorf("expected 2 events, received %d", received.Load())
	}

	t.Logf("E2E test passed: booked room, recorded payment, subscription received %d events", received.Load())
}
