// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

using Opc.Ua;
using Opc.Ua.Server;
using System.Buffers.Binary;

namespace OpcUaTestServer;

public class TestNodeManager : CustomNodeManager2
{
    public const string NamespaceUri = "urn:opcua:testserver";
    private const ushort ServerLogId = 1000;
    private const int SubscriptionPushIntervalMs = 5000;
    private const int MaxRecordsPerResponse = 3;

    private readonly List<TestLogRecord> _fixedRecords = LogRecordData.GetFixedRecords();
    private LogObjectState? _serverLogNode;
    private Timer? _pushTimer;
    private int _pushIndex;

    public TestNodeManager(IServerInternal server, ApplicationConfiguration configuration)
        : base(server, configuration, NamespaceUri)
    {
        SystemContext.NodeIdFactory = this;
    }

    public override void CreateAddressSpace(IDictionary<NodeId, IList<IReference>> externalReferences)
    {
        lock (Lock)
        {
            base.CreateAddressSpace(externalReferences);
            if (!externalReferences.TryGetValue(ObjectIds.ObjectsFolder, out var references))
            {
                references = new List<IReference>();
                externalReferences[ObjectIds.ObjectsFolder] = references;
            }

            var log = new LogObjectState(null);
            log.Create(SystemContext, new NodeId(ServerLogId, NamespaceIndex),
                new QualifiedName("ServerLog", NamespaceIndex), new LocalizedText("ServerLog"), true);
            log.Description = new LocalizedText("OPC UA Part 26 Log Object for testing");
            log.EventNotifier = EventNotifiers.SubscribeToEvents;
            log.AddReference(ReferenceTypeIds.HasComponent, true, ObjectIds.ObjectsFolder);
            references.Add(new NodeStateReference(ReferenceTypeIds.HasComponent, false, log.NodeId));
            var getRecords = log.GetRecords ?? throw new InvalidOperationException("LogObject has no GetRecords method");
            getRecords.OnCall = OnGetRecordsCalled;

            // Existing subscription clients inspect this property on the notifier node.
            var conditionClass = PropertyState<NodeId>.With<VariantBuilder>(log);
            conditionClass.NodeId = new NodeId((ushort)1005, NamespaceIndex);
            conditionClass.BrowseName = new QualifiedName("ConditionClassId", 0);
            conditionClass.DisplayName = new LocalizedText("ConditionClassId");
            conditionClass.TypeDefinitionId = VariableTypeIds.PropertyType;
            conditionClass.ReferenceTypeId = ReferenceTypeIds.HasProperty;
            conditionClass.DataType = DataTypeIds.NodeId;
            conditionClass.ValueRank = ValueRanks.Scalar;
            conditionClass.AccessLevel = AccessLevels.CurrentRead;
            conditionClass.UserAccessLevel = AccessLevels.CurrentRead;
            conditionClass.Value = ObjectTypeIds.LogEntryConditionClassType;
            log.AddChild(conditionClass);

            var conditionSubClass = PropertyState<ArrayOf<NodeId>>.With<VariantBuilder>(log);
            conditionSubClass.NodeId = new NodeId((ushort)1007, NamespaceIndex);
            conditionSubClass.BrowseName = new QualifiedName("ConditionSubClassId", 0);
            conditionSubClass.DisplayName = new LocalizedText("ConditionSubClassId");
            conditionSubClass.TypeDefinitionId = VariableTypeIds.PropertyType;
            conditionSubClass.ReferenceTypeId = ReferenceTypeIds.HasProperty;
            conditionSubClass.DataType = DataTypeIds.NodeId;
            conditionSubClass.ValueRank = ValueRanks.OneDimension;
            conditionSubClass.AccessLevel = AccessLevels.CurrentRead;
            conditionSubClass.UserAccessLevel = AccessLevels.CurrentRead;
            conditionSubClass.Value = ArrayOf.ToArrayOf(new[] { ObjectTypeIds.LogEntryConditionClassType });
            log.AddChild(conditionSubClass);

            AddPredefinedNode(SystemContext, log);
            _serverLogNode = log;
            Console.WriteLine($"Address space created. ServerLog NodeId: {log.NodeId}");
            Console.WriteLine($"Loaded {_fixedRecords.Count} fixed test records.");
            _pushTimer = new Timer(FireLogEvent, null, TimeSpan.FromSeconds(1),
                TimeSpan.FromMilliseconds(SubscriptionPushIntervalMs));
        }
    }

    private void FireLogEvent(object? state)
    {
        var node = _serverLogNode;
        if (node == null || _fixedRecords.Count == 0)
            return;

        var record = _fixedRecords[(Interlocked.Increment(ref _pushIndex) - 1) % _fixedRecords.Count];
        try
        {
            var e = new BaseLogEventState(null);
            e.Initialize(SystemContext, node, (EventSeverity)record.Severity, new LocalizedText(record.Message));
            (e.EventType ?? throw new InvalidOperationException("EventType is missing")).Value = ObjectTypeIds.BaseLogEventType;
            (e.SourceNode ?? throw new InvalidOperationException("SourceNode is missing")).Value = node.NodeId;
            (e.SourceName ?? throw new InvalidOperationException("SourceName is missing")).Value = record.SourceName;
            (e.Time ?? throw new InvalidOperationException("Time is missing")).Value = DateTimeUtc.Now;
            (e.ReceiveTime ?? throw new InvalidOperationException("ReceiveTime is missing")).Value = DateTimeUtc.Now;
            (e.Severity ?? throw new InvalidOperationException("Severity is missing")).Value = record.Severity;
            node.ReportEvent(SystemContext, e);
            Console.WriteLine($"[event #{_pushIndex}] severity={record.Severity} msg={record.Message}");
        }

        catch (Exception ex)
        {
            Console.WriteLine($"[event #{_pushIndex}] ERROR firing event: {ex.Message}");
        }
    }

