// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

// Package testserver provides test utilities for the OPC UA receiver
package testdata

import (
	"fmt"
	"math/rand"
	"time"
)

var severities = []struct {
	Level uint16
	Text  string
}{
	{50, "Trace"},
	{150, "Debug"},
	{300, "Info"},
	{500, "Warn"},
	{700, "Error"},
	{900, "Fatal"},
}

var sources = []string{
	"SystemComponent",
	"DeviceDriver",
	"NetworkModule",
	"DataLogger",
	"SecurityModule",
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

	return OPCUALogRecord{
		Timestamp:    time.Now().Add(-time.Duration(r.Intn(3600)) * time.Second),
		Severity:     severity.Level,
		SeverityText: severity.Text,
		Message:      messages[r.Intn(len(messages))],
		Source:       sources[r.Intn(len(sources))],
		TraceID:      fmt.Sprintf("%032x", r.Int63()),
		SpanID:       fmt.Sprintf("%016x", r.Int63()),
		TraceFlags:   byte(r.Intn(2)), // 0 or 1
		Attributes: map[string]interface{}{
			"component": "test",
			"version":   "1.0.0",
			"index":     seed,
		},
	}
}

// GenerateLogRecordWithDetails creates a log record with specific values
func GenerateLogRecordWithDetails(timestamp time.Time, severity uint16, message, source string) OPCUALogRecord {
	severityText := "Info"
	for _, s := range severities {
		if s.Level >= severity {
			severityText = s.Text
			break
		}
	}

	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	return OPCUALogRecord{
		Timestamp:    timestamp,
		Severity:     severity,
		SeverityText: severityText,
		Message:      message,
		Source:       source,
		TraceID:      fmt.Sprintf("%032x", r.Int63()),
		SpanID:       fmt.Sprintf("%016x", r.Int63()),
		TraceFlags:   1,
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
