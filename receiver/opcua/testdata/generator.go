// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

// Package testserver provides test utilities for the OPC UA receiver
package testdata

import (
	"fmt"
	"math/rand"
	"time"
)

// severities lists one representative value per Part 26 §5.4 level (Table 5).
var severities = []uint16{25, 75, 125, 175, 225, 275, 350, 500}

// sourceNodes maps source names to their NodeId components (namespace, numeric id).
// Namespace 1 corresponds to the test server's custom namespace.
var sourceNodes = []struct {
	Name      string
	Namespace uint16
	ID        uint32
}{
	{"SystemComponent", 1, 100},
	{"DeviceDriver", 1, 101},
	{"NetworkModule", 1, 102},
	{"DataLogger", 1, 103},
	{"SecurityModule", 1, 104},
}

var messages = []string{
	"Operation completed successfully",
	"Connection established",
	"Data processing started",
	"Configuration updated",
	"Sensor reading collected",
	"Warning: high memory usage",
	"Error: connection timeout",
	"Critical: system overload",
}

// GenerateSampleLogRecord creates a random log record for testing
func GenerateSampleLogRecord(seed int) OPCUALogRecord {
	r := rand.New(rand.NewSource(time.Now().UnixNano() + int64(seed)))

	severity := severities[r.Intn(len(severities))]

	src := sourceNodes[r.Intn(len(sourceNodes))]
	return OPCUALogRecord{
		Timestamp:       time.Now().Add(-time.Duration(r.Intn(3600)) * time.Second),
		Severity:        severity,
		Message:         messages[r.Intn(len(messages))],
		SourceName:      src.Name,
		SourceNamespace: src.Namespace,
		SourceIDType:    "Numeric",
		SourceID:        fmt.Sprintf("%d", src.ID),
		TraceID:         fmt.Sprintf("%032x", r.Int63()),
		SpanID:          fmt.Sprintf("%016x", r.Int63()),
		TraceFlags:      byte(r.Intn(2)), // 0 or 1
		Attributes: map[string]interface{}{
			"component": "test",
			"version":   "1.0.0",
			"index":     seed,
		},
	}
}

// GenerateLogRecordWithDetails creates a log record with specific values.
// sourceName is used as opcua.source.name; namespace and numeric id default to 1/100.
func GenerateLogRecordWithDetails(timestamp time.Time, severity uint16, message, sourceName string) OPCUALogRecord {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	return OPCUALogRecord{
		Timestamp:       timestamp,
		Severity:        severity,
		Message:         message,
		SourceName:      sourceName,
		SourceNamespace: 1,
		SourceIDType:    "Numeric",
		SourceID:        "100",
		TraceID:         fmt.Sprintf("%032x", r.Int63()),
		SpanID:          fmt.Sprintf("%016x", r.Int63()),
		TraceFlags:      1,
		Attributes: map[string]interface{}{
			"test": true,
		},
	}
}

// GenerateSampleLogRecords creates multiple random log records
func GenerateSampleLogRecords(count int) []OPCUALogRecord {
	records := make([]OPCUALogRecord, count)
	for i := 0; i < count; i++ {
		records[i] = GenerateSampleLogRecord(i)
	}
	return records
}
