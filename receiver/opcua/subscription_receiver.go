// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0
package opcua

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gopcua/opcua"
	"github.com/gopcua/opcua/ua"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/receiver"
	"go.uber.org/zap"

	"github.com/bruegth/opentelemetry-collector-opcua-receiver/receiver/opcua/testdata"
)

// subscriptionReceiver implements receiver.Logs using OPC UA Subscriptions.
//
// Instead of polling on an interval it registers one MonitoredItem per
// log_object_path; the server then pushes ExtensionObject values whenever a
// new LogRecord arrives (OPC UA Part 26 §7).
//
// Lifecycle:
//
//	Start  → connect → create subscription → register MonitoredItems → runLoop goroutine
//	Shutdown → cancel ctx → wait doneCh → cancel subscription → close client
type subscriptionReceiver struct {
	config      *Config
	settings    receiver.Settings
	consumer    consumer.Logs
	transformer *Transformer

	// live OPC UA resources – nil until Start succeeds
	client       *opcua.Client
	subscription *opcua.Subscription
	notifCh      chan *opcua.PublishNotificationData

	// handleToPath maps the client-assigned MonitoredItem handle back to the
	// log_object_path string so we can build correct resource attributes.
	handleToPath map[uint32]string

	cancelFn context.CancelFunc
	doneCh   chan struct{}
}

var _ receiver.Logs = (*subscriptionReceiver)(nil)

// newSubscriptionReceiver creates (but does not start) a subscriptionReceiver.
func newSubscriptionReceiver(
	config *Config,
	settings receiver.Settings,
	nextConsumer consumer.Logs,
) (*subscriptionReceiver, error) {
	if nextConsumer == nil {
		return nil, fmt.Errorf("nil nextConsumer")
	}
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}
	return &subscriptionReceiver{
		config:       config,
		settings:     settings,
		consumer:     nextConsumer,
		transformer:  NewTransformer(config.Endpoint, config.Resource.ServiceName, config.Resource.ServiceNamespace),
		handleToPath: make(map[uint32]string),
		doneCh:       make(chan struct{}),
	}, nil
}

// Start connects to the OPC UA server, creates a subscription, and launches
// the background notification loop.
func (r *subscriptionReceiver) Start(ctx context.Context, _ component.Host) error {
	r.settings.Logger.Info("Starting OPC UA subscription receiver",
		zap.String("endpoint", r.config.Endpoint),
		zap.Duration("publishing_interval", r.config.Subscription.PublishingInterval),
	)

	if err := r.connect(ctx); err != nil {
		return fmt.Errorf("opcua subscription receiver: %w", err)
	}

	loopCtx, cancel := context.WithCancel(context.Background())
	r.cancelFn = cancel
	go r.runLoop(loopCtx)
	return nil
}

// Shutdown stops the background goroutine and releases OPC UA resources.
func (r *subscriptionReceiver) Shutdown(ctx context.Context) error {
	r.settings.Logger.Info("Shutting down OPC UA subscription receiver")
	if r.cancelFn != nil {
		r.cancelFn()
	}
	select {
	case <-r.doneCh:
	case <-ctx.Done():
		r.settings.Logger.Warn("Shutdown deadline reached before run-loop exited")
	}
	r.cleanup(ctx)
	return nil
}

// cleanup cancels the subscription and closes the client connection.
// Safe to call multiple times or when resources are nil.
func (r *subscriptionReceiver) cleanup(ctx context.Context) {
	if r.subscription != nil {
		if err := r.subscription.Cancel(ctx); err != nil {
			r.settings.Logger.Warn("Failed to cancel OPC UA subscription", zap.Error(err))
		}
		r.subscription = nil
	}
	if r.client != nil {
		if err := r.client.Close(ctx); err != nil {
			r.settings.Logger.Warn("Failed to close OPC UA client", zap.Error(err))
		}
		r.client = nil
	}
}

