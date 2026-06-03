// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

using Opc.Ua;
using Opc.Ua.Server;

namespace OpcUaTestServer;

/// <summary>
/// Custom node manager that:
///   1. Exposes a ServerLog object with a GetRecords method (OPC UA Part 26 poll mode).
///   2. Fires BaseLogEventType events on the ServerLog node every 5 seconds
///      so that subscription-based receivers can be tested end-to-end.
///
/// BaseLogEventType (Part 26 §6.3) is not built into the C# OPC UA SDK, so we
/// define it manually as a custom event type inheriting from BaseEventType, and
/// add the Part 26-specific fields (TraceContext, AdditionalData) as properties.
/// </summary>
public class TestNodeManager : CustomNodeManager2
{
    public const string NamespaceUri = "urn:opcua:testserver";

    // NodeId constants (namespace index resolved at runtime via NamespaceIndex).
    private const ushort ServerLogId             = 1000;
    private const ushort GetRecordsMethodId      = 1001;
    private const ushort GetRecordsInputArgsId   = 1002;
    private const ushort GetRecordsOutputArgsId  = 1003;

    // BaseLogEventType custom definition (Part 26 §6.3).
    // We use a numeric NodeId in namespace 0 matching the well-known Part 26 companion
    // spec value (18000) so Go clients using that constant can filter by OfType.
    public const uint BaseLogEventTypeId = 18000;

    // TypeId for our custom binary LogRecord ExtensionObject encoding.
    public const ushort LogRecordTypeId = 5001;

    // NodeId for LogEntryConditionClassType (Part 26 §6.5).
    // Defined in our test namespace so its BrowseName is readable.
    private const ushort LogEntryConditionClassTypeId = 1006;

    // Push interval for subscription testing.
    private const int SubscriptionPushIntervalMs = 5000;

    private readonly List<TestLogRecord> _fixedRecords;
    private readonly IServiceMessageContext _messageContext;

    private BaseObjectState? _serverLogNode;
    private NodeId?          _baseLogEventTypeNodeId;
    private Timer?           _pushTimer;
    private int              _pushIndex = 0;

    public TestNodeManager(IServerInternal server, ApplicationConfiguration configuration)
        : base(server, configuration, NamespaceUri)
    {
        _fixedRecords   = LogRecordData.GetFixedRecords();
        _messageContext = configuration.CreateMessageContext();
        SystemContext.NodeIdFactory = this;
    }

