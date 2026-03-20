// Copyright (C) Eventuous HQ OÜ. All rights reserved
// Licensed under the Apache License, Version 2.0.

package kurrentdb_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/kurrent-io/KurrentDB-Client-Go/kurrentdb"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func setupClient(t *testing.T) *kurrentdb.Client {
	t.Helper()
	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:        "eventstore/eventstore:24.10",
		ExposedPorts: []string{"2113/tcp"},
		Env: map[string]string{
			"EVENTSTORE_INSECURE":                  "true",
			"EVENTSTORE_ENABLE_ATOM_PUB_OVER_HTTP": "true",
			"EVENTSTORE_MEM_DB":                    "true",
			"EVENTSTORE_RUN_PROJECTIONS":           "All",
		},
		WaitingFor: wait.ForHTTP("/health/live").WithPort("2113/tcp").WithStatusCodeMatcher(func(status int) bool {
			return status == 204
		}),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("failed to start EventStoreDB container: %v", err)
	}
	t.Cleanup(func() { container.Terminate(ctx) })

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatal(err)
	}
	port, err := container.MappedPort(ctx, "2113")
	if err != nil {
		t.Fatal(err)
	}

	connStr := fmt.Sprintf("kurrentdb://%s:%s?tls=false", host, port.Port())
	settings, err := kurrentdb.ParseConnectionString(connStr)
	if err != nil {
		t.Fatal(err)
	}

	client, err := kurrentdb.NewClient(settings)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })

	return client
}
