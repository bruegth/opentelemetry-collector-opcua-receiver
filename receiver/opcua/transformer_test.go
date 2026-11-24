// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0
package opcua

import (
	"context"
	"testing"
	"time"

	"github.com/gopcua/opcua"
	"github.com/gopcua/opcua/id"
	"github.com/gopcua/opcua/ua"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/consumer/consumertest"
	"go.opentelemetry.io/collector/receiver/receivertest"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/bruegth/opentelemetry-collector-opcua-receiver/receiver/opcua/testdata"
)

// ── helpers ───────────────────────────────────────────────────────────────

func subscriptionConfig() *Config {
	cfg := createDefaultConfig().(*Config)
	cfg.CollectionMode = CollectionModeSubscription
	cfg.LogObjectPaths = []string{"ns=2;i=1000"}
	return cfg
}

func newTestReceiver(t *testing.T) *subscriptionReceiver {
	t.Helper()
	recv, err := newSubscriptionReceiver(subscriptionConfig(), receivertest.NewNopSettings(Type), consumertest.NewNop())
	require.NoError(t, err)
	recv.handleToPath[1] = "ns=2;i=1000"
	return recv
}

// makeEventNotification constructs a PublishNotificationData carrying an
// EventNotificationList with a single event whose fields match logEventSelectClauses().
func makeEventNotification(handle uint32, fields []*ua.Variant) *opcua.PublishNotificationData {
	return &opcua.PublishNotificationData{
		Value: &ua.EventNotificationList{
			Events: []*ua.EventFieldList{
				{
					ClientHandle: handle,
					EventFields:  fields,
				},
			},
		},
	}
}

// makeEventFields builds a full-length field slice for logEventSelectClauses.
func makeEventFields(ts time.Time, severity uint16, msg, sourceName string) []*ua.Variant {
	fields := make([]*ua.Variant, fieldIdxCount)
	fields[fieldIdxTime] = ua.MustVariant(ts)
	fields[fieldIdxSeverity] = ua.MustVariant(severity)
	fields[fieldIdxMessage] = ua.MustVariant(&ua.LocalizedText{Text: msg})
	fields[fieldIdxSourceName] = ua.MustVariant(sourceName)
	fields[fieldIdxSourceNode] = ua.MustVariant(ua.NewNumericNodeID(1, 100))
	fields[fieldIdxTraceContext] = ua.MustVariant(uint32(0))   // no trace
	fields[fieldIdxAdditionalData] = ua.MustVariant(uint32(0)) // no additional data
	return fields
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
	cfg.Endpoint = ""
	_, err := newSubscriptionReceiver(cfg, receivertest.NewNopSettings(Type), consumertest.NewNop())
	assert.Error(t, err)
}

// ── Config.Validate ───────────────────────────────────────────────────────

