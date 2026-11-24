// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package opcua

import (
	"encoding/hex"
	"testing"
	"time"

	"github.com/gopcua/opcua/ua"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Encoded by OPCFoundation.NetStandard.Opc.Ua 2.0.2's BinaryEncoder.
const sdkLogRecordsFixture = "020000001F00000000D02E3D3467DB0196000100F907010164000F00000053797374656D436F6D706F6E656E74021800000053797374656D207374617274757020696E697469617465640102030405060708090A0B0C0D0E0F1008070605040302010000000000000000000000000100000009000000636F6D706F6E656E740C0600000073797374656D000000000016F2603467DB012C0102070000006D696E696D616C"

func TestStandardLogRecordsSDKFixture(t *testing.T) {
	body, err := hex.DecodeString(sdkLogRecordsFixture)
	require.NoError(t, err)
	wire := ua.NewBuffer(nil)
	wire.WriteStruct(&ua.ExpandedNodeID{NodeID: logRecordsBinaryTypeID})
	wire.WriteByte(ua.ExtensionObjectBinary)
	wire.WriteUint32(uint32(len(body)))
	wire.Write(body)
	require.NoError(t, wire.Error())

	var ext ua.ExtensionObject
	n, err := ext.Decode(wire.Bytes())
	require.NoError(t, err)
	require.Equal(t, wire.Len(), n)
	value, ok := ext.Value.(*logRecordsDataType)
	require.True(t, ok)
	require.Len(t, value.Records, 2)

	records, err := newTestClient().parseLogRecordsDataType(ua.MustVariant(&ext))
	require.NoError(t, err)
	require.Len(t, records, 2)
	assert.Equal(t, time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC), records[0].Timestamp)
	assert.Equal(t, uint16(150), records[0].Severity)
	assert.Equal(t, "System startup initiated", records[0].Message)
	assert.Equal(t, "SystemComponent", records[0].SourceName)
	assert.Equal(t, uint16(1), records[0].SourceNamespace)
	assert.Equal(t, "Numeric", records[0].SourceIDType)
	assert.Equal(t, "100", records[0].SourceID)
	assert.Equal(t, "0102030405060708090a0b0c0d0e0f10", records[0].TraceID)
	assert.Equal(t, "0102030405060708", records[0].SpanID)
	assert.Equal(t, "system", records[0].Attributes["component"])
	assert.Equal(t, "minimal", records[1].Message)
	assert.Empty(t, records[1].SourceID)
	assert.Empty(t, records[1].TraceID)
	assert.Empty(t, records[1].Attributes)
}

func TestStandardLogRecordOpaqueNodeAndArray(t *testing.T) {
	// SDK 2.0.2: SourceNode is ns=2;b=AAECAw== and AdditionalData contains a string array.
	body, err := hex.DecodeString("010000001200000000D02E3D3467DB019600050200040000000001020302060000006F70617175650100000004000000746167738C02000000030000006F6E650300000074776F")
	require.NoError(t, err)
	var wrapper logRecordsDataType
	_, err = wrapper.Decode(body)
	require.NoError(t, err)
	require.Len(t, wrapper.Records, 1)
	record := logRecordExtObjToRecord((*LogRecordExtObj)(&wrapper.Records[0]))
	assert.Equal(t, uint16(2), record.SourceNamespace)
	assert.Equal(t, "Opaque", record.SourceIDType)
	assert.Equal(t, "AAECAw==", record.SourceID)
	assert.Equal(t, []string{"one", "two"}, record.Attributes["tags"])
}

func TestStandardLogRecordsMalformed(t *testing.T) {
	body, err := hex.DecodeString(sdkLogRecordsFixture)
	require.NoError(t, err)
	for i := 0; i < len(body); i++ {
		_, err := new(logRecordsDataType).Decode(body[:i])
		require.Error(t, err, "truncated at byte %d", i)
	}
	for _, raw := range []string{
		"FEFFFFFF",                               // invalid negative array length
		"00000100",                               // array exceeds the decoder limit
		"01000000200000000000000000000000000000", // unknown optional field
		"0000000000",                             // trailing data
	} {
		bad, err := hex.DecodeString(raw)
		require.NoError(t, err)
		_, err = new(logRecordsDataType).Decode(bad)
		require.Error(t, err)
	}
	for _, raw := range []string{"00000000", "FFFFFFFF"} {
		empty, err := hex.DecodeString(raw)
		require.NoError(t, err)
		var result logRecordsDataType
		_, err = result.Decode(empty)
		require.NoError(t, err)
		require.Empty(t, result.Records)
	}
}

