// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

namespace OpcUaTestServer;

/// <summary>
/// Holds the 10 fixed test log records used for integration testing.
/// All timestamps are deterministic so the file export output can be compared.
/// </summary>
public static class LogRecordData
{
    /// <summary>
    /// Base time for all test records: 2025-01-15T10:00:00Z
    /// </summary>
    public static readonly DateTime BaseTime = new DateTime(2025, 1, 15, 10, 0, 0, DateTimeKind.Utc);

    public static List<TestLogRecord> GetFixedRecords()
    {
        return new List<TestLogRecord>
        {
            new TestLogRecord
            {
                Timestamp = BaseTime,
                Severity = 150,
                Message = "System startup initiated",
                Source = "SystemComponent"
            },
            new TestLogRecord
            {
                Timestamp = BaseTime.AddMinutes(1),
                Severity = 300,
                Message = "Configuration loaded successfully",
                Source = "SystemComponent"
            },
            new TestLogRecord
            {
                Timestamp = BaseTime.AddMinutes(2),
                Severity = 300,
                Message = "Connection established to database",
                Source = "NetworkModule"
            },
            new TestLogRecord
            {
                Timestamp = BaseTime.AddMinutes(3),
                Severity = 150,
                Message = "Data processing pipeline started",
                Source = "DataLogger"
            },
            new TestLogRecord
            {
                Timestamp = BaseTime.AddMinutes(4),
                Severity = 300,
                Message = "Sensor reading collected: temperature=22.5",
                Source = "DataLogger"
            },
            new TestLogRecord
            {
                Timestamp = BaseTime.AddMinutes(5),
                Severity = 500,
                Message = "High memory usage detected: 85%",
                Source = "SystemComponent"
            },
            new TestLogRecord
            {
                Timestamp = BaseTime.AddMinutes(6),
                Severity = 300,
                Message = "Security scan completed",
                Source = "SecurityModule"
            },
            new TestLogRecord
            {
                Timestamp = BaseTime.AddMinutes(7),
                Severity = 700,
                Message = "Connection timeout to external service",
                Source = "NetworkModule"
            },
            new TestLogRecord
            {
                Timestamp = BaseTime.AddMinutes(8),
                Severity = 300,
                Message = "Backup process completed",
                Source = "DataLogger"
            },
            new TestLogRecord
            {
                Timestamp = BaseTime.AddMinutes(9),
                Severity = 150,
                Message = "Garbage collection cycle completed",
                Source = "SystemComponent"
            }
        };
    }
}

/// <summary>
/// Represents a single test log record.
/// </summary>
public class TestLogRecord
{
    public DateTime Timestamp { get; set; }
    public ushort Severity { get; set; }
    public string Message { get; set; } = string.Empty;
    public string Source { get; set; } = string.Empty;
}
