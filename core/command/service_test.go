package command_test

import (
	"context"
	"errors"
	"testing"

	eventuous "github.com/eventuous/eventuous-go/core"
	"github.com/eventuous/eventuous-go/core/command"
	"github.com/eventuous/eventuous-go/core/store"
	"github.com/eventuous/eventuous-go/core/test/memstore"
	"github.com/eventuous/eventuous-go/core/test/testdomain"
	"github.com/google/uuid"
)

func newService(s *memstore.Store) *command.Service[testdomain.BookingState] {
	return command.New[testdomain.BookingState](s, s, testdomain.BookingFold, testdomain.BookingState{})
}

// seedEvents directly appends raw events to the memstore for test setup.
func seedEvents(t *testing.T, s *memstore.Store, stream eventuous.StreamName, events ...any) {
	t.Helper()
	newEvents := make([]store.NewStreamEvent, len(events))
	for i, e := range events {
		newEvents[i] = store.NewStreamEvent{ID: uuid.New(), Payload: e}
	}
	_, err := s.AppendEvents(context.Background(), stream, eventuous.VersionNoStream, newEvents)
	if err != nil {
		t.Fatal(err)
	}
}

func registerBookRoom(svc *command.Service[testdomain.BookingState]) {
	command.On(svc, command.Handler[testdomain.BookRoom, testdomain.BookingState]{
		Expected: eventuous.IsNew,
		Stream:   func(cmd testdomain.BookRoom) eventuous.StreamName { return testdomain.BookingStream(cmd.BookingID) },
		Act: func(ctx context.Context, state testdomain.BookingState, cmd testdomain.BookRoom) ([]any, error) {
			return []any{testdomain.RoomBooked{BookingID: cmd.BookingID, RoomID: cmd.RoomID, Price: cmd.Price}}, nil
		},
	})
}

func registerRecordPayment(svc *command.Service[testdomain.BookingState]) {
	command.On(svc, command.Handler[testdomain.RecordPayment, testdomain.BookingState]{
		Expected: eventuous.IsExisting,
		Stream: func(cmd testdomain.RecordPayment) eventuous.StreamName {
			return testdomain.BookingStream(cmd.BookingID)
		},
		Act: func(ctx context.Context, state testdomain.BookingState, cmd testdomain.RecordPayment) ([]any, error) {
			return []any{testdomain.PaymentRecorded{BookingID: cmd.BookingID, Amount: cmd.Amount}}, nil
		},
	})
}

func registerImportBooking(svc *command.Service[testdomain.BookingState]) {
	command.On(svc, command.Handler[testdomain.ImportBooking, testdomain.BookingState]{
		Expected: eventuous.IsAny,
		Stream: func(cmd testdomain.ImportBooking) eventuous.StreamName {
			return testdomain.BookingStream(cmd.BookingID)
		},
		Act: func(ctx context.Context, state testdomain.BookingState, cmd testdomain.ImportBooking) ([]any, error) {
			return []any{testdomain.BookingImported{BookingID: cmd.BookingID, RoomID: cmd.RoomID, Price: cmd.Price}}, nil
		},
	})
}

// --- OnNew tests ---

func TestService_OnNew_Success(t *testing.T) {
	s := memstore.New()
	svc := newService(s)
	registerBookRoom(svc)

	cmd := testdomain.BookRoom{BookingID: "booking-1", RoomID: "room-42", Price: 150.0}
	result, err := svc.Handle(context.Background(), cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.NewEvents) != 1 {
		t.Fatalf("expected 1 new event, got %d", len(result.NewEvents))
	}
	if result.State.RoomID != "room-42" {
		t.Errorf("expected RoomID=room-42, got %s", result.State.RoomID)
	}
	if result.State.Price != 150.0 {
		t.Errorf("expected Price=150.0, got %f", result.State.Price)
	}
}

func TestService_OnNew_FailsIfStreamExists(t *testing.T) {
	s := memstore.New()
	svc := newService(s)
	registerBookRoom(svc)

	stream := testdomain.BookingStream("booking-2")
	seedEvents(t, s, stream, testdomain.RoomBooked{BookingID: "booking-2", RoomID: "room-1", Price: 100.0})

	cmd := testdomain.BookRoom{BookingID: "booking-2", RoomID: "room-99", Price: 200.0}
	_, err := svc.Handle(context.Background(), cmd)
	if err == nil {
		t.Fatal("expected error when stream already exists for IsNew, got nil")
	}
}

