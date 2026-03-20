// Copyright (C) Eventuous HQ OÜ. All rights reserved
// Licensed under the Apache License, Version 2.0.

package command_test

import (
	"context"
	"errors"
	"testing"

	eventuous "github.com/eventuous/eventuous-go/core"
	"github.com/eventuous/eventuous-go/core/aggregate"
	"github.com/eventuous/eventuous-go/core/command"
	"github.com/eventuous/eventuous-go/core/test/memstore"
	"github.com/eventuous/eventuous-go/core/test/testdomain"
)

func newAggService(s *memstore.Store) *command.AggregateService[testdomain.BookingState] {
	return command.NewAggregateService[testdomain.BookingState](s, s, testdomain.BookingFold, testdomain.BookingState{})
}

// aggStreamName returns the stream name as the AggregateService generates it.
// Stream name = "{StateTypeName}-{id}" where StateTypeName = "BookingState".
func aggStreamName(id string) eventuous.StreamName {
	return eventuous.NewStreamName("BookingState", id)
}

func registerAggBookRoom(svc *command.AggregateService[testdomain.BookingState]) {
	command.OnAggregate(svc, command.AggregateHandler[testdomain.BookRoom, testdomain.BookingState]{
		Expected: eventuous.IsNew,
		ID:       func(cmd testdomain.BookRoom) string { return cmd.BookingID },
		Act: func(ctx context.Context, agg *aggregate.Aggregate[testdomain.BookingState], cmd testdomain.BookRoom) error {
			if err := agg.EnsureNew(); err != nil {
				return err
			}
			agg.Apply(testdomain.RoomBooked{BookingID: cmd.BookingID, RoomID: cmd.RoomID, Price: cmd.Price})
			return nil
		},
	})
}

func registerAggRecordPayment(svc *command.AggregateService[testdomain.BookingState]) {
	command.OnAggregate(svc, command.AggregateHandler[testdomain.RecordPayment, testdomain.BookingState]{
		Expected: eventuous.IsExisting,
		ID:       func(cmd testdomain.RecordPayment) string { return cmd.BookingID },
		Act: func(ctx context.Context, agg *aggregate.Aggregate[testdomain.BookingState], cmd testdomain.RecordPayment) error {
			if err := agg.EnsureExists(); err != nil {
				return err
			}
			agg.Apply(testdomain.PaymentRecorded{BookingID: cmd.BookingID, Amount: cmd.Amount})
			return nil
		},
	})
}

func registerAggImportBooking(svc *command.AggregateService[testdomain.BookingState]) {
	command.OnAggregate(svc, command.AggregateHandler[testdomain.ImportBooking, testdomain.BookingState]{
		Expected: eventuous.IsAny,
		ID:       func(cmd testdomain.ImportBooking) string { return cmd.BookingID },
		Act: func(ctx context.Context, agg *aggregate.Aggregate[testdomain.BookingState], cmd testdomain.ImportBooking) error {
			agg.Apply(testdomain.BookingImported{BookingID: cmd.BookingID, RoomID: cmd.RoomID, Price: cmd.Price})
			return nil
		},
	})
}

// --- OnNew tests ---

