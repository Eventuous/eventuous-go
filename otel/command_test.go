package otel_test

import (
	"context"
	"errors"
	"testing"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/eventuous/eventuous-go/core/command"
	otelevt "github.com/eventuous/eventuous-go/otel"
)

// mockHandler is a simple CommandHandler for use in tests.
type mockHandler[S any] struct {
	result *command.Result[S]
	err    error
}

func (m *mockHandler[S]) Handle(_ context.Context, _ any) (*command.Result[S], error) {
	return m.result, m.err
}

type testState struct{}

type testCommand struct{}

func TestTraceCommands_CreatesSpan(t *testing.T) {
	ctx := context.Background()

	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	defer func() { _ = tp.Shutdown(ctx) }()
	tracer := tp.Tracer("test")

	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	defer func() { _ = mp.Shutdown(ctx) }()
	meter := mp.Meter("test")

	inner := &mockHandler[testState]{
		result: &command.Result[testState]{},
		err:    nil,
	}
	handler := otelevt.TraceCommands[testState](inner, tracer, meter)

	_, err := handler.Handle(ctx, testCommand{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	span := spans[0]
	expectedName := "command/testCommand"
	if span.Name != expectedName {
		t.Errorf("expected span name %q, got %q", expectedName, span.Name)
	}
}

func TestTraceCommands_RecordsError(t *testing.T) {
	ctx := context.Background()

	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	defer func() { _ = tp.Shutdown(ctx) }()
	tracer := tp.Tracer("test")

	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	defer func() { _ = mp.Shutdown(ctx) }()
	meter := mp.Meter("test")

	handlerErr := errors.New("handler failed")
	inner := &mockHandler[testState]{
		result: nil,
		err:    handlerErr,
	}
	handler := otelevt.TraceCommands[testState](inner, tracer, meter)

	_, err := handler.Handle(ctx, testCommand{})
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

	// Verify error metric was recorded.
	var rm metricdata.ResourceMetrics
	if err2 := reader.Collect(ctx, &rm); err2 != nil {
		t.Fatalf("failed to collect metrics: %v", err2)
	}

	found := false
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == "eventuous.command.errors" {
				found = true
			}
		}
	}
	if !found {
		t.Error("expected eventuous.command.errors metric to be recorded")
	}
}
