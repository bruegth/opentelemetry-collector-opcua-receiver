// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

// Package testserver provides test utilities for the OPC UA receiver
package testdata

import "time"

// OPCUALogRecord represents a log record from an OPC UA server
// This is a simplified representation for testing purposes
type OPCUALogRecord struct {
	Timestamp       time.Time
	Severity        uint16
	Message         string
	SourceName      string // opcua.source.name: human-readable name of the log source
	SourceNamespace uint16 // opcua.source.namespace: NodeId namespace index
	SourceIDType    string // opcua.source.id_type: NodeId identifier type ("Numeric", "String", "Guid", "Opaque")
	SourceID        string // opcua.source.id: NodeId identifier value
	TraceID         string // 32-character hex string
	SpanID          string // 16-character hex string
	TraceFlags      byte
	Attributes      map[string]interface{}
}

// TraceContext represents trace context from OPC UA
type TraceContext struct {
	TraceID string
	SpanID  string
	Flags   byte
}
