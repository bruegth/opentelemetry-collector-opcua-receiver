// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0
package opcua

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gopcua/opcua"
	"github.com/gopcua/opcua/id"
	"github.com/gopcua/opcua/ua"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/receiver"
	"go.uber.org/zap"

	"github.com/bruegth/opentelemetry-collector-opcua-receiver/receiver/opcua/testdata"
)

// baseLogEventTypeID is the well-known NodeID for BaseLogEventType per OPC UA Part 26 §6.3.
var baseLogEventTypeID = ua.NewNumericNodeID(0, 18000)

// Event field index constants – must stay in sync with logEventSelectClauses().
const (
	fieldIdxTime           = 0
	fieldIdxSeverity       = 1
	fieldIdxMessage        = 2
	fieldIdxSourceName     = 3
	fieldIdxSourceNode     = 4
	fieldIdxTraceContext   = 5
	fieldIdxAdditionalData = 6
	fieldIdxCount          = 7
)

// logEventSelectClauses returns the SimpleAttributeOperand select clauses for
// a BaseLogEventType event subscription.  The order defines the field indices above.
// TypeDefinitionID uses BaseEventType (ns=0;i=2041) which is always registered in
// the server type hierarchy; Part 26-specific fields fall back gracefully when absent.
func logEventSelectClauses() []*ua.SimpleAttributeOperand {
	baseEventTypeID := ua.NewNumericNodeID(0, id.BaseEventType)
	mkField := func(name string) *ua.SimpleAttributeOperand {
		return &ua.SimpleAttributeOperand{
			TypeDefinitionID: baseEventTypeID,
			BrowsePath:       []*ua.QualifiedName{{NamespaceIndex: 0, Name: name}},
			AttributeID:      ua.AttributeIDValue,
		}
	}
	return []*ua.SimpleAttributeOperand{
		mkField("Time"),           // 0
		mkField("Severity"),       // 1
		mkField("Message"),        // 2
		mkField("SourceName"),     // 3
		mkField("SourceNode"),     // 4
		mkField("TraceContext"),   // 5
		mkField("AdditionalData"), // 6
	}
}

// logEventWhereClause returns an empty ContentFilter (no restrictions).
// We rely on the select clauses and the event notifier node itself to scope
// which events are delivered. An empty where clause is valid per OPC UA Part 4
// §7.17.3 and avoids server-side decoding issues with LiteralOperand encoding
// variants that differ between SDK implementations.
func logEventWhereClause() *ua.ContentFilter {
	return &ua.ContentFilter{}
}

// eventMonitoredItemRequest creates a MonitoredItemCreateRequest that subscribes
// to BaseLogEventType events on an event-notifier node (AttributeIDEventNotifier).
func eventMonitoredItemRequest(nodeID *ua.NodeID, handle uint32, queueSize uint32) *ua.MonitoredItemCreateRequest {
	filter := ua.EventFilter{
		SelectClauses: logEventSelectClauses(),
		WhereClause:   logEventWhereClause(),
	}
	filterExt := &ua.ExtensionObject{
		EncodingMask: ua.ExtensionObjectBinary,
		TypeID: &ua.ExpandedNodeID{
			NodeID: ua.NewNumericNodeID(0, id.EventFilter_Encoding_DefaultBinary),
		},
		Value: filter,
	}
	return &ua.MonitoredItemCreateRequest{
		ItemToMonitor: &ua.ReadValueID{
			NodeID:       nodeID,
			AttributeID:  ua.AttributeIDEventNotifier,
			DataEncoding: &ua.QualifiedName{},
		},
		MonitoringMode: ua.MonitoringModeReporting,
		RequestedParameters: &ua.MonitoringParameters{
			ClientHandle:     handle,
			SamplingInterval: 1.0,
			Filter:           filterExt,
			QueueSize:        queueSize,
			DiscardOldest:    true,
		},
	}
}

