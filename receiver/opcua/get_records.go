// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package opcua

import (
	"context"
	"fmt"
	"time"

	"github.com/gopcua/opcua/ua"
	"go.uber.org/zap"

	"github.com/bruegth/opentelemetry-collector-opcua-receiver/receiver/opcua/testdata"
)

// callGetRecordsMethod invokes the OPC UA Part 26 GetRecords method on a LogObject
func (c *opcuaClient) callGetRecordsMethod(
	ctx context.Context,
	logObjectID *ua.NodeID,
	startTime, endTime time.Time,
	maxRecords uint32,
	minSeverity uint16,
	continuationPoint []byte,
) ([]testdata.OPCUALogRecord, []byte, error) {

	// Find the GetRecords method NodeID by browsing the LogObject's children.
	getRecordsMethodID, err := c.findGetRecordsMethod(ctx, logObjectID)
	if err != nil {
		c.logger.Warn("Could not discover GetRecords method via browsing, using standard ID ns=0;i=11550",
			zap.String("log_object_id", logObjectID.String()),
			zap.Error(err))
		getRecordsMethodID = ua.NewNumericNodeID(0, 11550)
	} else {
		c.logger.Debug("Using discovered GetRecords method",
			zap.String("method_id", getRecordsMethodID.String()))
	}

	// Build LogRecordMask - request all optional fields
	// Bit 0: EventType, Bit 1: SourceNode, Bit 2: SourceName, Bit 3: TraceContext, Bit 4: AdditionalData
	logRecordMask := uint32(0x1F) // All bits set (0b11111)

	// Build input arguments according to OPC UA Part 26 §5.3
	inputArgs := []*ua.Variant{
		ua.MustVariant(startTime),         // StartTime (DateTime)
		ua.MustVariant(endTime),           // EndTime (DateTime)
		ua.MustVariant(maxRecords),        // MaxReturnRecords (UInt32)
		ua.MustVariant(minSeverity),       // MinimumSeverity (UInt16)
		ua.MustVariant(logRecordMask),     // RequestMask (LogRecordMask/UInt32)
		ua.MustVariant(continuationPoint), // ContinuationPointIn (ByteString)
	}

	// Create CallMethodRequest
	req := &ua.CallMethodRequest{
		ObjectID:       logObjectID,
		MethodID:       getRecordsMethodID,
		InputArguments: inputArgs,
	}

	c.logger.Debug("Calling GetRecords method",
		zap.String("log_object_id", logObjectID.String()),
		zap.Time("start_time", startTime),
		zap.Time("end_time", endTime),
		zap.Uint32("max_records", maxRecords),
		zap.Uint16("min_severity", minSeverity),
		zap.Bool("has_continuation_point", len(continuationPoint) > 0))

	// Execute the Call service
	result, err := c.client.Call(ctx, req)
	if err != nil {
		return nil, nil, fmt.Errorf("Call service failed: %w", err)
	}

	// Check for method call errors
	if result.StatusCode != ua.StatusOK {
		// Check for specific error codes
		switch result.StatusCode {
		case ua.StatusBadInvalidArgument:
			return nil, nil, fmt.Errorf("invalid argument: EndTime < StartTime or invalid severity range")
		case ua.StatusBadContinuationPointInvalid:
			c.logger.Warn("Continuation point invalid, restarting query without continuation point")
			// Retry without continuation point
			if len(continuationPoint) > 0 {
				return c.callGetRecordsMethod(ctx, logObjectID, startTime, endTime, maxRecords, minSeverity, nil)
			}
			return nil, nil, fmt.Errorf("continuation point invalid")
		default:
			return nil, nil, fmt.Errorf("GetRecords method call failed with status: %v", result.StatusCode)
		}
	}

	// Parse output arguments
	// Expected: [0] = LogRecordsDataTypeResults, [1] = ContinuationPointOut
	if len(result.OutputArguments) < 2 {
		return nil, nil, fmt.Errorf("unexpected number of output arguments: %d (expected 2)", len(result.OutputArguments))
	}

	// Parse LogRecords array from first output argument
	logRecords, err := c.parseLogRecordsDataType(result.OutputArguments[0])
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse LogRecords: %w", err)
	}

	// Extract continuation point from second output argument
	var nextContinuationPoint []byte
	if cpVariant := result.OutputArguments[1]; cpVariant != nil {
		if cp, ok := cpVariant.Value().([]byte); ok {
			nextContinuationPoint = cp
		}
	}

	c.logger.Debug("GetRecords method completed",
		zap.Int("records_count", len(logRecords)),
		zap.Bool("has_continuation_point", len(nextContinuationPoint) > 0))

	return logRecords, nextContinuationPoint, nil
}

