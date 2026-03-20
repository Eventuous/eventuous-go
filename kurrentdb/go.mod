module github.com/eventuous/eventuous-go/kurrentdb

go 1.23.0

replace github.com/eventuous/eventuous-go/core => ../core

require (
	github.com/eventuous/eventuous-go/core v0.0.0
	github.com/google/uuid v1.6.0
	github.com/kurrent-io/KurrentDB-Client-Go v1.1.1
)

require (
	golang.org/x/net v0.38.0 // indirect
	golang.org/x/sys v0.31.0 // indirect
	golang.org/x/text v0.23.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250324211829-b45e905df463 // indirect
	google.golang.org/grpc v1.71.0 // indirect
	google.golang.org/protobuf v1.36.6 // indirect
)
