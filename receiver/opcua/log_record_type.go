// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package opcua

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/gopcua/opcua/ua"
)

// LogRecordExtObj is the Go representation of a binary-encoded OPC UA Part 26 LogRecord
// returned by OPC UA servers implementing the GetRecords method.
//
// Binary field order (OPC UA Part 26 §5.4, all optional fields present when mask=0x1F):
//
//  1. DateTime             – Time         (mandatory)
//  2. UInt16               – Severity     (mandatory)
//  3. NodeId               – EventType    (optional, bit 0)
//  4. NodeId               – SourceNode   (optional, bit 1)
//  5. String               – SourceName   (optional, bit 2)
//  6. LocalizedText        – Message      (mandatory)
//  7. TraceContextDataType – TraceContext (optional, bit 3)
//     Guid   (16 bytes: Data1 LE-UInt32 + Data2 LE-UInt16 + Data3 LE-UInt16 + Data4 [8]byte)
//     UInt64 – SpanId        (0 = absent)
//     UInt64 – ParentSpanId  (0 = root span)
//     String – ParentIdentifier
//  8. NameValuePair[]      – AdditionalData (optional, bit 4)
//     Int32  – element count (0 = empty, encoded as UInt32 then cast)
//     per element: String (Name) + Variant (Value)
//
// This type is registered with gopcua's ExtensionObject type registry
// so that it is automatically decoded when received over the wire.
type LogRecordExtObj struct {
	// Mandatory fields
	Time     time.Time
	Severity uint16
	Message  string

	// Optional fields (bit 0–2)
	EventTypeNode *ua.NodeID
	SourceNode    *ua.NodeID
	SourceName    string

	// TraceContext (bit 3); SpanID == 0 means no trace context
	TraceIDBytes     [16]byte // raw W3C byte order (same as Guid wire bytes)
	SpanID           uint64   // big-endian uint64 value of W3C SpanId
	ParentSpanID     uint64   // 0 for root span
	ParentIdentifier string

	// AdditionalData (bit 4)
	AdditionalData map[string]interface{}
}

// LogRecordExtObjTypeID is the NodeID used to identify LogRecord ExtensionObjects.
// Must match the TypeId used by the OPC UA server.
// The C# test server uses ExpandedNodeId(5001) which encodes as ns=0;i=5001.
var LogRecordExtObjTypeID = ua.NewNumericNodeID(0, 5001)

func init() {
	ua.RegisterExtensionObject(LogRecordExtObjTypeID, new(LogRecordExtObj))
}

// unixToOpcuaTicksOffset is the number of 100ns ticks between the OPC UA epoch
// (1601-01-01) and the Unix epoch (1970-01-01).
const unixToOpcuaTicksOffset int64 = 116444736000000000

// Decode implements the gopcua codec interface for binary deserialization.
// Field order matches OPC UA Part 26 §5.4 with all optional fields present (mask=0x1F).
func (l *LogRecordExtObj) Decode(b []byte) (int, error) {
	buf := ua.NewBuffer(b)

	// 1. DateTime: Int64 (100ns ticks since 1601-01-01)
	ticks := buf.ReadInt64()
	if ticks > 0 {
		unixNano := (ticks - unixToOpcuaTicksOffset) * 100
		l.Time = time.Unix(0, unixNano).UTC()
	}

	// 2. UInt16: Severity
	l.Severity = buf.ReadUint16()

	// 3. NodeId: EventType (OPC UA binary NodeId encoding)
	l.EventTypeNode = readNodeIDFromBuffer(buf)

	// 4. NodeId: SourceNode (OPC UA binary NodeId encoding)
	l.SourceNode = readNodeIDFromBuffer(buf)

	// 5. String: SourceName
	l.SourceName = buf.ReadString()

	// 6. LocalizedText: Message
	// OPC UA LocalizedText binary encoding:
	//   Byte: EncodingMask (bit 0 = has locale, bit 1 = has text)
	//   If bit 0: String (locale)
	//   If bit 1: String (text)
	encodingMask := buf.ReadByte()
	if encodingMask&0x01 != 0 {
		_ = buf.ReadString() // locale – discard
	}
	if encodingMask&0x02 != 0 {
		l.Message = buf.ReadString()
	}

	// 7. TraceContextDataType (inline, always encoded; SpanID==0 means absent)
	//    Guid: Data1 (LE UInt32) + Data2 (LE UInt16) + Data3 (LE UInt16) + Data4 ([8]byte)
	//    The C# side creates the Guid with new Guid(traceIdBytes), which preserves byte order,
	//    so the wire bytes are identical to the original W3C TraceId bytes.
	data1 := buf.ReadUint32()
	data2 := buf.ReadUint16()
	data3 := buf.ReadUint16()
	binary.LittleEndian.PutUint32(l.TraceIDBytes[0:4], data1)
	binary.LittleEndian.PutUint16(l.TraceIDBytes[4:6], data2)
	binary.LittleEndian.PutUint16(l.TraceIDBytes[6:8], data3)
	for i := 8; i < 16; i++ {
		l.TraceIDBytes[i] = buf.ReadByte()
	}
	// SpanId and ParentSpanId: stored as UInt64 (big-endian numeric value, little-endian on wire)
	l.SpanID = uint64(buf.ReadInt64())       //nolint:gosec // intentional bit-pattern cast
	l.ParentSpanID = uint64(buf.ReadInt64()) //nolint:gosec
	l.ParentIdentifier = buf.ReadString()

	// 8. AdditionalData: NameValuePair[]
	//    Int32 count (encoded as UInt32, -1 = null array interpreted as 0)
	count := int32(buf.ReadUint32()) //nolint:gosec
	if count > 0 {
		l.AdditionalData = make(map[string]interface{}, count)
		for i := int32(0); i < count; i++ {
			name := buf.ReadString()
			value := readVariantValue(buf)
			if name != "" {
				l.AdditionalData[name] = value
			}
		}
	}

	return buf.Pos(), buf.Error()
}

