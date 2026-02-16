// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package opcua

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config with defaults",
			config: &Config{
				Endpoint:           "opc.tcp://localhost:4840",
				SecurityPolicy:     "None",
				SecurityMode:       "None",
				Auth:               AuthConfig{Type: "anonymous"},
				LogObjectPaths:     []string{"Objects/ServerLog"},
				CollectionInterval: 30 * time.Second,
				MaxRecordsPerCall:  1000,
				ConnectionTimeout:  30 * time.Second,
				RequestTimeout:     10 * time.Second,
				Filter:             FilterConfig{MinSeverity: "Info"},
			},
			wantErr: false,
		},
		{
			name: "missing endpoint",
			config: &Config{
				Endpoint: "",
			},
			wantErr: true,
			errMsg:  "endpoint must be specified",
		},
		{
			name: "invalid endpoint protocol",
			config: &Config{
				Endpoint: "http://localhost:4840",
			},
			wantErr: true,
			errMsg:  "endpoint must start with opc.tcp://",
		},
		{
			name: "collection interval too short",
			config: &Config{
				Endpoint:           "opc.tcp://localhost:4840",
				CollectionInterval: 500 * time.Millisecond,
			},
			wantErr: true,
			errMsg:  "collection_interval must be at least 1 second",
		},
		{
			name: "max records too low",
			config: &Config{
				Endpoint:           "opc.tcp://localhost:4840",
				CollectionInterval: 30 * time.Second,
				MaxRecordsPerCall:  0,
			},
			wantErr: true,
			errMsg:  "max_records_per_call must be between 1 and 10000",
		},
		{
			name: "max records too high",
			config: &Config{
				Endpoint:           "opc.tcp://localhost:4840",
				CollectionInterval: 30 * time.Second,
				MaxRecordsPerCall:  15000,
			},
			wantErr: true,
			errMsg:  "max_records_per_call must be between 1 and 10000",
		},
		{
			name: "invalid security policy",
			config: &Config{
				Endpoint:           "opc.tcp://localhost:4840",
				SecurityPolicy:     "InvalidPolicy",
				CollectionInterval: 30 * time.Second,
				MaxRecordsPerCall:  1000,
			},
			wantErr: true,
			errMsg:  "invalid security_policy",
		},
		{
			name: "invalid security mode",
			config: &Config{
				Endpoint:           "opc.tcp://localhost:4840",
				SecurityPolicy:     "None",
				SecurityMode:       "InvalidMode",
				CollectionInterval: 30 * time.Second,
				MaxRecordsPerCall:  1000,
			},
			wantErr: true,
			errMsg:  "invalid security_mode",
		},
		{
			name: "username_password auth without credentials",
			config: &Config{
				Endpoint:           "opc.tcp://localhost:4840",
				SecurityPolicy:     "None",
				SecurityMode:       "None",
				Auth:               AuthConfig{Type: "username_password"},
				CollectionInterval: 30 * time.Second,
				MaxRecordsPerCall:  1000,
				LogObjectPaths:     []string{"Objects/ServerLog"},
			},
			wantErr: true,
			errMsg:  "username and password are required",
		},
		{
			name: "certificate auth without cert files",
			config: &Config{
				Endpoint:           "opc.tcp://localhost:4840",
				SecurityPolicy:     "None",
				SecurityMode:       "None",
				Auth:               AuthConfig{Type: "certificate"},
				CollectionInterval: 30 * time.Second,
				MaxRecordsPerCall:  1000,
				LogObjectPaths:     []string{"Objects/ServerLog"},
			},
			wantErr: true,
			errMsg:  "cert_file and key_file are required",
		},
		{
			name: "invalid severity level",
			config: &Config{
				Endpoint:           "opc.tcp://localhost:4840",
				SecurityPolicy:     "None",
				SecurityMode:       "None",
				Auth:               AuthConfig{Type: "anonymous"},
				CollectionInterval: 30 * time.Second,
				MaxRecordsPerCall:  1000,
				LogObjectPaths:     []string{"Objects/ServerLog"},
				Filter:             FilterConfig{MinSeverity: "InvalidLevel"},
			},
			wantErr: true,
			errMsg:  "invalid min_severity",
		},
		{
			name: "no log object paths",
			config: &Config{
				Endpoint:           "opc.tcp://localhost:4840",
				SecurityPolicy:     "None",
				SecurityMode:       "None",
				Auth:               AuthConfig{Type: "anonymous"},
				CollectionInterval: 30 * time.Second,
				MaxRecordsPerCall:  1000,
				LogObjectPaths:     []string{},
			},
			wantErr: true,
			errMsg:  "at least one log_object_path must be specified",
		},
		{
			name: "valid config with all security options",
			config: &Config{
				Endpoint:       "opc.tcp://server.local:4840",
				SecurityPolicy: "Basic256Sha256",
				SecurityMode:   "SignAndEncrypt",
				Auth: AuthConfig{
					Type:     "username_password",
					Username: "user",
					Password: "pass",
				},
				LogObjectPaths:     []string{"Objects/ServerLog", "Objects/DeviceLog"},
				CollectionInterval: 60 * time.Second,
				MaxRecordsPerCall:  500,
				ConnectionTimeout:  30 * time.Second,
				RequestTimeout:     10 * time.Second,
				Filter: FilterConfig{
					MinSeverity:   "Warn",
					MaxLogRecords: 5000,
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestDefaultConfig(t *testing.T) {
	factory := NewFactory()
	cfg := factory.CreateDefaultConfig()

	require.NotNil(t, cfg)
	opcuaCfg, ok := cfg.(*Config)
	require.True(t, ok)

	assert.Equal(t, "opc.tcp://localhost:4840", opcuaCfg.Endpoint)
	assert.Equal(t, "None", opcuaCfg.SecurityPolicy)
	assert.Equal(t, "None", opcuaCfg.SecurityMode)
	assert.Equal(t, "anonymous", opcuaCfg.Auth.Type)
	assert.Equal(t, []string{"Objects/ServerLog"}, opcuaCfg.LogObjectPaths)
	assert.Equal(t, 30*time.Second, opcuaCfg.CollectionInterval)
	assert.Equal(t, 1000, opcuaCfg.MaxRecordsPerCall)
	assert.Equal(t, "Info", opcuaCfg.Filter.MinSeverity)

	// Validate default config
	err := opcuaCfg.Validate()
	assert.NoError(t, err)
}
