package otel_test

import (
	"context"
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
