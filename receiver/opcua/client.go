// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package opcua

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/gopcua/opcua"
	"github.com/gopcua/opcua/ua"
	"go.uber.org/zap"

	"github.com/bruegth/opentelemetry-collector-opcua-receiver/receiver/opcua/testdata"
)

// opcuaClient implements the OPCUAClient interface using the gopcua library
type opcuaClient struct {
	config       *Config
	logger       *zap.Logger
	client       *opcua.Client
	mu           sync.Mutex
	logObjectIDs []*ua.NodeID // Support multiple LogObject nodes
}

// newOPCUAClient creates a new OPC UA client
func newOPCUAClient(config *Config, logger *zap.Logger) *opcuaClient {
	return &opcuaClient{
		config: config,
		logger: logger,
	}
}

// Connect establishes connection to the OPC UA server
func (c *opcuaClient) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Build connection options
	endpoints, err := opcua.GetEndpoints(ctx, c.config.Endpoint)
	if err != nil {
		return fmt.Errorf("failed to get endpoints: %w", err)
	}

	if len(endpoints) == 0 {
		return fmt.Errorf("no endpoints available at %s", c.config.Endpoint)
	}

	// Select appropriate endpoint based on security settings
	ep := c.selectEndpoint(endpoints)
	if ep == nil {
		return fmt.Errorf("no suitable endpoint found for security settings")
	}

	// Build client options
	opts := []opcua.Option{
		opcua.SecurityFromEndpoint(ep, ua.UserTokenTypeAnonymous),
	}

	// Add authentication
	switch c.config.Auth.Type {
	case "username_password":
		opts = append(opts, opcua.AuthUsername(c.config.Auth.Username, c.config.Auth.Password))
	case "certificate":
		if c.config.TLS.CertFile != "" && c.config.TLS.KeyFile != "" {
			opts = append(opts, opcua.CertificateFile(c.config.TLS.CertFile))
			opts = append(opts, opcua.PrivateKeyFile(c.config.TLS.KeyFile))
		}
	case "anonymous":
		opts = append(opts, opcua.AuthAnonymous())
	}

	// Add request timeout
	opts = append(opts, opcua.RequestTimeout(c.config.RequestTimeout))

	// Create client using the configured endpoint URL (not the discovered one,
	// which may contain the server's internal hostname instead of the network-reachable name).
	client, err := opcua.NewClient(c.config.Endpoint, opts...)
	if err != nil {
		return fmt.Errorf("failed to create OPC UA client: %w", err)
	}

	c.client = client

	// Connect with timeout
	connectCtx, cancel := context.WithTimeout(ctx, c.config.ConnectionTimeout)
	defer cancel()

	if err := c.client.Connect(connectCtx); err != nil {
		return fmt.Errorf("failed to connect to OPC UA server: %w", err)
	}

	c.logger.Info("Connected to OPC UA server",
		zap.String("endpoint", ep.EndpointURL),
		zap.String("security_policy", ep.SecurityPolicyURI),
		zap.String("security_mode", ep.SecurityMode.String()))

	// Discover LogObject nodes from configured paths
	if err := c.discoverLogObjects(ctx); err != nil {
		c.logger.Warn("Failed to discover LogObject nodes from configured paths", zap.Error(err))
		// Fallback: try standard ServerLog node (NodeID 2042 in namespace 0)
		c.logger.Info("Attempting to use default ServerLog node as fallback")
		if err := c.tryDefaultServerLog(ctx); err != nil {
			return fmt.Errorf("failed to discover any LogObject nodes: %w", err)
		}
	}

	if len(c.logObjectIDs) == 0 {
		return fmt.Errorf("no LogObject nodes found")
	}

	c.logger.Info("Successfully discovered LogObject nodes",
		zap.Int("count", len(c.logObjectIDs)))

	return nil
}

// Disconnect closes the connection to the OPC UA server
func (c *opcuaClient) Disconnect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.client != nil {
		if err := c.client.Close(ctx); err != nil {
			return fmt.Errorf("failed to disconnect from OPC UA server: %w", err)
		}
		c.client = nil
		c.logger.Info("Disconnected from OPC UA server")
	}

	return nil
}

// IsConnected checks if the client is currently connected
func (c *opcuaClient) IsConnected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.client != nil && c.client.State() == opcua.Connected
}