// parseLogRecordsDataType parses the LogRecordsDataType variant into LogRecord structures
func (c *opcuaClient) parseLogRecordsDataType(variant *ua.Variant) ([]testdata.OPCUALogRecord, error) {
	if variant == nil {
		return []testdata.OPCUALogRecord{}, nil
	}

	// The LogRecordsDataType contains an array of LogRecord ExtensionObjects
	value := variant.Value()

	// Handle different possible response formats
	switch v := value.(type) {
	case []interface{}:
		return c.parseLogRecordArray(v)
	case []*ua.ExtensionObject:
		return c.parseExtensionObjectArray(v)
	case nil:
		return []testdata.OPCUALogRecord{}, nil
	default:
		c.logger.Warn("Unexpected LogRecords data type",
			zap.String("type", fmt.Sprintf("%T", value)))
		return []testdata.OPCUALogRecord{}, nil
	}
}

// parseLogRecordArray parses an array of log records
func (c *opcuaClient) parseLogRecordArray(records []interface{}) ([]testdata.OPCUALogRecord, error) {
	var result []testdata.OPCUALogRecord

	for i, record := range records {
		logRecord, err := c.parseLogRecord(record)
		if err != nil {
			c.logger.Warn("Failed to parse log record",
				zap.Int("index", i),
				zap.Error(err))
			continue
		}
		result = append(result, logRecord)
	}

	return result, nil
}

// parseExtensionObjectArray parses an array of ExtensionObjects containing LogRecords
func (c *opcuaClient) parseExtensionObjectArray(objects []*ua.ExtensionObject) ([]testdata.OPCUALogRecord, error) {
	var result []testdata.OPCUALogRecord

	for i, obj := range objects {
		if obj == nil {
			continue
		}

		logRecord, err := c.parseLogRecordFromExtensionObject(obj)
		if err != nil {
			c.logger.Warn("Failed to parse ExtensionObject",
				zap.Int("index", i),
				zap.Error(err))
			continue
		}
		result = append(result, logRecord)
	}

	return result, nil
}

// parseLogRecord parses a single LogRecord from interface{}
func (c *opcuaClient) parseLogRecord(data interface{}) (testdata.OPCUALogRecord, error) {
	// Try to extract fields from a map or struct
	if m, ok := data.(map[string]interface{}); ok {
		return c.parseLogRecordFromMap(m)
	}

	return testdata.OPCUALogRecord{}, fmt.Errorf("unsupported log record format: %T", data)
}

// parseLogRecordFromExtensionObject parses LogRecord from an ExtensionObject.
// The ExtensionObject's binary body is automatically decoded by gopcua into a
// LogRecordExtObj if the type was registered (see log_record_type.go).
func (c *opcuaClient) parseLogRecordFromExtensionObject(obj *ua.ExtensionObject) (testdata.OPCUALogRecord, error) {
	c.logger.Debug("Parsing LogRecord from ExtensionObject",
		zap.String("type_id", obj.TypeID.String()))

	// Check if gopcua successfully decoded the ExtensionObject into our registered type
	if lr, ok := obj.Value.(*LogRecordExtObj); ok && lr != nil {
		return logRecordExtObjToRecord(lr), nil
	}

	// Fallback: if the Value is raw bytes (type not registered due to namespace mismatch),
	// manually decode the binary body using our LogRecordExtObj decoder.
	if raw, ok := obj.Value.([]byte); ok && len(raw) > 0 {
		c.logger.Debug("Falling back to manual binary decoding for ExtensionObject",
			zap.String("type_id", obj.TypeID.String()),
			zap.Int("body_len", len(raw)))
		lr := &LogRecordExtObj{}
		if _, err := lr.Decode(raw); err != nil {
			return testdata.OPCUALogRecord{}, fmt.Errorf("failed to manually decode ExtensionObject body: %w", err)
		}
		return logRecordExtObjToRecord(lr), nil
	}

	if obj.Value == nil {
		return testdata.OPCUALogRecord{}, fmt.Errorf("ExtensionObject Value is nil (unknown TypeID %s)", obj.TypeID.String())
	}

	return testdata.OPCUALogRecord{}, fmt.Errorf("unsupported ExtensionObject value type: %T", obj.Value)
}