func TestConfigValidate_SubscriptionMode(t *testing.T) {
	t.Run("valid subscription mode", func(t *testing.T) {
		assert.NoError(t, subscriptionConfig().Validate())
	})
	t.Run("invalid collection_mode value", func(t *testing.T) {
		cfg := subscriptionConfig()
		cfg.CollectionMode = "streaming"
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
	t.Run("poll mode collection_interval still required", func(t *testing.T) {
		cfg := subscriptionConfig()
		cfg.CollectionMode = CollectionModePoll
		cfg.CollectionInterval = 0
		assert.Error(t, cfg.Validate())
	})
	t.Run("subscription mode ignores collection_interval", func(t *testing.T) {
		cfg := subscriptionConfig()
		cfg.CollectionInterval = 0
		assert.NoError(t, cfg.Validate())
	})
}

// ── Shutdown without Start ────────────────────────────────────────────────

func TestSubscriptionReceiver_ShutdownWithoutStart(t *testing.T) {
	recv, err := newSubscriptionReceiver(subscriptionConfig(), receivertest.NewNopSettings(Type), consumertest.NewNop())
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	assert.NoError(t, recv.Shutdown(ctx))
}

// ── decodeNotification ────────────────────────────────────────────────────

func TestDecodeNotification_DataChangIgnored(t *testing.T) {
	// DataChangeNotification must be silently ignored in event mode.
	recv := newTestReceiver(t)
	notif := &opcua.PublishNotificationData{
		Value: &ua.DataChangeNotification{
			MonitoredItems: []*ua.MonitoredItemNotification{},
		},
	}
	logs := recv.decodeNotification(notif)
	assert.Equal(t, 0, logs.LogRecordCount())
}

func TestDecodeNotification_EmptyEventList(t *testing.T) {
	recv := newTestReceiver(t)
	notif := &opcua.PublishNotificationData{
		Value: &ua.EventNotificationList{Events: nil},
	}
	logs := recv.decodeNotification(notif)
	assert.Equal(t, 0, logs.LogRecordCount())
}

func TestDecodeNotification_NilEvent(t *testing.T) {
	recv := newTestReceiver(t)
	notif := &opcua.PublishNotificationData{
		Value: &ua.EventNotificationList{
			Events: []*ua.EventFieldList{nil},
		},
	}
	logs := recv.decodeNotification(notif)
	assert.Equal(t, 0, logs.LogRecordCount())
}

func TestDecodeNotification_TooFewFields(t *testing.T) {
	// Events with fewer fields than expected should be skipped gracefully.
	recv := newTestReceiver(t)
	notif := makeEventNotification(1, []*ua.Variant{
		ua.MustVariant(time.Now()),
		ua.MustVariant(uint16(75)),
		// missing remaining fields
	})
	logs := recv.decodeNotification(notif)
	assert.Equal(t, 0, logs.LogRecordCount())
}

func TestDecodeNotification_ValidEvent(t *testing.T) {
	recv := newTestReceiver(t)

	ts := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	fields := makeEventFields(ts, 160, "disk pressure detected", "StorageSubsystem")
	notif := makeEventNotification(1, fields)

	logs := recv.decodeNotification(notif)
	require.Equal(t, 1, logs.LogRecordCount())

	lr := logs.ResourceLogs().At(0).ScopeLogs().At(0).LogRecords().At(0)
	assert.Equal(t, "disk pressure detected", lr.Body().Str())
	assert.Equal(t, "ns=2;i=1000", lr.Attributes().AsRaw()["opcua.log_object_path"])
}

func TestDecodeNotification_TimestampFromEvent(t *testing.T) {
	recv := newTestReceiver(t)

	ts := time.Date(2024, 1, 15, 8, 30, 0, 0, time.UTC)
	fields := makeEventFields(ts, 75, "hello", "Source")
	notif := makeEventNotification(1, fields)

	logs := recv.decodeNotification(notif)
	require.Equal(t, 1, logs.LogRecordCount())
	lr := logs.ResourceLogs().At(0).ScopeLogs().At(0).LogRecords().At(0)
	assert.Equal(t, ts.UnixNano(), lr.Timestamp().AsTime().UnixNano())
}

func TestDecodeNotification_MultipleEvents(t *testing.T) {
	recv := newTestReceiver(t)

	events := []*ua.EventFieldList{
		{ClientHandle: 1, EventFields: makeEventFields(time.Now(), 75, "first", "S1")},
		{ClientHandle: 1, EventFields: makeEventFields(time.Now(), 160, "second", "S2")},
	}
	notif := &opcua.PublishNotificationData{
		Value: &ua.EventNotificationList{Events: events},
	}

	logs := recv.decodeNotification(notif)
	assert.Equal(t, 2, logs.LogRecordCount())
}

func TestDecodeNotification_MinSeverity(t *testing.T) {
	tests := []struct {
		name     string
		severity string
		below    uint16
		at       uint16
	}{
		{"trace", "Trace", 0, 1},
		{"debug", "Debug", 0, 1},
		{"default", "", 50, 51},
		{"info", "Info", 50, 51},
		{"warn", "Warn", 150, 151},
		{"warning", "Warning", 150, 151},
		{"error", "Error", 200, 201},
		{"fatal", "Fatal", 400, 401},
		{"emergency", "Emergency", 400, 401},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recv := newTestReceiver(t)
			recv.config.Filter.MinSeverity = tt.severity
			notif := &opcua.PublishNotificationData{Value: &ua.EventNotificationList{
				Events: []*ua.EventFieldList{
					{ClientHandle: 1, EventFields: makeEventFields(time.Now(), tt.below, "below", "source")},
					{ClientHandle: 1, EventFields: makeEventFields(time.Now(), tt.at, "at", "source")},
					{ClientHandle: 1, EventFields: makeEventFields(time.Now(), 1000, "above", "source")},
				},
			}}
			logs := recv.decodeNotification(notif)
			require.Equal(t, 2, logs.LogRecordCount())
			assert.Equal(t, "at", logs.ResourceLogs().At(0).ScopeLogs().At(0).LogRecords().At(0).Body().Str())
			assert.Equal(t, "above", logs.ResourceLogs().At(1).ScopeLogs().At(0).LogRecords().At(0).Body().Str())
		})
	}
}

