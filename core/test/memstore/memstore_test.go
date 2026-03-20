package memstore_test

import (
	"testing"

	"github.com/eventuous/eventuous-go/core/test/memstore"
	"github.com/eventuous/eventuous-go/core/test/storetest"
)

func TestMemStore(t *testing.T) {
	s := memstore.New()
	storetest.RunAll(t, s)
}
