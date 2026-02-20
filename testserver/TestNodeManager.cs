// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

using Opc.Ua;
using Opc.Ua.Server;

namespace OpcUaTestServer;

/// <summary>
/// Custom node manager that creates a ServerLog object with a GetRecords method
/// implementing OPC UA Part 26 log record retrieval for testing.
/// </summary>
public class TestNodeManager : CustomNodeManager2
{
    public const string NamespaceUri = "urn:opcua:testserver";

    private const ushort ServerLogId = 1000;
    private const ushort GetRecordsMethodId = 1001;
    private const ushort GetRecordsInputArgsId = 1002;
    private const ushort GetRecordsOutputArgsId = 1003;

    // TypeId for our custom LogRecord encoding
    public const ushort LogRecordTypeId = 5001;

    private readonly List<TestLogRecord> _fixedRecords;
    private readonly IServiceMessageContext _messageContext;

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

            // Create the GetRecords method as a child of ServerLog
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

            // Define input arguments
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

            // Define output arguments
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

            // Set the method handler
            getRecordsMethod.OnCallMethod = new GenericMethodCalledEventHandler(OnGetRecordsCalled);

            // Add the method (with its argument properties) as a child of ServerLog
            // BEFORE adding ServerLog to predefined nodes, so the HasComponent reference is preserved.
            serverLogFolder.AddChild(getRecordsMethod);

            // Add the complete hierarchy as a single predefined node tree
            AddPredefinedNode(SystemContext, serverLogFolder);

