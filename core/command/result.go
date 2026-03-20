package command

// Result of a handled command.
type Result[S any] struct {
	State          S
	NewEvents      []any
	GlobalPosition uint64
	StreamVersion  int64
}
