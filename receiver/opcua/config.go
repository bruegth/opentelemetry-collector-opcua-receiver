// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package opcua

import (
	"errors"
	"fmt"
	"strings"
	"time"
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

	// CollectionInterval is the interval between log collection attempts
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

	if cfg.CollectionInterval < 1*time.Second {
		return fmt.Errorf("collection_interval must be at least 1 second, got: %s", cfg.CollectionInterval)
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
