package codec

// Codec encodes and decodes events for storage.
type Codec interface {
	Encode(event any) (data []byte, eventType string, contentType string, err error)
	Decode(data []byte, eventType string) (event any, err error)
}
