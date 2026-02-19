// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

using System.Diagnostics;

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
                Timestamp   = BaseTime,
                Severity    = 150,
                Message     = "System startup initiated",
                SourceName  = "SystemComponent",
                SourceNode  = "ns=1;i=100",
                EventType   = "ns=0;i=2041",
                TraceContext = CreateTraceContext(
                    "0102030405060708090a0b0c0d0e0f10",
                    "0102030405060708"),
                AdditionalData = new Dictionary<string, string>
                {
                    { "component", "system" },
                    { "version",   "1.0.0"  }
                }
            },
            new TestLogRecord
            {
                Timestamp   = BaseTime.AddMinutes(1),
                Severity    = 300,
                Message     = "Configuration loaded successfully",
                SourceName  = "SystemComponent",
                SourceNode  = "ns=1;i=100",
                EventType   = "ns=0;i=2041",
                TraceContext = CreateTraceContext(
                    "aabbccddeeff00112233445566778899",
                    "aabbccddeeff0011"),
                AdditionalData = new Dictionary<string, string>
                {
                    { "config_file", "/etc/server/config.yaml" }
                }
            },
            new TestLogRecord
            {
                Timestamp  = BaseTime.AddMinutes(2),
                Severity   = 300,
                Message    = "Connection established to database",
                SourceName = "NetworkModule",
                SourceNode = "ns=1;i=102",
                EventType  = "ns=0;i=2041"
            },
            new TestLogRecord
            {
                Timestamp  = BaseTime.AddMinutes(3),
                Severity   = 150,
                Message    = "Data processing pipeline started",
                SourceName = "DataLogger",
                SourceNode = "ns=1;i=103",
                EventType  = "ns=0;i=2041"
            },
            new TestLogRecord
            {
                Timestamp   = BaseTime.AddMinutes(4),
                Severity    = 300,
                Message     = "Sensor reading collected: temperature=22.5",
                SourceName  = "DataLogger",
                SourceNode  = "ns=1;i=103",
                EventType   = "ns=0;i=2041",
                AdditionalData = new Dictionary<string, string>
                {
                    { "sensor_id",   "temp-01" },
                    { "unit",        "Celsius" }
                }
            },
            new TestLogRecord
            {
                Timestamp  = BaseTime.AddMinutes(5),
                Severity   = 500,
                Message    = "High memory usage detected: 85%",
                SourceName = "SystemComponent",
                SourceNode = "ns=1;i=100",
                EventType  = "ns=0;i=2041",
                TraceContext = CreateTraceContext(
                    "ffeeddccbbaa99887766554433221100",
                    "ffeeddccbbaa9988"),
                AdditionalData = new Dictionary<string, string>
                {
                    { "memory_percent", "85" }
                }
            },
            new TestLogRecord
            {
                Timestamp  = BaseTime.AddMinutes(6),
                Severity   = 300,
                Message    = "Security scan completed",
                SourceName = "SecurityModule",
                SourceNode = "ns=1;i=104",
                EventType  = "ns=0;i=2041"
            },
            new TestLogRecord
            {
                Timestamp  = BaseTime.AddMinutes(7),
                Severity   = 700,
                Message    = "Connection timeout to external service",
                SourceName = "NetworkModule",
                SourceNode = "ns=1;i=102",
                EventType  = "ns=0;i=2041",
                TraceContext = CreateTraceContext(
                    "deadbeefcafe000011223344aabbccdd",
                    "deadbeefcafe0000",
                    parentSpanIdHex: "0102030405060708"),
                AdditionalData = new Dictionary<string, string>
                {
                    { "service",         "external-api" },
                    { "timeout_seconds", "30"           }
                }
            },
            new TestLogRecord
            {
                Timestamp  = BaseTime.AddMinutes(8),
                Severity   = 300,
                Message    = "Backup process completed",
                SourceName = "DataLogger",
                SourceNode = "ns=1;i=103",
                EventType  = "ns=0;i=2041"
            },
            new TestLogRecord
            {
                Timestamp  = BaseTime.AddMinutes(9),
                Severity   = 150,
                Message    = "Garbage collection cycle completed",
                SourceName = "SystemComponent",
                SourceNode = "ns=1;i=100",
                EventType  = "ns=0;i=2041"
            }
        };
    }

    /// <summary>
    /// Creates a deterministic TraceContext from W3C hex strings using the Activity model types.
    /// </summary>
    /// <param name="traceIdHex">32-char W3C TraceId hex string.</param>
    /// <param name="spanIdHex">16-char W3C SpanId hex string.</param>
    /// <param name="parentSpanIdHex">Optional 16-char parent SpanId hex string; null for a root span.</param>
    private static TestTraceContext CreateTraceContext(
        string traceIdHex,
        string spanIdHex,
        string? parentSpanIdHex = null)
    {
        // Use W3C Activity model types for parsing
        var traceId = ActivityTraceId.CreateFromString(traceIdHex.AsSpan());
        var spanId  = ActivitySpanId.CreateFromString(spanIdHex.AsSpan());

        // Convert ActivityTraceId → Guid (byte order preserved via CopyTo + new Guid(byte[]))
        Span<byte> traceBytes = stackalloc byte[16];
        traceId.CopyTo(traceBytes);
        var traceGuid = new Guid(traceBytes);

        // Convert ActivitySpanId → ulong (W3C big-endian bytes → reverse → little-endian value)
        Span<byte> spanBytes = stackalloc byte[8];
        spanId.CopyTo(spanBytes);
        spanBytes.Reverse();
        ulong spanIdValue = BitConverter.ToUInt64(spanBytes);

        ulong parentSpanIdValue = 0;
        if (parentSpanIdHex != null)
        {
            var parentSpanId = ActivitySpanId.CreateFromString(parentSpanIdHex.AsSpan());
            Span<byte> parentBytes = stackalloc byte[8];
            parentSpanId.CopyTo(parentBytes);
            parentBytes.Reverse();
            parentSpanIdValue = BitConverter.ToUInt64(parentBytes);
        }

        return new TestTraceContext
        {
            TraceId          = traceGuid,
            SpanId           = spanIdValue,
            ParentSpanId     = parentSpanIdValue,
            ParentIdentifier = null
        };
    }
}

