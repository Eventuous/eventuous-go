// Copyright (C) Eventuous HQ OÜ. All rights reserved
// Licensed under the Apache License, Version 2.0.

package httpapi_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	eventuous "github.com/eventuous/eventuous-go/core"
	"github.com/eventuous/eventuous-go/core/command"
	"github.com/eventuous/eventuous-go/core/test/memstore"
	"github.com/eventuous/eventuous-go/samples/booking/domain"
	"github.com/eventuous/eventuous-go/samples/booking/httpapi"
	"github.com/eventuous/eventuous-go/samples/booking/readmodel"
)

func setupRouter(t *testing.T) (*http.ServeMux, *readmodel.BookingReadModel) {
	t.Helper()
	store := memstore.New()
	svc := command.New[domain.BookingState](store, store, domain.BookingFold, domain.BookingState{})
	command.On(svc, command.Handler[domain.BookRoom, domain.BookingState]{
		Expected: eventuous.IsNew,
		Stream:   func(cmd domain.BookRoom) eventuous.StreamName { return domain.BookingStream(cmd.BookingID) },
		Act:      domain.HandleBookRoom,
	})
	command.On(svc, command.Handler[domain.RecordPayment, domain.BookingState]{
		Expected: eventuous.IsExisting,
		Stream:   func(cmd domain.RecordPayment) eventuous.StreamName { return domain.BookingStream(cmd.BookingID) },
		Act:      domain.HandleRecordPayment,
	})
	command.On(svc, command.Handler[domain.CancelBooking, domain.BookingState]{
		Expected: eventuous.IsExisting,
		Stream:   func(cmd domain.CancelBooking) eventuous.StreamName { return domain.BookingStream(cmd.BookingID) },
		Act:      domain.HandleCancelBooking,
	})

	rm := readmodel.New()
	mux := http.NewServeMux()
	httpapi.Register(mux, svc, rm)
	return mux, rm
}

func TestBookRoom(t *testing.T) {
	mux, _ := setupRouter(t)

	body := `{"guestId":"g1","roomId":"r1","checkIn":"2026-04-01","checkOut":"2026-04-05","price":500,"currency":"USD"}`
	req := httptest.NewRequest(http.MethodPost, "/bookings", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	if resp["bookingId"] == nil || resp["bookingId"] == "" {
		t.Error("expected bookingId in response")
	}
}

func TestRecordPayment(t *testing.T) {
	mux, _ := setupRouter(t)

	// First book a room.
	body := `{"guestId":"g1","roomId":"r1","checkIn":"2026-04-01","checkOut":"2026-04-05","price":500,"currency":"USD"}`
	req := httptest.NewRequest(http.MethodPost, "/bookings", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	var bookResp map[string]any
	json.Unmarshal(w.Body.Bytes(), &bookResp)
	bookingID := bookResp["bookingId"].(string)

	// Record payment.
	payBody := `{"amount":200,"currency":"USD","paymentId":"p1"}`
	req = httptest.NewRequest(http.MethodPost, "/bookings/"+bookingID+"/payments", bytes.NewBufferString(payBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var payResp map[string]any
	json.Unmarshal(w.Body.Bytes(), &payResp)
	if payResp["outstanding"].(float64) != 300 {
		t.Errorf("expected outstanding=300, got %v", payResp["outstanding"])
	}
}

func TestGetBooking_NotFound(t *testing.T) {
	mux, _ := setupRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/bookings/nonexistent", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestGetGuestBookings_Empty(t *testing.T) {
	mux, _ := setupRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/guests/nobody/bookings", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp []any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp) != 0 {
		t.Errorf("expected empty array, got %v", resp)
	}
}
