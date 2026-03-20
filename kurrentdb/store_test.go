package kurrentdb_test

import (
	"testing"

	"github.com/eventuous/eventuous-go/core/test/storetest"
	kdb "github.com/eventuous/eventuous-go/kurrentdb"
)

func TestKurrentDBStore(t *testing.T) {
	client := setupClient(t)
	s := kdb.NewStore(client, storetest.NewCodec())
	storetest.RunAll(t, s)
}