func TestDecodeNotification_InvalidSeverity(t *testing.T) {
	tests := []struct {
		name     string
		severity *ua.Variant
	}{
		{"missing", nil},
		{"wrong type", ua.MustVariant("151")},
		{"negative", ua.MustVariant(int16(-1))},
		{"zero", ua.MustVariant(uint16(0))},
		{"above range", ua.MustVariant(uint16(1001))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recv := newTestReceiver(t)
			recv.config.Filter.MinSeverity = "Debug"
			core, observed := observer.New(zap.WarnLevel)
			recv.settings.Logger = zap.New(core)
			fields := makeEventFields(time.Now(), 151, "invalid", "source")
			fields[fieldIdxSeverity] = tt.severity

			logs := recv.decodeNotification(makeEventNotification(1, fields))
			assert.Zero(t, logs.LogRecordCount())
			require.Equal(t, 1, observed.Len())
			assert.Contains(t, observed.All()[0].ContextMap()["error"], "invalid event severity")
		})
	}
}

// ── decodeEventFields ─────────────────────────────────────────────────────

func TestDecodeEventFields_AllFields(t *testing.T) {
	recv := newTestReceiver(t)
	ts := time.Now().UTC()

	fields := make([]*ua.Variant, fieldIdxCount)
	fields[fieldIdxTime] = ua.MustVariant(ts)
	fields[fieldIdxSeverity] = ua.MustVariant(uint16(210))
	fields[fieldIdxMessage] = ua.MustVariant(&ua.LocalizedText{Text: "pump failure"})
	fields[fieldIdxSourceName] = ua.MustVariant("HydraulicUnit")
	fields[fieldIdxSourceNode] = ua.MustVariant(ua.NewNumericNodeID(2, 999))
	fields[fieldIdxTraceContext] = ua.MustVariant(uint32(0))
	fields[fieldIdxAdditionalData] = ua.MustVariant(uint32(0))

	rec, err := recv.decodeEventFields(fields)
	require.NoError(t, err)

	assert.Equal(t, ts.Unix(), rec.Timestamp.Unix())
	assert.Equal(t, uint16(210), rec.Severity)
	assert.Equal(t, "pump failure", rec.Message)
	assert.Equal(t, "HydraulicUnit", rec.SourceName)
	assert.Equal(t, uint16(2), rec.SourceNamespace)
	assert.Equal(t, "Numeric", rec.SourceIDType)
	assert.Equal(t, "999", rec.SourceID)
}

func TestDecodeEventFields_MissingTimestampDefaultsToNow(t *testing.T) {
	recv := newTestReceiver(t)
	before := time.Now()

	fields := makeEventFields(time.Time{}, 75, "msg", "src")
	fields[fieldIdxTime] = ua.MustVariant(uint32(0)) // not a time.Time

	rec, err := recv.decodeEventFields(fields)
	require.NoError(t, err)
	assert.True(t, rec.Timestamp.After(before) || rec.Timestamp.Equal(before))
}

func TestDecodeEventFields_StringSourceNode(t *testing.T) {
	recv := newTestReceiver(t)
	fields := makeEventFields(time.Now(), 75, "msg", "src")
	fields[fieldIdxSourceNode] = ua.MustVariant(ua.NewStringNodeID(3, "MyTag"))

	rec, err := recv.decodeEventFields(fields)
	require.NoError(t, err)
	assert.Equal(t, "String", rec.SourceIDType)
	assert.Equal(t, "MyTag", rec.SourceID)
	assert.Equal(t, uint16(3), rec.SourceNamespace)
}

// ── logEventSelectClauses ─────────────────────────────────────────────────

