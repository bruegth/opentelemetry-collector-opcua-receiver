# OPC UA Test Data Package

This package provides mock OPC UA server and client implementations for integration testing of the OPC UA receiver.

## Overview

The test data package includes:

- **MockServer**: A simulated OPC UA server that implements the GetRecords method
- **MockClient**: A mock OPC UA client that works with MockServer
- **Sample Data Generation**: Utilities for generating test log records

## Components

### MockServer

A lightweight mock server that:
- Stores log records in memory
- Implements the OPC UA Part 26 GetRecords method
- Supports pagination with continuation points
- Handles time range and severity filtering

### MockClient

A mock client that:
- Connects to MockServer
- Calls the GetRecords method
- Parses responses into OPCUALogRecord structures
- Supports pagination

### Types and Generators

- `OPCUALogRecord`: Structure representing an OPC UA log record
- `GenerateSampleLogRecord()`: Creates sample log records for testing
- `GenerateLogRecordWithDetails()`: Creates customized log records

## Usage

### Basic Integration Test

```go
import (
    "context"
    "testing"
    "time"
    "github.com/bruegth/opentelemetry-collector-opcua-receiver/receiver/opcua/testdata"
)

func TestWithMockServer(t *testing.T) {
    ctx := context.Background()

    // Create and start mock server
    server := testdata.NewMockServer("opc.tcp://localhost:4840", logger)
    err := server.Start(ctx)
    require.NoError(t, err)
    defer server.Stop(ctx)

    // Add test data
    records := []testdata.OPCUALogRecord{
        {
            Timestamp: time.Now(),
            Severity:  150,
            Message:   "Test message",
            Attributes: make(map[string]interface{}),
        },
    }
    server.AddLogRecords(records)

    // Create and connect mock client
    client := testdata.NewMockClient(server, logger)
    err = client.Connect(ctx)
    require.NoError(t, err)
    defer client.Disconnect(ctx)

    // Call GetRecords
    startTime := time.Now().Add(-1 * time.Hour)
    endTime := time.Now()
    results, cp, err := client.GetRecords(ctx, startTime, endTime, 100, nil)
    require.NoError(t, err)

    // Verify results
    assert.Equal(t, 1, len(results))
    assert.Nil(t, cp) // No continuation point for small result set
}
```

### Testing Pagination

```go
func TestPagination(t *testing.T) {
    server := testdata.NewMockServer("opc.tcp://localhost:4840", logger)
    server.Start(ctx)
    defer server.Stop(ctx)

    // Add many records to trigger pagination
    for i := 0; i < 150; i++ {
        server.AddLogRecord(testdata.GenerateSampleLogRecord())
    }

    client := testdata.NewMockClient(server, logger)
    client.Connect(ctx)
    defer client.Disconnect(ctx)

    // Request with small batch size
    var allRecords []testdata.OPCUALogRecord
    continuationPoint := []byte(nil)

    for {
        records, cp, err := client.GetRecords(ctx, startTime, endTime, 50, continuationPoint)
        require.NoError(t, err)

        allRecords = append(allRecords, records...)

        if len(cp) == 0 {
            break
        }
        continuationPoint = cp
    }

    assert.Equal(t, 150, len(allRecords))
}
```

### Testing Severity Filtering

```go
func TestSeverityFiltering(t *testing.T) {
    server := testdata.NewMockServer("opc.tcp://localhost:4840", logger)
    server.Start(ctx)
    defer server.Stop(ctx)

    // Add records with different severities
    server.AddLogRecords([]testdata.OPCUALogRecord{
        {Severity: 50, Message: "Debug"},   // Below threshold
        {Severity: 150, Message: "Info"},    // Below threshold
        {Severity: 250, Message: "Warning"}, // Above threshold
        {Severity: 350, Message: "Error"},   // Above threshold
    })

    client := testdata.NewMockClient(server, logger)
    client.Connect(ctx)

    // Request with minimum severity of Warning (201)
    records, _, err := client.GetRecordsWithSeverity(ctx, startTime, endTime, 100, 201, nil)
    require.NoError(t, err)

    assert.Equal(t, 2, len(records)) // Only Warning and Error
}
```

## Severity Levels

OPC UA severity mapping:

| Severity Range | Level     | OpenTelemetry Equivalent |
|----------------|-----------|--------------------------|
| 1-50           | Debug     | Debug                    |
| 51-100         | Trace     | Trace                    |
| 101-200        | Info      | Info                     |
| 201-300        | Warning   | Warn                     |
| 301-400        | Error     | Error                    |
| 401-1000       | Emergency | Fatal                    |

## Mock Server Features

### Adding Records

```go
// Add single record
server.AddLogRecord(testdata.OPCUALogRecord{...})

// Add multiple records
server.AddLogRecords([]testdata.OPCUALogRecord{...})

// Clear all records
server.ClearLogRecords()

// Get record count
count := server.GetLogRecordsCount()
```

### Server Control

```go
// Check if running
if server.IsRunning() {
    // Server is active
}

// Get endpoint
endpoint := server.Endpoint()
```

## Mock Client Features

### Connection Management

```go
// Connect to server
err := client.Connect(ctx)

// Disconnect from server
err := client.Disconnect(ctx)

// Check connection status
if client.IsConnected() {
    // Client is connected
}
```

### Fetching Records

```go
// Basic GetRecords (min severity = 0)
records, cp, err := client.GetRecords(ctx, startTime, endTime, maxRecords, continuationPoint)

// GetRecords with minimum severity
records, cp, err := client.GetRecordsWithSeverity(ctx, startTime, endTime, maxRecords, minSeverity, continuationPoint)
```

## Integration Test Examples

See [scraper_integration_test.go](../scraper_integration_test.go) for complete integration test examples:

- `TestScraperIntegration`: Basic end-to-end test
- `TestScraperIntegrationPagination`: Testing pagination with large datasets
- `TestScraperIntegrationFiltering`: Testing severity filtering

## Architecture

```
┌─────────────────┐
│  Integration    │
│     Test        │
└────────┬────────┘
         │
         ↓
┌─────────────────┐        ┌──────────────────┐
│  MockClient     │───────→│   MockServer     │
│  Adapter        │        │   (Handler)      │
└─────────────────┘        └──────────────────┘
         │                          │
         │                          ↓
         │                  ┌──────────────┐
         │                  │  In-Memory   │
         │                  │  Log Storage │
         │                  └──────────────┘
         ↓
┌─────────────────┐
│     Scraper     │
└─────────────────┘
         │
         ↓
┌─────────────────┐
│  Transformer    │
└─────────────────┘
         │
         ↓
┌─────────────────┐
│  OpenTelemetry  │
│      Logs       │
└─────────────────┘
```

## Limitations

- This is a mock implementation for testing only
- Does not implement full OPC UA protocol
- Not suitable for production use
- No network communication (in-memory only)
- No authentication/security implementation

## Future Enhancements

Potential improvements for the test server:

- Support for OPC UA subscriptions
- Event simulation
- Network protocol simulation
- Authentication testing
- Error injection for fault testing
- Performance testing utilities

## Contributing

When adding new test scenarios, please:

1. Add test cases to [scraper_integration_test.go](../scraper_integration_test.go)
2. Document new features in this README
3. Ensure all existing tests still pass
4. Add comments explaining complex test scenarios

## See Also

- [OPC UA Part 26 Specification](https://reference.opcfoundation.org/Core/Part26/)
- [OpenTelemetry Logs](https://opentelemetry.io/docs/specs/otel/logs/)
- [gopcua Library](https://github.com/gopcua/opcua)
