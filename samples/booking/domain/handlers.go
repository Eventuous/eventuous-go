// Copyright (C) Eventuous HQ OÜ. All rights reserved
// Licensed under the Apache License, Version 2.0.

package domain

import (
	"context"
	"errors"

	eventuous "github.com/eventuous/eventuous-go/core"
)

// Validation errors returned by command handlers.
var (
	ErrBookingCancelled   = errors.New("booking is cancelled")
	ErrBookingAlreadyPaid = errors.New("booking is already paid")
	ErrAlreadyCancelled   = errors.New("booking is already cancelled")
)

// BookingStream returns the stream name for a booking.
func BookingStream(id string) eventuous.StreamName {
	return eventuous.NewStreamName("Booking", id)
}

// HandleBookRoom handles the BookRoom command.
func HandleBookRoom(_ context.Context, _ BookingState, cmd BookRoom) ([]any, error) {
	return []any{
		RoomBooked{
			BookingID: cmd.BookingID,
			GuestID:   cmd.GuestID,
			RoomID:    cmd.RoomID,
			CheckIn:   cmd.CheckIn,
			CheckOut:  cmd.CheckOut,
			Price:     cmd.Price,
			Currency:  cmd.Currency,
		},
	}, nil
}

// HandleRecordPayment handles the RecordPayment command.
func HandleRecordPayment(_ context.Context, state BookingState, cmd RecordPayment) ([]any, error) {
	if state.Cancelled {
		return nil, ErrBookingCancelled
	}
	if state.Paid {
		return nil, ErrBookingAlreadyPaid
	}
	return []any{
		PaymentRecorded{
			BookingID: cmd.BookingID,
			Amount:    cmd.Amount,
			Currency:  cmd.Currency,
			PaymentID: cmd.PaymentID,
		},
	}, nil
}

// HandleCancelBooking handles the CancelBooking command.
func HandleCancelBooking(_ context.Context, state BookingState, cmd CancelBooking) ([]any, error) {
	if state.Cancelled {
		return nil, ErrAlreadyCancelled
	}
	return []any{
		BookingCancelled{
			BookingID: cmd.BookingID,
			Reason:    cmd.Reason,
		},
	}, nil
}