            Console.WriteLine($"Address space created. ServerLog NodeId: ns={NamespaceIndex};i={ServerLogId}");
            Console.WriteLine($"GetRecords method NodeId: ns={NamespaceIndex};i={GetRecordsMethodId}");
            Console.WriteLine($"Loaded {_fixedRecords.Count} fixed test records.");
        }
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
        // Parse input arguments
        DateTime startTime = (DateTime)inputArguments[0];
        DateTime endTime = (DateTime)inputArguments[1];
        uint maxRecords = (uint)inputArguments[2];
        ushort minSeverity = (ushort)inputArguments[3];
        // uint logRecordMask = (uint)inputArguments[4];
        byte[]? continuationPoint = inputArguments[5] as byte[];

        Console.WriteLine($"GetRecords called: StartTime={startTime:O}, EndTime={endTime:O}, " +
                          $"MaxRecords={maxRecords}, MinSeverity={minSeverity}");

        // Validate
        if (endTime < startTime)
        {
            return new ServiceResult(StatusCodes.BadInvalidArgument);
        }

        // Filter records
        var filtered = _fixedRecords
            .Where(r => r.Timestamp >= startTime && r.Timestamp <= endTime)
            .Where(r => r.Severity >= minSeverity)
            .ToList();

        // Handle continuation point
        int startIndex = 0;
        if (continuationPoint != null && continuationPoint.Length >= 4)
        {
            startIndex = BitConverter.ToInt32(continuationPoint, 0);
        }

        if (startIndex > 0 && startIndex < filtered.Count)
        {
            filtered = filtered.Skip(startIndex).ToList();
        }
        else if (startIndex >= filtered.Count && startIndex > 0)
        {
            filtered = new List<TestLogRecord>();
        }

        // Apply max records limit
        byte[]? nextContinuationPoint = null;
        if (maxRecords > 0 && filtered.Count > (int)maxRecords)
        {
            filtered = filtered.Take((int)maxRecords).ToList();
            int nextOffset = startIndex + (int)maxRecords;
            nextContinuationPoint = BitConverter.GetBytes(nextOffset);
        }

        Console.WriteLine($"Returning {filtered.Count} records");

        // Build output: array of ExtensionObjects with binary-encoded LogRecords
        var records = new ExtensionObject[filtered.Count];
        for (int i = 0; i < filtered.Count; i++)
        {
            records[i] = EncodeLogRecord(filtered[i]);
        }

        outputArguments[0] = records;
        outputArguments[1] = nextContinuationPoint ?? Array.Empty<byte>();

        return ServiceResult.Good;
    }

    /// <summary>
    /// Encodes a log record as an ExtensionObject with a binary body following OPC UA Part 26 §5.4.
    ///
    /// Binary field order (client requests mask 0x1F → all optional fields always present):
    ///   1. DateTime             – Time       (mandatory)
    ///   2. UInt16               – Severity   (mandatory)
    ///   3. NodeId               – EventType  (optional, bit 0)
    ///   4. NodeId               – SourceNode (optional, bit 1)
    ///   5. String               – SourceName (optional, bit 2)
    ///   6. LocalizedText        – Message    (mandatory)
    ///   7. TraceContextDataType – TraceContext (optional, bit 3)
    ///        Guid   (16 bytes OPC UA Guid)  – TraceId
    ///        UInt64                         – SpanId        (0 = absent)
    ///        UInt64                         – ParentSpanId  (0 = root)
    ///        String                         – ParentIdentifier (null = local)
    ///   8. NameValuePair[]      – AdditionalData (optional, bit 4)
    ///        Int32  – element count (0 = empty)
    ///        per element: String (Name) + Variant (Value)
    /// </summary>
    private ExtensionObject EncodeLogRecord(TestLogRecord record)
    {
        using var stream = new MemoryStream();
        using (var encoder = new BinaryEncoder(stream, _messageContext, true))
        {
            // 1. DateTime: Time
            encoder.WriteDateTime(null, record.Timestamp);

            // 2. UInt16: Severity
            encoder.WriteUInt16(null, record.Severity);

            // 3. NodeId: EventType
            var eventTypeNodeId = string.IsNullOrEmpty(record.EventType)
                ? NodeId.Null
                : NodeId.Parse(record.EventType);
            encoder.WriteNodeId(null, eventTypeNodeId);

            // 4. NodeId: SourceNode
            var sourceNodeId = string.IsNullOrEmpty(record.SourceNode)
                ? NodeId.Null
                : NodeId.Parse(record.SourceNode);
            encoder.WriteNodeId(null, sourceNodeId);

            // 5. String: SourceName
            encoder.WriteString(null, record.SourceName);

            // 6. LocalizedText: Message
            encoder.WriteLocalizedText(null, new LocalizedText(record.Message));

            // 7. TraceContextDataType (inline, always written; SpanId=0 signals absent)
            //    Guid (OPC UA binary: Data1:UInt32 LE + Data2:UInt16 LE + Data3:UInt16 LE + Data4:[8]byte)
            var traceGuid = record.TraceContext?.TraceId ?? Guid.Empty;
            var guidBytes = traceGuid.ToByteArray(); // preserves byte order for new Guid(byte[]) round-trip
            encoder.WriteUInt32(null, BitConverter.ToUInt32(guidBytes, 0));  // Data1
            encoder.WriteUInt16(null, BitConverter.ToUInt16(guidBytes, 4));  // Data2
            encoder.WriteUInt16(null, BitConverter.ToUInt16(guidBytes, 6));  // Data3
            for (int i = 8; i < 16; i++) encoder.WriteByte(null, guidBytes[i]); // Data4

            encoder.WriteUInt64(null, record.TraceContext?.SpanId ?? 0UL);
            encoder.WriteUInt64(null, record.TraceContext?.ParentSpanId ?? 0UL);
            encoder.WriteString(null, record.TraceContext?.ParentIdentifier);

            // 8. AdditionalData: NameValuePair[]
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

        // TypeId identifies our custom encoding.
        // Use namespace index 0 so the Go client's registered type (ns=0;i=5001)
        // matches the wire format. gopcua's type registry is keyed by NodeID.String().
        var typeId = new ExpandedNodeId(LogRecordTypeId);
        return new ExtensionObject(typeId, body);
    }
}