// GetRecords retrieves log records from all configured LogObject nodes
func (c *opcuaClient) GetRecords(ctx context.Context, startTime, endTime time.Time, maxRecords int) ([]testdata.OPCUALogRecord, error) {
	c.mu.Lock()
	client := c.client
	logObjectIDs := c.logObjectIDs
	c.mu.Unlock()

	if client == nil {
		return nil, fmt.Errorf("client not connected")
	}

	if len(logObjectIDs) == 0 {
		return nil, fmt.Errorf("no LogObject nodes configured")
	}

	// Collect records from all LogObject nodes
	var allRecords []testdata.OPCUALogRecord
	recordsPerNode := maxRecords / len(logObjectIDs)
	if recordsPerNode < 1 {
		recordsPerNode = 1
	}

	// Convert minimum severity from config
	minSeverity := c.getMinSeverityValue()

	for _, logObjectID := range logObjectIDs {
		// Call GetRecords with pagination support
		continuationPoint := []byte(nil)
		nodeRecords := 0

		for {
			records, nextContinuationPoint, err := c.callGetRecordsMethod(
				ctx,
				logObjectID,
				startTime,
				endTime,
				uint32(recordsPerNode-nodeRecords),
				minSeverity,
				continuationPoint,
			)

			if err != nil {
				c.logger.Warn("Failed to call GetRecords method on LogObject",
					zap.String("node_id", logObjectID.String()),
					zap.Error(err))
				break
			}

			allRecords = append(allRecords, records...)
			nodeRecords += len(records)

			// Check if we have more records via continuation point
			if len(nextContinuationPoint) == 0 || nodeRecords >= recordsPerNode {
				break
			}

			continuationPoint = nextContinuationPoint
		}
	}

	return allRecords, nil
}

