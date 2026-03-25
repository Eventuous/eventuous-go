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
