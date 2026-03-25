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
