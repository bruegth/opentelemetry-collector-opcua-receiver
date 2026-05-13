// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0
package opcua

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// CollectionMode controls how the receiver gathers log records from the OPC UA server.
type CollectionMode string

const (
	// CollectionModePoll uses the GetRecords method call on a fixed interval (default).
	CollectionModePoll CollectionMode = "poll"
	// CollectionModeSubscription uses OPC UA Subscriptions / MonitoredItems so the
	// server pushes notifications when new log records arrive.
	CollectionModeSubscription CollectionMode = "subscription"
)

// Config defines configuration for the OPC UA receiver
type Config struct {
	// Endpoint is the OPC UA server endpoint URL (e.g., opc.tcp://localhost:4840)
	Endpoint string `mapstructure:"endpoint"`
	// SecurityPolicy defines the security policy (None, Basic256, Basic256Sha256, etc.)
	SecurityPolicy string `mapstructure:"security_policy"`
	// SecurityMode defines the security mode (None, Sign, SignAndEncrypt)
	SecurityMode string `mapstructure:"security_mode"`
	// Auth contains authentication configuration
	Auth AuthConfig `mapstructure:"auth"`
	// LogObjectPaths are the paths to browse for LogObject nodes
	LogObjectPaths []string `mapstructure:"log_object_paths"`
	// CollectionInterval is the interval between log collection attempts (poll mode only)
	CollectionInterval time.Duration `mapstructure:"collection_interval"`
	// MaxRecordsPerCall is the maximum number of records to retrieve per GetRecords call
	MaxRecordsPerCall int `mapstructure:"max_records_per_call"`
	// Filter contains log filtering options
	Filter FilterConfig `mapstructure:"filter"`
	// ConnectionTimeout is the timeout for establishing OPC UA connection
	ConnectionTimeout time.Duration `mapstructure:"connection_timeout"`
	// RequestTimeout is the timeout for individual OPC UA requests
	RequestTimeout time.Duration `mapstructure:"request_timeout"`
	// TLS contains TLS/certificate configuration
	TLS TLSConfig `mapstructure:"tls"`
	// Resource contains resource-level OTel attributes attached to every log record.
	Resource ResourceConfig `mapstructure:"resource"`

	// CollectionMode selects poll (default) or subscription-based collection.
	// poll:         periodic GetRecords calls on CollectionInterval
	// subscription: OPC UA Subscription / MonitoredItem push delivery
	CollectionMode CollectionMode `mapstructure:"collection_mode"`

	// Subscription holds options only relevant when CollectionMode == "subscription".
	Subscription SubscriptionConfig `mapstructure:"subscription"`
}

// SubscriptionConfig holds options specific to subscription-based collection.
type SubscriptionConfig struct {
	// PublishingInterval is how often the server batches and sends change
	// notifications. Lower values give lower latency at the cost of more
	// network traffic. Defaults to 1s.
	PublishingInterval time.Duration `mapstructure:"publishing_interval"`

	// QueueSize is the server-side monitored-item queue depth per node.
	// Increase when the server may produce bursts of log records faster
	// than the collector can drain them. Defaults to 100.
	QueueSize uint32 `mapstructure:"queue_size"`

	// ReconnectDelay is how long to wait before re-establishing a broken
	// subscription. Defaults to 5s.
	ReconnectDelay time.Duration `mapstructure:"reconnect_delay"`

	// MaxReconnectAttempts caps the number of reconnect tries before the
	// receiver gives up and stops. 0 means unlimited. Defaults to 0.
	MaxReconnectAttempts uint32 `mapstructure:"max_reconnect_attempts"`
}

// AuthConfig defines authentication configuration
type AuthConfig struct {
	// Type is the authentication type (anonymous, username_password, certificate)
	Type string `mapstructure:"type"`
	// Username for username/password authentication
	Username string `mapstructure:"username"`
	// Password for username/password authentication
	Password string `mapstructure:"password"`
}

// FilterConfig defines log filtering options
type FilterConfig struct {
	// MinSeverity is the minimum severity level to collect (Trace, Debug, Info, Warn, Error, Fatal)
	MinSeverity string `mapstructure:"min_severity"`
	// MaxLogRecords is the maximum total number of log records to collect
	MaxLogRecords int `mapstructure:"max_log_records"`
}

