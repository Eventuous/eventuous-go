// Copyright (C) Eventuous HQ OÜ. All rights reserved
// Licensed under the Apache License, Version 2.0.

package testdomain

type BookRoom struct {
	BookingID string
	RoomID    string
	CheckIn   string
	CheckOut  string
	Price     float64
}

type ImportBooking struct {
	BookingID string
	RoomID    string
	Price     float64
}

type RecordPayment struct {
	BookingID string
	Amount    float64
}

type CancelBooking struct {
	BookingID string
	Reason    string
}