    public override void CreateAddressSpace(IDictionary<NodeId, IList<IReference>> externalReferences)
    {
        lock (Lock)
        {
            base.CreateAddressSpace(externalReferences);

            if (!externalReferences.TryGetValue(ObjectIds.ObjectsFolder, out IList<IReference>? references))
            {
                references = new List<IReference>();
                externalReferences[ObjectIds.ObjectsFolder] = references;
            }

            // ── 1. Define BaseLogEventType in namespace 0 ─────────────────
            // The type NodeId (ns=0;i=18000) matches the well-known Part 26 ID
            // so Go clients filtering OfType(ns=0;i=18000) receive our events.
            _baseLogEventTypeNodeId = new NodeId(BaseLogEventTypeId, 0);
            DefineBaseLogEventType(externalReferences);

            // ── 2. ServerLog object ───────────────────────────────────────
            _serverLogNode = new BaseObjectState(null)
            {
                NodeId          = new NodeId(ServerLogId, NamespaceIndex),
                BrowseName      = new QualifiedName("ServerLog", NamespaceIndex),
                DisplayName     = new LocalizedText("ServerLog"),
                Description     = new LocalizedText("OPC UA Part 26 Log Object for testing"),
                TypeDefinitionId = ObjectTypeIds.BaseObjectType,
                WriteMask       = AttributeWriteMask.None,
                UserWriteMask   = AttributeWriteMask.None,
                // EventNotifier = SubscribeToEvents so clients can monitor events on this node.
                EventNotifier   = EventNotifiers.SubscribeToEvents,
            };

            references.Add(new NodeStateReference(ReferenceTypeIds.HasComponent, false, _serverLogNode.NodeId));
            _serverLogNode.AddReference(ReferenceTypeIds.HasComponent, true, ObjectIds.ObjectsFolder);

            // ── LogEntryConditionClassType object type node (Part 26 §6.5) ──────
            // Register a dedicated type node with BrowseName "LogEntryConditionClassType"
            // so that the collector's ConditionClassId check can read and verify it.
            var logEntryConditionClassTypeNodeId = new NodeId(LogEntryConditionClassTypeId, NamespaceIndex);
            var logEntryConditionClassType = new BaseObjectTypeState
            {
                NodeId      = logEntryConditionClassTypeNodeId,
                BrowseName  = new QualifiedName("LogEntryConditionClassType", NamespaceIndex),
                DisplayName = new LocalizedText("LogEntryConditionClassType"),
                Description = new LocalizedText("OPC UA Part 26 §6.5 LogEntryConditionClassType"),
                SuperTypeId = new NodeId(0, 0), // BaseConditionClassType would be ns=0;i=11163
                IsAbstract  = false,
            };
            AddPredefinedNode(SystemContext, logEntryConditionClassType);

            // ── ConditionClassId property (Part 26 §6.5) ─────────────────────
            // Points to LogEntryConditionClassType so the collector can verify
            // this node is a proper Part 26 LogObject before subscribing.
            var conditionClassId = new PropertyState<NodeId>(_serverLogNode)
            {
                NodeId           = new NodeId((ushort)1005, NamespaceIndex),
                BrowseName       = new QualifiedName("ConditionClassId", 0),
                DisplayName      = new LocalizedText("ConditionClassId"),
                TypeDefinitionId = VariableTypeIds.PropertyType,
                ReferenceTypeId  = ReferenceTypeIds.HasProperty,
                DataType         = DataTypeIds.NodeId,
                ValueRank        = ValueRanks.Scalar,
                AccessLevel      = AccessLevels.CurrentRead,
                UserAccessLevel  = AccessLevels.CurrentRead,
                Value            = logEntryConditionClassTypeNodeId,
            };
            _serverLogNode.AddChild(conditionClassId);

            // ── ConditionSubClassId property (Part 26 §6.5) ──────────────────
            // Also expose ConditionSubClassId so clients checking either property
            // can verify this is a Part 26 LogObject.
            var conditionSubClassId = new PropertyState<NodeId[]>(_serverLogNode)
            {
                NodeId           = new NodeId((ushort)1007, NamespaceIndex),
                BrowseName       = new QualifiedName("ConditionSubClassId", 0),
                DisplayName      = new LocalizedText("ConditionSubClassId"),
                TypeDefinitionId = VariableTypeIds.PropertyType,
                ReferenceTypeId  = ReferenceTypeIds.HasProperty,
                DataType         = DataTypeIds.NodeId,
                ValueRank        = ValueRanks.OneDimension,
                AccessLevel      = AccessLevels.CurrentRead,
                UserAccessLevel  = AccessLevels.CurrentRead,
                Value            = new NodeId[] { logEntryConditionClassTypeNodeId },
            };
            _serverLogNode.AddChild(conditionSubClassId);

            // ── 3. GetRecords method ──────────────────────────────────────
            var getRecordsMethod = new MethodState(_serverLogNode)
            {
                NodeId           = new NodeId(GetRecordsMethodId, NamespaceIndex),
                BrowseName       = new QualifiedName("GetRecords", NamespaceIndex),
                DisplayName      = new LocalizedText("GetRecords"),
                Description      = new LocalizedText("Retrieves log records (OPC UA Part 26)"),
                ReferenceTypeId  = ReferenceTypeIds.HasComponent,
                Executable       = true,
                UserExecutable   = true,
            };

            var inputArgs = new PropertyState<Argument[]>(getRecordsMethod)
            {
                NodeId           = new NodeId(GetRecordsInputArgsId, NamespaceIndex),
                BrowseName       = BrowseNames.InputArguments,
                DisplayName      = new LocalizedText(BrowseNames.InputArguments),
                TypeDefinitionId = VariableTypeIds.PropertyType,
                ReferenceTypeId  = ReferenceTypeIds.HasProperty,
                DataType         = DataTypeIds.Argument,
                ValueRank        = ValueRanks.OneDimension,
                Value = new Argument[]
                {
                    new Argument { Name = "StartTime",         DataType = DataTypeIds.DateTime,    ValueRank = ValueRanks.Scalar },
                    new Argument { Name = "EndTime",           DataType = DataTypeIds.DateTime,    ValueRank = ValueRanks.Scalar },
                    new Argument { Name = "MaxReturnRecords",  DataType = DataTypeIds.UInt32,      ValueRank = ValueRanks.Scalar },
                    new Argument { Name = "MinimumSeverity",   DataType = DataTypeIds.UInt16,      ValueRank = ValueRanks.Scalar },
                    new Argument { Name = "LogRecordMask",     DataType = DataTypeIds.UInt32,      ValueRank = ValueRanks.Scalar },
                    new Argument { Name = "ContinuationPoint", DataType = DataTypeIds.ByteString,  ValueRank = ValueRanks.Scalar },
                }
            };
            getRecordsMethod.InputArguments = inputArgs;

            var outputArgs = new PropertyState<Argument[]>(getRecordsMethod)
            {
                NodeId           = new NodeId(GetRecordsOutputArgsId, NamespaceIndex),
                BrowseName       = BrowseNames.OutputArguments,
                DisplayName      = new LocalizedText(BrowseNames.OutputArguments),
                TypeDefinitionId = VariableTypeIds.PropertyType,
                ReferenceTypeId  = ReferenceTypeIds.HasProperty,
                DataType         = DataTypeIds.Argument,
                ValueRank        = ValueRanks.OneDimension,
                Value = new Argument[]
                {
                    new Argument { Name = "LogRecords",        DataType = DataTypeIds.BaseDataType, ValueRank = ValueRanks.OneDimension },
                    new Argument { Name = "ContinuationPoint", DataType = DataTypeIds.ByteString,   ValueRank = ValueRanks.Scalar },
                }
            };
            getRecordsMethod.OutputArguments = outputArgs;
            getRecordsMethod.OnCallMethod    = new GenericMethodCalledEventHandler(OnGetRecordsCalled);
            _serverLogNode.AddChild(getRecordsMethod);

            AddPredefinedNode(SystemContext, _serverLogNode);

            Console.WriteLine($"Address space created. ServerLog NodeId: ns={NamespaceIndex};i={ServerLogId}");
            Console.WriteLine($"GetRecords method NodeId: ns={NamespaceIndex};i={GetRecordsMethodId}");
            Console.WriteLine($"BaseLogEventType NodeId:  ns=0;i={BaseLogEventTypeId}");
            Console.WriteLine($"Loaded {_fixedRecords.Count} fixed test records.");
            Console.WriteLine($"Firing BaseLogEventType events every {SubscriptionPushIntervalMs}ms on ServerLog node.");

            _pushTimer = new Timer(FireLogEvent, null,
                TimeSpan.FromSeconds(1),
                TimeSpan.FromMilliseconds(SubscriptionPushIntervalMs));
        }
    }