// ResourceConfig defines the OTel resource attributes that are emitted with every log record.
type ResourceConfig struct {
	// ServiceName sets the resource attribute service.name.
	// Defaults to "opcua-server" when empty.
	ServiceName string `mapstructure:"service_name"`
	// ServiceNamespace sets the resource attribute service.namespace.
	// Not emitted when empty.
	ServiceNamespace string `mapstructure:"service_namespace"`
}

// TLSConfig defines TLS/certificate configuration
type TLSConfig struct {
	// CertFile is the path to the client certificate file
	CertFile string `mapstructure:"cert_file"`
	// KeyFile is the path to the client private key file
	KeyFile string `mapstructure:"key_file"`
	// CAFile is the path to the CA certificate file
	CAFile string `mapstructure:"ca_file"`
	// InsecureSkipVerify skips certificate verification (for testing only)
	InsecureSkipVerify bool `mapstructure:"insecure_skip_verify"`
}

// Validate validates the configuration
func (cfg *Config) Validate() error {
	if cfg.Endpoint == "" {
		return errors.New("endpoint must be specified")
	}
	if !strings.HasPrefix(cfg.Endpoint, "opc.tcp://") {
		return fmt.Errorf("endpoint must start with opc.tcp://, got: %s", cfg.Endpoint)
	}

	// collection_interval is only required for poll mode
	if cfg.CollectionMode == CollectionModePoll || cfg.CollectionMode == "" {
		if cfg.CollectionInterval < 1*time.Second {
			return fmt.Errorf("collection_interval must be at least 1 second, got: %s", cfg.CollectionInterval)
		}
	}

	if cfg.CollectionMode != "" &&
		cfg.CollectionMode != CollectionModePoll &&
		cfg.CollectionMode != CollectionModeSubscription {
		return fmt.Errorf("collection_mode must be %q or %q, got: %s",
			CollectionModePoll, CollectionModeSubscription, cfg.CollectionMode)
	}

	if cfg.CollectionMode == CollectionModeSubscription {
		if cfg.Subscription.PublishingInterval < 0 {
			return errors.New("subscription.publishing_interval must be >= 0")
		}
	}

	if cfg.MaxRecordsPerCall < 1 || cfg.MaxRecordsPerCall > 10000 {
		return fmt.Errorf("max_records_per_call must be between 1 and 10000, got: %d", cfg.MaxRecordsPerCall)
	}

	validSecurityPolicies := []string{"None", "Basic256", "Basic256Sha256", "Aes128_Sha256_RsaOaep", "Aes256_Sha256_RsaPss"}
	if !contains(validSecurityPolicies, cfg.SecurityPolicy) {
		return fmt.Errorf("invalid security_policy: %s, must be one of: %v", cfg.SecurityPolicy, validSecurityPolicies)
	}

	validSecurityModes := []string{"None", "Sign", "SignAndEncrypt"}
	if !contains(validSecurityModes, cfg.SecurityMode) {
		return fmt.Errorf("invalid security_mode: %s, must be one of: %v", cfg.SecurityMode, validSecurityModes)
	}

	validAuthTypes := []string{"anonymous", "username_password", "certificate"}
	if !contains(validAuthTypes, cfg.Auth.Type) {
		return fmt.Errorf("invalid auth type: %s, must be one of: %v", cfg.Auth.Type, validAuthTypes)
	}

	if cfg.Auth.Type == "username_password" {
		if cfg.Auth.Username == "" || cfg.Auth.Password == "" {
			return errors.New("username and password are required for username_password authentication")
		}
	}
	if cfg.Auth.Type == "certificate" {
		if cfg.TLS.CertFile == "" || cfg.TLS.KeyFile == "" {
			return errors.New("cert_file and key_file are required for certificate authentication")
		}
	}

	validSeverities := []string{"Trace", "Debug", "Info", "Warn", "Error", "Fatal", ""}
	if !contains(validSeverities, cfg.Filter.MinSeverity) {
		return fmt.Errorf("invalid min_severity: %s, must be one of: Trace, Debug, Info, Warn, Error, Fatal", cfg.Filter.MinSeverity)
	}

	if cfg.Filter.MaxLogRecords < 0 {
		return fmt.Errorf("max_log_records must be non-negative, got: %d", cfg.Filter.MaxLogRecords)
	}

	if len(cfg.LogObjectPaths) == 0 {
		return errors.New("at least one log_object_path must be specified")
	}

	return nil
}

// contains checks if a slice contains a string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}