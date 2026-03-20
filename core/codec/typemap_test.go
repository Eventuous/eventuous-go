// Copyright (C) Eventuous HQ OÜ. All rights reserved
// Licensed under the Apache License, Version 2.0.

package codec_test

import (
	"testing"

	"github.com/eventuous/eventuous-go/core/codec"
)

type testEvent struct{ Value string }
type otherEvent struct{ Count int }

// TestRegister_AndTypeName verifies that a registered type can be looked up by name.
func TestRegister_AndTypeName(t *testing.T) {
	tm := codec.NewTypeMap()
	if err := codec.Register[testEvent](tm, "TestEvent"); err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	name, err := tm.TypeName(&testEvent{})
	if err != nil {
		t.Fatalf("TypeName failed: %v", err)
	}
	if name != "TestEvent" {
		t.Errorf("TypeName = %q, want %q", name, "TestEvent")
	}
}

// TestRegister_AndNewInstance verifies that NewInstance returns a *testEvent for a registered name.
func TestRegister_AndNewInstance(t *testing.T) {
	tm := codec.NewTypeMap()
	if err := codec.Register[testEvent](tm, "TestEvent"); err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	inst, err := tm.NewInstance("TestEvent")
	if err != nil {
		t.Fatalf("NewInstance failed: %v", err)
	}
	if _, ok := inst.(*testEvent); !ok {
		t.Errorf("NewInstance returned %T, want *testEvent", inst)
	}
}

// TestTypeName_UnregisteredType verifies that TypeName returns an error for an unregistered type.
func TestTypeName_UnregisteredType(t *testing.T) {
	tm := codec.NewTypeMap()
	_, err := tm.TypeName(&testEvent{})
	if err == nil {
		t.Fatal("expected error for unregistered type, got nil")
	}
}

// TestNewInstance_UnknownName verifies that NewInstance returns an error for an unknown name.
func TestNewInstance_UnknownName(t *testing.T) {
	tm := codec.NewTypeMap()
	_, err := tm.NewInstance("Unknown")
	if err == nil {
		t.Fatal("expected error for unknown name, got nil")
	}
}

// TestRegister_SameTypeDifferentName verifies that registering the same type under a different name returns an error.
func TestRegister_SameTypeDifferentName(t *testing.T) {
	tm := codec.NewTypeMap()
	if err := codec.Register[testEvent](tm, "TestEvent"); err != nil {
		t.Fatalf("first Register failed: %v", err)
	}
	if err := codec.Register[testEvent](tm, "OtherName"); err == nil {
		t.Fatal("expected error when registering same type with different name, got nil")
	}
}

// TestRegister_DifferentTypeSameName verifies that registering a different type under the same name returns an error.
func TestRegister_DifferentTypeSameName(t *testing.T) {
	tm := codec.NewTypeMap()
	if err := codec.Register[testEvent](tm, "SharedName"); err != nil {
		t.Fatalf("first Register failed: %v", err)
	}
	if err := codec.Register[otherEvent](tm, "SharedName"); err == nil {
		t.Fatal("expected error when registering different type with same name, got nil")
	}
}

// TestRegister_Idempotent verifies that registering the same type+name twice returns nil.
func TestRegister_Idempotent(t *testing.T) {
	tm := codec.NewTypeMap()
	if err := codec.Register[testEvent](tm, "TestEvent"); err != nil {
		t.Fatalf("first Register failed: %v", err)
	}
	if err := codec.Register[testEvent](tm, "TestEvent"); err != nil {
		t.Errorf("second Register (idempotent) returned error: %v", err)
	}
}

// TestTypeName_DistinguishesTypes verifies that TypeName correctly distinguishes between two registered types.
func TestTypeName_DistinguishesTypes(t *testing.T) {
	tm := codec.NewTypeMap()
	if err := codec.Register[testEvent](tm, "TestEvent"); err != nil {
		t.Fatalf("Register testEvent failed: %v", err)
	}
	if err := codec.Register[otherEvent](tm, "OtherEvent"); err != nil {
		t.Fatalf("Register otherEvent failed: %v", err)
	}

	name1, err := tm.TypeName(&testEvent{})
	if err != nil {
		t.Fatalf("TypeName(&testEvent{}) failed: %v", err)
	}
	if name1 != "TestEvent" {
		t.Errorf("TypeName(&testEvent{}) = %q, want %q", name1, "TestEvent")
	}

	name2, err := tm.TypeName(&otherEvent{})
	if err != nil {
		t.Fatalf("TypeName(&otherEvent{}) failed: %v", err)
	}
	if name2 != "OtherEvent" {
		t.Errorf("TypeName(&otherEvent{}) = %q, want %q", name2, "OtherEvent")
	}
}