// subscriptionReceiver implements receiver.Logs using OPC UA event subscriptions
// per OPC UA Part 26 §6 (LogObject and Events).
//
// The receiver monitors the EventNotifier attribute on each log_object_path node.
// The server fires BaseLogEventType events when new log records are generated.
type subscriptionReceiver struct {
	config      *Config
	settings    receiver.Settings
	consumer    consumer.Logs
	transformer *Transformer

	client       *opcua.Client
	subscription *opcua.Subscription
	notifCh      chan *opcua.PublishNotificationData
	handleToPath map[uint32]string

	cancelFn context.CancelFunc
	doneCh   chan struct{}
}

var _ receiver.Logs = (*subscriptionReceiver)(nil)

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

func (r *subscriptionReceiver) Start(ctx context.Context, _ component.Host) error {
	r.settings.Logger.Info("Starting OPC UA subscription receiver (event mode)",
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

func (r *subscriptionReceiver) connect(ctx context.Context) error {
	endpoints, err := opcua.GetEndpoints(ctx, r.config.Endpoint)
	if err != nil {
		return fmt.Errorf("get endpoints: %w", err)
	}
	if len(endpoints) == 0 {
		return fmt.Errorf("no endpoints at %s", r.config.Endpoint)
	}
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
		items = append(items, eventMonitoredItemRequest(nodeID, handle, r.config.Subscription.QueueSize))
	}

	if len(items) == 0 {
		return fmt.Errorf("no log_object_paths could be resolved to valid NodeIDs")
	}

	resp, err := sub.Monitor(ctx, ua.TimestampsToReturnBoth, items...)
	if err != nil {
		return fmt.Errorf("monitor event items: %w", err)
	}
	for i, res := range resp.Results {
		if res.StatusCode != ua.StatusOK {
			r.settings.Logger.Warn("Event MonitoredItem creation returned non-OK status",
				zap.Int("index", i),
				zap.Uint32("status_code", uint32(res.StatusCode)),
			)
		}
	}
	r.settings.Logger.Info("Monitoring log object event notifiers",
		zap.Int("count", len(items)))
	return nil
}

func (r *subscriptionReceiver) runLoop(ctx context.Context) {
	defer close(r.doneCh)
	reconnectAttempts := uint32(0)

	for {
		select {
		case <-ctx.Done():
			return
		case notif, ok := <-r.notifCh:
			if !ok {
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
				r.settings.Logger.Error("Failed to forward OPC UA log events", zap.Error(err))
			}
		}
	}
}

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

// decodeNotification converts an EventNotificationList into plog.Logs.
func (r *subscriptionReceiver) decodeNotification(notif *opcua.PublishNotificationData) plog.Logs {
	logs := plog.NewLogs()

	enl, ok := notif.Value.(*ua.EventNotificationList)
	if !ok {
		// Not an event notification (DataChange or KeepAlive) – ignore.
		return logs
	}

	for _, event := range enl.Events {
		if event == nil || len(event.EventFields) == 0 {
			continue
		}
		path := r.handleToPath[event.ClientHandle]
		rec, err := r.decodeEventFields(event.EventFields)
		if err != nil {
			r.settings.Logger.Debug("Failed to decode event fields",
				zap.String("path", path), zap.Error(err))
			continue
		}

		rl := logs.ResourceLogs().AppendEmpty()
		r.transformer.setResourceAttributes(rl.Resource().Attributes())

		sl := rl.ScopeLogs().AppendEmpty()
		sl.Scope().SetName("github.com/bruegth/opentelemetry-collector-opcua-receiver")
		sl.Scope().SetVersion("0.1.0")

		lr := sl.LogRecords().AppendEmpty()
		r.transformer.transformLogRecord(rec, lr)

		if path != "" {
			lr.Attributes().PutStr("opcua.log_object_path", path)
		}
	}
	return logs
}

// decodeEventFields maps the ordered EventFieldList values (matching
// logEventSelectClauses) into a testdata.OPCUALogRecord.
func (r *subscriptionReceiver) decodeEventFields(fields []*ua.Variant) (testdata.OPCUALogRecord, error) {
	if len(fields) < fieldIdxCount {
		return testdata.OPCUALogRecord{}, fmt.Errorf(
			"expected %d event fields, got %d", fieldIdxCount, len(fields))
	}

	rec := testdata.OPCUALogRecord{
		Attributes: make(map[string]interface{}),
	}

	if t, ok := variantTime(fields[fieldIdxTime]); ok {
		rec.Timestamp = t
	}
	if s, ok := variantUInt16(fields[fieldIdxSeverity]); ok {
		rec.Severity = s
	}
	if m, ok := variantLocalizedText(fields[fieldIdxMessage]); ok {
		rec.Message = m
	}
	if sn, ok := variantString(fields[fieldIdxSourceName]); ok {
		rec.SourceName = sn
	}
	if node, ok := variantNodeID(fields[fieldIdxSourceNode]); ok && node != nil {
		rec.SourceNamespace = node.Namespace()
		switch node.Type() {
		case ua.NodeIDTypeNumeric, ua.NodeIDTypeTwoByte, ua.NodeIDTypeFourByte:
			rec.SourceIDType = "Numeric"
			rec.SourceID = fmt.Sprintf("%d", node.IntID())
		case ua.NodeIDTypeString:
			rec.SourceIDType = "String"
			rec.SourceID = node.StringID()
		default:
			rec.SourceIDType = node.Type().String()
			rec.SourceID = node.String()
		}
	}

	// TraceContext: ExtensionObject wrapping LogRecordExtObj
	if fields[fieldIdxTraceContext] != nil {
		if ext, ok := fields[fieldIdxTraceContext].Value().(*ua.ExtensionObject); ok && ext != nil {
			if lrExt, ok := ext.Value.(*LogRecordExtObj); ok && lrExt != nil {
				rec.TraceID = lrExt.TraceIDHex()
				rec.SpanID = lrExt.SpanIDHex()
			}
		}
	}

	// AdditionalData: map[string]interface{}
	if fields[fieldIdxAdditionalData] != nil {
		if m, ok := fields[fieldIdxAdditionalData].Value().(map[string]interface{}); ok {
			for k, v := range m {
				rec.Attributes[k] = v
			}
		}
	}

	if rec.Timestamp.IsZero() {
		rec.Timestamp = time.Now().UTC()
	}
	return rec, nil
}

// ── Variant extraction helpers ────────────────────────────────────────────

func variantTime(v *ua.Variant) (time.Time, bool) {
	if v == nil {
		return time.Time{}, false
	}
	t, ok := v.Value().(time.Time)
	return t, ok
}

func variantUInt16(v *ua.Variant) (uint16, bool) {
	if v == nil {
		return 0, false
	}
	switch val := v.Value().(type) {
	case uint16:
		return val, true
	case int16:
		return uint16(val), true //nolint:gosec
	}
	return 0, false
}

func variantString(v *ua.Variant) (string, bool) {
	if v == nil {
		return "", false
	}
	s, ok := v.Value().(string)
	return s, ok
}

func variantLocalizedText(v *ua.Variant) (string, bool) {
	if v == nil {
		return "", false
	}
	switch val := v.Value().(type) {
	case *ua.LocalizedText:
		if val != nil {
			return val.Text, true
		}
	case string:
		return val, true
	}
	return "", false
}

func variantNodeID(v *ua.Variant) (*ua.NodeID, bool) {
	if v == nil {
		return nil, false
	}
	n, ok := v.Value().(*ua.NodeID)
	return n, ok
}

// resolveNodeID converts a log_object_path string to a *ua.NodeID.
func resolveNodeID(path string) (*ua.NodeID, error) {
	if strings.Contains(path, "=") {
		if nodeID, err := ua.ParseNodeID(path); err == nil {
			return nodeID, nil
		}
	}
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

// selectEndpointForConfig picks the best matching endpoint.
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