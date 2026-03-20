package eventuous_test

import (
	"testing"

	eventuous "github.com/eventuous/eventuous-go/core"
)

// TestStreamName_CategoryAndID verifies the basic StreamName construction and accessors.
func TestStreamName_CategoryAndID(t *testing.T) {
	sn := eventuous.NewStreamName("Booking", "123")

	if got := sn.Category(); got != "Booking" {
		t.Errorf("Category() = %q, want %q", got, "Booking")
	}
	if got := sn.ID(); got != "123" {
		t.Errorf("ID() = %q, want %q", got, "123")
	}
	if got := sn.String(); got != "Booking-123" {
		t.Errorf("String() = %q, want %q", got, "Booking-123")
	}
}

// TestStreamName_IDWithDashes verifies that only the first dash is used as separator.
func TestStreamName_IDWithDashes(t *testing.T) {
	sn := eventuous.NewStreamName("Booking", "abc-def-ghi")

	if got := sn.Category(); got != "Booking" {
		t.Errorf("Category() = %q, want %q", got, "Booking")
	}
	if got := sn.ID(); got != "abc-def-ghi" {
		t.Errorf("ID() = %q, want %q", got, "abc-def-ghi")
	}
	if got := sn.String(); got != "Booking-abc-def-ghi" {
		t.Errorf("String() = %q, want %q", got, "Booking-abc-def-ghi")
	}
}

// TestStreamName_NoDash verifies behavior when the stream name contains no dash.
func TestStreamName_NoDash(t *testing.T) {
	sn := eventuous.StreamName("NoDash")

	if got := sn.Category(); got != "NoDash" {
		t.Errorf("Category() = %q, want %q", got, "NoDash")
	}
	if got := sn.ID(); got != "" {
		t.Errorf("ID() = %q, want %q", got, "")
	}
}

// TestMetadata_CorrelationAndCausation verifies set/get round-trips for both IDs.
func TestMetadata_CorrelationAndCausation(t *testing.T) {
	m := eventuous.Metadata{}

	m2 := m.WithCorrelationID("corr-001").WithCausationID("caus-002")

	if got := m2.CorrelationID(); got != "corr-001" {
		t.Errorf("CorrelationID() = %q, want %q", got, "corr-001")
	}
	if got := m2.CausationID(); got != "caus-002" {
		t.Errorf("CausationID() = %q, want %q", got, "caus-002")
	}
}

// TestMetadata_WithCorrelationID_DoesNotMutateOriginal verifies immutability of With... methods.
func TestMetadata_WithCorrelationID_DoesNotMutateOriginal(t *testing.T) {
	original := eventuous.Metadata{}
	_ = original.WithCorrelationID("corr-999")

	if got := original.CorrelationID(); got != "" {
		t.Errorf("original CorrelationID() = %q after With..., want %q", got, "")
	}

	// Also verify with a non-empty original
	m := eventuous.Metadata{}.WithCorrelationID("corr-001")
	_ = m.WithCorrelationID("corr-002")

	if got := m.CorrelationID(); got != "corr-001" {
		t.Errorf("m.CorrelationID() = %q after second With..., want %q", got, "corr-001")
	}
}

// TestMetadata_EmptyMetadata verifies that accessors on empty/nil metadata return "".
func TestMetadata_EmptyMetadata(t *testing.T) {
	var m eventuous.Metadata

	if got := m.CorrelationID(); got != "" {
		t.Errorf("CorrelationID() on nil = %q, want %q", got, "")
	}
	if got := m.CausationID(); got != "" {
		t.Errorf("CausationID() on nil = %q, want %q", got, "")
	}
}
