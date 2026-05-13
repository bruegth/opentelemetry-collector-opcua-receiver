// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0
package opcua

import (
	"context"
	"testing"
	"time"

	"github.com/gopcua/opcua"
	"github.com/gopcua/opcua/ua"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/consumer/consumertest"
	"go.opentelemetry.io/collector/receiver/receivertest"

	"github.com/bruegth/opentelemetry-collector-opcua-receiver/receiver/opcua/testdata"
)

// ── helpers ──────────────────────────────────────────────────────────────────

func subscriptionConfig() *Config {
	cfg := createDefaultConfig().(*Config)
	cfg.CollectionMode = CollectionModeSubscription
	cfg.LogObjectPaths = []string{"ns=2;i=1001"}
	return cfg
}

// ── construction & validation ─────────────────────────────────────────────

func TestNewSubscriptionReceiver_OK(t *testing.T) {
	recv, err := newSubscriptionReceiver(subscriptionConfig(), receivertest.NewNopSettings(Type), consumertest.NewNop())
	require.NoError(t, err)
	assert.NotNil(t, recv)
}

func TestNewSubscriptionReceiver_NilConsumer(t *testing.T) {
	_, err := newSubscriptionReceiver(subscriptionConfig(), receivertest.NewNopSettings(Type), nil)
	assert.Error(t, err)
}

func TestNewSubscriptionReceiver_InvalidConfig(t *testing.T) {
	cfg := subscriptionConfig()
	cfg.Endpoint = "" // invalid
	_, err := newSubscriptionReceiver(cfg, receivertest.NewNopSettings(Type), consumertest.NewNop())
	assert.Error(t, err)
}

// ── Config.Validate additions ─────────────────────────────────────────────

func TestConfigValidate_SubscriptionMode(t *testing.T) {
	t.Run("valid subscription mode", func(t *testing.T) {
		cfg := subscriptionConfig()
		assert.NoError(t, cfg.Validate())
	})

	t.Run("invalid collection_mode value", func(t *testing.T) {
		cfg := subscriptionConfig()
		cfg.CollectionMode = "streaming" // not a valid value
		assert.Error(t, cfg.Validate())
	})

	t.Run("negative publishing_interval rejected", func(t *testing.T) {
		cfg := subscriptionConfig()
		cfg.Subscription.PublishingInterval = -1 * time.Millisecond
		assert.Error(t, cfg.Validate())
	})

	t.Run("zero publishing_interval accepted", func(t *testing.T) {
		cfg := subscriptionConfig()
		cfg.Subscription.PublishingInterval = 0
		assert.NoError(t, cfg.Validate())
	})

	// poll mode: collection_interval still enforced
	t.Run("poll mode collection_interval still required", func(t *testing.T) {
		cfg := subscriptionConfig()
		cfg.CollectionMode = CollectionModePoll
		cfg.CollectionInterval = 0
		assert.Error(t, cfg.Validate())
	})

	// subscription mode: collection_interval NOT required
	t.Run("subscription mode ignores collection_interval", func(t *testing.T) {
		cfg := subscriptionConfig()
		cfg.CollectionMode = CollectionModeSubscription
		cfg.CollectionInterval = 0 // would fail for poll
		assert.NoError(t, cfg.Validate())
	})
}

// ── Shutdown without Start ────────────────────────────────────────────────

func TestSubscriptionReceiver_ShutdownWithoutStart(t *testing.T) {
	recv, err := newSubscriptionReceiver(subscriptionConfig(), receivertest.NewNopSettings(Type), consumertest.NewNop())
	require.NoError(t, err)
	// doneCh is open; Shutdown must not block indefinitely or panic.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	assert.NoError(t, recv.Shutdown(ctx))
}

// ── decodeNotification ────────────────────────────────────────────────────

func newTestReceiver(t *testing.T) *subscriptionReceiver {
	t.Helper()
	recv, err := newSubscriptionReceiver(subscriptionConfig(), receivertest.NewNopSettings(Type), consumertest.NewNop())
	require.NoError(t, err)
	recv.handleToPath[1] = "ns=2;i=1001"
	return recv
}

