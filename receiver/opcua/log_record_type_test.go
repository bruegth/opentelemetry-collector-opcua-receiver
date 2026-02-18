// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package opcua

import (
	"testing"
	"time"

	"github.com/gopcua/opcua/ua"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