// Encode implements the gopcua codec interface for binary serialization.
func (l *LogRecordExtObj) Encode() ([]byte, error) {
	buf := ua.NewBuffer(nil)

	// 1. DateTime
	ticks := l.Time.UnixNano()/100 + unixToOpcuaTicksOffset
	buf.WriteInt64(ticks)

	// 2. UInt16: Severity
	buf.WriteUint16(l.Severity)

	// 3. NodeId: EventType
	writeNodeIDToBuffer(buf, l.EventTypeNode)

	// 4. NodeId: SourceNode
	writeNodeIDToBuffer(buf, l.SourceNode)

	// 5. String: SourceName
	buf.WriteString(l.SourceName)

	// 6. LocalizedText: Message (text only, no locale)
	buf.WriteByte(0x02) // encoding mask: has text only
	buf.WriteString(l.Message)

	// 7. TraceContext: Guid + UInt64 + UInt64 + String
	buf.WriteUint32(binary.LittleEndian.Uint32(l.TraceIDBytes[0:4]))
	buf.WriteUint16(binary.LittleEndian.Uint16(l.TraceIDBytes[4:6]))
	buf.WriteUint16(binary.LittleEndian.Uint16(l.TraceIDBytes[6:8]))
	for i := 8; i < 16; i++ {
		buf.WriteByte(l.TraceIDBytes[i])
	}
	buf.WriteInt64(int64(l.SpanID))       //nolint:gosec
	buf.WriteInt64(int64(l.ParentSpanID)) //nolint:gosec
	buf.WriteString(l.ParentIdentifier)

	// 8. AdditionalData: Int32 count + NameValuePairs
	buf.WriteUint32(uint32(len(l.AdditionalData)))
	for name, value := range l.AdditionalData {
		buf.WriteString(name)
		writeVariantValue(buf, value)
	}

	return buf.Bytes(), buf.Error()
}

// String returns a human-readable representation.
func (l *LogRecordExtObj) String() string {
	sourceNode := ""
	if l.SourceNode != nil {
		sourceNode = l.SourceNode.String()
	}
	traceID := ""
	if l.SpanID != 0 {
		traceID = hex.EncodeToString(l.TraceIDBytes[:])
	}
	return fmt.Sprintf(
		"LogRecord{Time: %s, Severity: %d, Message: %q, SourceName: %q, SourceNode: %q, TraceID: %q, SpanID: %016x, AdditionalData: %d}",
		l.Time.Format(time.RFC3339), l.Severity, l.Message, l.SourceName, sourceNode,
		traceID, l.SpanID, len(l.AdditionalData))
}

// TraceIDHex returns the TraceId as a 32-character lowercase hex string (W3C format).
// Returns an empty string when no trace context is present (SpanID == 0).
func (l *LogRecordExtObj) TraceIDHex() string {
	if l.SpanID == 0 {
		return ""
	}
	return hex.EncodeToString(l.TraceIDBytes[:])
}

// SpanIDHex returns the SpanId as a 16-character lowercase hex string (W3C format).
func (l *LogRecordExtObj) SpanIDHex() string {
	if l.SpanID == 0 {
		return ""
	}
	return fmt.Sprintf("%016x", l.SpanID)
}

// --- NodeId binary helpers ---

