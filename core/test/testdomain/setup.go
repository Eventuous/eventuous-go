// Copyright (C) Eventuous HQ OÜ. All rights reserved
// Licensed under the Apache License, Version 2.0.

package testdomain

import (
	eventuous "github.com/eventuous/eventuous-go/core"
	"github.com/eventuous/eventuous-go/core/codec"
)

// NewTypeMap creates a TypeMap with all booking events registered.
func NewTypeMap() *codec.TypeMap {
	tm := codec.NewTypeMap()
	mustRegister[RoomBooked](tm, "RoomBooked")
	mustRegister[BookingImported](tm, "BookingImported")
	mustRegister[PaymentRecorded](tm, "PaymentRecorded")
	mustRegister[BookingCancelled](tm, "BookingCancelled")
	return tm
}

// mustRegister registers an event type in the TypeMap and panics if it fails.
func mustRegister[E any](tm *codec.TypeMap, name string) {
	if err := codec.Register[E](tm, name); err != nil {
		panic(err)
	}
}

// NewCodec creates a JSON codec with all booking events registered.
func NewCodec() codec.Codec {
	return codec.NewJSON(NewTypeMap())
}

// BookingStream returns the stream name for a booking.
func BookingStream(id string) eventuous.StreamName {
	return eventuous.NewStreamName("Booking", id)
}