// --- OnExisting tests ---

func TestService_OnExisting_Success(t *testing.T) {
	s := memstore.New()
	svc := newService(s)
	registerRecordPayment(svc)

	stream := testdomain.BookingStream("booking-3")
	seedEvents(t, s, stream, testdomain.RoomBooked{BookingID: "booking-3", RoomID: "room-7", Price: 200.0})

	cmd := testdomain.RecordPayment{BookingID: "booking-3", Amount: 100.0}
	result, err := svc.Handle(context.Background(), cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.State.AmountPaid != 100.0 {
		t.Errorf("expected AmountPaid=100.0, got %f", result.State.AmountPaid)
	}
	if len(result.NewEvents) != 1 {
		t.Fatalf("expected 1 new event, got %d", len(result.NewEvents))
	}
}

func TestService_OnExisting_FailsIfStreamMissing(t *testing.T) {
	s := memstore.New()
	svc := newService(s)
	registerRecordPayment(svc)

	cmd := testdomain.RecordPayment{BookingID: "no-such-booking", Amount: 50.0}
	_, err := svc.Handle(context.Background(), cmd)
	if err == nil {
		t.Fatal("expected error when stream does not exist for IsExisting, got nil")
	}
	if !errors.Is(err, eventuous.ErrStreamNotFound) {
		t.Errorf("expected ErrStreamNotFound, got %v", err)
	}
}

// --- OnAny tests ---

func TestService_OnAny_NewStream(t *testing.T) {
	s := memstore.New()
	svc := newService(s)
	registerImportBooking(svc)

	cmd := testdomain.ImportBooking{BookingID: "booking-new", RoomID: "room-5", Price: 300.0}
	result, err := svc.Handle(context.Background(), cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.State.RoomID != "room-5" {
		t.Errorf("expected RoomID=room-5, got %s", result.State.RoomID)
	}
}

func TestService_OnAny_ExistingStream(t *testing.T) {
	s := memstore.New()
	svc := newService(s)
	registerImportBooking(svc)

	stream := testdomain.BookingStream("booking-existing")
	seedEvents(t, s, stream, testdomain.RoomBooked{BookingID: "booking-existing", RoomID: "room-old", Price: 100.0})

	cmd := testdomain.ImportBooking{BookingID: "booking-existing", RoomID: "room-new", Price: 200.0}
	result, err := svc.Handle(context.Background(), cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.NewEvents) != 1 {
		t.Fatalf("expected 1 new event, got %d", len(result.NewEvents))
	}
}

// --- Other tests ---

func TestService_HandlerNotFound(t *testing.T) {
	s := memstore.New()
	svc := newService(s)
	// No handlers registered.

	cmd := testdomain.BookRoom{BookingID: "x"}
	_, err := svc.Handle(context.Background(), cmd)
	if err == nil {
		t.Fatal("expected ErrHandlerNotFound, got nil")
	}
	if !errors.Is(err, eventuous.ErrHandlerNotFound) {
		t.Errorf("expected ErrHandlerNotFound, got %v", err)
	}
}

func TestService_NoOp(t *testing.T) {
	s := memstore.New()
	svc := newService(s)

	// Register a handler that returns no events.
	command.On(svc, command.Handler[testdomain.BookRoom, testdomain.BookingState]{
		Expected: eventuous.IsAny,
		Stream:   func(cmd testdomain.BookRoom) eventuous.StreamName { return testdomain.BookingStream(cmd.BookingID) },
		Act: func(ctx context.Context, state testdomain.BookingState, cmd testdomain.BookRoom) ([]any, error) {
			return []any{}, nil
		},
	})

	cmd := testdomain.BookRoom{BookingID: "noop-booking"}
	result, err := svc.Handle(context.Background(), cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.NewEvents) != 0 {
		t.Errorf("expected 0 new events, got %d", len(result.NewEvents))
	}
	// Verify nothing was appended.
	exists, _ := s.StreamExists(context.Background(), testdomain.BookingStream("noop-booking"))
	if exists {
		t.Error("expected stream to not exist after no-op command")
	}
}
