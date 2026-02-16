// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

//go:generate mdatagen metadata.yaml

// Package opcua implements a receiver for collecting logs from OPC UA servers.
// It supports the OPC UA Part 26 LogObject specification for retrieving log records
// and converts them to OpenTelemetry log format.
package opcua // import "github.com/bruegth/opentelemetry-collector-opcua-receiver/receiver/opcua"