// connect establishes the OPC UA session and sets up the subscription with all
// MonitoredItems. It mirrors the connection logic of opcuaClient.Connect so
// that auth and security settings work identically in both modes.
func (r *subscriptionReceiver) connect(ctx context.Context) error {
	// ── 1. Discover endpoints ──────────────────────────────────────────────
	endpoints, err := opcua.GetEndpoints(ctx, r.config.Endpoint)
	if err != nil {
		return fmt.Errorf("get endpoints: %w", err)
	}
	if len(endpoints) == 0 {
		return fmt.Errorf("no endpoints at %s", r.config.Endpoint)
	}

	// ── 2. Build client options (mirrors opcuaClient.Connect) ─────────────
	ep := selectEndpointForConfig(endpoints, r.config)
	if ep == nil {
		return fmt.Errorf("no endpoint matches security_policy=%s security_mode=%s",
			r.config.SecurityPolicy, r.config.SecurityMode)
	}

	opts := []opcua.Option{
		opcua.SecurityFromEndpoint(ep, ua.UserTokenTypeAnonymous),
		opcua.RequestTimeout(r.config.RequestTimeout),
	}
	switch r.config.Auth.Type {
	case "username_password":
		opts = append(opts, opcua.AuthUsername(r.config.Auth.Username, r.config.Auth.Password))
	case "certificate":
		if r.config.TLS.CertFile != "" {
			opts = append(opts, opcua.CertificateFile(r.config.TLS.CertFile))
		}
		if r.config.TLS.KeyFile != "" {
			opts = append(opts, opcua.PrivateKeyFile(r.config.TLS.KeyFile))
		}
	default:
		opts = append(opts, opcua.AuthAnonymous())
	}

	// ── 3. Connect ─────────────────────────────────────────────────────────
	client, err := opcua.NewClient(r.config.Endpoint, opts...)
	if err != nil {
		return fmt.Errorf("create client: %w", err)
	}
	connCtx, cancel := context.WithTimeout(ctx, r.config.ConnectionTimeout)
	defer cancel()
	if err := client.Connect(connCtx); err != nil {
		return fmt.Errorf("connect to %s: %w", r.config.Endpoint, err)
	}
	r.client = client
	r.settings.Logger.Info("OPC UA subscription receiver connected",
		zap.String("endpoint", r.config.Endpoint))

	// ── 4. Create subscription ─────────────────────────────────────────────
	bufSize := int(r.config.Subscription.QueueSize)
	if bufSize < 32 {
		bufSize = 32
	}
	r.notifCh = make(chan *opcua.PublishNotificationData, bufSize)

	sub, err := client.Subscribe(ctx, &opcua.SubscriptionParameters{
		Interval: r.config.Subscription.PublishingInterval,
	}, r.notifCh)
	if err != nil {
		return fmt.Errorf("create subscription: %w", err)
	}
	r.subscription = sub
	r.settings.Logger.Info("OPC UA subscription created",
		zap.Uint32("subscription_id", sub.SubscriptionID),
		zap.Duration("revised_interval", sub.RevisedPublishingInterval),
	)

	// ── 5. Register MonitoredItems ─────────────────────────────────────────
	r.handleToPath = make(map[uint32]string)
	items := make([]*ua.MonitoredItemCreateRequest, 0, len(r.config.LogObjectPaths))

	for i, path := range r.config.LogObjectPaths {
		nodeID, err := resolveNodeID(path)
		if err != nil {
			r.settings.Logger.Warn("Cannot resolve log_object_path, skipping",
				zap.String("path", path), zap.Error(err))
			continue
		}
		handle := uint32(i + 1)
		r.handleToPath[handle] = path

		req := opcua.NewMonitoredItemCreateRequestWithDefaults(nodeID, ua.AttributeIDValue, handle)
		req.RequestedParameters.QueueSize = r.config.Subscription.QueueSize
		req.RequestedParameters.DiscardOldest = true
		items = append(items, req)
	}

	if len(items) == 0 {
		return fmt.Errorf("no log_object_paths could be resolved to valid NodeIDs")
	}

	resp, err := sub.Monitor(ctx, ua.TimestampsToReturnBoth, items...)
	if err != nil {
		return fmt.Errorf("monitor items: %w", err)
	}
	for i, res := range resp.Results {
		if res.StatusCode != ua.StatusOK {
			r.settings.Logger.Warn("MonitoredItem creation returned non-OK status",
				zap.Int("index", i),
				zap.Uint32("status_code", uint32(res.StatusCode)),
			)
		}
	}
	r.settings.Logger.Info("Monitoring log object nodes",
		zap.Int("count", len(items)))
	return nil
}

