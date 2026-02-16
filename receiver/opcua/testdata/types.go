// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

// Package testserver provides test utilities for the OPC UA receiver
package testdata

import "time"

// OPCUALogRecord represents a log record from an OPC UA server
// This is a simplified representation for testing purposes
type OPCUALogRecord struct {
	Timestamp    time.Time
	Severity     uint16
	SeverityText string
	Message      string
	Source       string
	TraceID      string // 32-character hex string
	SpanID       string // 16-character hex string
	TraceFlags   byte
	Attributes   map[string]interface{}
}

// TraceContext represents trace context from OPC UA
type TraceContext struct {
	TraceID string
	SpanID  string
	Flags   byte
}
