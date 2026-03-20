// Copyright (C) Eventuous HQ OÜ. All rights reserved
// Licensed under the Apache License, Version 2.0.

package codec

import (
	"fmt"
	"reflect"
	"sync"
)

// TypeMap holds bidirectional type <-> name mappings for event serialization.
// It is safe for concurrent use.
type TypeMap struct {
	mu       sync.RWMutex
	toName   map[reflect.Type]string
	fromName map[string]reflect.Type
}

// NewTypeMap creates and returns an empty TypeMap.
func NewTypeMap() *TypeMap {
	return &TypeMap{
		toName:   make(map[reflect.Type]string),
		fromName: make(map[string]reflect.Type),
	}
}

// Register maps the Go type E to the persistent event type name.
// It returns an error if:
//   - E is already registered under a different name, or
//   - name is already registered for a different type.
//
// Registering the same type with the same name is idempotent and returns nil.
func Register[E any](tm *TypeMap, name string) error {
	t := reflect.TypeOf((*E)(nil)).Elem()

	tm.mu.Lock()
	defer tm.mu.Unlock()

	// Check whether this type is already mapped.
	if existing, ok := tm.toName[t]; ok {
		if existing != name {
			return fmt.Errorf("codec: type %s is already registered as %q, cannot register as %q", t, existing, name)
		}
		// Same type, same name — idempotent.
		return nil
	}

	// Check whether this name is already mapped to a different type.
	if existingType, ok := tm.fromName[name]; ok {
		if existingType != t {
			return fmt.Errorf("codec: name %q is already registered for type %s, cannot register for %s", name, existingType, t)
		}
		// Should not reach here (covered above), but guard anyway.
		return nil
	}

	tm.toName[t] = name
	tm.fromName[name] = t
	return nil
}

// TypeName returns the registered event type name for the given event value.
// The value may be a pointer or non-pointer; both are resolved to the base type.
// Returns an error if the type has not been registered.
func (tm *TypeMap) TypeName(event any) (string, error) {
	t := reflect.TypeOf(event)
	// Dereference pointer(s) so that both *T and T resolve to the same entry.
	for t != nil && t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t == nil {
		return "", fmt.Errorf("codec: cannot determine type of nil event")
	}

	tm.mu.RLock()
	name, ok := tm.toName[t]
	tm.mu.RUnlock()

	if !ok {
		return "", fmt.Errorf("codec: type %s is not registered", t)
	}
	return name, nil
}

// NewInstance creates and returns a new zero-value pointer of the type
// registered under name. This is used by decoders to unmarshal events into
// the correct Go type.
// Returns an error if name has not been registered.
func (tm *TypeMap) NewInstance(name string) (any, error) {
	tm.mu.RLock()
	t, ok := tm.fromName[name]
	tm.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("codec: name %q is not registered", name)
	}
	// reflect.New(t) returns a Value of type *T; .Interface() boxes it as any.
	return reflect.New(t).Interface(), nil
}