func TestAggService_OnNew_Success(t *testing.T) {
	s := memstore.New()
	svc := newAggService(s)
	registerAggBookRoom(svc)

	cmd := testdomain.BookRoom{BookingID: "agg-booking-1", RoomID: "room-42", Price: 150.0}
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

func TestAggService_OnNew_FailsIfExists(t *testing.T) {
	s := memstore.New()
	svc := newAggService(s)
	registerAggBookRoom(svc)

	// Seed an existing event using the same stream name the service will use.
	stream := aggStreamName("agg-booking-2")
	seedEvents(t, s, stream, testdomain.RoomBooked{BookingID: "agg-booking-2", RoomID: "room-1", Price: 100.0})

	cmd := testdomain.BookRoom{BookingID: "agg-booking-2", RoomID: "room-99", Price: 200.0}
	_, err := svc.Handle(context.Background(), cmd)
	if err == nil {
		t.Fatal("expected error when aggregate already exists for IsNew, got nil")
	}
	if !errors.Is(err, aggregate.ErrAggregateExists) {
		t.Errorf("expected ErrAggregateExists, got %v", err)
	}
}

// --- OnExisting tests ---

func TestAggService_OnExisting_Success(t *testing.T) {
	s := memstore.New()
	svc := newAggService(s)
	registerAggRecordPayment(svc)

	stream := aggStreamName("agg-booking-3")
	seedEvents(t, s, stream, testdomain.RoomBooked{BookingID: "agg-booking-3", RoomID: "room-7", Price: 200.0})

	cmd := testdomain.RecordPayment{BookingID: "agg-booking-3", Amount: 100.0}
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

func TestAggService_OnExisting_FailsIfMissing(t *testing.T) {
	s := memstore.New()
	svc := newAggService(s)
	registerAggRecordPayment(svc)

	cmd := testdomain.RecordPayment{BookingID: "no-such-booking", Amount: 50.0}
	_, err := svc.Handle(context.Background(), cmd)
	if err == nil {
		t.Fatal("expected error when aggregate does not exist for IsExisting, got nil")
	}
	if !errors.Is(err, aggregate.ErrAggregateNotExists) {
		t.Errorf("expected ErrAggregateNotExists, got %v", err)
	}
}

// --- OnAny tests ---

func TestAggService_OnAny_Works(t *testing.T) {
	// OnAny on a new stream.
	t.Run("NewStream", func(t *testing.T) {
		s := memstore.New()
		svc := newAggService(s)
		registerAggImportBooking(svc)

		cmd := testdomain.ImportBooking{BookingID: "agg-booking-new", RoomID: "room-5", Price: 300.0}
		result, err := svc.Handle(context.Background(), cmd)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.State.RoomID != "room-5" {
			t.Errorf("expected RoomID=room-5, got %s", result.State.RoomID)
		}
		if len(result.NewEvents) != 1 {
			t.Fatalf("expected 1 new event, got %d", len(result.NewEvents))
		}
	})

	// OnAny on an existing stream.
	t.Run("ExistingStream", func(t *testing.T) {
		s := memstore.New()
		svc := newAggService(s)
		registerAggImportBooking(svc)

		stream := aggStreamName("agg-booking-existing")
		seedEvents(t, s, stream, testdomain.RoomBooked{BookingID: "agg-booking-existing", RoomID: "room-old", Price: 100.0})

		cmd := testdomain.ImportBooking{BookingID: "agg-booking-existing", RoomID: "room-new", Price: 200.0}
		result, err := svc.Handle(context.Background(), cmd)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result.NewEvents) != 1 {
			t.Fatalf("expected 1 new event, got %d", len(result.NewEvents))
		}
	})
}

// --- Other tests ---

func TestAggService_HandlerNotFound(t *testing.T) {
	s := memstore.New()
	svc := newAggService(s)
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

func TestAggService_NoOp(t *testing.T) {
	s := memstore.New()
	svc := newAggService(s)

	// Register a handler that does not call Apply — no changes.
	command.OnAggregate(svc, command.AggregateHandler[testdomain.BookRoom, testdomain.BookingState]{
		Expected: eventuous.IsAny,
		ID:       func(cmd testdomain.BookRoom) string { return cmd.BookingID },
		Act: func(ctx context.Context, agg *aggregate.Aggregate[testdomain.BookingState], cmd testdomain.BookRoom) error {
			// Intentionally do nothing — no Apply call.
			return nil
		},
	})

	cmd := testdomain.BookRoom{BookingID: "noop-agg-booking"}
	result, err := svc.Handle(context.Background(), cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.NewEvents) != 0 {
		t.Errorf("expected 0 new events, got %d", len(result.NewEvents))
	}
	// Verify nothing was appended.
	exists, _ := s.StreamExists(context.Background(), aggStreamName("noop-agg-booking"))
	if exists {
		t.Error("expected stream to not exist after no-op command")
	}
}
