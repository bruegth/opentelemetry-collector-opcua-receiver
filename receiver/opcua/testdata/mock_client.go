// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package testdata

import (
	"context"
	"fmt"
	"time"

	"github.com/gopcua/opcua/ua"
	"go.uber.org/zap"
)

// MockClient is a mock OPC UA client that works with MockServer
// It implements the same interface as the real opcuaClient for testing
type MockClient struct {
	server *MockServer
	logger *zap.Logger

	connected bool
}

// NewMockClient creates a new mock OPC UA client connected to a mock server
func NewMockClient(server *MockServer, logger *zap.Logger) *MockClient {
	if logger == nil {
		logger = zap.NewNop()
	}

	return &MockClient{
		server: server,
		logger: logger,
	}
}

// Connect simulates connecting to the OPC UA server
func (c *MockClient) Connect(ctx context.Context) error {
	if c.connected {
		return fmt.Errorf("already connected")
	}

	if !c.server.IsRunning() {
		return fmt.Errorf("server is not running")
	}

	c.connected = true
	c.logger.Debug("Mock client connected", zap.String("endpoint", c.server.Endpoint()))

	return nil
}

// Disconnect simulates disconnecting from the server
func (c *MockClient) Disconnect(ctx context.Context) error {
	if !c.connected {
		return nil
	}

	c.connected = false
	c.logger.Debug("Mock client disconnected")

	return nil
}

// GetRecords simulates calling the GetRecords method on the server
func (c *MockClient) GetRecords(
	ctx context.Context,
	startTime, endTime time.Time,
	maxRecords int,
	continuationPoint []byte,
) ([]OPCUALogRecord, []byte, error) {
	return c.GetRecordsWithSeverity(ctx, startTime, endTime, maxRecords, 0, continuationPoint)
}

// GetRecordsWithSeverity simulates calling the GetRecords method with minimum severity
func (c *MockClient) GetRecordsWithSeverity(
	ctx context.Context,
	startTime, endTime time.Time,
	maxRecords int,
	minSeverity uint16,
	continuationPoint []byte,
) ([]OPCUALogRecord, []byte, error) {
	if !c.connected {
		return nil, nil, fmt.Errorf("not connected to server")
	}

	c.logger.Debug("Mock client GetRecords called",
		zap.Time("start_time", startTime),
		zap.Time("end_time", endTime),
		zap.Int("max_records", maxRecords),
		zap.Uint16("min_severity", minSeverity))

	// Create a CallMethodRequest
	req := &ua.CallMethodRequest{
		ObjectID: ua.NewNumericNodeID(0, 2042), // Server object
		MethodID: ua.NewNumericNodeID(0, 11550), // GetRecords method
		InputArguments: []*ua.Variant{
			ua.MustVariant(startTime),
			ua.MustVariant(endTime),
			ua.MustVariant(uint32(maxRecords)),
			ua.MustVariant(minSeverity),
			ua.MustVariant(uint32(0x1F)), // log record mask
			ua.MustVariant(continuationPoint),
		},
	}

	// Call the server's handler
	result, err := c.server.callHandler(ctx, req)
	if err != nil {
		return nil, nil, fmt.Errorf("call failed: %w", err)
	}

	if result.StatusCode != ua.StatusOK {
		return nil, nil, fmt.Errorf("call failed with status: %v", result.StatusCode)
	}

	// Parse output arguments
	if len(result.OutputArguments) < 2 {
		return nil, nil, fmt.Errorf("unexpected number of output arguments: %d", len(result.OutputArguments))
	}

	// Extract records from first output argument
	recordsValue := result.OutputArguments[0].Value()
	var records []OPCUALogRecord

	if recordMaps, ok := recordsValue.([]interface{}); ok {
		for _, recordMap := range recordMaps {
			if m, ok := recordMap.(map[string]interface{}); ok {
				record := parseRecordMap(m)
				records = append(records, record)
			}
		}
	}

	// Extract continuation point from second output argument
	var nextCP []byte
	if cpValue := result.OutputArguments[1].Value(); cpValue != nil {
		if cp, ok := cpValue.([]byte); ok {
			nextCP = cp
		}
	}

	c.logger.Debug("Mock client GetRecords completed",
		zap.Int("records_count", len(records)),
		zap.Bool("has_continuation", len(nextCP) > 0))

	return records, nextCP, nil
}

// parseRecordMap parses a map into an OPCUALogRecord
func parseRecordMap(m map[string]interface{}) OPCUALogRecord {
	record := OPCUALogRecord{
		Attributes: make(map[string]interface{}),
	}

	if timeVal, ok := m["Time"].(time.Time); ok {
		record.Timestamp = timeVal
	}

	if sevVal, ok := m["Severity"].(uint16); ok {
		record.Severity = sevVal
		record.SeverityText = severityToText(sevVal)
	}

	if msgVal, ok := m["Message"].(string); ok {
		record.Message = msgVal
	}

	if sourceVal, ok := m["SourceName"].(string); ok {
		record.Source = sourceVal
	}

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

	if additionalData, ok := m["AdditionalData"].([]interface{}); ok {
		for _, item := range additionalData {
			if nvp, ok := item.(map[string]interface{}); ok {
				if name, ok := nvp["Name"].(string); ok {
					record.Attributes[name] = nvp["Value"]
				}
			}
		}
	}

	return record
}

// severityToText converts severity to text
func severityToText(severity uint16) string {
	switch {
	case severity >= 1 && severity <= 50:
		return "Debug"
	case severity >= 51 && severity <= 100:
		return "Trace"
	case severity >= 101 && severity <= 200:
		return "Info"
	case severity >= 201 && severity <= 300:
		return "Warning"
	case severity >= 301 && severity <= 400:
		return "Error"
	case severity >= 401 && severity <= 1000:
		return "Emergency"
	default:
		return "Unknown"
	}
}

// IsConnected returns whether the client is connected
func (c *MockClient) IsConnected() bool {
	return c.connected
}
