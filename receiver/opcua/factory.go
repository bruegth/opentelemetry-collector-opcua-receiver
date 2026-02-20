// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package opcua

import (
	"context"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/receiver"
)

var (
	// Type is the type of this receiver
	Type = component.MustNewType("opcua")

	// Stability level of the receiver
	stability = component.StabilityLevelAlpha
)

// NewFactory creates a factory for OPC UA receiver
func NewFactory() receiver.Factory {
	return receiver.NewFactory(
		Type,
		createDefaultConfig,
		receiver.WithLogs(createLogsReceiver, stability),
	)
}

// createDefaultConfig creates the default configuration for the receiver
func createDefaultConfig() component.Config {
	return &Config{
		Endpoint:       "opc.tcp://localhost:4840",
		SecurityPolicy: "None",
		SecurityMode:   "None",
		Auth: AuthConfig{
			Type: "anonymous",
		},
		LogObjectPaths:     []string{"Objects/ServerLog"},
		CollectionInterval: 30 * time.Second,
		MaxRecordsPerCall:  1000,
		ConnectionTimeout:  30 * time.Second,
		RequestTimeout:     10 * time.Second,
		Filter: FilterConfig{
			MinSeverity:   "Info",
			MaxLogRecords: 10000,
		},
		TLS: TLSConfig{
			InsecureSkipVerify: false,
		},
		Resource: ResourceConfig{
			ServiceName: "opcua-server",
		},
	}
}

// createLogsReceiver creates a logs receiver based on the config
func createLogsReceiver(
	ctx context.Context,
	set receiver.Settings,
	cfg component.Config,
	nextConsumer consumer.Logs,
) (receiver.Logs, error) {
	receiverConfig := cfg.(*Config)

	return newLogsReceiver(receiverConfig, set, nextConsumer)
}
