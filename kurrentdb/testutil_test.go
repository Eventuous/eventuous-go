package kurrentdb_test

import (
	"testing"

	"github.com/kurrent-io/KurrentDB-Client-Go/kurrentdb"
)

func setupClient(t *testing.T) *kurrentdb.Client {
	t.Helper()

	settings, err := kurrentdb.ParseConnectionString("kurrentdb://localhost:2113?tls=false")
	if err != nil {
		t.Fatalf("failed to parse connection string: %v", err)
	}

	client, err := kurrentdb.NewClient(settings)
	if err != nil {
		t.Fatalf("failed to create KurrentDB client: %v", err)
	}

	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Logf("warning: failed to close KurrentDB client: %v", err)
		}
	})

	return client
}