func TestLogEventSelectClauses_Count(t *testing.T) {
	clauses := logEventSelectClauses()
	assert.Equal(t, fieldIdxCount, len(clauses))
}

func TestLogEventSelectClauses_TypeDefinitionID(t *testing.T) {
	// TypeDefinitionID must be BaseEventType (ns=0;i=2041) — always registered
	// in the server type hierarchy. BaseLogEventType may not be natively
	// registered on all servers, so we use the base type for compatibility.
	expectedID := ua.NewNumericNodeID(0, id.BaseEventType)
	for _, clause := range logEventSelectClauses() {
		assert.Equal(t, expectedID, clause.TypeDefinitionID)
		assert.Equal(t, ua.AttributeIDValue, clause.AttributeID)
	}
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
				if tt.path == "Objects/ServerLog" || tt.path == "Objects/Server/ServerLog" || tt.path == "ServerLog" || tt.path == "Objects/Server/ServerDiagnostics/ServerLog" {
					assert.Equal(t, ua.NewNumericNodeID(0, 19372), nodeID)
				}
			}
		})
	}
}

func TestSelectEndpointForConfig(t *testing.T) {
	open := &ua.EndpointDescription{SecurityPolicyURI: ua.SecurityPolicyURINone, SecurityMode: ua.MessageSecurityModeNone}
	secure128 := &ua.EndpointDescription{SecurityPolicyURI: ua.SecurityPolicyURIAes128Sha256RsaOaep, SecurityMode: ua.MessageSecurityModeSignAndEncrypt}
	secure := &ua.EndpointDescription{SecurityPolicyURI: ua.SecurityPolicyURIAes256Sha256RsaPss, SecurityMode: ua.MessageSecurityModeSignAndEncrypt}
	endpoints := []*ua.EndpointDescription{nil, open, secure128, secure}
	assert.Same(t, open, selectEndpointForConfig(endpoints, &Config{SecurityPolicy: "None", SecurityMode: "None"}))
	assert.Same(t, secure128, selectEndpointForConfig(endpoints, &Config{SecurityPolicy: "Aes128_Sha256_RsaOaep", SecurityMode: "SignAndEncrypt"}))
	assert.Same(t, secure, selectEndpointForConfig(endpoints, &Config{SecurityPolicy: "Aes256_Sha256_RsaPss", SecurityMode: "SignAndEncrypt"}))
	assert.Nil(t, selectEndpointForConfig(endpoints, &Config{SecurityPolicy: "Basic256Sha256", SecurityMode: "SignAndEncrypt"}))
	assert.Nil(t, selectEndpointForConfig(endpoints, &Config{SecurityPolicy: "Aes256_Sha256_RsaPss", SecurityMode: "Sign"}))
}

func TestPart26TraceFlags(t *testing.T) {
	record := logRecordExtObjToRecord(&LogRecordExtObj{TraceIDBytes: fixedTraceIDBytes(), SpanID: 1})
	logs := NewTransformer("opc.tcp://localhost:4840", "", "").TransformLogs([]testdata.OPCUALogRecord{record})
	flags := logs.ResourceLogs().At(0).ScopeLogs().At(0).LogRecords().At(0).Flags()
	assert.Equal(t, uint32(0x02), uint32(flags))
	assert.False(t, flags.IsSampled())
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

// ── end-to-end: decodeNotification → LogsSink ────────────────────────────

func TestSubscriptionReceiver_EndToEnd_Events(t *testing.T) {
	sink := &consumertest.LogsSink{}
	recv, err := newSubscriptionReceiver(subscriptionConfig(), receivertest.NewNopSettings(Type), sink)
	require.NoError(t, err)
	recv.handleToPath[1] = "ns=2;i=1000"

	msgs := []struct {
		severity uint16
		msg      string
	}{
		{75, "first event"},
		{160, "second event"},
		{210, "third event"},
	}

	for _, m := range msgs {
		fields := makeEventFields(time.Now(), m.severity, m.msg, "TestSource")
		notif := makeEventNotification(1, fields)
		logs := recv.decodeNotification(notif)
		require.Equal(t, 1, logs.LogRecordCount())
		require.NoError(t, sink.ConsumeLogs(context.Background(), logs))
	}

	assert.Equal(t, 3, sink.LogRecordCount())
}
