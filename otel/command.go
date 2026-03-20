// Copyright (C) Eventuous HQ OÜ. All rights reserved
// Licensed under the Apache License, Version 2.0.

package otel

import (
	"context"
	"reflect"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/eventuous/eventuous-go/core/command"
)

// TracedCommandHandler wraps a CommandHandler with OpenTelemetry tracing and metrics.
type TracedCommandHandler[S any] struct {
	inner    command.CommandHandler[S]
	tracer   trace.Tracer
	meter    metric.Meter
	duration metric.Float64Histogram // command handling duration
	errors   metric.Int64Counter     // error count
}

// TraceCommands wraps a command handler with tracing and metrics.
func TraceCommands[S any](inner command.CommandHandler[S], tracer trace.Tracer, meter metric.Meter) *TracedCommandHandler[S] {
	duration, _ := meter.Float64Histogram(
		"eventuous.command.duration",
		metric.WithDescription("Duration of command handling in seconds"),
		metric.WithUnit("s"),
	)
	errors, _ := meter.Int64Counter(
		"eventuous.command.errors",
		metric.WithDescription("Number of command handling errors"),
	)
	return &TracedCommandHandler[S]{
		inner:    inner,
		tracer:   tracer,
		meter:    meter,
		duration: duration,
		errors:   errors,
	}
}

func (t *TracedCommandHandler[S]) Handle(ctx context.Context, cmd any) (*command.Result[S], error) {
	cmdType := reflect.TypeOf(cmd).Name()

	ctx, span := t.tracer.Start(ctx, "command/"+cmdType,
		trace.WithAttributes(attribute.String("eventuous.command_type", cmdType)),
	)
	defer span.End()

	start := time.Now()
	result, err := t.inner.Handle(ctx, cmd)
	elapsed := time.Since(start).Seconds()

	t.duration.Record(ctx, elapsed, metric.WithAttributes(
		attribute.String("command_type", cmdType),
	))

	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		span.RecordError(err)
		t.errors.Add(ctx, 1, metric.WithAttributes(
			attribute.String("command_type", cmdType),
		))
	} else {
		span.SetStatus(codes.Ok, "")
	}

	return result, err
}