// selectEndpoint selects an appropriate endpoint based on security configuration
func (c *opcuaClient) selectEndpoint(endpoints []*ua.EndpointDescription) *ua.EndpointDescription {
	// Try to find an endpoint matching the configured security
	for _, ep := range endpoints {
		// Check security policy
		policyMatch := false
		switch c.config.SecurityPolicy {
		case "None":
			policyMatch = ep.SecurityPolicyURI == ua.SecurityPolicyURINone
		case "Basic256":
			policyMatch = ep.SecurityPolicyURI == "http://opcfoundation.org/UA/SecurityPolicy#Basic256"
		case "Basic256Sha256":
			policyMatch = ep.SecurityPolicyURI == ua.SecurityPolicyURIBasic256Sha256
		default:
			policyMatch = ep.SecurityPolicyURI == ua.SecurityPolicyURINone
		}

		// Check security mode
		modeMatch := false
		switch c.config.SecurityMode {
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

	// Fallback to first endpoint with matching mode or first available
	for _, ep := range endpoints {
		if c.config.SecurityMode == "None" && ep.SecurityMode == ua.MessageSecurityModeNone {
			return ep
		}
	}

	// Last resort: return first endpoint
	if len(endpoints) > 0 {
		return endpoints[0]
	}

	return nil
}

// discoverLogObjects discovers LogObject nodes based on configured paths
func (c *opcuaClient) discoverLogObjects(ctx context.Context) error {
	if len(c.config.LogObjectPaths) == 0 {
		return fmt.Errorf("no log_object_paths configured")
	}

	var discoveredNodes []*ua.NodeID
	var errors []error

	for _, path := range c.config.LogObjectPaths {
		c.logger.Debug("Attempting to resolve LogObject path", zap.String("path", path))

		nodeID, err := c.translateBrowsePathToNodeID(ctx, path)
		if err != nil {
			c.logger.Warn("Failed to resolve LogObject path",
				zap.String("path", path),
				zap.Error(err))
			errors = append(errors, fmt.Errorf("path %s: %w", path, err))
			continue
		}

		// Verify the node exists and is accessible
		if err := c.verifyNodeExists(ctx, nodeID); err != nil {
			c.logger.Warn("LogObject node not accessible",
				zap.String("path", path),
				zap.String("node_id", nodeID.String()),
				zap.Error(err))
			errors = append(errors, fmt.Errorf("path %s (node %s): %w", path, nodeID.String(), err))
			continue
		}

		c.logger.Info("Discovered LogObject node",
			zap.String("path", path),
			zap.String("node_id", nodeID.String()))
		discoveredNodes = append(discoveredNodes, nodeID)
	}

	if len(discoveredNodes) == 0 {
		return fmt.Errorf("failed to discover any LogObject nodes: %v", errors)
	}

	c.logObjectIDs = discoveredNodes
	return nil
}

// tryDefaultServerLog attempts to use the standard ServerLog node as fallback
func (c *opcuaClient) tryDefaultServerLog(ctx context.Context) error {
	// Standard ServerLog node (NodeID 2042 in namespace 0)
	defaultNodeID := ua.NewNumericNodeID(0, 2042)

	if err := c.verifyNodeExists(ctx, defaultNodeID); err != nil {
		return fmt.Errorf("default ServerLog node not accessible: %w", err)
	}

	c.logger.Info("Using default ServerLog node", zap.String("node_id", defaultNodeID.String()))
	c.logObjectIDs = []*ua.NodeID{defaultNodeID}
	return nil
}

// verifyNodeExists checks if a node exists and is accessible
func (c *opcuaClient) verifyNodeExists(ctx context.Context, nodeID *ua.NodeID) error {
	req := &ua.ReadRequest{
		MaxAge:             2000,
		TimestampsToReturn: ua.TimestampsToReturnBoth,
		NodesToRead: []*ua.ReadValueID{
			{
				NodeID:      nodeID,
				AttributeID: ua.AttributeIDNodeClass,
			},
		},
	}

	resp, err := c.client.Read(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to read node: %w", err)
	}

	if len(resp.Results) == 0 {
		return fmt.Errorf("no results returned")
	}

	if resp.Results[0].Status != ua.StatusOK {
		return fmt.Errorf("node not accessible, status: %v", resp.Results[0].Status)
	}

	return nil
}

// translateBrowsePathToNodeID converts a browse path string or NodeID string to a NodeID
func (c *opcuaClient) translateBrowsePathToNodeID(ctx context.Context, path string) (*ua.NodeID, error) {
	// First, try to parse as a NodeID string (e.g., "ns=0;i=2042" or "i=2042")
	if nodeID, err := ua.ParseNodeID(path); err == nil {
		c.logger.Debug("Parsed path as NodeID", zap.String("path", path), zap.String("node_id", nodeID.String()))
		return nodeID, nil
	}

	// Otherwise, treat as browse path and use known mappings
	nodeID, err := c.resolveBrowsePath(path)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve browse path %s: %w", path, err)
	}

	return nodeID, nil
}

// resolveBrowsePath resolves known browse paths to NodeIDs
func (c *opcuaClient) resolveBrowsePath(path string) (*ua.NodeID, error) {
	// Map of known browse paths to their NodeIDs
	knownPaths := map[string]*ua.NodeID{
		"Objects/ServerLog":                      ua.NewNumericNodeID(0, 2042),
		"Objects/Server/ServerLog":               ua.NewNumericNodeID(0, 2042),
		"ServerLog":                              ua.NewNumericNodeID(0, 2042),
		"Objects/Server/ServerDiagnostics/ServerLog": ua.NewNumericNodeID(0, 2042),
	}

	// Check if path matches a known mapping
	if nodeID, ok := knownPaths[path]; ok {
		c.logger.Debug("Resolved browse path using known mapping",
			zap.String("path", path),
			zap.String("node_id", nodeID.String()))
		return nodeID, nil
	}

	// For unknown paths, try to browse the address space
	// This is a simplified implementation - a full implementation would use
	// the TranslateBrowsePathsToNodeIDs service
	return nil, fmt.Errorf("unknown browse path: %s (use NodeID format like 'ns=0;i=2042' or add to known paths)", path)
}

// findGetRecordsMethod browses the children of a LogObject node to find a method
// named "GetRecords". Returns the method's NodeID or an error if not found.
func (c *opcuaClient) findGetRecordsMethod(ctx context.Context, logObjectID *ua.NodeID) (*ua.NodeID, error) {
	// Try browsing with HasComponent first, then fall back to all references
	referenceTypes := []*ua.NodeID{
		ua.NewNumericNodeID(0, 47), // HasComponent
		nil,                        // All references (no filter)
	}

	for _, refType := range referenceTypes {
		desc := &ua.BrowseDescription{
			NodeID:          logObjectID,
			BrowseDirection: ua.BrowseDirectionForward,
			IncludeSubtypes: true,
			NodeClassMask:   uint32(ua.NodeClassMethod),
			ResultMask:      uint32(ua.BrowseResultMaskBrowseName),
		}
		if refType != nil {
			desc.ReferenceTypeID = refType
		}

		req := &ua.BrowseRequest{
			NodesToBrowse: []*ua.BrowseDescription{desc},
		}

		resp, err := c.client.Browse(ctx, req)
		if err != nil {
			c.logger.Debug("Browse for GetRecords failed", zap.Error(err))
			continue
		}

		if len(resp.Results) == 0 || resp.Results[0].StatusCode != ua.StatusOK {
			c.logger.Debug("Browse returned no results or bad status",
				zap.Int("results", len(resp.Results)))
			continue
		}

		for _, ref := range resp.Results[0].References {
			c.logger.Debug("Found child of LogObject",
				zap.String("browse_name", ref.BrowseName.Name),
				zap.String("node_id", ref.NodeID.NodeID.String()))
			if ref.BrowseName.Name == "GetRecords" {
				return ref.NodeID.NodeID, nil
			}
		}
	}

	return nil, fmt.Errorf("GetRecords method not found under %s", logObjectID.String())
}

// readLogRecords reads log records from the OPC UA server
// Note: This is a simplified implementation. A full implementation would:
// 1. Call the GetRecords method on the LogObject (OPC UA Part 26)
// 2. Handle ContinuationPoints for pagination
// 3. Parse the returned ExtensionObject array
