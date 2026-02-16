// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package opcua

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.uber.org/zap"

	"github.com/bruegth/opentelemetry-collector-opcua-receiver/receiver/opcua/testdata"
)

// scraper handles log collection from OPC UA servers
type scraper struct {
	config           *Config
	settings         component.TelemetrySettings
	transformer      *Transformer
	client           OPCUAClient
	lastCollectTime  time.Time
}

// OPCUAClient defines the interface for OPC UA client operations
// This interface allows for easier testing with mock implementations
type OPCUAClient interface {
	Connect(ctx context.Context) error
	Disconnect(ctx context.Context) error
	IsConnected() bool
	GetRecords(ctx context.Context, startTime, endTime time.Time, maxRecords int) ([]testdata.OPCUALogRecord, error)
}

// newScraper creates a new scraper
func newScraper(config *Config, settings component.TelemetrySettings) *scraper {
	return &scraper{
		config:          config,
		settings:        settings,
		transformer:     NewTransformer(config.Endpoint),
		lastCollectTime: time.Now().Add(-config.CollectionInterval), // Start from one interval ago
	}
}

// start initializes the scraper
func (s *scraper) start(ctx context.Context, host component.Host) error {
	// Create OPC UA client
	s.client = newOPCUAClient(s.config, s.settings.Logger)

	// Connect to OPC UA server
	if err := s.client.Connect(ctx); err != nil {
		s.settings.Logger.Error("Failed to connect to OPC UA server",
			zap.String("endpoint", s.config.Endpoint),
			zap.Error(err))
		return fmt.Errorf("failed to connect to OPC UA server: %w", err)
	}

	s.settings.Logger.Info("Successfully connected to OPC UA server",
		zap.String("endpoint", s.config.Endpoint))

	return nil
}

// shutdown stops the scraper
func (s *scraper) shutdown(ctx context.Context) error {
	if s.client != nil {
		if err := s.client.Disconnect(ctx); err != nil {
			s.settings.Logger.Error("Failed to disconnect from OPC UA server", zap.Error(err))
			return err
		}
	}
	return nil
}

// scrape collects logs from the OPC UA server
func (s *scraper) scrape(ctx context.Context) (plog.Logs, error) {
	// Check if client is connected
	if s.client == nil || !s.client.IsConnected() {
		// Try to reconnect
		if s.client != nil {
			s.settings.Logger.Info("Attempting to reconnect to OPC UA server")
			if err := s.client.Connect(ctx); err != nil {
				return plog.NewLogs(), fmt.Errorf("failed to reconnect: %w", err)
			}
		} else {
			return plog.NewLogs(), fmt.Errorf("client not initialized")
		}
	}

	// Calculate time range for this collection
	endTime := time.Now()
	startTime := s.lastCollectTime

	// Collect log records
	s.settings.Logger.Debug("Collecting OPC UA logs",
		zap.Time("start_time", startTime),
		zap.Time("end_time", endTime),
		zap.Int("max_records", s.config.MaxRecordsPerCall))

	records, err := s.client.GetRecords(ctx, startTime, endTime, s.config.MaxRecordsPerCall)
	if err != nil {
		s.settings.Logger.Error("Failed to get records from OPC UA server", zap.Error(err))
		return plog.NewLogs(), fmt.Errorf("failed to get records: %w", err)
	}

	s.settings.Logger.Info("Collected OPC UA log records",
		zap.Int("record_count", len(records)))

	// Update last collect time
	s.lastCollectTime = endTime

	// Transform OPC UA records to OpenTelemetry logs
	logs := s.transformer.TransformLogs(records)

	return logs, nil
}
