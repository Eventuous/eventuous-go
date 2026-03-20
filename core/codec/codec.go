// Copyright (C) Eventuous HQ OÜ. All rights reserved
// Licensed under the Apache License, Version 2.0.

package codec

// Codec encodes and decodes events for storage.
type Codec interface {
	Encode(event any) (data []byte, eventType string, contentType string, err error)
	Decode(data []byte, eventType string) (event any, err error)
}
