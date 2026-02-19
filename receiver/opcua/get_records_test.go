// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package opcua

import (
	"testing"
	"time"

	"github.com/gopcua/opcua/ua"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func newTestClient() *opcuaClient {
	return &opcuaClient{
		config: &Config{
			Filter: FilterConfig{MinSeverity: "Info"},
		},
		logger: zap.NewNop(),
	}
}

func TestParseLogRecordFromExtensionObject_ValidLogRecord(t *testing.T) {
	c := newTestClient()

	lr := &LogRecordExtObj{
		Time:       time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC),
		Severity:   300,
		Message:    "Configuration loaded successfully",
		SourceName: "SystemComponent",
	}

	obj := &ua.ExtensionObject{
		TypeID: &ua.ExpandedNodeID{NodeID: LogRecordExtObjTypeID},
		Value:  lr,
	}

	record, err := c.parseLogRecordFromExtensionObject(obj)
	require.NoError(t, err)

	assert.True(t, lr.Time.Equal(record.Timestamp))
	assert.Equal(t, uint16(300), record.Severity)
	assert.Equal(t, "Critical", severityToText(record.Severity))
	assert.Equal(t, "Configuration loaded successfully", record.Message)
	assert.Equal(t, "SystemComponent", record.SourceName)
	assert.NotNil(t, record.Attributes)
}

func TestParseLogRecordFromExtensionObject_NilValue(t *testing.T) {
	c := newTestClient()

	obj := &ua.ExtensionObject{
		TypeID: &ua.ExpandedNodeID{NodeID: ua.NewNumericNodeID(0, 9999)},
		Value:  nil,
	}

	_, err := c.parseLogRecordFromExtensionObject(obj)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ExtensionObject Value is nil")
}

func TestParseLogRecordFromExtensionObject_UnknownValueType(t *testing.T) {
	c := newTestClient()

	obj := &ua.ExtensionObject{
		TypeID: &ua.ExpandedNodeID{NodeID: ua.NewNumericNodeID(0, 1234)},
		Value:  "unexpected string value",
	}

	_, err := c.parseLogRecordFromExtensionObject(obj)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported ExtensionObject value type")
}

func TestParseExtensionObjectArray(t *testing.T) {
	c := newTestClient()

	objects := []*ua.ExtensionObject{
		{
			TypeID: &ua.ExpandedNodeID{NodeID: LogRecordExtObjTypeID},
			Value: &LogRecordExtObj{
				Time:       time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC),
				Severity:   150,
				Message:    "First record",
				SourceName: "Source1",
			},
		},
		nil, // nil entries should be skipped
		{
			TypeID: &ua.ExpandedNodeID{NodeID: LogRecordExtObjTypeID},
			Value: &LogRecordExtObj{
				Time:       time.Date(2025, 1, 15, 10, 1, 0, 0, time.UTC),
				Severity:   700,
				Message:    "Second record",
				SourceName: "Source2",
			},
		},
	}

	records, err := c.parseExtensionObjectArray(objects)
	require.NoError(t, err)
	assert.Len(t, records, 2)

	assert.Equal(t, "First record", records[0].Message)
	assert.Equal(t, "Source1", records[0].SourceName)

	assert.Equal(t, "Second record", records[1].Message)
	assert.Equal(t, "Source2", records[1].SourceName)
}

func TestParseExtensionObjectArray_WithFailedEntries(t *testing.T) {
	c := newTestClient()

	objects := []*ua.ExtensionObject{
		{
			TypeID: &ua.ExpandedNodeID{NodeID: LogRecordExtObjTypeID},
			Value: &LogRecordExtObj{
				Time:       time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC),
				Severity:   300,
				Message:    "Good record",
				SourceName: "Source",
			},
		},
		{
			// This one has nil Value, will fail but should be skipped gracefully
			TypeID: &ua.ExpandedNodeID{NodeID: ua.NewNumericNodeID(0, 9999)},
			Value:  nil,
		},
	}

	records, err := c.parseExtensionObjectArray(objects)
	require.NoError(t, err)
	assert.Len(t, records, 1)
	assert.Equal(t, "Good record", records[0].Message)
}

