// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package opcua

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/plog"

	"github.com/bruegth/opentelemetry-collector-opcua-receiver/receiver/opcua/testdata"
)

func TestTransformLogs(t *testing.T) {
	transformer := NewTransformer("opc.tcp://test:4840")

	timestamp := time.Now()
	opcuaRecords := []testdata.OPCUALogRecord{
		{
			Timestamp:    timestamp,
			Severity:     300,
			SeverityText: "Info",
			Message:      "Test message 1",
			Source:       "TestSource",
			TraceID:      "0123456789abcdef0123456789abcdef",
			SpanID:       "0123456789abcdef",
			TraceFlags:   1,
			Attributes: map[string]interface{}{
				"key1": "value1",
				"key2": 42,
			},
		},
		{
			Timestamp:    timestamp.Add(time.Second),
			Severity:     700,
			SeverityText: "Error",
			Message:      "Test error message",
			Source:       "ErrorSource",
			TraceID:      "fedcba9876543210fedcba9876543210",
			SpanID:       "fedcba9876543210",
			TraceFlags:   0,
			Attributes: map[string]interface{}{
				"error_code": 500,
			},
		},
	}

	logs := transformer.TransformLogs(opcuaRecords)

	require.Equal(t, 1, logs.ResourceLogs().Len())

	resourceLogs := logs.ResourceLogs().At(0)

	// Check resource attributes
	resource := resourceLogs.Resource()
	endpointAttr, ok := resource.Attributes().Get("opcua.server.endpoint")
	require.True(t, ok)
	assert.Equal(t, "opc.tcp://test:4840", endpointAttr.Str())

	serviceNameAttr, ok := resource.Attributes().Get("service.name")
	require.True(t, ok)
	assert.Equal(t, "opcua-server", serviceNameAttr.Str())

	// Check scope logs
	require.Equal(t, 1, resourceLogs.ScopeLogs().Len())
	scopeLogs := resourceLogs.ScopeLogs().At(0)
	assert.Equal(t, "github.com/bruegth/opentelemetry-collector-opcua-receiver", scopeLogs.Scope().Name())

	// Check log records
	require.Equal(t, 2, scopeLogs.LogRecords().Len())

	// First log record
	logRecord1 := scopeLogs.LogRecords().At(0)
	assert.Equal(t, plog.SeverityNumberInfo, logRecord1.SeverityNumber())
	assert.Equal(t, "Info", logRecord1.SeverityText())
	assert.Equal(t, "Test message 1", logRecord1.Body().Str())

	sourceAttr, ok := logRecord1.Attributes().Get("opcua.source")
	require.True(t, ok)
	assert.Equal(t, "TestSource", sourceAttr.Str())

	key1Attr, ok := logRecord1.Attributes().Get("key1")
	require.True(t, ok)
	assert.Equal(t, "value1", key1Attr.Str())

	key2Attr, ok := logRecord1.Attributes().Get("key2")
	require.True(t, ok)
	assert.Equal(t, int64(42), key2Attr.Int())

	// Second log record
	logRecord2 := scopeLogs.LogRecords().At(1)
	assert.Equal(t, plog.SeverityNumberError, logRecord2.SeverityNumber())
	assert.Equal(t, "Error", logRecord2.SeverityText())
	assert.Equal(t, "Test error message", logRecord2.Body().Str())
}

func TestTransformLogsEmpty(t *testing.T) {
	transformer := NewTransformer("opc.tcp://test:4840")

	logs := transformer.TransformLogs([]testdata.OPCUALogRecord{})

	assert.Equal(t, 0, logs.ResourceLogs().Len())
}

func TestMapSeverity(t *testing.T) {
	transformer := NewTransformer("opc.tcp://test:4840")

	tests := []struct {
		opcuaSeverity    uint16
		expectedSeverity plog.SeverityNumber
	}{
		{50, plog.SeverityNumberTrace},
		{99, plog.SeverityNumberTrace},
		{100, plog.SeverityNumberDebug},
		{150, plog.SeverityNumberDebug},
		{199, plog.SeverityNumberDebug},
		{200, plog.SeverityNumberInfo},
		{300, plog.SeverityNumberInfo},
		{399, plog.SeverityNumberInfo},
		{400, plog.SeverityNumberWarn},
		{500, plog.SeverityNumberWarn},
		{599, plog.SeverityNumberWarn},
		{600, plog.SeverityNumberError},
		{700, plog.SeverityNumberError},
		{799, plog.SeverityNumberError},
		{800, plog.SeverityNumberFatal},
		{900, plog.SeverityNumberFatal},
		{1000, plog.SeverityNumberFatal},
	}

	for _, tt := range tests {
		t.Run(string(rune(tt.opcuaSeverity)), func(t *testing.T) {
			result := transformer.mapSeverity(tt.opcuaSeverity)
			assert.Equal(t, tt.expectedSeverity, result)
		})
	}
}

