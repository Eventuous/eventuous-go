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
