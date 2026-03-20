// Copyright (C) Eventuous HQ OÜ. All rights reserved
// Licensed under the Apache License, Version 2.0.

package command

// Result of a handled command.
type Result[S any] struct {
	State          S
	NewEvents      []any
	GlobalPosition uint64
	StreamVersion  int64
}