    public override void DeleteAddressSpace()
    {
        _pushTimer?.Dispose();
        _pushTimer = null;
        _serverLogNode = null;
        base.DeleteAddressSpace();
    }

    private ServiceResult OnGetRecordsCalled(
        ISystemContext context, MethodState method, NodeId objectId,
        DateTimeUtc startTime, DateTimeUtc endTime, uint maxRecords, ushort minSeverity,
        uint requestMask, ByteString continuationPoint,
        ref LogRecordsDataType results, ref ByteString nextContinuationPoint)
    {
        if (endTime.ToDateTime() < startTime.ToDateTime())
            return new ServiceResult(StatusCodes.BadInvalidArgument);
        if (minSeverity is < 1 or > 1000)
            return new ServiceResult(StatusCodes.BadOutOfRange);

        var filtered = _fixedRecords
            .Where(r => r.Timestamp >= startTime.ToDateTime() && r.Timestamp <= endTime.ToDateTime())
            .Where(r => r.Severity >= minSeverity)
            .ToList();

        int startIndex = 0;
        if (!continuationPoint.IsNull && !continuationPoint.IsEmpty)
        {
            if (continuationPoint.Length != 30)
                return new ServiceResult(StatusCodes.BadContinuationPointInvalid);
            var token = continuationPoint.ToArray();
            startIndex = BinaryPrimitives.ReadInt32LittleEndian(token);
            if (startIndex < 0 || startIndex >= filtered.Count ||
                !token.SequenceEqual(CreateContinuationPoint(startIndex, startTime, endTime, maxRecords, minSeverity, requestMask)))
                return new ServiceResult(StatusCodes.BadContinuationPointInvalid);
        }

        var remaining = filtered.Skip(startIndex);
        int pageSize = maxRecords == 0 ? MaxRecordsPerResponse : (int)Math.Min(maxRecords, MaxRecordsPerResponse);
        var page = remaining.Take(pageSize).ToList();
        nextContinuationPoint = startIndex + page.Count < filtered.Count
            ? ByteString.From(CreateContinuationPoint(startIndex + page.Count, startTime, endTime, maxRecords, minSeverity, requestMask))
            : default;
        results = new LogRecordsDataType
        {
            LogRecordArray = ArrayOf.ToArrayOf(page.Select(r => ToLogRecord(r, requestMask)))
        };
        Console.WriteLine($"Returning {page.Count} records (max={maxRecords}, offset={startIndex})");
        return ServiceResult.Good;
    }

    private static byte[] CreateContinuationPoint(int offset, DateTimeUtc startTime, DateTimeUtc endTime,
        uint maxRecords, ushort minSeverity, uint requestMask)
    {
        var token = new byte[30];
        BinaryPrimitives.WriteInt32LittleEndian(token.AsSpan(0, 4), offset);
        BinaryPrimitives.WriteInt64LittleEndian(token.AsSpan(4, 8), startTime.ToDateTime().Ticks);
        BinaryPrimitives.WriteInt64LittleEndian(token.AsSpan(12, 8), endTime.ToDateTime().Ticks);
        BinaryPrimitives.WriteUInt32LittleEndian(token.AsSpan(20, 4), maxRecords);
        BinaryPrimitives.WriteUInt16LittleEndian(token.AsSpan(24, 2), minSeverity);
        BinaryPrimitives.WriteUInt32LittleEndian(token.AsSpan(26, 4), requestMask);
        return token;
    }

    private static LogRecord ToLogRecord(TestLogRecord record, uint requestMask)
    {
        uint mask = requestMask & (uint)(LogRecordFields.EventType | LogRecordFields.SourceNode | LogRecordFields.SourceName);
        if ((requestMask & (uint)LogRecordFields.TraceContext) != 0 && record.TraceContext != null)
            mask |= (uint)LogRecordFields.TraceContext;
        if ((requestMask & (uint)LogRecordFields.AdditionalData) != 0 && record.AdditionalData != null)
            mask |= (uint)LogRecordFields.AdditionalData;

        return new LogRecord
        {
            EncodingMask = mask,
            Time = new DateTimeUtc(record.Timestamp),
            Severity = record.Severity,
            EventType = NodeId.Parse(record.EventType),
            SourceNode = NodeId.Parse(record.SourceNode),
            SourceName = record.SourceName,
            Message = new LocalizedText(record.Message),
            TraceContext = record.TraceContext == null ? new TraceContextDataType() : new TraceContextDataType
            {
                TraceId = new Uuid(record.TraceContext.TraceId),
                SpanId = record.TraceContext.SpanId,
                ParentSpanId = record.TraceContext.ParentSpanId,
                ParentIdentifier = record.TraceContext.ParentIdentifier ?? string.Empty
            },
            AdditionalData = ArrayOf.ToArrayOf((record.AdditionalData ?? new Dictionary<string, string>())
                .Select(pair => new NameValuePair { Name = pair.Key, Value = new Variant(pair.Value) }))
        };
    }
}
