// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package opcua

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.uber.org/zap"

	"github.com/bruegth/opentelemetry-collector-opcua-receiver/receiver/opcua/testdata"
)

// TestScraperIntegration tests the complete scraper flow with a mock OPC UA server
func TestScraperIntegration(t *testing.T) {
	ctx := context.Background()

	// Create mock server
	logger := zap.NewNop()
	mockServer := testdata.NewMockServer("opc.tcp://localhost:54840", logger)

	// Start the server
	err := mockServer.Start(ctx)
	require.NoError(t, err)
	defer func() {
		err := mockServer.Stop(ctx)
		assert.NoError(t, err)
	}()

	// Add sample log records to the server
	now := time.Now()
	sampleRecords := []testdata.OPCUALogRecord{
		{
			Timestamp: now.Add(-10 * time.Minute),
			Severity:  150, // Info

			Message:    "Test log message 1",
			SourceName: "TestSource",
			Attributes: map[string]interface{}{
				"key1": "value1",
			},
		},
		{
			Timestamp:  now.Add(-5 * time.Minute),
			Severity:   250, // Warning
			Message:    "Test log message 2",
			SourceName: "TestSource",
			TraceID:    "0102030405060708090a0b0c0d0e0f10",
			SpanID:     "0102030405060708",
			TraceFlags: 1,
			Attributes: map[string]interface{}{
				"key2": "value2",
			},
		},
		{
			Timestamp:  now.Add(-2 * time.Minute),
			Severity:   350, // Error
			Message:    "Test log message 3",
			SourceName: "TestSource",
			Attributes: make(map[string]interface{}),
		},
	}
	mockServer.AddLogRecords(sampleRecords)

	// Create configuration
	config := &Config{
		Endpoint:           mockServer.Endpoint(),
		CollectionInterval: 30 * time.Second,
		MaxRecordsPerCall:  100,
		Filter: FilterConfig{
			MinSeverity:   "Info",
			MaxLogRecords: 1000,
		},
		LogObjectPaths: []string{"Objects/ServerLog"},
	}

	// Create mock client
	mockClient := testdata.NewMockClient(mockServer, logger)

	// Connect the mock client
	err = mockClient.Connect(ctx)
	require.NoError(t, err)
	defer func() {
		err := mockClient.Disconnect(ctx)
		assert.NoError(t, err)
	}()

	// Create scraper with mock client
	transformer := NewTransformer(mockServer.Endpoint(), "opcua-server", "")
	settings := componenttest.NewNopTelemetrySettings()
	scr := &scraper{
		config:      config,
		settings:    settings,
		transformer: transformer,
		client:      &mockClientAdapter{mockClient: mockClient, config: config},
	}

	// Run scraper
	logs, err := scr.scrape(ctx)
	require.NoError(t, err)
	require.NotNil(t, logs)

	// Verify results
	assert.Equal(t, 1, logs.ResourceLogs().Len(), "Should have 1 resource log")

	resourceLogs := logs.ResourceLogs().At(0)
	assert.Equal(t, 1, resourceLogs.ScopeLogs().Len(), "Should have 1 scope log")

	scopeLogs := resourceLogs.ScopeLogs().At(0)
	logRecords := scopeLogs.LogRecords()

	assert.Equal(t, 3, logRecords.Len(), "Should have 3 log records")

	// Verify first log record
	logRecord := logRecords.At(0)
	assert.Equal(t, "Test log message 1", logRecord.Body().AsString())
	assert.NotZero(t, logRecord.Timestamp())

	// Verify second log record with trace context
	logRecord2 := logRecords.At(1)
	assert.Equal(t, "Test log message 2", logRecord2.Body().AsString())
	assert.False(t, logRecord2.TraceID().IsEmpty(), "Should have trace ID")
	assert.False(t, logRecord2.SpanID().IsEmpty(), "Should have span ID")

	// Verify third log record
	logRecord3 := logRecords.At(2)
	assert.Equal(t, "Test log message 3", logRecord3.Body().AsString())

	t.Log("Integration test completed successfully")
}

// TestScraperIntegrationPagination tests pagination with continuation points
func TestScraperIntegrationPagination(t *testing.T) {
	ctx := context.Background()

	// Create mock server
	logger := zap.NewNop()
	mockServer := testdata.NewMockServer("opc.tcp://localhost:54841", logger)

	err := mockServer.Start(ctx)
	require.NoError(t, err)
	defer func() {
		err := mockServer.Stop(ctx)
		assert.NoError(t, err)
	}()

	// Add many log records to trigger pagination
	now := time.Now()
	var manyRecords []testdata.OPCUALogRecord
	for i := 0; i < 150; i++ {
		manyRecords = append(manyRecords, testdata.OPCUALogRecord{
			Timestamp: now.Add(-time.Duration(150-i) * time.Minute),
			Severity:  150,

			Message:    "Paginated log message",
			SourceName: "TestSource",
			Attributes: make(map[string]interface{}),
		})
	}
	mockServer.AddLogRecords(manyRecords)

	// Create configuration with small batch size
	config := &Config{
		Endpoint:           mockServer.Endpoint(),
		CollectionInterval: 30 * time.Second,
		MaxRecordsPerCall:  50, // Small batch to trigger pagination
		Filter: FilterConfig{
			MinSeverity:   "Info",
			MaxLogRecords: 1000,
		},
		LogObjectPaths: []string{"Objects/ServerLog"},
	}

	// Create mock client
	mockClient := testdata.NewMockClient(mockServer, logger)
	err = mockClient.Connect(ctx)
	require.NoError(t, err)
	defer func() {
		if err := mockClient.Disconnect(ctx); err != nil {
			t.Logf("Failed to disconnect mock client: %v", err)
		}
	}()

	// Create scraper
	transformer := NewTransformer(mockServer.Endpoint(), "opcua-server", "")
	settings := componenttest.NewNopTelemetrySettings()
	scr := &scraper{
		config:      config,
		settings:    settings,
		transformer: transformer,
		client:      &mockClientAdapter{mockClient: mockClient, config: config},
	}

	// Run scraper
	logs, err := scr.scrape(ctx)
	require.NoError(t, err)
	require.NotNil(t, logs)

	// Verify all records were collected across multiple pages
	totalRecords := 0
	for i := 0; i < logs.ResourceLogs().Len(); i++ {
		resourceLog := logs.ResourceLogs().At(i)
		for j := 0; j < resourceLog.ScopeLogs().Len(); j++ {
			scopeLog := resourceLog.ScopeLogs().At(j)
			totalRecords += scopeLog.LogRecords().Len()
		}
	}

	assert.Equal(t, 150, totalRecords, "Should collect all 150 records across pages")
	t.Logf("Successfully collected %d records with pagination", totalRecords)
}

