// Copyright (C) Eventuous HQ OÜ. All rights reserved
// Licensed under the Apache License, Version 2.0.

package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/kurrent-io/KurrentDB-Client-Go/kurrentdb"

	eventuous "github.com/eventuous/eventuous-go/core"
	"github.com/eventuous/eventuous-go/core/command"
	"github.com/eventuous/eventuous-go/core/subscription"
	kdb "github.com/eventuous/eventuous-go/kurrentdb"
	"github.com/eventuous/eventuous-go/samples/booking/domain"
	"github.com/eventuous/eventuous-go/samples/booking/httpapi"
	"github.com/eventuous/eventuous-go/samples/booking/readmodel"
)

func main() {
	// 1. Configuration.
	kurrentURL := os.Getenv("KURRENTDB_URL")
	if kurrentURL == "" {
		kurrentURL = "esdb://localhost:2113?tls=false"
	}

	// 2. KurrentDB client.
	settings, err := kurrentdb.ParseConnectionString(kurrentURL)
	if err != nil {
		log.Fatalf("invalid KurrentDB connection string: %v", err)
	}
	client, err := kurrentdb.NewClient(settings)
	if err != nil {
		log.Fatalf("failed to create KurrentDB client: %v", err)
	}

	// 3. Type map and codec.
	typeMap := domain.NewTypeMap()
	jsonCodec := domain.NewCodecFromTypeMap(typeMap)

	// 4. Event store.
	store := kdb.NewStore(client, jsonCodec)

	// 5. Command service.
	svc := command.New[domain.BookingState](store, store, typeMap, domain.BookingFold, domain.BookingState{})
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

	// 6. Read model.
	rm := readmodel.New()

	// 7. Graceful shutdown context.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// 8. Catch-up subscription.
	sub := kdb.NewCatchUp(client, jsonCodec, "booking-projections",
		kdb.WithHandler(rm),
		kdb.WithMiddleware(subscription.WithLogging(slog.Default())),
	)
	go func() {
		if err := sub.Start(ctx); err != nil {
			log.Printf("subscription stopped: %v", err)
		}
	}()

	// 9. HTTP server.
	mux := http.NewServeMux()
	httpapi.Register(mux, svc, rm)

	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	go func() {
		<-ctx.Done()
		slog.Info("shutting down HTTP server")
		server.Shutdown(context.Background())
	}()

	slog.Info("starting booking sample", "addr", ":8080", "kurrentdb", kurrentURL)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("HTTP server error: %v", err)
	}
}