func TestDecodeNotification_StatusChange(t *testing.T) {
	// A StatusChangeNotification must be silently ignored.
	recv := newTestReceiver(t)
	notif := &opcua.PublishNotificationData{
		Value: &ua.StatusChangeNotification{Status: ua.StatusBad},
	}
	logs := recv.decodeNotification(notif)
	assert.Equal(t, 0, logs.LogRecordCount())
}

func TestDecodeNotification_EmptyDataChange(t *testing.T) {
	recv := newTestReceiver(t)
	notif := &opcua.PublishNotificationData{
		Value: &ua.DataChangeNotification{
			MonitoredItems: []*ua.MonitoredItemNotification{},
		},
	}
	logs := recv.decodeNotification(notif)
	assert.Equal(t, 0, logs.LogRecordCount())
}

func TestDecodeNotification_NilItemValue(t *testing.T) {
	recv := newTestReceiver(t)
	notif := &opcua.PublishNotificationData{
		Value: &ua.DataChangeNotification{
			MonitoredItems: []*ua.MonitoredItemNotification{
				{ClientHandle: 1, Value: nil},
			},
		},
	}
	logs := recv.decodeNotification(notif)
	assert.Equal(t, 0, logs.LogRecordCount())
}

func TestDecodeNotification_NonExtObj(t *testing.T) {
	// A plain string value is not a LogRecordExtObj – should be skipped gracefully.
	recv := newTestReceiver(t)
	notif := &opcua.PublishNotificationData{
		Value: &ua.DataChangeNotification{
			MonitoredItems: []*ua.MonitoredItemNotification{
				{
					ClientHandle: 1,
					Value: &ua.DataValue{
						Value: ua.MustVariant("unexpected string"),
					},
				},
			},
		},
	}
	logs := recv.decodeNotification(notif)
	assert.Equal(t, 0, logs.LogRecordCount())
}

func TestDecodeNotification_ValidLogRecord(t *testing.T) {
	recv := newTestReceiver(t)

	ext := &LogRecordExtObj{
		Time:       time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC),
		Severity:   160, // Warning range (151–200)
		Message:    "disk pressure detected",
		SourceName: "StorageSubsystem",
		AdditionalData: map[string]interface{}{
			"disk.free_pct": float64(5),
		},
	}

	notif := &opcua.PublishNotificationData{
		Value: &ua.DataChangeNotification{
			MonitoredItems: []*ua.MonitoredItemNotification{
				{
					ClientHandle: 1,
					Value:        &ua.DataValue{Value: ua.MustVariant(ua.NewExtensionObject(ext))},
				},
			},
		},
	}

	logs := recv.decodeNotification(notif)
	require.Equal(t, 1, logs.LogRecordCount())

	lr := logs.ResourceLogs().At(0).ScopeLogs().At(0).LogRecords().At(0)
	assert.Equal(t, "disk pressure detected", lr.Body().Str())
	assert.Equal(t, "ns=2;i=1001", lr.Attributes().AsRaw()["opcua.log_object_path"])
}

func TestDecodeNotification_SourceTimestampOverride(t *testing.T) {
	// When the DataValue has a SourceTimestamp, it must override the record's own Time.
	recv := newTestReceiver(t)

	serverTime := time.Date(2024, 1, 15, 8, 30, 0, 0, time.UTC)
	ext := &LogRecordExtObj{
		Time:     time.Date(2024, 1, 15, 8, 0, 0, 0, time.UTC), // earlier
		Severity: 75,
		Message:  "overridden",
	}

	notif := &opcua.PublishNotificationData{
		Value: &ua.DataChangeNotification{
			MonitoredItems: []*ua.MonitoredItemNotification{
				{
					ClientHandle: 1,
					Value: &ua.DataValue{
						Value:           ua.MustVariant(ua.NewExtensionObject(ext)),
						SourceTimestamp: serverTime,
					},
				},
			},
		},
	}

	logs := recv.decodeNotification(notif)
	require.Equal(t, 1, logs.LogRecordCount())
	lr := logs.ResourceLogs().At(0).ScopeLogs().At(0).LogRecords().At(0)
	assert.Equal(t, serverTime.UnixNano(), lr.Timestamp().AsTime().UnixNano())
}

