// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

using Opc.Ua;
using Opc.Ua.Server;

namespace OpcUaTestServer;

/// <summary>
/// Custom node manager that creates a ServerLog object with a GetRecords method
/// implementing OPC UA Part 26 log record retrieval for testing.
///
/// Also exposes a monitorable variable node (ns=2;i=1004) that is updated with
/// a new LogRecord ExtensionObject every 5 seconds so that subscription-based
/// receivers can be tested without polling.
/// </summary>
public class TestNodeManager : CustomNodeManager2
{
    public const string NamespaceUri = "urn:opcua:testserver";

    private const ushort ServerLogId = 1000;
    private const ushort GetRecordsMethodId = 1001;
    private const ushort GetRecordsInputArgsId = 1002;
    private const ushort GetRecordsOutputArgsId = 1003;

    // Variable node pushed to subscription clients every 5 seconds.
    private const ushort LatestLogRecordId = 1004;
    private const int SubscriptionPushIntervalMs = 5000;

    // TypeId for our custom LogRecord encoding
    public const ushort LogRecordTypeId = 5001;

    private readonly List<TestLogRecord> _fixedRecords;
    private readonly IServiceMessageContext _messageContext;

    // Variable node reference kept so we can update its value from the timer.
    private BaseDataVariableState? _latestLogRecordNode;
    private Timer? _pushTimer;
    private int _pushIndex = 0;

    public TestNodeManager(IServerInternal server, ApplicationConfiguration configuration)
        : base(server, configuration, NamespaceUri)
    {
        _fixedRecords = LogRecordData.GetFixedRecords();
        _messageContext = configuration.CreateMessageContext();
        SystemContext.NodeIdFactory = this;
    }