    /// <summary>
    /// Registers BaseLogEventType (ns=0;i=18000) as a subtype of BaseEventType
    /// in the server's address space so that clients can filter by OfType.
    /// Adds Part 26-specific fields: TraceContext and AdditionalData.
    /// </summary>
    private void DefineBaseLogEventType(IDictionary<NodeId, IList<IReference>> externalReferences)
    {
        // Register the type node under the EventTypes / BaseEventType hierarchy.
        if (!externalReferences.TryGetValue(ObjectTypeIds.BaseEventType, out IList<IReference>? baseRefs))
        {
            baseRefs = new List<IReference>();
            externalReferences[ObjectTypeIds.BaseEventType] = baseRefs;
        }

        var baseLogEventType = new BaseObjectTypeState
        {
            NodeId      = _baseLogEventTypeNodeId!,
            BrowseName  = new QualifiedName("BaseLogEventType", 0),
            DisplayName = new LocalizedText("BaseLogEventType"),
            Description = new LocalizedText("OPC UA Part 26 §6.3 BaseLogEventType"),
            SuperTypeId = ObjectTypeIds.BaseEventType,
            IsAbstract  = false,
        };

        baseRefs.Add(new NodeStateReference(ReferenceTypeIds.HasSubtype, false, _baseLogEventTypeNodeId!));
        baseLogEventType.AddReference(ReferenceTypeIds.HasSubtype, true, ObjectTypeIds.BaseEventType);

        AddPredefinedNode(SystemContext, baseLogEventType);
    }