// ── logRecordExtObjToOPCUALogRecord ──────────────────────────────────────

func TestLogRecordExtObjToOPCUALogRecord_Fields(t *testing.T) {
	ext := &LogRecordExtObj{
		Time:       time.Now().UTC(),
		Severity:   210,
		Message:    "pump failure",
		SourceName: "HydraulicUnit",
		SourceNode: ua.NewNumericNodeID(2, 999),
		AdditionalData: map[string]interface{}{
			"pump.rpm": int64(0),
		},
	}

	rec := logRecordExtObjToOPCUALogRecord(ext)

	assert.Equal(t, ext.Time, rec.Timestamp)
	assert.Equal(t, uint16(210), rec.Severity)
	assert.Equal(t, "pump failure", rec.Message)
	assert.Equal(t, "HydraulicUnit", rec.SourceName)
	assert.Equal(t, uint16(2), rec.SourceNamespace)
	assert.Equal(t, "Numeric", rec.SourceIDType)
	assert.Equal(t, "999", rec.SourceID)
	assert.Equal(t, int64(0), rec.Attributes["pump.rpm"])
}

func TestLogRecordExtObjToOPCUALogRecord_NilSourceNode(t *testing.T) {
	ext := &LogRecordExtObj{
		Time:     time.Now().UTC(),
		Severity: 75,
		Message:  "hello",
	}
	rec := logRecordExtObjToOPCUALogRecord(ext)
	assert.Equal(t, "", rec.SourceIDType)
	assert.NotNil(t, rec.Attributes)
}

// ── resolveNodeID ─────────────────────────────────────────────────────────

func TestResolveNodeID(t *testing.T) {
	tests := []struct {
		path    string
		wantErr bool
	}{
		{"ns=2;i=1001", false},
		{"ns=2;s=ServerLog", false},
		{"Objects/ServerLog", false},
		{"Objects/Server/ServerLog", false},
		{"ServerLog", false},
		{"Objects/Server/ServerDiagnostics/ServerLog", false},
		{"unknown/arbitrary/path", true},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			nodeID, err := resolveNodeID(tt.path)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, nodeID)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, nodeID)
			}
		})
	}
}

// ── default config ────────────────────────────────────────────────────────

func TestDefaultConfig_SubscriptionDefaults(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	assert.Equal(t, CollectionModePoll, cfg.CollectionMode)
	assert.Equal(t, time.Second, cfg.Subscription.PublishingInterval)
	assert.Equal(t, uint32(100), cfg.Subscription.QueueSize)
	assert.Equal(t, 5*time.Second, cfg.Subscription.ReconnectDelay)
	assert.Equal(t, uint32(0), cfg.Subscription.MaxReconnectAttempts)
}

// ── end-to-end: decodeNotification → consumertest.LogsSink ───────────────

func TestSubscriptionReceiver_EndToEnd_Decode(t *testing.T) {
	sink := &consumertest.LogsSink{}
	cfg := subscriptionConfig()
	recv, err := newSubscriptionReceiver(cfg, receivertest.NewNopSettings(Type), sink)
	require.NoError(t, err)
	recv.handleToPath[1] = "ns=2;i=1001"

	// Simulate two notifications arriving.
	records := []testdata.OPCUALogRecord{
		{Timestamp: time.Now(), Severity: 75, Message: "first"},
		{Timestamp: time.Now(), Severity: 160, Message: "second"},
	}

	for i, r := range records {
		ext := &LogRecordExtObj{
			Time:     r.Timestamp,
			Severity: r.Severity,
			Message:  r.Message,
		}
		notif := &opcua.PublishNotificationData{
			Value: &ua.DataChangeNotification{
				MonitoredItems: []*ua.MonitoredItemNotification{
					{
						ClientHandle: uint32(i%len(recv.handleToPath)) + 1,
						Value:        &ua.DataValue{Value: ua.MustVariant(ua.NewExtensionObject(ext))},
					},
				},
			},
		}
		logs := recv.decodeNotification(notif)
		require.Equal(t, 1, logs.LogRecordCount())
		require.NoError(t, sink.ConsumeLogs(context.Background(), logs))
	}

	assert.Equal(t, 2, sink.LogRecordCount())
}