func TestSetTraceContext(t *testing.T) {
	transformer := NewTransformer("opc.tcp://test:4840")

	opcuaRecord := testdata.OPCUALogRecord{
		Timestamp:    time.Now(),
		Severity:     300,
		SeverityText: "Info",
		Message:      "Test message with trace",
		TraceID:      "0123456789abcdef0123456789abcdef",
		SpanID:       "0123456789abcdef",
		TraceFlags:   1,
	}

	logs := transformer.TransformLogs([]testdata.OPCUALogRecord{opcuaRecord})

	require.Equal(t, 1, logs.LogRecordCount())
	logRecord := logs.ResourceLogs().At(0).ScopeLogs().At(0).LogRecords().At(0)

	// Verify trace ID is set
	traceID := logRecord.TraceID()
	assert.NotEqual(t, [16]byte{}, traceID)

	// Verify span ID is set
	spanID := logRecord.SpanID()
	assert.NotEqual(t, [8]byte{}, spanID)

	// Verify sampled flag is set
	flags := logRecord.Flags()
	assert.True(t, flags.IsSampled())
}

func TestPutAttribute(t *testing.T) {
	transformer := NewTransformer("opc.tcp://test:4840")

	opcuaRecord := testdata.OPCUALogRecord{
		Timestamp:    time.Now(),
		Severity:     300,
		SeverityText: "Info",
		Message:      "Test message",
		Attributes: map[string]interface{}{
			"string_attr": "value",
			"int_attr":    42,
			"int64_attr":  int64(100),
			"float_attr":  3.14,
			"bool_attr":   true,
		},
	}

	logs := transformer.TransformLogs([]testdata.OPCUALogRecord{opcuaRecord})

	logRecord := logs.ResourceLogs().At(0).ScopeLogs().At(0).LogRecords().At(0)
	attrs := logRecord.Attributes()

	// Verify string attribute
	val, ok := attrs.Get("string_attr")
	require.True(t, ok)
	assert.Equal(t, "value", val.Str())

	// Verify int attribute
	val, ok = attrs.Get("int_attr")
	require.True(t, ok)
	assert.Equal(t, int64(42), val.Int())

	// Verify int64 attribute
	val, ok = attrs.Get("int64_attr")
	require.True(t, ok)
	assert.Equal(t, int64(100), val.Int())

	// Verify float attribute
	val, ok = attrs.Get("float_attr")
	require.True(t, ok)
	assert.Equal(t, 3.14, val.Double())

	// Verify bool attribute
	val, ok = attrs.Get("bool_attr")
	require.True(t, ok)
	assert.Equal(t, true, val.Bool())
}

func TestGenerateSampleLogRecord(t *testing.T) {
	record := testdata.GenerateSampleLogRecord(1)

	assert.NotEmpty(t, record.Message)
	assert.NotEmpty(t, record.Source)
	assert.NotEmpty(t, record.SeverityText)
	assert.Greater(t, record.Severity, uint16(0))
	assert.NotEmpty(t, record.TraceID)
	assert.NotEmpty(t, record.SpanID)
	assert.NotNil(t, record.Attributes)
}

func TestGenerateLogRecordWithDetails(t *testing.T) {
	timestamp := time.Now()
	record := testdata.GenerateLogRecordWithDetails(timestamp, 500, "Custom message", "CustomSource")

	assert.Equal(t, timestamp, record.Timestamp)
	assert.Equal(t, uint16(500), record.Severity)
	assert.Equal(t, "Custom message", record.Message)
	assert.Equal(t, "CustomSource", record.Source)
	assert.Equal(t, "Warn", record.SeverityText)
}
