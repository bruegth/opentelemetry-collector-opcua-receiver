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
    /// Encodes a log record as an ExtensionObject with a binary body.
    /// Binary format (OPC UA binary encoding, all fields sequential):
    ///   1. DateTime     - Time (Int64, 100ns ticks since 1601-01-01)
    ///   2. UInt16       - Severity
    ///   3. LocalizedText - Message (encoding mask + optional locale + text)
    ///   4. String       - SourceName (Int32 length + UTF-8 bytes, or -1 for null)
    /// </summary>
    private ExtensionObject EncodeLogRecord(TestLogRecord record)
    {
        using var stream = new MemoryStream();
        using (var encoder = new BinaryEncoder(stream, _messageContext, true))
        {
            // DateTime: Int64 - OPC UA DateTime (100ns intervals since 1601-01-01)
            encoder.WriteDateTime(null, record.Timestamp);

            // UInt16: Severity
            encoder.WriteUInt16(null, record.Severity);

            // LocalizedText: Message
            encoder.WriteLocalizedText(null, new LocalizedText(record.Message));

            // String: SourceName
            encoder.WriteString(null, record.Source);
        }

        byte[] body = stream.ToArray();

        // TypeId identifies our custom encoding.
        // Use namespace index 0 so the Go client's registered type (ns=0;i=5001)
        // matches the wire format. gopcua's type registry is keyed by NodeID.String().
        var typeId = new ExpandedNodeId(LogRecordTypeId);
        return new ExtensionObject(typeId, body);
    }
}
