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

// GetGuestBookings returns a copy of the bookings list for a guest.
func (rm *BookingReadModel) GetGuestBookings(guestID string) []BookingSummary {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	src := rm.guests[guestID]
	if src == nil {
		return nil
	}
	out := make([]BookingSummary, len(src))
	copy(out, src)
	return out
}
