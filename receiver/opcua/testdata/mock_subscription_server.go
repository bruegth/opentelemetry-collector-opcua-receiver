// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0
package testdata

import (
	"sync"
	"time"
)

// MockSubscriptionServer simulates the push side of an OPC UA server for
// subscription receiver tests.  Callers push OPCUALogRecord values via
// Push(); the subscription receiver test helper reads from C.
type MockSubscriptionServer struct {
	C       chan OPCUALogRecord
	mu      sync.Mutex
	stopped bool
}

// NewMockSubscriptionServer creates a buffered mock subscription server.
func NewMockSubscriptionServer(bufSize int) *MockSubscriptionServer {
	if bufSize <= 0 {
		bufSize = 64
	}
	return &MockSubscriptionServer{
		C: make(chan OPCUALogRecord, bufSize),
	}
}

// Push queues a log record as if the OPC UA server had notified the client.
func (s *MockSubscriptionServer) Push(record OPCUALogRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.stopped {
		s.C <- record
	}
}

// PushTestRecord is a convenience helper that creates a record with sane
// defaults and pushes it.
func (s *MockSubscriptionServer) PushTestRecord(msg string, severity uint16) {
	s.Push(OPCUALogRecord{
		Timestamp:  time.Now().UTC(),
		Severity:   severity,
		Message:    msg,
		SourceName: "MockSubscriptionServer",
		Attributes: map[string]interface{}{"test.source": "mock"},
	})
}

// Stop closes the channel, signalling that no more records will arrive.
func (s *MockSubscriptionServer) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.stopped {
		s.stopped = true
		close(s.C)
	}
}