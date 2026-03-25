module github.com/eventuous/eventuous-go/samples/booking

go 1.25.0

replace (
	github.com/eventuous/eventuous-go/core => ../../core
	github.com/eventuous/eventuous-go/kurrentdb => ../../kurrentdb
)

require (
	github.com/eventuous/eventuous-go/core v0.0.0
	github.com/eventuous/eventuous-go/kurrentdb v0.0.0-00010101000000-000000000000
	github.com/kurrent-io/KurrentDB-Client-Go v1.1.2
)

require (
	github.com/google/uuid v1.6.0
	golang.org/x/net v0.50.0 // indirect
	golang.org/x/sys v0.41.0 // indirect
	golang.org/x/text v0.34.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260209200024-4cfbd4190f57 // indirect
	google.golang.org/grpc v1.79.3 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)