    public override void CreateAddressSpace(IDictionary<NodeId, IList<IReference>> externalReferences)
    {
        lock (Lock)
        {
            base.CreateAddressSpace(externalReferences);

            // Get the Objects folder to add our nodes under it
            if (!externalReferences.TryGetValue(ObjectIds.ObjectsFolder, out IList<IReference>? references))
            {
                references = new List<IReference>();
                externalReferences[ObjectIds.ObjectsFolder] = references;
            }

            // Create the ServerLog object (BaseObjectType, not FolderType,
            // because it owns methods via HasComponent references)
            var serverLogFolder = new BaseObjectState(null)
            {
                NodeId = new NodeId(ServerLogId, NamespaceIndex),
                BrowseName = new QualifiedName("ServerLog", NamespaceIndex),
                DisplayName = new LocalizedText("ServerLog"),
                Description = new LocalizedText("OPC UA Part 26 Log Object for testing"),
                TypeDefinitionId = ObjectTypeIds.BaseObjectType,
                WriteMask = AttributeWriteMask.None,
                UserWriteMask = AttributeWriteMask.None
            };

            // Add HasComponent reference from Objects folder to ServerLog
            references.Add(new NodeStateReference(
                ReferenceTypeIds.HasComponent,
                false,
                serverLogFolder.NodeId));
            serverLogFolder.AddReference(
                ReferenceTypeIds.HasComponent,
                true,
                ObjectIds.ObjectsFolder);

            // ── GetRecords method ─────────────────────────────────────────────

            var getRecordsMethod = new MethodState(serverLogFolder)
            {
                NodeId = new NodeId(GetRecordsMethodId, NamespaceIndex),
                BrowseName = new QualifiedName("GetRecords", NamespaceIndex),
                DisplayName = new LocalizedText("GetRecords"),
                Description = new LocalizedText("Retrieves log records (OPC UA Part 26)"),
                ReferenceTypeId = ReferenceTypeIds.HasComponent,
                Executable = true,
                UserExecutable = true
            };

            var inputArgs = new PropertyState<Argument[]>(getRecordsMethod)
            {
                NodeId = new NodeId(GetRecordsInputArgsId, NamespaceIndex),
                BrowseName = BrowseNames.InputArguments,
                DisplayName = new LocalizedText(BrowseNames.InputArguments),
                TypeDefinitionId = VariableTypeIds.PropertyType,
                ReferenceTypeId = ReferenceTypeIds.HasProperty,
                DataType = DataTypeIds.Argument,
                ValueRank = ValueRanks.OneDimension,
                Value = new Argument[]
                {
                    new Argument { Name = "StartTime", DataType = DataTypeIds.DateTime, ValueRank = ValueRanks.Scalar },
                    new Argument { Name = "EndTime", DataType = DataTypeIds.DateTime, ValueRank = ValueRanks.Scalar },
                    new Argument { Name = "MaxReturnRecords", DataType = DataTypeIds.UInt32, ValueRank = ValueRanks.Scalar },
                    new Argument { Name = "MinimumSeverity", DataType = DataTypeIds.UInt16, ValueRank = ValueRanks.Scalar },
                    new Argument { Name = "LogRecordMask", DataType = DataTypeIds.UInt32, ValueRank = ValueRanks.Scalar },
                    new Argument { Name = "ContinuationPoint", DataType = DataTypeIds.ByteString, ValueRank = ValueRanks.Scalar }
                }
            };
            getRecordsMethod.InputArguments = inputArgs;

            var outputArgs = new PropertyState<Argument[]>(getRecordsMethod)
            {
                NodeId = new NodeId(GetRecordsOutputArgsId, NamespaceIndex),
                BrowseName = BrowseNames.OutputArguments,
                DisplayName = new LocalizedText(BrowseNames.OutputArguments),
                TypeDefinitionId = VariableTypeIds.PropertyType,
                ReferenceTypeId = ReferenceTypeIds.HasProperty,
                DataType = DataTypeIds.Argument,
                ValueRank = ValueRanks.OneDimension,
                Value = new Argument[]
                {
                    new Argument { Name = "LogRecords", DataType = DataTypeIds.BaseDataType, ValueRank = ValueRanks.OneDimension },
                    new Argument { Name = "ContinuationPoint", DataType = DataTypeIds.ByteString, ValueRank = ValueRanks.Scalar }
                }
            };
            getRecordsMethod.OutputArguments = outputArgs;
            getRecordsMethod.OnCallMethod = new GenericMethodCalledEventHandler(OnGetRecordsCalled);
            serverLogFolder.AddChild(getRecordsMethod);

            // ── LatestLogRecord variable node (for subscription mode) ──────────
            //
            // Subscription clients monitor this node (ns=2;i=1004).  The server
            // updates it every 5 seconds with the next fixed test record, encoded
            // as a LogRecord ExtensionObject (same binary format as GetRecords).
            // This lets the subscription receiver be tested end-to-end without
            // modifying the polling path.

            _latestLogRecordNode = new BaseDataVariableState(serverLogFolder)
            {
                NodeId = new NodeId(LatestLogRecordId, NamespaceIndex),
                BrowseName = new QualifiedName("LatestLogRecord", NamespaceIndex),
                DisplayName = new LocalizedText("LatestLogRecord"),
                Description = new LocalizedText("Most recent LogRecord – updated every 5 s for subscription testing"),
                ReferenceTypeId = ReferenceTypeIds.HasComponent,
                TypeDefinitionId = VariableTypeIds.BaseDataVariableType,
                DataType = DataTypeIds.BaseDataType,
                ValueRank = ValueRanks.Scalar,
                AccessLevel = AccessLevels.CurrentRead,
                UserAccessLevel = AccessLevels.CurrentRead,
                Historizing = false,
                Value = new ExtensionObject(), // placeholder until first push
                StatusCode = StatusCodes.Good,
                Timestamp = DateTime.UtcNow
            };

            serverLogFolder.AddChild(_latestLogRecordNode);

            // Add the complete hierarchy as a single predefined node tree
            AddPredefinedNode(SystemContext, serverLogFolder);

            Console.WriteLine($"Address space created. ServerLog NodeId: ns={NamespaceIndex};i={ServerLogId}");
            Console.WriteLine($"GetRecords method NodeId: ns={NamespaceIndex};i={GetRecordsMethodId}");
            Console.WriteLine($"LatestLogRecord NodeId:   ns={NamespaceIndex};i={LatestLogRecordId}");
            Console.WriteLine($"Loaded {_fixedRecords.Count} fixed test records.");

            // Start the push timer after the address space is ready.
            _pushTimer = new Timer(PushNextLogRecord, null,
                TimeSpan.FromSeconds(1),                          // first push after 1 s
                TimeSpan.FromMilliseconds(SubscriptionPushIntervalMs));
        }
    }

    /// <summary>
    /// Timer callback: cycles through the fixed test records and writes the
    /// next one to the LatestLogRecord variable node so that subscribed clients
    /// receive a DataChangeNotification.
    /// </summary>
    private void PushNextLogRecord(object? state)
    {
        if (_latestLogRecordNode == null || _fixedRecords.Count == 0)
            return;

        var record = _fixedRecords[_pushIndex % _fixedRecords.Count];
        _pushIndex++;

        // Re-stamp the record with the current time so timestamps are realistic.
        var stamped = new TestLogRecord
        {
            Timestamp      = DateTime.UtcNow,
            Severity       = record.Severity,
            Message        = record.Message,
            SourceName     = record.SourceName,
            SourceNode     = record.SourceNode,
            EventType      = record.EventType,
            TraceContext   = record.TraceContext,
            AdditionalData = record.AdditionalData
        };

        var extObj = EncodeLogRecord(stamped);

        lock (Lock)
        {
            _latestLogRecordNode.Value = extObj;
            _latestLogRecordNode.StatusCode = StatusCodes.Good;
            _latestLogRecordNode.Timestamp = DateTime.UtcNow;
            _latestLogRecordNode.ClearChangeMasks(SystemContext, false);
        }

        Console.WriteLine($"[push #{_pushIndex}] severity={stamped.Severity} msg={stamped.Message}");
    }

