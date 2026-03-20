package subscription

import "context"

// EventHandler processes a single event.
type EventHandler interface {
	HandleEvent(ctx context.Context, msg *ConsumeContext) error
}

// HandlerFunc adaptor — like http.HandlerFunc.
type HandlerFunc func(ctx context.Context, msg *ConsumeContext) error

func (f HandlerFunc) HandleEvent(ctx context.Context, msg *ConsumeContext) error {
	return f(ctx, msg)
}
