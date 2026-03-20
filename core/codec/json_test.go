package codec_test

import (
	"encoding/json"
	"testing"

	"github.com/eventuous/eventuous-go/core/codec"
)

type sampleEvent struct {
	Name  string `json:"name"`
	Value int    `json:"value"`
}

// TestJSONCodec_Encode verifies that Encode produces valid JSON, the correct
// event type name, and the "application/json" content type.
func TestJSONCodec_Encode(t *testing.T) {
	tm := codec.NewTypeMap()
	if err := codec.Register[sampleEvent](tm, "SampleEvent"); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	c := codec.NewJSON(tm)
	evt := sampleEvent{Name: "hello", Value: 42}

	data, eventType, contentType, err := c.Encode(evt)
	if err != nil {
		t.Fatalf("Encode returned error: %v", err)
	}

	if eventType != "SampleEvent" {
		t.Errorf("eventType = %q, want %q", eventType, "SampleEvent")
	}
	if contentType != "application/json" {
		t.Errorf("contentType = %q, want %q", contentType, "application/json")
	}

	// Verify data is valid JSON matching the struct.
	var decoded sampleEvent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("data is not valid JSON: %v", err)
	}
	if decoded != evt {
		t.Errorf("decoded = %+v, want %+v", decoded, evt)
	}
}

// TestJSONCodec_Decode verifies that Decode returns the original struct value
// (not a pointer) when given valid JSON and a registered type name.
func TestJSONCodec_Decode(t *testing.T) {
	tm := codec.NewTypeMap()
	if err := codec.Register[sampleEvent](tm, "SampleEvent"); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	c := codec.NewJSON(tm)
	data := []byte(`{"name":"world","value":99}`)

	got, err := c.Decode(data, "SampleEvent")
	if err != nil {
		t.Fatalf("Decode returned error: %v", err)
	}

	want := sampleEvent{Name: "world", Value: 99}
	gotEvt, ok := got.(sampleEvent)
	if !ok {
		t.Fatalf("Decode returned %T, want sampleEvent (not a pointer)", got)
	}
	if gotEvt != want {
		t.Errorf("Decode = %+v, want %+v", gotEvt, want)
	}
}

// TestJSONCodec_RoundTrip verifies that encoding then decoding produces a value
// equal to the original.
func TestJSONCodec_RoundTrip(t *testing.T) {
	tm := codec.NewTypeMap()
	if err := codec.Register[sampleEvent](tm, "SampleEvent"); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	c := codec.NewJSON(tm)
	original := sampleEvent{Name: "roundtrip", Value: 7}

	data, eventType, _, err := c.Encode(original)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	got, err := c.Decode(data, eventType)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	gotEvt, ok := got.(sampleEvent)
	if !ok {
		t.Fatalf("Decode returned %T, want sampleEvent", got)
	}
	if gotEvt != original {
		t.Errorf("RoundTrip = %+v, want %+v", gotEvt, original)
	}
}

// TestJSONCodec_Encode_UnregisteredType verifies that Encode returns an error
// when the event type has not been registered in the TypeMap.
func TestJSONCodec_Encode_UnregisteredType(t *testing.T) {
	tm := codec.NewTypeMap()
	c := codec.NewJSON(tm)

	_, _, _, err := c.Encode(sampleEvent{Name: "nope", Value: 0})
	if err == nil {
		t.Fatal("expected error for unregistered type, got nil")
	}
}

// TestJSONCodec_Decode_UnknownType verifies that Decode returns an error when
// the event type name is not registered in the TypeMap.
func TestJSONCodec_Decode_UnknownType(t *testing.T) {
	tm := codec.NewTypeMap()
	c := codec.NewJSON(tm)

	_, err := c.Decode([]byte(`{}`), "UnknownEvent")
	if err == nil {
		t.Fatal("expected error for unknown type name, got nil")
	}
}

// TestJSONCodec_Decode_InvalidJSON verifies that Decode returns an error when
// the provided bytes are not valid JSON.
func TestJSONCodec_Decode_InvalidJSON(t *testing.T) {
	tm := codec.NewTypeMap()
	if err := codec.Register[sampleEvent](tm, "SampleEvent"); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	c := codec.NewJSON(tm)

	_, err := c.Decode([]byte(`not-valid-json`), "SampleEvent")
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}