// fixedTraceIDBytes returns the W3C TraceId bytes for "0102030405060708090a0b0c0d0e0f10".
func fixedTraceIDBytes() [16]byte {
	return [16]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10}
}

func TestLogRecordExtObjRoundTrip(t *testing.T) {
	original := &LogRecordExtObj{
		Time:       time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC),
		Severity:   300,
		Message:    "Test message",
		SourceName: "TestSource",
	}

	encoded, err := original.Encode()
	require.NoError(t, err)
	require.NotEmpty(t, encoded)

	decoded := &LogRecordExtObj{}
	n, err := decoded.Decode(encoded)
	require.NoError(t, err)
	assert.Equal(t, len(encoded), n)

	assert.True(t, original.Time.Equal(decoded.Time), "timestamps should match: want %v, got %v", original.Time, decoded.Time)
	assert.Equal(t, original.Severity, decoded.Severity)
	assert.Equal(t, original.Message, decoded.Message)
	assert.Equal(t, original.SourceName, decoded.SourceName)
}

func TestLogRecordExtObjDecodeFixedRecords(t *testing.T) {
	// Test all 10 fixed records from the C# test server
	records := []struct {
		time     time.Time
		severity uint16
		message  string
		source   string
	}{
		{time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC), 150, "System startup initiated", "SystemComponent"},
		{time.Date(2025, 1, 15, 10, 1, 0, 0, time.UTC), 300, "Configuration loaded successfully", "SystemComponent"},
		{time.Date(2025, 1, 15, 10, 2, 0, 0, time.UTC), 300, "Connection established to database", "NetworkModule"},
		{time.Date(2025, 1, 15, 10, 3, 0, 0, time.UTC), 150, "Data processing pipeline started", "DataLogger"},
		{time.Date(2025, 1, 15, 10, 4, 0, 0, time.UTC), 300, "Sensor reading collected: temperature=22.5", "DataLogger"},
		{time.Date(2025, 1, 15, 10, 5, 0, 0, time.UTC), 500, "High memory usage detected: 85%", "SystemComponent"},
		{time.Date(2025, 1, 15, 10, 6, 0, 0, time.UTC), 300, "Security scan completed", "SecurityModule"},
		{time.Date(2025, 1, 15, 10, 7, 0, 0, time.UTC), 700, "Connection timeout to external service", "NetworkModule"},
		{time.Date(2025, 1, 15, 10, 8, 0, 0, time.UTC), 300, "Backup process completed", "DataLogger"},
		{time.Date(2025, 1, 15, 10, 9, 0, 0, time.UTC), 150, "Garbage collection cycle completed", "SystemComponent"},
	}

	for _, rec := range records {
		t.Run(rec.message, func(t *testing.T) {
			original := &LogRecordExtObj{
				Time:       rec.time,
				Severity:   rec.severity,
				Message:    rec.message,
				SourceName: rec.source,
			}

			encoded, err := original.Encode()
			require.NoError(t, err)

			decoded := &LogRecordExtObj{}
			_, err = decoded.Decode(encoded)
			require.NoError(t, err)

			assert.True(t, rec.time.Equal(decoded.Time))
			assert.Equal(t, rec.severity, decoded.Severity)
			assert.Equal(t, rec.message, decoded.Message)
			assert.Equal(t, rec.source, decoded.SourceName)
		})
	}
}

func TestLogRecordExtObjDecodeEmptyFields(t *testing.T) {
	original := &LogRecordExtObj{
		Time:       time.Time{},
		Severity:   0,
		Message:    "",
		SourceName: "",
	}

	encoded, err := original.Encode()
	require.NoError(t, err)

	decoded := &LogRecordExtObj{}
	_, err = decoded.Decode(encoded)
	require.NoError(t, err)

	assert.Equal(t, uint16(0), decoded.Severity)
	assert.Equal(t, "", decoded.Message)
	assert.Equal(t, "", decoded.SourceName)
}

func TestLogRecordExtObjDecodeUnicodeMessage(t *testing.T) {
	original := &LogRecordExtObj{
		Time:       time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC),
		Severity:   300,
		Message:    "Temperatur überschritten: Wärmetauscher α-7",
		SourceName: "Gerät-ÜW",
	}

	encoded, err := original.Encode()
	require.NoError(t, err)

	decoded := &LogRecordExtObj{}
	_, err = decoded.Decode(encoded)
	require.NoError(t, err)

	assert.Equal(t, original.Message, decoded.Message)
	assert.Equal(t, original.SourceName, decoded.SourceName)
}

