# CLAUDE.md

## What Is This

Go port of Eventuous, a production-grade Event Sourcing library. See docs/specs/ for design specs.

## Build & Test

```
# Build all modules
(cd core && go build ./...)
(cd kurrentdb && go build ./...)
(cd otel && go build ./...)

# Test core (no infra needed)
(cd core && go test ./...)

# Test kurrentdb (needs EventStoreDB on :2113)
docker compose up -d
(cd kurrentdb && go test ./...)
```

## Module Structure

- `core/` — domain, persistence interfaces, command services, subscriptions (near-zero deps)
- `kurrentdb/` — KurrentDB/EventStoreDB integration (depends on core + esdb client)
- `otel/` — OpenTelemetry tracing and metrics (depends on core + OTel SDK)

## Code Style

- Go 1.23+, use generics where they reduce boilerplate
- Errors over panics. Sentinel errors with `errors.Is()`.
- `context.Context` as first parameter on all I/O functions
- `slog` for structured logging
- Functional options for configuration
- Table-driven tests