// runLoop reads from the notification channel and forwards decoded log records
// to the consumer pipeline. It also handles re-connection after failures.
func (r *subscriptionReceiver) runLoop(ctx context.Context) {
	defer close(r.doneCh)

	reconnectAttempts := uint32(0)

	for {
		select {
		case <-ctx.Done():
			return

		case notif, ok := <-r.notifCh:
			if !ok {
				// Channel closed – the subscription broke.
				if err := r.reconnect(ctx, &reconnectAttempts); err != nil {
					r.settings.Logger.Error("OPC UA subscription permanently lost", zap.Error(err))
					return
				}
				continue
			}

			if notif.Error != nil {
				r.settings.Logger.Warn("OPC UA publish notification error", zap.Error(notif.Error))
				continue
			}

			logs := r.decodeNotification(notif)
			if logs.LogRecordCount() == 0 {
				continue
			}

			if err := r.consumer.ConsumeLogs(ctx, logs); err != nil {
				r.settings.Logger.Error("Failed to forward OPC UA log records", zap.Error(err))
			}
		}
	}
}

// reconnect tears down stale resources and re-runs connect with back-off.
func (r *subscriptionReceiver) reconnect(ctx context.Context, attempts *uint32) error {
	maxAttempts := r.config.Subscription.MaxReconnectAttempts
	delay := r.config.Subscription.ReconnectDelay

	for {
		if maxAttempts > 0 && *attempts >= maxAttempts {
			return fmt.Errorf("exceeded max_reconnect_attempts (%d)", maxAttempts)
		}
		*attempts++
		r.settings.Logger.Info("Attempting OPC UA subscription reconnect",
			zap.Uint32("attempt", *attempts))

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}

		// Tear down stale resources before re-connecting.
		r.cleanup(ctx)
		r.handleToPath = make(map[uint32]string)

		if err := r.connect(ctx); err != nil {
			r.settings.Logger.Warn("Reconnect attempt failed",
				zap.Error(err), zap.Uint32("attempt", *attempts))
			continue
		}

		r.settings.Logger.Info("OPC UA subscription reconnected")
		*attempts = 0
		return nil
	}
}

// decodeNotification converts a raw OPC UA PublishNotificationData into a
// plog.Logs batch.
//
// Each DataChangeNotification item is expected to carry a LogRecordExtObj
// ExtensionObject (registered in log_record_type.go).  gopcua decodes the
// ExtensionObject automatically via the type registry; we just need to
// type-assert and hand it to the Transformer.
func (r *subscriptionReceiver) decodeNotification(notif *opcua.PublishNotificationData) plog.Logs {
	logs := plog.NewLogs()

	dcn, ok := notif.Value.(*ua.DataChangeNotification)
	if !ok {
		// StatusChangeNotification or KeepAlive – nothing to do.
		return logs
	}

	for _, item := range dcn.MonitoredItems {
		if item.Value == nil || item.Value.Value == nil || item.Value.Value.Value() == nil {
			continue
		}

		// DataValue.Value is *ua.Variant; unwrap to interface{} with .Value().
		// gopcua wraps decoded ExtensionObjects in *ua.ExtensionObject, so we
		// unwrap that layer before asserting to *LogRecordExtObj.
		rawVal := item.Value.Value.Value()
		if eo, isEO := rawVal.(*ua.ExtensionObject); isEO {
			rawVal = eo.Value
		}
		extObj, ok := rawVal.(*LogRecordExtObj)
		if !ok {
			r.settings.Logger.Debug("Subscription item value is not a LogRecordExtObj – skipping",
				zap.String("type", fmt.Sprintf("%T", rawVal)),
			)
			continue
		}

		path := r.handleToPath[item.ClientHandle]
		opcuaRecord := logRecordExtObjToOPCUALogRecord(extObj)

		rl := logs.ResourceLogs().AppendEmpty()
		r.transformer.setResourceAttributes(rl.Resource().Attributes())

		sl := rl.ScopeLogs().AppendEmpty()
		sl.Scope().SetName("github.com/bruegth/opentelemetry-collector-opcua-receiver")
		sl.Scope().SetVersion("0.1.0")

		lr := sl.LogRecords().AppendEmpty()
		r.transformer.transformLogRecord(opcuaRecord, lr)

		// Override timestamp with the server-side source timestamp when present.
		if !item.Value.SourceTimestamp.IsZero() {
			lr.SetTimestamp(pcommon.NewTimestampFromTime(item.Value.SourceTimestamp))
		}

		// Attach the log_object_path as an extra attribute for traceability.
		if path != "" {
			lr.Attributes().PutStr("opcua.log_object_path", path)
		}
	}
	return logs
}