    /// <summary>
    /// Handler for the GetRecords method call.
    /// </summary>
    private ServiceResult OnGetRecordsCalled(
        ISystemContext context,
        MethodState method,
        IList<object> inputArguments,
        IList<object> outputArguments)
    {
        DateTime startTime = (DateTime)inputArguments[0];
        DateTime endTime = (DateTime)inputArguments[1];
        uint maxRecords = (uint)inputArguments[2];
        ushort minSeverity = (ushort)inputArguments[3];
        byte[]? continuationPoint = inputArguments[5] as byte[];

        Console.WriteLine($"GetRecords called: StartTime={startTime:O}, EndTime={endTime:O}, " +
                          $"MaxRecords={maxRecords}, MinSeverity={minSeverity}");

        if (endTime < startTime)
            return new ServiceResult(StatusCodes.BadInvalidArgument);

        var filtered = _fixedRecords
            .Where(r => r.Timestamp >= startTime && r.Timestamp <= endTime)
            .Where(r => r.Severity >= minSeverity)
            .ToList();

        int startIndex = 0;
        if (continuationPoint != null && continuationPoint.Length >= 4)
            startIndex = BitConverter.ToInt32(continuationPoint, 0);

        if (startIndex > 0 && startIndex < filtered.Count)
            filtered = filtered.Skip(startIndex).ToList();
        else if (startIndex >= filtered.Count && startIndex > 0)
            filtered = new List<TestLogRecord>();

        byte[]? nextContinuationPoint = null;
        if (maxRecords > 0 && filtered.Count > (int)maxRecords)
        {
            filtered = filtered.Take((int)maxRecords).ToList();
            int nextOffset = startIndex + (int)maxRecords;
            nextContinuationPoint = BitConverter.GetBytes(nextOffset);
        }

        Console.WriteLine($"Returning {filtered.Count} records");

        var records = new ExtensionObject[filtered.Count];
        for (int i = 0; i < filtered.Count; i++)
            records[i] = EncodeLogRecord(filtered[i]);

        outputArguments[0] = records;
        outputArguments[1] = nextContinuationPoint ?? Array.Empty<byte>();

        return ServiceResult.Good;
    }

    /// <summary>
    /// Encodes a log record as an ExtensionObject with a binary body following OPC UA Part 26 §5.4.
    /// </summary>
    private ExtensionObject EncodeLogRecord(TestLogRecord record)
    {
        using var stream = new MemoryStream();
        using (var encoder = new BinaryEncoder(stream, _messageContext, true))
        {
            encoder.WriteDateTime(null, record.Timestamp);
            encoder.WriteUInt16(null, record.Severity);

            var eventTypeNodeId = string.IsNullOrEmpty(record.EventType)
                ? NodeId.Null : NodeId.Parse(record.EventType);
            encoder.WriteNodeId(null, eventTypeNodeId);

            var sourceNodeId = string.IsNullOrEmpty(record.SourceNode)
                ? NodeId.Null : NodeId.Parse(record.SourceNode);
            encoder.WriteNodeId(null, sourceNodeId);

            encoder.WriteString(null, record.SourceName);
            encoder.WriteLocalizedText(null, new LocalizedText(record.Message));

            var traceGuid = record.TraceContext?.TraceId ?? Guid.Empty;
            var guidBytes = traceGuid.ToByteArray();
            encoder.WriteUInt32(null, BitConverter.ToUInt32(guidBytes, 0));
            encoder.WriteUInt16(null, BitConverter.ToUInt16(guidBytes, 4));
            encoder.WriteUInt16(null, BitConverter.ToUInt16(guidBytes, 6));
            for (int i = 8; i < 16; i++) encoder.WriteByte(null, guidBytes[i]);

            encoder.WriteUInt64(null, record.TraceContext?.SpanId ?? 0UL);
            encoder.WriteUInt64(null, record.TraceContext?.ParentSpanId ?? 0UL);
            encoder.WriteString(null, record.TraceContext?.ParentIdentifier);

            var data = record.AdditionalData;
            encoder.WriteInt32(null, data?.Count ?? 0);
            if (data != null)
            {
                foreach (var (key, value) in data)
                {
                    encoder.WriteString(null, key);
                    encoder.WriteVariant(null, new Variant(value));
                }
            }
        }

        byte[] body = stream.ToArray();
        var typeId = new ExpandedNodeId(LogRecordTypeId);
        return new ExtensionObject(typeId, body);
    }
}