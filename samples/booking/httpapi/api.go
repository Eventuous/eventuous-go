// Copyright (C) Eventuous HQ OÜ. All rights reserved
// Licensed under the Apache License, Version 2.0.

package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"

	eventuous "github.com/eventuous/eventuous-go/core"
	"github.com/eventuous/eventuous-go/core/command"
	"github.com/eventuous/eventuous-go/samples/booking/domain"
	"github.com/eventuous/eventuous-go/samples/booking/readmodel"
)

// Register adds all booking HTTP routes to the mux.
func Register(
	mux *http.ServeMux,
	svc command.CommandHandler[domain.BookingState],
	rm *readmodel.BookingReadModel,
) {
	mux.HandleFunc("POST /bookings", handleBookRoom(svc))
	mux.HandleFunc("POST /bookings/{id}/payments", handleRecordPayment(svc))
	mux.HandleFunc("POST /bookings/{id}/cancel", handleCancelBooking(svc))
	mux.HandleFunc("GET /bookings/{id}", handleGetBooking(rm))
	mux.HandleFunc("GET /guests/{id}/bookings", handleGetGuestBookings(rm))
}

func handleBookRoom(svc command.CommandHandler[domain.BookingState]) http.HandlerFunc {
	type request struct {
		GuestID  string  `json:"guestId"`
		RoomID   string  `json:"roomId"`
		CheckIn  string  `json:"checkIn"`
		CheckOut string  `json:"checkOut"`
		Price    float64 `json:"price"`
		Currency string  `json:"currency"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		bookingID := uuid.New().String()
		result, err := svc.Handle(r.Context(), domain.BookRoom{
			BookingID: bookingID,
			GuestID:   req.GuestID,
			RoomID:    req.RoomID,
			CheckIn:   req.CheckIn,
			CheckOut:  req.CheckOut,
			Price:     req.Price,
			Currency:  req.Currency,
		})
		if err != nil {
			writeCommandError(w, err)
			return
		}

		writeJSON(w, http.StatusCreated, map[string]any{
			"bookingId":     bookingID,
			"streamVersion": result.StreamVersion,
		})
	}
}

func handleRecordPayment(svc command.CommandHandler[domain.BookingState]) http.HandlerFunc {
	type request struct {
		Amount    float64 `json:"amount"`
		Currency  string  `json:"currency"`
		PaymentID string  `json:"paymentId"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		bookingID := r.PathValue("id")

		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		result, err := svc.Handle(r.Context(), domain.RecordPayment{
			BookingID: bookingID,
			Amount:    req.Amount,
			Currency:  req.Currency,
			PaymentID: req.PaymentID,
		})
		if err != nil {
			writeCommandError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"bookingId":   bookingID,
			"outstanding": result.State.Outstanding,
			"paid":        result.State.Paid,
		})
	}
}

func handleCancelBooking(svc command.CommandHandler[domain.BookingState]) http.HandlerFunc {
	type request struct {
		Reason string `json:"reason"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		bookingID := r.PathValue("id")

		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		_, err := svc.Handle(r.Context(), domain.CancelBooking{
			BookingID: bookingID,
			Reason:    req.Reason,
		})
		if err != nil {
			writeCommandError(w, err)
			return
		}

		w.WriteHeader(http.StatusOK)
	}
}

func handleGetBooking(rm *readmodel.BookingReadModel) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		doc, ok := rm.GetBooking(id)
		if !ok {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, http.StatusOK, doc)
	}
}

func handleGetGuestBookings(rm *readmodel.BookingReadModel) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		guestID := r.PathValue("id")
		bookings := rm.GetGuestBookings(guestID)
		if bookings == nil {
			bookings = []readmodel.BookingSummary{}
		}
		writeJSON(w, http.StatusOK, bookings)
	}
}

// writeJSON encodes v as JSON and writes it with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// writeError writes a JSON error response.
func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

// writeCommandError maps domain/framework errors to HTTP status codes.
func writeCommandError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, eventuous.ErrStreamNotFound):
		writeError(w, http.StatusNotFound, err)
	case errors.Is(err, eventuous.ErrOptimisticConcurrency):
		writeError(w, http.StatusConflict, err)
	case errors.Is(err, eventuous.ErrHandlerNotFound):
		writeError(w, http.StatusBadRequest, err)
	case errors.Is(err, domain.ErrBookingCancelled),
		errors.Is(err, domain.ErrBookingAlreadyPaid),
		errors.Is(err, domain.ErrAlreadyCancelled):
		writeError(w, http.StatusUnprocessableEntity, err)
	default:
		writeError(w, http.StatusInternalServerError, err)
	}
}
