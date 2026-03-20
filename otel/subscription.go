package otel

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/eventuous/eventuous-go/core/subscription"
)

// TracingMiddleware creates a subscription middleware that traces event handling.
func TracingMiddleware(tracer trace.Tracer) subscription.Middleware {
	return func(next subscription.EventHandler) subscription.EventHandler {
		return subscription.HandlerFunc(func(ctx context.Context, msg *subscription.ConsumeContext) error {
			ctx, span := tracer.Start(ctx, "event/"+msg.EventType,
				trace.WithAttributes(
					attribute.String("eventuous.event_type", msg.EventType),
					attribute.String("eventuous.stream", msg.Stream.String()),
					attribute.Int64("eventuous.position", int64(msg.Position)),
				),
			)
			defer span.End()

			err := next.HandleEvent(ctx, msg)
			if err != nil {
				span.SetStatus(codes.Error, err.Error())
				span.RecordError(err)
			} else {
				span.SetStatus(codes.Ok, "")
			}
			return err
		})
	}
}
