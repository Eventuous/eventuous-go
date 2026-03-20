package otel_test

import (
	"context"
	"errors"
	"testing"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	eventuous "github.com/eventuous/eventuous-go/core"
	"github.com/eventuous/eventuous-go/core/subscription"
	otelevt "github.com/eventuous/eventuous-go/otel"
)

func TestTracingMiddleware_CreatesSpan(t *testing.T) {
	ctx := context.Background()

	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	defer func() { _ = tp.Shutdown(ctx) }()
	tracer := tp.Tracer("test")

	// Build a handler chain: tracing middleware → no-op handler.
	noopHandler := subscription.HandlerFunc(func(_ context.Context, _ *subscription.ConsumeContext) error {
		return nil
	})
	handler := subscription.Chain(noopHandler, otelevt.TracingMiddleware(tracer))

	msg := &subscription.ConsumeContext{
		EventType: "OrderPlaced",
		Stream:    eventuous.StreamName("order-1"),
		Position:  42,
	}

	if err := handler.HandleEvent(ctx, msg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	span := spans[0]
	expectedName := "event/OrderPlaced"
	if span.Name != expectedName {
		t.Errorf("expected span name %q, got %q", expectedName, span.Name)
	}
}

func TestTracingMiddleware_RecordsError(t *testing.T) {
	ctx := context.Background()

	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	defer func() { _ = tp.Shutdown(ctx) }()
	tracer := tp.Tracer("test")

	handlerErr := errors.New("handler failed")
	errHandler := subscription.HandlerFunc(func(_ context.Context, _ *subscription.ConsumeContext) error {
		return handlerErr
	})
	handler := subscription.Chain(errHandler, otelevt.TracingMiddleware(tracer))

	msg := &subscription.ConsumeContext{
		EventType: "OrderFailed",
		Stream:    eventuous.StreamName("order-2"),
		Position:  7,
	}

	err := handler.HandleEvent(ctx, msg)
	if !errors.Is(err, handlerErr) {
		t.Fatalf("expected error %v, got %v", handlerErr, err)
	}

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	span := spans[0]
	if span.Status.Code.String() != "Error" {
		t.Errorf("expected span status Error, got %q", span.Status.Code.String())
	}
	if span.Status.Description != handlerErr.Error() {
		t.Errorf("expected status description %q, got %q", handlerErr.Error(), span.Status.Description)
	}
}
