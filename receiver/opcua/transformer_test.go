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
	transformer := NewTransformer("opc.tcp://test:4840", "opcua-server", "")

	timestamp := time.Now()
	opcuaRecords := []testdata.OPCUALogRecord{
		{
			Timestamp: timestamp,
			Severity:  300,

			Message:         "Test message 1",
			SourceName:      "TestSource",
			SourceNamespace: 1,
			SourceIDType:    "Numeric",
			SourceID:        "100",
			TraceID:         "0123456789abcdef0123456789abcdef",
			SpanID:          "0123456789abcdef",
			TraceFlags:      1,
			Attributes: map[string]interface{}{
				"key1": "value1",
				"key2": 42,
			},
		},
		{
			Timestamp: timestamp.Add(time.Second),
			Severity:  700,

			Message:         "Test error message",
			SourceName:      "ErrorSource",
			SourceNamespace: 2,
			SourceIDType:    "String",
			SourceID:        "ErrorDevice",
			TraceID:         "fedcba9876543210fedcba9876543210",
			SpanID:          "fedcba9876543210",
			TraceFlags:      0,
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
	serverAddrAttr, ok := resource.Attributes().Get("server.address")
	require.True(t, ok)
	assert.Equal(t, "test", serverAddrAttr.Str())

	serverPortAttr, ok := resource.Attributes().Get("server.port")
	require.True(t, ok)
	assert.Equal(t, int64(4840), serverPortAttr.Int())

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
	assert.Equal(t, plog.SeverityNumberError2, logRecord1.SeverityNumber())
	assert.Equal(t, "Critical", logRecord1.SeverityText())
	assert.Equal(t, "Test message 1", logRecord1.Body().Str())

	sourceNameAttr, ok := logRecord1.Attributes().Get("opcua.source.name")
	require.True(t, ok)
	assert.Equal(t, "TestSource", sourceNameAttr.Str())

	sourceNsAttr, ok := logRecord1.Attributes().Get("opcua.source.namespace")
	require.True(t, ok)
	assert.Equal(t, int64(1), sourceNsAttr.Int())

	sourceIDTypeAttr, ok := logRecord1.Attributes().Get("opcua.source.id_type")
	require.True(t, ok)
	assert.Equal(t, "Numeric", sourceIDTypeAttr.Str())

	sourceIDAttr, ok := logRecord1.Attributes().Get("opcua.source.id")
	require.True(t, ok)
	assert.Equal(t, "100", sourceIDAttr.Str())

	key1Attr, ok := logRecord1.Attributes().Get("key1")
	require.True(t, ok)
	assert.Equal(t, "value1", key1Attr.Str())

	key2Attr, ok := logRecord1.Attributes().Get("key2")
	require.True(t, ok)
	assert.Equal(t, int64(42), key2Attr.Int())

	// Second log record
	logRecord2 := scopeLogs.LogRecords().At(1)
	assert.Equal(t, plog.SeverityNumberFatal, logRecord2.SeverityNumber())
	assert.Equal(t, "Emergency", logRecord2.SeverityText())
	assert.Equal(t, "Test error message", logRecord2.Body().Str())
}

func TestTransformLogsResourceConfig(t *testing.T) {
	tests := []struct {
		name             string
		serviceName      string
		serviceNamespace string
		wantName         string
		wantNamespace    string
		hasNamespace     bool
	}{
		{
			name:         "custom service name",
			serviceName:  "my-plc-server",
			wantName:     "my-plc-server",
			hasNamespace: false,
		},
		{
			name:             "custom service name and namespace",
			serviceName:      "assembly-line",
			serviceNamespace: "production",
			wantName:         "assembly-line",
			wantNamespace:    "production",
			hasNamespace:     true,
		},
		{
			name:         "empty service name falls back to default",
			serviceName:  "",
			wantName:     "opcua-server",
			hasNamespace: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transformer := NewTransformer("opc.tcp://test:4840", tt.serviceName, tt.serviceNamespace)
			logs := transformer.TransformLogs([]testdata.OPCUALogRecord{
				{Timestamp: time.Now(), Severity: 150, Message: "probe"},
			})

			resource := logs.ResourceLogs().At(0).Resource()

			nameAttr, ok := resource.Attributes().Get("service.name")
			require.True(t, ok)
			assert.Equal(t, tt.wantName, nameAttr.Str())

			nsAttr, nsOK := resource.Attributes().Get("service.namespace")
			if tt.hasNamespace {
				require.True(t, nsOK, "service.namespace should be present")
				assert.Equal(t, tt.wantNamespace, nsAttr.Str())
			} else {
				assert.False(t, nsOK, "service.namespace should be absent when empty")
			}
		})
	}
}

func TestTransformLogsEmpty(t *testing.T) {
	transformer := NewTransformer("opc.tcp://test:4840", "opcua-server", "")

	logs := transformer.TransformLogs([]testdata.OPCUALogRecord{})

	assert.Equal(t, 0, logs.ResourceLogs().Len())
}

func TestMapSeverity(t *testing.T) {
	transformer := NewTransformer("opc.tcp://test:4840", "opcua-server", "")

	// OPC UA Part 26 §5.4 Table 5 → OTel SeverityNumber mapping
	tests := []struct {
		opcuaSeverity    uint16
		expectedSeverity plog.SeverityNumber
	}{
		// Debug: 1–50
		{1, plog.SeverityNumberDebug},
		{25, plog.SeverityNumberDebug},
		{50, plog.SeverityNumberDebug},
		// Information: 51–100
		{51, plog.SeverityNumberInfo},
		{75, plog.SeverityNumberInfo},
		{100, plog.SeverityNumberInfo},
		// Notice: 101–150
		{101, plog.SeverityNumberInfo4},
		{125, plog.SeverityNumberInfo4},
		{150, plog.SeverityNumberInfo4},
		// Warning: 151–200
		{151, plog.SeverityNumberWarn},
		{175, plog.SeverityNumberWarn},
		{200, plog.SeverityNumberWarn},
		// Error: 201–250
		{201, plog.SeverityNumberError},
		{225, plog.SeverityNumberError},
		{250, plog.SeverityNumberError},
		// Critical: 251–300
		{251, plog.SeverityNumberError2},
		{275, plog.SeverityNumberError2},
		{300, plog.SeverityNumberError2},
		// Alert: 301–400
		{301, plog.SeverityNumberError3},
		{350, plog.SeverityNumberError3},
		{400, plog.SeverityNumberError3},
		// Emergency: 401–1000
		{401, plog.SeverityNumberFatal},
		{700, plog.SeverityNumberFatal},
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
	transformer := NewTransformer("opc.tcp://test:4840", "opcua-server", "")

	opcuaRecord := testdata.OPCUALogRecord{
		Timestamp: time.Now(),
		Severity:  300,

		Message:    "Test message with trace",
		TraceID:    "0123456789abcdef0123456789abcdef",
		SpanID:     "0123456789abcdef",
		TraceFlags: 1,
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
	transformer := NewTransformer("opc.tcp://test:4840", "opcua-server", "")

	opcuaRecord := testdata.OPCUALogRecord{
		Timestamp: time.Now(),
		Severity:  300,

		Message: "Test message",
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
	assert.NotEmpty(t, record.SourceName)
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
	assert.Equal(t, "CustomSource", record.SourceName)
}