// TestScraperIntegrationFiltering tests severity filtering
func TestScraperIntegrationFiltering(t *testing.T) {
	ctx := context.Background()

	// Create mock server
	logger := zap.NewNop()
	mockServer := testdata.NewMockServer("opc.tcp://localhost:54842", logger)

	err := mockServer.Start(ctx)
	require.NoError(t, err)
	defer func() {
		if err := mockServer.Stop(ctx); err != nil {
			t.Logf("Failed to stop mock server: %v", err)
		}
	}()

	// Add records with different severities
	now := time.Now()
	records := []testdata.OPCUALogRecord{
		{Timestamp: now, Severity: 50, Message: "Debug message", Attributes: make(map[string]interface{})},
		{Timestamp: now, Severity: 150, Message: "Info message", Attributes: make(map[string]interface{})},
		{Timestamp: now, Severity: 250, Message: "Warning message", Attributes: make(map[string]interface{})},
		{Timestamp: now, Severity: 350, Message: "Error message", Attributes: make(map[string]interface{})},
	}
	mockServer.AddLogRecords(records)

	// Test with Warning minimum severity
	config := &Config{
		Endpoint:           mockServer.Endpoint(),
		CollectionInterval: 30 * time.Second,
		MaxRecordsPerCall:  100,
		Filter: FilterConfig{
			MinSeverity:   "Warning",
			MaxLogRecords: 1000,
		},
		LogObjectPaths: []string{"Objects/ServerLog"},
	}

	mockClient := testdata.NewMockClient(mockServer, logger)
	err = mockClient.Connect(ctx)
	require.NoError(t, err)
	defer func() {
		if err := mockClient.Disconnect(ctx); err != nil {
			t.Logf("Failed to disconnect mock client: %v", err)
		}
	}()

	transformer := NewTransformer(mockServer.Endpoint(), "opcua-server", "")
	settings := componenttest.NewNopTelemetrySettings()
	scr := &scraper{
		config:      config,
		settings:    settings,
		transformer: transformer,
		client:      &mockClientAdapter{mockClient: mockClient, config: config},
	}

	logs, err := scr.scrape(ctx)
	require.NoError(t, err)

	// Count records
	totalRecords := 0
	for i := 0; i < logs.ResourceLogs().Len(); i++ {
		resourceLog := logs.ResourceLogs().At(i)
		for j := 0; j < resourceLog.ScopeLogs().Len(); j++ {
			scopeLog := resourceLog.ScopeLogs().At(j)
			totalRecords += scopeLog.LogRecords().Len()
		}
	}

	// Should only get Warning and Error (2 records), not Debug and Info
	assert.Equal(t, 2, totalRecords, "Should only get Warning and Error records")
	t.Log("Filtering test completed successfully")
}

// mockClientAdapter adapts testdata.MockClient to OPCUAClient interface
type mockClientAdapter struct {
	mockClient *testdata.MockClient
	config     *Config
}

func (m *mockClientAdapter) Connect(ctx context.Context) error {
	return m.mockClient.Connect(ctx)
}

func (m *mockClientAdapter) Disconnect(ctx context.Context) error {
	return m.mockClient.Disconnect(ctx)
}

func (m *mockClientAdapter) IsConnected() bool {
	return m.mockClient.IsConnected()
}

func (m *mockClientAdapter) GetRecords(
	ctx context.Context,
	startTime, endTime time.Time,
	maxRecords int,
) ([]testdata.OPCUALogRecord, error) {
	// Handle pagination like the real client does
	var allRecords []testdata.OPCUALogRecord
	continuationPoint := []byte(nil)

	// Get minimum severity from config
	minSeverity := getMinSeverityValueFromConfig(m.config.Filter.MinSeverity)

	// Keep fetching records using continuation points until no more records
	for {
		records, nextCP, err := m.mockClient.GetRecordsWithSeverity(ctx, startTime, endTime, maxRecords, minSeverity, continuationPoint)
		if err != nil {
			return nil, err
		}

		allRecords = append(allRecords, records...)

		// If no continuation point, we're done
		if len(nextCP) == 0 {
			break
		}

		// Continue with next page
		continuationPoint = nextCP
	}

	return allRecords, nil
}

// getMinSeverityValueFromConfig converts config severity string to numeric value
func getMinSeverityValueFromConfig(minSeverity string) uint16 {
	switch minSeverity {
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
