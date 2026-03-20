// Copyright (C) Eventuous HQ OÜ. All rights reserved
// Licensed under the Apache License, Version 2.0.

package testdomain

type BookingState struct {
	ID         string
	RoomID     string
	Price      float64
	AmountPaid float64
	Active     bool
	Cancelled  bool
}

func BookingFold(state BookingState, event any) BookingState {
	switch e := event.(type) {
	case RoomBooked:
		return BookingState{ID: e.BookingID, RoomID: e.RoomID, Price: e.Price, Active: true}
	case BookingImported:
		return BookingState{ID: e.BookingID, RoomID: e.RoomID, Price: e.Price, Active: true}
	case PaymentRecorded:
		state.AmountPaid += e.Amount
		return state
	case BookingCancelled:
		state.Active = false
		state.Cancelled = true
		return state
	default:
		return state
	}
}