// logRecordExtObjToRecord converts a decoded LogRecordExtObj into an OPCUALogRecord,
// mapping source NodeId components, trace context, and additional data attributes.
func logRecordExtObjToRecord(lr *LogRecordExtObj) testdata.OPCUALogRecord {
	ns, idType, id := nodeIDComponents(lr.SourceNode)
	record := testdata.OPCUALogRecord{
		Timestamp:       lr.Time,
		Severity:        lr.Severity,
		Message:         lr.Message,
		SourceName:      lr.SourceName,
		SourceNamespace: ns,
		SourceIDType:    idType,
		SourceID:        id,
		Attributes:      make(map[string]interface{}),
	}

	// Populate trace context (SpanID == 0 signals no trace context)
	if lr.SpanID != 0 {
		record.TraceID = lr.TraceIDHex()
		record.SpanID = lr.SpanIDHex()
		record.TraceFlags = 0x01 // sampled
	}

	// Promote AdditionalData entries to log attributes
	for k, v := range lr.AdditionalData {
		record.Attributes[k] = v
	}

	return record
}

// parseLogRecordFromMap parses LogRecord from a map structure
func (c *opcuaClient) parseLogRecordFromMap(m map[string]interface{}) (testdata.OPCUALogRecord, error) {
	record := testdata.OPCUALogRecord{
		Attributes: make(map[string]interface{}),
	}

	// Parse mandatory fields: Time, Severity, Message
	if timeVal, ok := m["Time"].(time.Time); ok {
		record.Timestamp = timeVal
	}

	if sevVal, ok := m["Severity"].(uint16); ok {
		record.Severity = sevVal
	}

	if msgVal, ok := m["Message"]; ok {
		if localizedText, ok := msgVal.(map[string]interface{}); ok {
			if text, ok := localizedText["Text"].(string); ok {
				record.Message = text
			}
		} else if text, ok := msgVal.(string); ok {
			record.Message = text
		}
	}

	// Parse optional fields
	if sourceNameVal, ok := m["SourceName"].(string); ok {
		record.SourceName = sourceNameVal
	}

	if sourceNodeVal, ok := m["SourceNode"].(string); ok && sourceNodeVal != "" {
		if nodeID, err := ua.ParseNodeID(sourceNodeVal); err == nil {
			ns, idType, id := nodeIDComponents(nodeID)
			record.SourceNamespace = ns
			record.SourceIDType = idType
			record.SourceID = id
		}
	}

	// Parse TraceContext
	if traceCtx, ok := m["TraceContext"].(map[string]interface{}); ok {
		if traceID, ok := traceCtx["TraceId"].(string); ok {
			record.TraceID = traceID
		}
		if spanID, ok := traceCtx["SpanId"].(string); ok {
			record.SpanID = spanID
		}
		if flags, ok := traceCtx["TraceFlags"].(byte); ok {
			record.TraceFlags = flags
		}
	}

	// Parse AdditionalData
	if additionalData, ok := m["AdditionalData"].([]interface{}); ok {
		for _, item := range additionalData {
			if nvp, ok := item.(map[string]interface{}); ok {
				if name, ok := nvp["Name"].(string); ok {
					record.Attributes[name] = nvp["Value"]
				}
			}
		}
	}

	return record, nil
}

// nodeIDComponents extracts namespace, identifier type, and identifier value from a NodeID.
// Returns zero values and empty strings when nodeID is nil.
func nodeIDComponents(nodeID *ua.NodeID) (namespace uint16, idType string, id string) {
	if nodeID == nil {
		return 0, "", ""
	}
	namespace = nodeID.Namespace()
	switch nodeID.Type() {
	case ua.NodeIDTypeString:
		idType = "String"
		id = nodeID.StringID()
	case ua.NodeIDTypeGUID:
		idType = "Guid"
		id = nodeID.String() // full string representation for GUID
	case ua.NodeIDTypeByteString:
		idType = "Opaque"
		id = fmt.Sprintf("%d", nodeID.IntID())
	default: // NodeIDTypeTwoByte, NodeIDTypeFourByte, NodeIDTypeNumeric
		idType = "Numeric"
		id = fmt.Sprintf("%d", nodeID.IntID())
	}
	return namespace, idType, id
}

// getMinSeverityValue converts config severity string to numeric value
func (c *opcuaClient) getMinSeverityValue() uint16 {
	switch c.config.Filter.MinSeverity {
	case "Trace":
		return 51
	case "Debug":
		return 1
	case "Info":
		return 101
	case "Warn", "Warning":
		return 201
	case "Error":
		return 301
	case "Fatal", "Emergency":
		return 401
	default:
		return 101 // Default to Info
	}
}
