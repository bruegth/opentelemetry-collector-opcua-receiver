// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package testdata

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/gopcua/opcua/ua"
	"go.uber.org/zap"
)

// MockServer is a simple mock OPC UA server for integration testing
// It simulates server responses without implementing the full OPC UA protocol
type MockServer struct {
	endpoint string
	logger   *zap.Logger

	mu      sync.RWMutex
	records []OPCUALogRecord
	running bool

	// For simulation
	callHandler func(ctx context.Context, req *ua.CallMethodRequest) (*ua.CallMethodResult, error)
}

// NewMockServer creates a new mock OPC UA server
func NewMockServer(endpoint string, logger *zap.Logger) *MockServer {
	if logger == nil {
		logger = zap.NewNop()
	}
	if endpoint == "" {
		endpoint = "opc.tcp://127.0.0.1:4840"
	}

	srv := &MockServer{
		endpoint: endpoint,
		logger:   logger,
		records:  make([]OPCUALogRecord, 0),
	}

	// Set up the default call handler for GetRecords method
	srv.callHandler = srv.defaultCallHandler

	return srv
}

// Start starts the mock server
func (s *MockServer) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return fmt.Errorf("server already running")
	}

	s.running = true
	s.logger.Info("Mock OPC UA server started", zap.String("endpoint", s.endpoint))

	return nil
}

// Stop stops the mock server
func (s *MockServer) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil
	}

	s.running = false
	s.logger.Info("Mock OPC UA server stopped")

	return nil
}

// AddLogRecord adds a log record to the server's storage
func (s *MockServer) AddLogRecord(record OPCUALogRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = append(s.records, record)
}

// AddLogRecords adds multiple log records
func (s *MockServer) AddLogRecords(records []OPCUALogRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = append(s.records, records...)
}

// ClearLogRecords clears all stored log records
func (s *MockServer) ClearLogRecords() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = make([]OPCUALogRecord, 0)
}

// GetLogRecordsCount returns the number of stored log records
func (s *MockServer) GetLogRecordsCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.records)
}

// defaultCallHandler handles OPC UA Call method requests
func (s *MockServer) defaultCallHandler(ctx context.Context, req *ua.CallMethodRequest) (*ua.CallMethodResult, error) {
	s.logger.Debug("Mock server handling Call request",
		zap.String("method_id", req.MethodID.String()))

	// Check if this is a GetRecords call (method ID 11550)
	if req.MethodID.IntID() != 11550 {
		return &ua.CallMethodResult{
			StatusCode: ua.StatusBadMethodInvalid,
		}, nil
	}

	// Parse input arguments
	if len(req.InputArguments) < 6 {
		return &ua.CallMethodResult{
			StatusCode: ua.StatusBadArgumentsMissing,
		}, nil
	}

	startTime := req.InputArguments[0].Value().(time.Time)
	endTime := req.InputArguments[1].Value().(time.Time)
	maxRecords := req.InputArguments[2].Value().(uint32)
	minSeverity := req.InputArguments[3].Value().(uint16)
	// logRecordMask := req.InputArguments[4].Value().(uint32)
	continuationPoint := req.InputArguments[5].Value().([]byte)

	// Validate time range
	if endTime.Before(startTime) {
		return &ua.CallMethodResult{
			StatusCode: ua.StatusBadInvalidArgument,
		}, nil
	}

	// Get filtered records
	filtered, nextCP := s.getFilteredRecords(startTime, endTime, maxRecords, minSeverity, continuationPoint)

	s.logger.Debug("Mock server returning records",
		zap.Int("count", len(filtered)),
		zap.Bool("has_continuation", len(nextCP) > 0))

	// Convert records to OPC UA format (simplified)
	recordsVariant := s.convertRecordsToVariant(filtered)

	return &ua.CallMethodResult{
		StatusCode: ua.StatusOK,
		OutputArguments: []*ua.Variant{
			recordsVariant,
			ua.MustVariant(nextCP),
		},
	}, nil
}

// getFilteredRecords filters records based on criteria
func (s *MockServer) getFilteredRecords(
	startTime, endTime time.Time,
	maxRecords uint32,
	minSeverity uint16,
	continuationPoint []byte,
) ([]OPCUALogRecord, []byte) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Filter by time and severity
	var filtered []OPCUALogRecord
	for _, record := range s.records {
		if record.Timestamp.Before(startTime) || record.Timestamp.After(endTime) {
			continue
		}
		if record.Severity < minSeverity {
			continue
		}
		filtered = append(filtered, record)
	}

	// Handle continuation point
	startIndex := 0
	if len(continuationPoint) > 0 && len(continuationPoint) >= 4 {
		startIndex = int(uint32(continuationPoint[0]) |
			uint32(continuationPoint[1])<<8 |
			uint32(continuationPoint[2])<<16 |
			uint32(continuationPoint[3])<<24)
	}

	if startIndex >= len(filtered) {
		return []OPCUALogRecord{}, nil
	}
	filtered = filtered[startIndex:]

	// Apply max records limit
	var nextContinuationPoint []byte
	if maxRecords > 0 && len(filtered) > int(maxRecords) {
		filtered = filtered[:maxRecords]

		// Create continuation point
		nextOffset := startIndex + int(maxRecords)
		nextContinuationPoint = []byte{
			byte(nextOffset),
			byte(nextOffset >> 8),
			byte(nextOffset >> 16),
			byte(nextOffset >> 24),
		}
	}

	return filtered, nextContinuationPoint
}

// convertRecordsToVariant converts log records to OPC UA Variant format
func (s *MockServer) convertRecordsToVariant(records []OPCUALogRecord) *ua.Variant {
	// Create array of maps representing LogRecords
	var recordMaps []interface{}

	for _, record := range records {
		recordMap := map[string]interface{}{
			"Time":     record.Timestamp,
			"Severity": record.Severity,
			"Message":  record.Message,
		}

		if record.SourceName != "" {
			recordMap["SourceName"] = record.SourceName
		}

		if record.TraceID != "" || record.SpanID != "" {
			recordMap["TraceContext"] = map[string]interface{}{
				"TraceId":    record.TraceID,
				"SpanId":     record.SpanID,
				"TraceFlags": record.TraceFlags,
			}
		}

		if len(record.Attributes) > 0 {
			var additionalData []interface{}
			for key, value := range record.Attributes {
				additionalData = append(additionalData, map[string]interface{}{
					"Name":  key,
					"Value": value,
				})
			}
			recordMap["AdditionalData"] = additionalData
		}

		recordMaps = append(recordMaps, recordMap)
	}

	return ua.MustVariant(recordMaps)
}

// IsRunning returns whether the server is running
func (s *MockServer) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// Endpoint returns the server's endpoint
func (s *MockServer) Endpoint() string {
	return s.endpoint
}