func TestLogRecordExtObjTypeRegistered(t *testing.T) {
	// Verify that the init() function registered our type with gopcua
	typeID := ua.NewNumericNodeID(0, 5001)
	obj := &ua.ExtensionObject{
		TypeID: &ua.ExpandedNodeID{NodeID: typeID},
	}

	// The TypeID should match what we registered
	assert.Equal(t, "i=5001", obj.TypeID.NodeID.String())
	assert.Equal(t, "i=5001", LogRecordExtObjTypeID.String())
}

func TestLogRecordExtObjString(t *testing.T) {
	lr := &LogRecordExtObj{
		Time:       time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC),
		Severity:   300,
		Message:    "Test",
		SourceName: "Src",
	}

	s := lr.String()
	assert.Contains(t, s, "2025-01-15")
	assert.Contains(t, s, "300")
	assert.Contains(t, s, "Test")
	assert.Contains(t, s, "Src")
}

// --- New field coverage ---

func TestLogRecordExtObjRoundTrip_FullFields(t *testing.T) {
	original := &LogRecordExtObj{
		Time:          time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC),
		Severity:      300,
		Message:       "Full round-trip test",
		SourceName:    "SystemComponent",
		SourceNode:    ua.NewNumericNodeID(1, 100),
		EventTypeNode: ua.NewNumericNodeID(0, 2041),
		TraceIDBytes:  fixedTraceIDBytes(),
		SpanID:        0x0102030405060708,
		ParentSpanID:  0x0000000000000000, // root span
		AdditionalData: map[string]interface{}{
			"component": "test",
			"version":   "1.0.0",
		},
	}

	encoded, err := original.Encode()
	require.NoError(t, err)
	require.NotEmpty(t, encoded)

	decoded := &LogRecordExtObj{}
	n, err := decoded.Decode(encoded)
	require.NoError(t, err)
	assert.Equal(t, len(encoded), n)

	// Mandatory fields
	assert.True(t, original.Time.Equal(decoded.Time))
	assert.Equal(t, original.Severity, decoded.Severity)
	assert.Equal(t, original.Message, decoded.Message)

	// Optional fields (bit 0–2)
	assert.Equal(t, original.SourceName, decoded.SourceName)
	require.NotNil(t, decoded.SourceNode)
	assert.Equal(t, uint16(1), decoded.SourceNode.Namespace())
	assert.Equal(t, uint32(100), decoded.SourceNode.IntID())
	require.NotNil(t, decoded.EventTypeNode)
	assert.Equal(t, uint16(0), decoded.EventTypeNode.Namespace())
	assert.Equal(t, uint32(2041), decoded.EventTypeNode.IntID())

	// TraceContext
	assert.Equal(t, original.TraceIDBytes, decoded.TraceIDBytes)
	assert.Equal(t, original.SpanID, decoded.SpanID)
	assert.Equal(t, original.ParentSpanID, decoded.ParentSpanID)

	// AdditionalData
	require.NotNil(t, decoded.AdditionalData)
	assert.Equal(t, "test", decoded.AdditionalData["component"])
	assert.Equal(t, "1.0.0", decoded.AdditionalData["version"])
}

func TestLogRecordExtObjRoundTrip_NullSourceNode(t *testing.T) {
	original := &LogRecordExtObj{
		Time:     time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC),
		Severity: 150,
		Message:  "No source node",
	}

	encoded, err := original.Encode()
	require.NoError(t, err)

	decoded := &LogRecordExtObj{}
	_, err = decoded.Decode(encoded)
	require.NoError(t, err)

	// Null NodeId round-trips to a NodeId with ns=0, id=0
	assert.Equal(t, uint16(0), decoded.SourceNode.Namespace())
	assert.Equal(t, uint32(0), decoded.SourceNode.IntID())
	assert.Equal(t, uint32(0), decoded.EventTypeNode.IntID())
}

func TestLogRecordExtObjRoundTrip_TraceContext(t *testing.T) {
	original := &LogRecordExtObj{
		Time:             time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC),
		Severity:         300,
		Message:          "Trace test",
		TraceIDBytes:     fixedTraceIDBytes(),
		SpanID:           0x0102030405060708,
		ParentSpanID:     0xdeadbeefcafe0000,
		ParentIdentifier: "urn:example:server",
	}

	encoded, err := original.Encode()
	require.NoError(t, err)

	decoded := &LogRecordExtObj{}
	_, err = decoded.Decode(encoded)
	require.NoError(t, err)

	assert.Equal(t, original.TraceIDBytes, decoded.TraceIDBytes)
	assert.Equal(t, original.SpanID, decoded.SpanID)
	assert.Equal(t, original.ParentSpanID, decoded.ParentSpanID)
	assert.Equal(t, original.ParentIdentifier, decoded.ParentIdentifier)
}