// logRecordExtObjToOPCUALogRecord converts the wire-decoded LogRecordExtObj
// (from log_record_type.go) into the testdata.OPCUALogRecord that Transformer
// already knows how to handle. This keeps all field-mapping logic in one place.
func logRecordExtObjToOPCUALogRecord(ext *LogRecordExtObj) testdata.OPCUALogRecord {
	rec := testdata.OPCUALogRecord{
		Timestamp:  ext.Time,
		Severity:   ext.Severity,
		Message:    ext.Message,
		SourceName: ext.SourceName,
		TraceID:    ext.TraceIDHex(),
		SpanID:     ext.SpanIDHex(),
		Attributes: ext.AdditionalData,
	}
	if ext.SourceNode != nil {
		rec.SourceNamespace = ext.SourceNode.Namespace()
		switch ext.SourceNode.Type() {
		case ua.NodeIDTypeNumeric:
			rec.SourceIDType = "Numeric"
			rec.SourceID = fmt.Sprintf("%d", ext.SourceNode.IntID())
		case ua.NodeIDTypeString:
			rec.SourceIDType = "String"
			rec.SourceID = ext.SourceNode.StringID()
		default:
			rec.SourceIDType = ext.SourceNode.Type().String()
			rec.SourceID = ext.SourceNode.String()
		}
	}
	if rec.Attributes == nil {
		rec.Attributes = make(map[string]interface{})
	}
	return rec
}

// resolveNodeID converts a log_object_path string to a *ua.NodeID.
// Accepts standard NodeID strings ("ns=2;i=1001", "ns=2;s=ServerLog", "i=2042")
// and the browse-path shortcuts that opcuaClient.resolveBrowsePath supports.
// Plain strings without a recognised NodeID prefix are treated as browse paths,
// not passed to ua.ParseNodeID (which accepts any string as a string-type NodeID).
func resolveNodeID(path string) (*ua.NodeID, error) {
	// Only attempt ParseNodeID when the path looks like a NodeID literal, i.e.
	// it contains "=" which separates the type prefix from the identifier value.
	// Examples: "ns=2;i=1001", "i=2042", "ns=2;s=Tag", "g=...", "b=..."
	if strings.Contains(path, "=") {
		if nodeID, err := ua.ParseNodeID(path); err == nil {
			return nodeID, nil
		}
	}

	// Fall back to the static browse-path table that client.go maintains.
	knownPaths := map[string]uint32{
		"Objects/ServerLog":        2042,
		"Objects/Server/ServerLog": 2042,
		"ServerLog":                2042,
		"Objects/Server/ServerDiagnostics/ServerLog": 2042,
	}
	if id, ok := knownPaths[path]; ok {
		return ua.NewNumericNodeID(0, id), nil
	}

	return nil, fmt.Errorf(
		"cannot resolve %q to a NodeID – use a NodeID string (e.g. \"ns=2;i=1001\") "+
			"or a known browse path", path)
}

// selectEndpointForConfig is the endpoint-selection logic extracted from
// opcuaClient.selectEndpoint so both code paths share the same behaviour.
func selectEndpointForConfig(endpoints []*ua.EndpointDescription, cfg *Config) *ua.EndpointDescription {
	for _, ep := range endpoints {
		policyMatch := false
		switch cfg.SecurityPolicy {
		case "None":
			policyMatch = ep.SecurityPolicyURI == ua.SecurityPolicyURINone
		case "Basic256":
			policyMatch = ep.SecurityPolicyURI == "http://opcfoundation.org/UA/SecurityPolicy#Basic256"
		case "Basic256Sha256":
			policyMatch = ep.SecurityPolicyURI == ua.SecurityPolicyURIBasic256Sha256
		default:
			policyMatch = ep.SecurityPolicyURI == ua.SecurityPolicyURINone
		}
		modeMatch := false
		switch cfg.SecurityMode {
		case "None":
			modeMatch = ep.SecurityMode == ua.MessageSecurityModeNone
		case "Sign":
			modeMatch = ep.SecurityMode == ua.MessageSecurityModeSign
		case "SignAndEncrypt":
			modeMatch = ep.SecurityMode == ua.MessageSecurityModeSignAndEncrypt
		default:
			modeMatch = ep.SecurityMode == ua.MessageSecurityModeNone
		}
		if policyMatch && modeMatch {
			return ep
		}
	}
	// Fallback: any endpoint matching the security mode.
	for _, ep := range endpoints {
		if cfg.SecurityMode == "None" && ep.SecurityMode == ua.MessageSecurityModeNone {
			return ep
		}
	}
	if len(endpoints) > 0 {
		return endpoints[0]
	}
	return nil
}