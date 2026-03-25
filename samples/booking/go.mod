module github.com/eventuous/eventuous-go/samples/booking

go 1.25

replace (
	github.com/eventuous/eventuous-go/core => ../../core
	github.com/eventuous/eventuous-go/kurrentdb => ../../kurrentdb
)

require github.com/eventuous/eventuous-go/core v0.0.0
