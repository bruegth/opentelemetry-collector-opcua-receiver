// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package opcua

import (
	"encoding/hex"
	"fmt"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"

	"github.com/bruegth/opentelemetry-collector-opcua-receiver/receiver/opcua/testdata"
)

// Transformer converts OPC UA log records to OpenTelemetry format
type Transformer struct {
	serverEndpoint string
}

// NewTransformer creates a new transformer
func NewTransformer(serverEndpoint string) *Transformer {
	return &Transformer{
		serverEndpoint: serverEndpoint,
	}
}

// TransformLogs converts OPC UA log records to OpenTelemetry plog.Logs
func (t *Transformer) TransformLogs(opcuaRecords []testdata.OPCUALogRecord) plog.Logs {
	logs := plog.NewLogs()

	if len(opcuaRecords) == 0 {
		return logs
	}

	// Create resource logs
	resourceLogs := logs.ResourceLogs().AppendEmpty()

	// Set resource attributes
	resource := resourceLogs.Resource()
	t.setResourceAttributes(resource.Attributes())

	// Create scope logs
	scopeLogs := resourceLogs.ScopeLogs().AppendEmpty()
	scopeLogs.Scope().SetName("github.com/bruegth/opentelemetry-collector-opcua-receiver")
	scopeLogs.Scope().SetVersion("0.1.0")

	// Transform each OPC UA log record
	for _, opcuaRecord := range opcuaRecords {
		logRecord := scopeLogs.LogRecords().AppendEmpty()
		t.transformLogRecord(opcuaRecord, logRecord)
	}

	return logs
}

// setResourceAttributes sets resource-level attributes
func (t *Transformer) setResourceAttributes(attrs pcommon.Map) {
	attrs.PutStr("opcua.server.endpoint", t.serverEndpoint)
	attrs.PutStr("service.name", "opcua-server")
	attrs.PutStr("telemetry.sdk.name", "opentelemetry-collector-opcua")
	attrs.PutStr("telemetry.sdk.language", "go")
}

// transformLogRecord converts a single OPC UA log record to OTEL format
func (t *Transformer) transformLogRecord(opcuaRecord testdata.OPCUALogRecord, logRecord plog.LogRecord) {
	// Set timestamp
	logRecord.SetTimestamp(pcommon.NewTimestampFromTime(opcuaRecord.Timestamp))
	logRecord.SetObservedTimestamp(pcommon.NewTimestampFromTime(time.Now()))

	// Map severity
	severityNumber := t.mapSeverity(opcuaRecord.Severity)
	logRecord.SetSeverityNumber(severityNumber)
	logRecord.SetSeverityText(opcuaRecord.SeverityText)

	// Set log body
	logRecord.Body().SetStr(opcuaRecord.Message)

	// Set attributes
	attrs := logRecord.Attributes()

	if opcuaRecord.Source != "" {
		attrs.PutStr("opcua.source", opcuaRecord.Source)
	}

	// Add custom attributes from OPC UA log
	for key, value := range opcuaRecord.Attributes {
		t.putAttribute(attrs, key, value)
	}

	// Set trace context if available
	if opcuaRecord.TraceID != "" && opcuaRecord.SpanID != "" {
		t.setTraceContext(logRecord, opcuaRecord.TraceID, opcuaRecord.SpanID, opcuaRecord.TraceFlags)
	}
}

// mapSeverity maps OPC UA severity levels to OpenTelemetry SeverityNumber
func (t *Transformer) mapSeverity(opcuaSeverity uint16) plog.SeverityNumber {
	// OPC UA severity mapping (Part 26):
	// 0-99: Trace
	// 100-199: Debug
	// 200-399: Info
	// 400-599: Warn
	// 600-799: Error
	// 800-1000: Fatal

	switch {
	case opcuaSeverity < 100:
		return plog.SeverityNumberTrace
	case opcuaSeverity < 200:
		return plog.SeverityNumberDebug
	case opcuaSeverity < 400:
		return plog.SeverityNumberInfo
	case opcuaSeverity < 600:
		return plog.SeverityNumberWarn
	case opcuaSeverity < 800:
		return plog.SeverityNumberError
	default:
		return plog.SeverityNumberFatal
	}
}

// setTraceContext sets the trace context from OPC UA
func (t *Transformer) setTraceContext(logRecord plog.LogRecord, traceID, spanID string, flags byte) {
	// Parse TraceID (32-character hex string to 16 bytes)
	traceIDBytes, err := hex.DecodeString(traceID)
	if err == nil && len(traceIDBytes) == 16 {
		var traceIDArray [16]byte
		copy(traceIDArray[:], traceIDBytes)
		logRecord.SetTraceID(pcommon.TraceID(traceIDArray))
	}

	// Parse SpanID (16-character hex string to 8 bytes)
	spanIDBytes, err := hex.DecodeString(spanID)
	if err == nil && len(spanIDBytes) == 8 {
		var spanIDArray [8]byte
		copy(spanIDArray[:], spanIDBytes)
		logRecord.SetSpanID(pcommon.SpanID(spanIDArray))
	}

	// Set trace flags
	logFlags := plog.DefaultLogRecordFlags
	if flags&0x01 != 0 {
		logFlags = logFlags.WithIsSampled(true)
	}
	logRecord.SetFlags(logFlags)
}

// putAttribute adds an attribute with type detection
func (t *Transformer) putAttribute(attrs pcommon.Map, key string, value interface{}) {
	switch v := value.(type) {
	case string:
		attrs.PutStr(key, v)
	case int:
		attrs.PutInt(key, int64(v))
	case int64:
		attrs.PutInt(key, v)
	case float64:
		attrs.PutDouble(key, v)
	case bool:
		attrs.PutBool(key, v)
	default:
		attrs.PutStr(key, fmt.Sprintf("%v", v))
	}
}
