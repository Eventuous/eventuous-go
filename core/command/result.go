// Copyright (C) Eventuous HQ OÜ. All rights reserved
// Licensed under the Apache License, Version 2.0.

package command

// Change pairs a domain event with its registered type name.
type Change struct {
	Event     any
	EventType string
}

// Result of a handled command.
type Result[S any] struct {
	State          S
	Changes        []Change
	GlobalPosition uint64
	StreamVersion  int64
}