/// <summary>
/// Represents the OPC UA Part 26 §5.5.3 TraceContextDataType inline encoding.
/// Fields (binary order): TraceId (Guid), SpanId (UInt64), ParentSpanId (UInt64), ParentIdentifier (String).
/// </summary>
public class TestTraceContext
{
    /// <summary>OPC UA Guid encoding of the W3C 128-bit TraceId.</summary>
    public Guid TraceId { get; set; }

    /// <summary>W3C SpanId encoded as UInt64 (big-endian byte value).</summary>
    public ulong SpanId { get; set; }

    /// <summary>Parent SpanId; 0 for a root span.</summary>
    public ulong ParentSpanId { get; set; }

    /// <summary>ApplicationUri of the parent OPC UA application; null when local.</summary>
    public string? ParentIdentifier { get; set; }
}

/// <summary>
/// Represents a single OPC UA Part 26 LogRecord for testing.
/// </summary>
public class TestLogRecord
{
    /// <summary>Time associated with this record (mandatory).</summary>
    public DateTime Timestamp { get; set; }

    /// <summary>Severity 1–1000 per Part 26 §5.4 Table 5 (mandatory).</summary>
    public ushort Severity { get; set; }

    /// <summary>EventType NodeId string, e.g. "ns=0;i=2041" (BaseEventType). Optional field (bit 0).</summary>
    public string EventType { get; set; } = "ns=0;i=2041";

    /// <summary>SourceNode NodeId string, e.g. "ns=1;i=100". Optional field (bit 1).</summary>
    public string SourceNode { get; set; } = string.Empty;

    /// <summary>Human-readable source name (BrowseName of SourceNode). Optional field (bit 2).</summary>
    public string SourceName { get; set; } = string.Empty;

    /// <summary>Human-readable log message (mandatory).</summary>
    public string Message { get; set; } = string.Empty;

    /// <summary>W3C-compatible distributed trace context. Optional field (bit 3). Null = absent.</summary>
    public TestTraceContext? TraceContext { get; set; }

    /// <summary>Additional name-value pairs. Optional field (bit 4). Null = absent.</summary>
    public Dictionary<string, string>? AdditionalData { get; set; }
}
