// Copyright (C) Eventuous HQ OÜ. All rights reserved
// Licensed under the Apache License, Version 2.0.

package domain

import "github.com/eventuous/eventuous-go/core/codec"

// Events

type RoomBooked struct {
	BookingID string  `json:"bookingId"`
	GuestID   string  `json:"guestId"`
	RoomID    string  `json:"roomId"`
	CheckIn   string  `json:"checkIn"`
	CheckOut  string  `json:"checkOut"`
	Price     float64 `json:"price"`
	Currency  string  `json:"currency"`
}

type PaymentRecorded struct {
	BookingID string  `json:"bookingId"`
	Amount    float64 `json:"amount"`
	Currency  string  `json:"currency"`
	PaymentID string  `json:"paymentId"`
}

type BookingCancelled struct {
	BookingID string `json:"bookingId"`
	Reason    string `json:"reason"`
}

// NewTypeMap creates a TypeMap with all booking events registered.
func NewTypeMap() *codec.TypeMap {
	tm := codec.NewTypeMap()
	mustRegister[RoomBooked](tm, "RoomBooked")
	mustRegister[PaymentRecorded](tm, "PaymentRecorded")
	mustRegister[BookingCancelled](tm, "BookingCancelled")
	return tm
}

// NewCodec creates a JSON codec with all booking events registered.
func NewCodec() codec.Codec {
	return codec.NewJSON(NewTypeMap())
}

func mustRegister[E any](tm *codec.TypeMap, name string) {
	if err := codec.Register[E](tm, name); err != nil {
		panic(err)
	}
}
