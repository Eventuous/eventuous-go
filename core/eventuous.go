package eventuous

import (
	"errors"
	"strings"
)

// StreamName is a typed string representing an event stream, following the
// "{Category}-{ID}" naming convention used by Eventuous.
type StreamName string

// NewStreamName creates a StreamName from a category and an ID, joining them
// with a "-" separator.
func NewStreamName(category, id string) StreamName {
	return StreamName(category + "-" + id)
}

// Category returns the part of the stream name before the first "-".
// If there is no "-", the entire name is returned as the category.
func (s StreamName) Category() string {
	raw := string(s)
	idx := strings.Index(raw, "-")
	if idx < 0 {
		return raw
	}
	return raw[:idx]
}

// ID returns the part of the stream name after the first "-".
// If there is no "-", an empty string is returned.
func (s StreamName) ID() string {
	raw := string(s)
	idx := strings.Index(raw, "-")
	if idx < 0 {
		return ""
	}
	return raw[idx+1:]
}

// String returns the underlying string value of the StreamName.
func (s StreamName) String() string {
	return string(s)
}

// ExpectedVersion is used for optimistic concurrency control when appending
// events to a stream.
type ExpectedVersion int64

const (
	// VersionNoStream indicates the stream must not exist when appending.
	VersionNoStream ExpectedVersion = -1
	// VersionAny disables optimistic concurrency checks entirely.
	VersionAny ExpectedVersion = -2
)

// ExpectedState controls how a command service loads existing state before
// executing a command.
type ExpectedState int

const (
	// IsNew requires the stream to not yet exist.
	IsNew ExpectedState = iota
	// IsExisting requires the stream to already exist.
	IsExisting
	// IsAny allows the stream to either exist or not.
	IsAny
)

// Metadata carries event metadata such as correlation and causation identifiers
// as well as arbitrary custom headers.
type Metadata map[string]any

// Well-known metadata key constants.
const (
	MetaCorrelationID = "eventuous.correlation-id"
	MetaCausationID   = "eventuous.causation-id"
	MetaMessageID     = "eventuous.message-id"
)

// clone returns a shallow copy of the Metadata map.
func (m Metadata) clone() Metadata {
	out := make(Metadata, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// CorrelationID returns the correlation ID stored in the metadata, or an empty
// string if it is not set.
func (m Metadata) CorrelationID() string {
	if m == nil {
		return ""
	}
	v, _ := m[MetaCorrelationID].(string)
	return v
}

// WithCorrelationID returns a new Metadata with the given correlation ID set.
// The original Metadata is not modified.
func (m Metadata) WithCorrelationID(id string) Metadata {
	out := m.clone()
	out[MetaCorrelationID] = id
	return out
}

// CausationID returns the causation ID stored in the metadata, or an empty
// string if it is not set.
func (m Metadata) CausationID() string {
	if m == nil {
		return ""
	}
	v, _ := m[MetaCausationID].(string)
	return v
}

// WithCausationID returns a new Metadata with the given causation ID set.
// The original Metadata is not modified.
func (m Metadata) WithCausationID(id string) Metadata {
	out := m.clone()
	out[MetaCausationID] = id
	return out
}

// Sentinel errors returned by eventuous operations.
var (
	// ErrStreamNotFound is returned when a requested stream does not exist.
	ErrStreamNotFound = errors.New("eventuous: stream not found")
	// ErrOptimisticConcurrency is returned when an append fails due to a
	// version mismatch.
	ErrOptimisticConcurrency = errors.New("eventuous: wrong expected version")
	// ErrAggregateNotFound is returned when an aggregate cannot be loaded.
	ErrAggregateNotFound = errors.New("eventuous: aggregate not found")
	// ErrHandlerNotFound is returned when no handler is registered for a command.
	ErrHandlerNotFound = errors.New("eventuous: command handler not found")
)
