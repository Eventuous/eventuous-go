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
			event: domain.BookingCancelled{BookingID: "b1", Reason: "changed plans"},
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