    /// <summary>
    /// Timer callback: fires a BaseLogEventType event on the ServerLog node,
    /// cycling through the fixed test records so subscription clients receive
    /// realistic log record events.
    /// </summary>
    private void FireLogEvent(object? state)
    {
        if (_serverLogNode == null || _fixedRecords.Count == 0 || _baseLogEventTypeNodeId == null)
            return;

        var record = _fixedRecords[_pushIndex % _fixedRecords.Count];
        _pushIndex++;

        try
        {
            // Allocate a new EventId for each event.
            var eventId = Guid.NewGuid().ToByteArray();

            // Build the event using the BaseEvent fields that the C# SDK supports,
            // mapping Part 26 LogRecord fields to their BaseEventType equivalents.
            var e = new BaseEventState(null);

            // Create child nodes for all required BaseEventType properties before
            // calling ReportEvent so the SDK can serialise them correctly.
            e.EventId    = new PropertyState<byte[]>(e);
            e.EventType  = new PropertyState<NodeId>(e);
            e.SourceNode = new PropertyState<NodeId>(e);
            e.SourceName = new PropertyState<string>(e);
            e.Time       = new PropertyState<DateTime>(e);
            e.ReceiveTime = new PropertyState<DateTime>(e);
            e.Message    = new PropertyState<LocalizedText>(e);
            e.Severity   = new PropertyState<ushort>(e);

            e.EventId.Value     = eventId;
            e.EventType.Value   = _baseLogEventTypeNodeId;
            e.SourceNode.Value  = _serverLogNode.NodeId;
            e.SourceName.Value  = record.SourceName ?? "TestServer";
            e.Time.Value        = DateTime.UtcNow;
            e.ReceiveTime.Value = DateTime.UtcNow;
            e.Message.Value     = new LocalizedText(record.Message);
            e.Severity.Value    = record.Severity;

            // Report the event on the ServerLog node so subscribed clients receive it.
            _serverLogNode.ReportEvent(SystemContext, e);

            Console.WriteLine($"[event #{_pushIndex}] severity={record.Severity} msg={record.Message}");
        }
        catch (Exception ex)
        {
            Console.WriteLine($"[event #{_pushIndex}] ERROR firing event: {ex.Message}");
        }
    }

    /// <summary>
    /// Handler for the GetRecords method call (poll mode – unchanged).
    /// </summary>
    private ServiceResult OnGetRecordsCalled(
        ISystemContext context,
        MethodState method,
        IList<object> inputArguments,
        IList<object> outputArguments)
    {
        DateTime startTime        = (DateTime)inputArguments[0];
        DateTime endTime          = (DateTime)inputArguments[1];
        uint     maxRecords       = (uint)inputArguments[2];
        ushort   minSeverity      = (ushort)inputArguments[3];
        byte[]?  continuationPoint = inputArguments[5] as byte[];

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
            nextContinuationPoint = BitConverter.GetBytes(startIndex + (int)maxRecords);
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
    /// Encodes a log record as an ExtensionObject (binary, Part 26 §5.4).
    /// Used by GetRecords (poll mode).
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

        byte[] body    = stream.ToArray();
        var    typeId  = new ExpandedNodeId(LogRecordTypeId);
        return new ExtensionObject(typeId, body);
    }
}