// readNodeIDFromBuffer decodes an OPC UA binary-encoded NodeId from buf.
// Encoding byte format (low nibble):
//
//	0x00 TwoByte  – 1 additional byte (Byte identifier, ns=0)
//	0x01 FourByte – 1 byte namespace (Byte) + 2 byte identifier (UInt16)
//	0x02 Numeric  – 2 byte namespace (UInt16) + 4 byte identifier (UInt32)
//	0x03 String   – 2 byte namespace + OPC UA String
func readNodeIDFromBuffer(buf *ua.Buffer) *ua.NodeID {
	encodingByte := buf.ReadByte()
	encodingType := encodingByte & 0x0F
	switch encodingType {
	case 0x00: // TwoByte
		id := uint32(buf.ReadByte())
		return ua.NewNumericNodeID(0, id)
	case 0x01: // FourByte
		ns := uint16(buf.ReadByte())
		id := uint32(buf.ReadUint16())
		return ua.NewNumericNodeID(ns, id)
	case 0x02: // Numeric
		ns := buf.ReadUint16()
		id := buf.ReadUint32()
		return ua.NewNumericNodeID(ns, id)
	case 0x03: // String
		ns := buf.ReadUint16()
		s := buf.ReadString()
		return ua.NewStringNodeID(ns, s)
	default:
		// For GUID (0x04) and ByteString (0x05) we return null – unexpected in test data
		return ua.NewNumericNodeID(0, 0)
	}
}

// writeNodeIDToBuffer encodes a NodeId in OPC UA binary format to buf.
// Null or nil NodeIds are written as TwoByte with identifier 0.
func writeNodeIDToBuffer(buf *ua.Buffer, nodeID *ua.NodeID) {
	if nodeID == nil || (nodeID.Namespace() == 0 && nodeID.IntID() == 0) {
		// Null NodeId: TwoByte encoding, id=0
		buf.WriteByte(0x00)
		buf.WriteByte(0x00)
		return
	}
	switch nodeID.Type() {
	case ua.NodeIDTypeString:
		buf.WriteByte(0x03)
		buf.WriteUint16(nodeID.Namespace())
		buf.WriteString(nodeID.StringID())
	default: // Numeric (TwoByte, FourByte, Numeric)
		ns := nodeID.Namespace()
		id := nodeID.IntID()
		if ns == 0 && id <= 0xFF {
			buf.WriteByte(0x00) // TwoByte
			buf.WriteByte(byte(id))
		} else if ns <= 0xFF && id <= 0xFFFF {
			buf.WriteByte(0x01) // FourByte
			buf.WriteByte(byte(ns))
			buf.WriteUint16(uint16(id))
		} else {
			buf.WriteByte(0x02) // Numeric
			buf.WriteUint16(ns)
			buf.WriteUint32(id)
		}
	}
}

// --- Variant helpers for AdditionalData ---

// readVariantValue reads a single OPC UA Variant scalar value from buf.
// Supports the types used in test AdditionalData (String, integers, float64).
// Returns nil for unsupported or null types.
func readVariantValue(buf *ua.Buffer) interface{} {
	typeByte := buf.ReadByte()
	typeID := typeByte & 0x3F // low 6 bits = built-in type ID
	switch typeID {
	case 1: // Boolean
		return buf.ReadByte() != 0
	case 2: // SByte
		return int8(buf.ReadByte()) //nolint:gosec
	case 3: // Byte
		return buf.ReadByte()
	case 4: // Int16
		return int16(buf.ReadUint16()) //nolint:gosec
	case 5: // UInt16
		return buf.ReadUint16()
	case 6: // Int32
		return int32(buf.ReadUint32()) //nolint:gosec
	case 7: // UInt32
		return buf.ReadUint32()
	case 8: // Int64
		return buf.ReadInt64()
	case 9: // UInt64
		return uint64(buf.ReadInt64()) //nolint:gosec
	case 10: // Float
		return buf.ReadFloat32()
	case 11: // Double
		return buf.ReadFloat64()
	case 12: // String
		return buf.ReadString()
	default:
		return nil
	}
}

// writeVariantValue writes a single OPC UA Variant scalar value to buf.
// Supports string, bool, integer and float types.
func writeVariantValue(buf *ua.Buffer, value interface{}) {
	switch v := value.(type) {
	case string:
		buf.WriteByte(12)
		buf.WriteString(v)
	case bool:
		buf.WriteByte(1)
		if v {
			buf.WriteByte(1)
		} else {
			buf.WriteByte(0)
		}
	case int:
		buf.WriteByte(6) // Int32
		buf.WriteUint32(uint32(v))
	case int32:
		buf.WriteByte(6)
		buf.WriteUint32(uint32(v))
	case int64:
		buf.WriteByte(8)
		buf.WriteInt64(v)
	case uint32:
		buf.WriteByte(7)
		buf.WriteUint32(v)
	case float64:
		buf.WriteByte(11) // Double
		buf.WriteFloat64(v)
	default:
		// Fallback: write as null (type 0)
		buf.WriteByte(0)
	}
}