func TestParseLogRecordsDataType_ExtensionObjects(t *testing.T) {
	c := newTestClient()

	lr := &LogRecordExtObj{
		Time:       time.Date(2025, 1, 15, 10, 5, 0, 0, time.UTC),
		Severity:   500,
		Message:    "High memory usage",
		SourceName: "SystemComponent",
	}

	extObjs := []*ua.ExtensionObject{
		{
			TypeID: &ua.ExpandedNodeID{NodeID: LogRecordExtObjTypeID},
			Value:  lr,
		},
	}

	variant := ua.MustVariant(extObjs)
	records, err := c.parseLogRecordsDataType(variant)
	require.NoError(t, err)
	assert.Len(t, records, 1)
	assert.Equal(t, "High memory usage", records[0].Message)
}

func TestParseLogRecordsDataType_Nil(t *testing.T) {
	c := newTestClient()

	records, err := c.parseLogRecordsDataType(nil)
	require.NoError(t, err)
	assert.Empty(t, records)
}

func TestSeverityToText(t *testing.T) {
	// OPC UA Part 26 §5.4 Table 5
	tests := []struct {
		severity uint16
		expected string
	}{
		// Debug: 1–50
		{1, "Debug"},
		{25, "Debug"},
		{50, "Debug"},
		// Information: 51–100
		{51, "Information"},
		{75, "Information"},
		{100, "Information"},
		// Notice: 101–150
		{101, "Notice"},
		{125, "Notice"},
		{150, "Notice"},
		// Warning: 151–200
		{151, "Warning"},
		{175, "Warning"},
		{200, "Warning"},
		// Error: 201–250
		{201, "Error"},
		{225, "Error"},
		{250, "Error"},
		// Critical: 251–300
		{251, "Critical"},
		{275, "Critical"},
		{300, "Critical"},
		// Alert: 301–400
		{301, "Alert"},
		{350, "Alert"},
		{400, "Alert"},
		// Emergency: 401–1000
		{401, "Emergency"},
		{700, "Emergency"},
		{1000, "Emergency"},
		// Unspecified
		{0, "Unspecified"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := severityToText(tt.severity)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetMinSeverityValue(t *testing.T) {
	tests := []struct {
		severity string
		expected uint16
	}{
		{"Trace", 51},
		{"Debug", 1},
		{"Info", 101},
		{"Warn", 201},
		{"Warning", 201},
		{"Error", 301},
		{"Fatal", 401},
		{"Emergency", 401},
		{"Unknown", 101}, // default
		{"", 101},        // default
	}

	for _, tt := range tests {
		t.Run(tt.severity, func(t *testing.T) {
			c := &opcuaClient{
				config: &Config{Filter: FilterConfig{MinSeverity: tt.severity}},
			}
			result := c.getMinSeverityValue()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// --- logRecordExtObjToRecord ---

func TestLogRecordExtObjToRecord_BasicFields(t *testing.T) {
	lr := &LogRecordExtObj{
		Time:       time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC),
		Severity:   300,
		Message:    "Test message",
		SourceName: "SystemComponent",
		SourceNode: ua.NewNumericNodeID(1, 100),
	}

	record := logRecordExtObjToRecord(lr)

	assert.True(t, lr.Time.Equal(record.Timestamp))
	assert.Equal(t, uint16(300), record.Severity)
	assert.Equal(t, "Test message", record.Message)
	assert.Equal(t, "SystemComponent", record.SourceName)
	assert.Equal(t, uint16(1), record.SourceNamespace)
	assert.Equal(t, "Numeric", record.SourceIDType)
	assert.Equal(t, "100", record.SourceID)
	assert.NotNil(t, record.Attributes)
}

func TestLogRecordExtObjToRecord_WithTraceContext(t *testing.T) {
	lr := &LogRecordExtObj{
		Time:         time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC),
		Severity:     150,
		Message:      "Traced record",
		TraceIDBytes: fixedTraceIDBytes(),
		SpanID:       0x0102030405060708,
		ParentSpanID: 0,
	}

	record := logRecordExtObjToRecord(lr)

	assert.Equal(t, "0102030405060708090a0b0c0d0e0f10", record.TraceID)
	assert.Equal(t, "0102030405060708", record.SpanID)
	assert.Equal(t, byte(0x01), record.TraceFlags)
}

func TestLogRecordExtObjToRecord_NoTraceContext(t *testing.T) {
	// SpanID == 0 means no trace context
	lr := &LogRecordExtObj{
		Time:     time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC),
		Severity: 150,
		Message:  "No trace",
		SpanID:   0,
	}

	record := logRecordExtObjToRecord(lr)

	assert.Empty(t, record.TraceID)
	assert.Empty(t, record.SpanID)
	assert.Equal(t, byte(0), record.TraceFlags)
}

func TestLogRecordExtObjToRecord_WithAdditionalData(t *testing.T) {
	lr := &LogRecordExtObj{
		Time:     time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC),
		Severity: 150,
		Message:  "Data record",
		AdditionalData: map[string]interface{}{
			"sensor_id": "temp-01",
			"value":     "22.5",
		},
	}

	record := logRecordExtObjToRecord(lr)

	assert.Equal(t, "temp-01", record.Attributes["sensor_id"])
	assert.Equal(t, "22.5", record.Attributes["value"])
}

// --- parseLogRecordFromExtensionObject with new fields ---

func TestParseLogRecordFromExtensionObject_WithTraceContext(t *testing.T) {
	c := newTestClient()

	lr := &LogRecordExtObj{
		Time:         time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC),
		Severity:     700,
		Message:      "Connection timeout",
		SourceName:   "NetworkModule",
		SourceNode:   ua.NewNumericNodeID(1, 102),
		TraceIDBytes: fixedTraceIDBytes(),
		SpanID:       0xdeadbeefcafe0000,
		ParentSpanID: 0x0102030405060708,
		AdditionalData: map[string]interface{}{
			"service": "external-api",
		},
	}

	obj := &ua.ExtensionObject{
		TypeID: &ua.ExpandedNodeID{NodeID: LogRecordExtObjTypeID},
		Value:  lr,
	}

	record, err := c.parseLogRecordFromExtensionObject(obj)
	require.NoError(t, err)

	assert.Equal(t, "Connection timeout", record.Message)
	assert.Equal(t, "NetworkModule", record.SourceName)
	assert.Equal(t, uint16(1), record.SourceNamespace)
	assert.Equal(t, "102", record.SourceID)

	assert.Equal(t, "0102030405060708090a0b0c0d0e0f10", record.TraceID)
	assert.Equal(t, "deadbeefcafe0000", record.SpanID)
	assert.Equal(t, byte(0x01), record.TraceFlags)

	assert.Equal(t, "external-api", record.Attributes["service"])
}

func TestParseLogRecordFromExtensionObject_BinaryFallback(t *testing.T) {
	c := newTestClient()

	// Build a LogRecordExtObj, encode it to raw bytes, and wrap in ExtensionObject
	// with a different TypeID to trigger the binary-fallback path.
	lr := &LogRecordExtObj{
		Time:         time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC),
		Severity:     300,
		Message:      "Fallback test",
		SourceName:   "FallbackSource",
		SourceNode:   ua.NewNumericNodeID(1, 200),
		TraceIDBytes: fixedTraceIDBytes(),
		SpanID:       0x0102030405060708,
	}

	raw, err := lr.Encode()
	require.NoError(t, err)

	obj := &ua.ExtensionObject{
		TypeID: &ua.ExpandedNodeID{NodeID: ua.NewNumericNodeID(0, 9999)}, // unknown type → raw bytes
		Value:  raw,
	}

	record, err := c.parseLogRecordFromExtensionObject(obj)
	require.NoError(t, err)

	assert.Equal(t, "Fallback test", record.Message)
	assert.Equal(t, "FallbackSource", record.SourceName)
	assert.Equal(t, "0102030405060708090a0b0c0d0e0f10", record.TraceID)
	assert.Equal(t, "0102030405060708", record.SpanID)
	assert.Equal(t, byte(0x01), record.TraceFlags)
}
