// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package opcua

import (
	"encoding/hex"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"

	"github.com/bruegth/opentelemetry-collector-opcua-receiver/receiver/opcua/testdata"
)

// Transformer converts OPC UA log records to OpenTelemetry format
type Transformer struct {
	serverEndpoint   string
	serviceName      string
	serviceNamespace string
}

// NewTransformer creates a new transformer
func NewTransformer(serverEndpoint, serviceName, serviceNamespace string) *Transformer {
	if serviceName == "" {
		serviceName = "opcua-server"
	}
	return &Transformer{
		serverEndpoint:   serverEndpoint,
		serviceName:      serviceName,
		serviceNamespace: serviceNamespace,
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

// setResourceAttributes sets resource-level attributes.
// server.address and server.port are the OTel semantic conventions for describing
// the remote server being connected to (not the local host running the collector).
func (t *Transformer) setResourceAttributes(attrs pcommon.Map) {
	attrs.PutStr("service.name", t.serviceName)
	if t.serviceNamespace != "" {
		attrs.PutStr("service.namespace", t.serviceNamespace)
	}

	// Parse the OPC UA endpoint URI (e.g. "opc.tcp://hostname:4840/path")
	// to extract server.address and server.port per OTel semantic conventions.
	if u, err := url.Parse(t.serverEndpoint); err == nil && u.Host != "" {
		host, portStr, err := net.SplitHostPort(u.Host)
		if err == nil {
			attrs.PutStr("server.address", host)
			if port, err := strconv.ParseInt(portStr, 10, 64); err == nil {
				attrs.PutInt("server.port", port)
			}
		} else {
			// No port in the host (unusual for OPC UA, but handle gracefully)
			attrs.PutStr("server.address", u.Host)
		}
	}
}

// transformLogRecord converts a single OPC UA log record to OTEL format
func (t *Transformer) transformLogRecord(opcuaRecord testdata.OPCUALogRecord, logRecord plog.LogRecord) {
	// Set timestamp
	logRecord.SetTimestamp(pcommon.NewTimestampFromTime(opcuaRecord.Timestamp))
	logRecord.SetObservedTimestamp(pcommon.NewTimestampFromTime(time.Now()))

	// Map severity
	severityNumber := t.mapSeverity(opcuaRecord.Severity)
	logRecord.SetSeverityNumber(severityNumber)
	logRecord.SetSeverityText(severityToText(opcuaRecord.Severity))

	// Set log body
	logRecord.Body().SetStr(opcuaRecord.Message)

	// Set attributes
	attrs := logRecord.Attributes()

	if opcuaRecord.SourceName != "" {
		attrs.PutStr("opcua.source.name", opcuaRecord.SourceName)
	}
	if opcuaRecord.SourceIDType != "" {
		attrs.PutInt("opcua.source.namespace", int64(opcuaRecord.SourceNamespace))
		attrs.PutStr("opcua.source.id_type", opcuaRecord.SourceIDType)
		attrs.PutStr("opcua.source.id", opcuaRecord.SourceID)
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

// mapSeverity maps an OPC UA Part 26 §5.4 severity value to an OpenTelemetry SeverityNumber.
// Severity text is not transmitted over OPC UA; it is derived separately by severityToText.
//
// Part 26 §5.4 Table 5 → OTel mapping:
//
//	1–50:    Debug       → SeverityNumberDebug
//	51–100:  Information → SeverityNumberInfo
//	101–150: Notice      → SeverityNumberInfo4
//	151–200: Warning     → SeverityNumberWarn
//	201–250: Error       → SeverityNumberError
//	251–300: Critical    → SeverityNumberError2
//	301–400: Alert       → SeverityNumberError3
//	401–1000: Emergency  → SeverityNumberFatal
func (t *Transformer) mapSeverity(opcuaSeverity uint16) plog.SeverityNumber {
	switch {
	case opcuaSeverity >= 1 && opcuaSeverity <= 50:
		return plog.SeverityNumberDebug
	case opcuaSeverity >= 51 && opcuaSeverity <= 100:
		return plog.SeverityNumberInfo
	case opcuaSeverity >= 101 && opcuaSeverity <= 150:
		return plog.SeverityNumberInfo4
	case opcuaSeverity >= 151 && opcuaSeverity <= 200:
		return plog.SeverityNumberWarn
	case opcuaSeverity >= 201 && opcuaSeverity <= 250:
		return plog.SeverityNumberError
	case opcuaSeverity >= 251 && opcuaSeverity <= 300:
		return plog.SeverityNumberError2
	case opcuaSeverity >= 301 && opcuaSeverity <= 400:
		return plog.SeverityNumberError3
	case opcuaSeverity >= 401 && opcuaSeverity <= 1000:
		return plog.SeverityNumberFatal
	default:
		return plog.SeverityNumberUnspecified
	}
}

// severityToText maps an OPC UA Part 26 §5.4 severity value to its text label.
// Severity text is not transmitted over OPC UA and must be derived from the numeric value.
//
// Part 26 §5.4 Table 5 ranges:
//
//	1–50:    Debug
//	51–100:  Information
//	101–150: Notice
//	151–200: Warning
//	201–250: Error
//	251–300: Critical
//	301–400: Alert
//	401–1000: Emergency
func severityToText(severity uint16) string {
	switch {
	case severity >= 1 && severity <= 50:
		return "Debug"
	case severity >= 51 && severity <= 100:
		return "Information"
	case severity >= 101 && severity <= 150:
		return "Notice"
	case severity >= 151 && severity <= 200:
		return "Warning"
	case severity >= 201 && severity <= 250:
		return "Error"
	case severity >= 251 && severity <= 300:
		return "Critical"
	case severity >= 301 && severity <= 400:
		return "Alert"
	case severity >= 401 && severity <= 1000:
		return "Emergency"
	default:
		return "Unspecified"
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
