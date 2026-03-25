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
