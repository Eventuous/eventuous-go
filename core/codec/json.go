// Copyright (C) Eventuous HQ OÜ. All rights reserved
// Licensed under the Apache License, Version 2.0.

package codec

import (
	"encoding/json"
	"reflect"
)

// JSONCodec serializes events as JSON using a TypeMap for type resolution.
type JSONCodec struct {
	types *TypeMap
}

// NewJSON creates a JSON codec backed by the given type map.
func NewJSON(types *TypeMap) *JSONCodec {
	return &JSONCodec{types: types}
}

// Encode serializes an event to JSON. Returns the JSON bytes, the event type
// name (from TypeMap), and content type "application/json".
func (c *JSONCodec) Encode(event any) ([]byte, string, string, error) {
	eventType, err := c.types.TypeName(event)
	if err != nil {
		return nil, "", "", err
	}

	data, err := json.Marshal(event)
	if err != nil {
		return nil, "", "", err
	}

	return data, eventType, "application/json", nil
}

// Decode deserializes JSON bytes into the Go type registered under eventType.
// Uses TypeMap.NewInstance to create the target, json.Unmarshal into it,
// then dereferences the pointer to return a value (not pointer).
func (c *JSONCodec) Decode(data []byte, eventType string) (any, error) {
	ptr, err := c.types.NewInstance(eventType)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(data, ptr); err != nil {
		return nil, err
	}

	// NewInstance returns a *T; dereference to return T (value, not pointer).
	return reflect.ValueOf(ptr).Elem().Interface(), nil
}
