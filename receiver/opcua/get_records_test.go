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
	assert.Equal(t, "Warning", record.SeverityText)
	assert.Equal(t, "Configuration loaded successfully", record.Message)
	assert.Equal(t, "SystemComponent", record.Source)
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
	assert.Equal(t, "Source1", records[0].Source)

	assert.Equal(t, "Second record", records[1].Message)
	assert.Equal(t, "Source2", records[1].Source)
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
	c := newTestClient()

	tests := []struct {
		severity uint16
		expected string
	}{
		{1, "Debug"},
		{50, "Debug"},
		{51, "Trace"},
		{100, "Trace"},
		{101, "Info"},
		{200, "Info"},
		{201, "Warning"},
		{300, "Warning"},
		{301, "Error"},
		{400, "Error"},
		{401, "Emergency"},
		{1000, "Emergency"},
		{0, "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := c.severityToText(tt.severity)
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
