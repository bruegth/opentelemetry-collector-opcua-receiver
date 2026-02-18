// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package opcua

import (
	"fmt"
	"time"

	"github.com/gopcua/opcua/ua"
)

// LogRecordExtObj is the Go representation of the binary-encoded LogRecord
// returned by OPC UA servers implementing Part 26 GetRecords.
//
// Binary format (matching the C# test server encoding):
//   1. DateTime      - Time (Int64, 100ns ticks since 1601-01-01)
//   2. UInt16        - Severity
//   3. LocalizedText - Message
//   4. String        - SourceName
//
// This type is registered with gopcua's ExtensionObject type registry
// so that it is automatically decoded when received over the wire.
type LogRecordExtObj struct {
	Time       time.Time
	Severity   uint16
	Message    string
	SourceName string
}

// LogRecordExtObjTypeID is the NodeID used to identify LogRecord ExtensionObjects.
// Must match the TypeId used by the OPC UA server.
// The C# test server uses ExpandedNodeId(5001, "urn:opcua:testserver"),
// which encodes as ns=0;i=5001 in the inner NodeID.
var LogRecordExtObjTypeID = ua.NewNumericNodeID(0, 5001)

func init() {
	ua.RegisterExtensionObject(LogRecordExtObjTypeID, new(LogRecordExtObj))
}

// unixToOpcuaTicksOffset is the number of 100ns ticks between the OPC UA epoch
// (1601-01-01) and the Unix epoch (1970-01-01). Using a fixed constant avoids
// time.Duration overflow which is limited to ~292 years.
const unixToOpcuaTicksOffset int64 = 116444736000000000

// Decode implements the gopcua codec interface for binary deserialization.
func (l *LogRecordExtObj) Decode(b []byte) (int, error) {
	buf := ua.NewBuffer(b)

	// 1. DateTime: Int64 (100ns ticks since 1601-01-01)
	ticks := buf.ReadInt64()
	if ticks > 0 {
		// Convert OPC UA ticks to Unix nanoseconds, then to time.Time.
		// opcuaTicks = unixNano/100 + unixToOpcuaTicksOffset
		// => unixNano = (opcuaTicks - offset) * 100
		unixNano := (ticks - unixToOpcuaTicksOffset) * 100
		l.Time = time.Unix(0, unixNano).UTC()
	}

	// 2. UInt16: Severity
	l.Severity = buf.ReadUint16()

	// 3. LocalizedText: Message
	// OPC UA LocalizedText binary encoding:
	//   - Byte: EncodingMask (bit 0 = has locale, bit 1 = has text)
	//   - If bit 0: String (locale)
	//   - If bit 1: String (text)
	encodingMask := buf.ReadByte()
	if encodingMask&0x01 != 0 {
		_ = buf.ReadString() // locale - discard
	}
	if encodingMask&0x02 != 0 {
		l.Message = buf.ReadString()
	}

	// 4. String: SourceName
	l.SourceName = buf.ReadString()

	return buf.Pos(), buf.Error()
}

// Encode implements the gopcua codec interface for binary serialization.
func (l *LogRecordExtObj) Encode() ([]byte, error) {
	buf := ua.NewBuffer(nil)

	// 1. DateTime: Int64 (100ns ticks since 1601-01-01)
	// unixNano = (opcuaTicks - offset) * 100  =>  opcuaTicks = unixNano/100 + offset
	ticks := l.Time.UnixNano()/100 + unixToOpcuaTicksOffset
	buf.WriteInt64(ticks)

	// 2. UInt16: Severity
	buf.WriteUint16(l.Severity)

	// 3. LocalizedText: Message (text only, no locale)
	buf.WriteByte(0x02) // encoding mask: has text only
	buf.WriteString(l.Message)

	// 4. String: SourceName
	buf.WriteString(l.SourceName)

	return buf.Bytes(), buf.Error()
}

// String returns a human-readable representation.
func (l *LogRecordExtObj) String() string {
	return fmt.Sprintf("LogRecord{Time: %s, Severity: %d, Message: %q, Source: %q}",
		l.Time.Format(time.RFC3339), l.Severity, l.Message, l.SourceName)
}