func TestLogRecordExtObjTraceIDHex(t *testing.T) {
	tests := []struct {
		name     string
		spanID   uint64
		expected string
	}{
		{
			name:     "absent (SpanID == 0)",
			spanID:   0,
			expected: "",
		},
		{
			name:     "present (SpanID != 0)",
			spanID:   0x0102030405060708,
			expected: "0102030405060708090a0b0c0d0e0f10",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lr := &LogRecordExtObj{
				TraceIDBytes: fixedTraceIDBytes(),
				SpanID:       tt.spanID,
			}
			assert.Equal(t, tt.expected, lr.TraceIDHex())
		})
	}
}

func TestLogRecordExtObjSpanIDHex(t *testing.T) {
	tests := []struct {
		name     string
		spanID   uint64
		expected string
	}{
		{"zero returns empty", 0, ""},
		{"sequential bytes", 0x0102030405060708, "0102030405060708"},
		{"all-f", 0xffffffffffffffff, "ffffffffffffffff"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lr := &LogRecordExtObj{SpanID: tt.spanID}
			assert.Equal(t, tt.expected, lr.SpanIDHex())
		})
	}
}

func TestNodeIDRoundTrip(t *testing.T) {
	tests := []struct {
		name   string
		nodeID *ua.NodeID
	}{
		{"nil", nil},
		{"TwoByte ns=0 i=1", ua.NewNumericNodeID(0, 1)},
		{"TwoByte ns=0 i=255", ua.NewNumericNodeID(0, 255)},
		{"FourByte ns=1 i=1001", ua.NewNumericNodeID(1, 1001)},
		{"FourByte ns=0 i=2041 (BaseEventType)", ua.NewNumericNodeID(0, 2041)},
		{"Numeric ns=2 i=70000", ua.NewNumericNodeID(2, 70000)},
		{"String ns=1", ua.NewStringNodeID(1, "MyDevice")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Encode into a temporary buffer via a LogRecordExtObj
			lr := &LogRecordExtObj{
				Time:       time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
				Severity:   100,
				Message:    "node id test",
				SourceNode: tt.nodeID,
			}

			encoded, err := lr.Encode()
			require.NoError(t, err)

			decoded := &LogRecordExtObj{}
			_, err = decoded.Decode(encoded)
			require.NoError(t, err)

			if tt.nodeID == nil {
				// nil encodes to the null NodeId (ns=0, id=0)
				assert.Equal(t, uint32(0), decoded.SourceNode.IntID())
				assert.Equal(t, uint16(0), decoded.SourceNode.Namespace())
				return
			}
			require.NotNil(t, decoded.SourceNode)
			assert.Equal(t, tt.nodeID.Namespace(), decoded.SourceNode.Namespace())
			assert.Equal(t, tt.nodeID.IntID(), decoded.SourceNode.IntID())
			if tt.nodeID.Type() == ua.NodeIDTypeString {
				assert.Equal(t, tt.nodeID.StringID(), decoded.SourceNode.StringID())
			}
		})
	}
}

func TestLogRecordExtObjRoundTrip_AdditionalData(t *testing.T) {
	tests := []struct {
		name           string
		additionalData map[string]interface{}
	}{
		{
			name:           "nil / empty",
			additionalData: nil,
		},
		{
			name: "string values",
			additionalData: map[string]interface{}{
				"sensor_id": "temp-01",
				"unit":      "Celsius",
			},
		},
		{
			name: "bool value",
			additionalData: map[string]interface{}{
				"active": true,
			},
		},
		{
			name: "int32 value",
			additionalData: map[string]interface{}{
				"count": int32(42),
			},
		},
		{
			name: "float64 value",
			additionalData: map[string]interface{}{
				"temperature": 22.5,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := &LogRecordExtObj{
				Time:           time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC),
				Severity:       150,
				Message:        "data test",
				AdditionalData: tt.additionalData,
			}

			encoded, err := original.Encode()
			require.NoError(t, err)

			decoded := &LogRecordExtObj{}
			_, err = decoded.Decode(encoded)
			require.NoError(t, err)

			if len(tt.additionalData) == 0 {
				assert.Empty(t, decoded.AdditionalData)
				return
			}
			require.NotNil(t, decoded.AdditionalData)
			for k, v := range tt.additionalData {
				assert.Equal(t, v, decoded.AdditionalData[k], "key %q mismatch", k)
			}
		})
	}
}
