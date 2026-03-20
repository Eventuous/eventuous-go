// Copyright (C) Eventuous HQ OÜ. All rights reserved
// Licensed under the Apache License, Version 2.0.

package kurrentdb_test

import (
	"testing"

	"github.com/eventuous/eventuous-go/core/test/storetest"
	kdb "github.com/eventuous/eventuous-go/kurrentdb"
)

func TestKurrentDBStore(t *testing.T) {
	client := setupClient(t)
	c := storetest.NewCodec()
	s := kdb.NewStore(client, c)
	storetest.RunAll(t, s)
}
