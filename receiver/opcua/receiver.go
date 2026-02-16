// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package opcua

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/receiver"
	"go.uber.org/zap"
)

// logsReceiver implements the logs receiver
type logsReceiver struct {
	config       *Config
	settings     receiver.Settings
	nextConsumer consumer.Logs
	scraper      *scraper
	cancel       context.CancelFunc
	done         chan struct{}
}

// newLogsReceiver creates a new logs receiver
func newLogsReceiver(
	config *Config,
	settings receiver.Settings,
	nextConsumer consumer.Logs,
) (receiver.Logs, error) {
	if nextConsumer == nil {
		return nil, fmt.Errorf("nil nextConsumer")
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	scraper := newScraper(config, settings.TelemetrySettings)

	return &logsReceiver{
		config:       config,
		settings:     settings,
		nextConsumer: nextConsumer,
		scraper:      scraper,
		done:         make(chan struct{}),
	}, nil
}

// Start starts the receiver
func (r *logsReceiver) Start(ctx context.Context, host component.Host) error {
	ctx, r.cancel = context.WithCancel(ctx)

	// Start the scraper
	if err := r.scraper.start(ctx, host); err != nil {
		return fmt.Errorf("failed to start scraper: %w", err)
	}

	// Start periodic collection
	go r.runCollection(ctx)

	r.settings.Logger.Info("OPC UA receiver started",
		zap.String("endpoint", r.config.Endpoint),
		zap.Duration("collection_interval", r.config.CollectionInterval))

	return nil
}

// Shutdown stops the receiver
func (r *logsReceiver) Shutdown(ctx context.Context) error {
	if r.cancel != nil {
		r.cancel()
	}

	// Wait for collection goroutine to finish or timeout
	select {
	case <-r.done:
		r.settings.Logger.Info("Collection goroutine finished")
	case <-ctx.Done():
		r.settings.Logger.Warn("Shutdown context timeout, forcing stop")
	case <-time.After(5 * time.Second):
		r.settings.Logger.Warn("Collection goroutine did not finish within timeout")
	}

	// Shutdown the scraper
	if err := r.scraper.shutdown(ctx); err != nil {
		return fmt.Errorf("failed to shutdown scraper: %w", err)
	}

	r.settings.Logger.Info("OPC UA receiver shut down")
	return nil
}

// runCollection runs the periodic log collection
func (r *logsReceiver) runCollection(ctx context.Context) {
	defer close(r.done)

	ticker := time.NewTicker(r.config.CollectionInterval)
	defer ticker.Stop()

	r.settings.Logger.Info("Starting periodic log collection",
		zap.Duration("interval", r.config.CollectionInterval))

	// Do an initial collection immediately
	r.collectAndConsume(ctx)

	for {
		select {
		case <-ctx.Done():
			r.settings.Logger.Info("Collection context cancelled, stopping")
			return
		case <-ticker.C:
			r.collectAndConsume(ctx)
		}
	}
}

// collectAndConsume collects logs and sends them to the next consumer
func (r *logsReceiver) collectAndConsume(ctx context.Context) {
	logs, err := r.scraper.scrape(ctx)
	if err != nil {
		r.settings.Logger.Error("Failed to scrape logs", zap.Error(err))
		return
	}

	if logs.LogRecordCount() == 0 {
		r.settings.Logger.Debug("No logs collected")
		return
	}

	// Send logs to next consumer in pipeline
	if err := r.nextConsumer.ConsumeLogs(ctx, logs); err != nil {
		r.settings.Logger.Error("Failed to consume logs", zap.Error(err))
	}
}
