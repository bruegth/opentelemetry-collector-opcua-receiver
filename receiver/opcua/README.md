# OPC UA Log Receiver

## Overview

The OPC UA Log Receiver collects log records from OPC UA servers implementing the LogObject specification (OPC UA Part 26). It converts OPC UA log records into OpenTelemetry log format, enabling observability for industrial automation systems.

## Status

**Stability**: Alpha
**Supported Pipeline Types**: logs

## Prerequisites

- OPC UA server with network connectivity
- Appropriate credentials or certificates (depending on security configuration)
- For full functionality: OPC UA server implementing Part 26 LogObject specification

## Configuration

### Basic Configuration

```yaml
receivers:
  opcua:
    endpoint: opc.tcp://localhost:4840
    collection_interval: 30s
```

### Full Configuration Example

```yaml
receivers:
  opcua:
    # OPC UA server endpoint (required)
    endpoint: opc.tcp://opcua.server.local:4840

    # Security settings
    security_policy: Basic256Sha256  # None, Basic256, Basic256Sha256, Aes128_Sha256_RsaOaep, Aes256_Sha256_RsaPss
    security_mode: SignAndEncrypt    # None, Sign, SignAndEncrypt

    # Authentication
    auth:
      type: username_password  # anonymous, username_password, certificate
      username: opcua_user
      password: ${env:OPCUA_PASSWORD}

    # LogObject node paths to collect from
    log_object_paths:
      - Objects/ServerLog
      - Objects/DeviceSets/Device1/Logs

    # Collection settings
    collection_interval: 30s
    max_records_per_call: 1000

    # Filtering options
    filter:
      min_severity: Info  # Trace, Debug, Info, Warn, Error, Fatal
      max_log_records: 10000

    # Connection timeouts
    connection_timeout: 30s
    request_timeout: 10s

    # TLS/Certificate configuration (for certificate auth)
    tls:
      cert_file: /path/to/client-cert.pem
      key_file: /path/to/client-key.pem
      ca_file: /path/to/ca-cert.pem
      insecure_skip_verify: false

exporters:
  debug:
    verbosity: detailed

service:
  pipelines:
    logs:
      receivers: [opcua]
      exporters: [debug]
```

### Configuration Parameters

#### Required

- **endpoint** (string): OPC UA server endpoint URL. Must start with `opc.tcp://`.

#### Optional

- **security_policy** (string): Security policy to use. Default: `None`
  - Options: `None`, `Basic256`, `Basic256Sha256`, `Aes128_Sha256_RsaOaep`, `Aes256_Sha256_RsaPss`

- **security_mode** (string): Security mode to use. Default: `None`
  - Options: `None`, `Sign`, `SignAndEncrypt`

- **auth** (object): Authentication configuration
  - **type** (string): Authentication type. Default: `anonymous`
    - Options: `anonymous`, `username_password`, `certificate`
  - **username** (string): Username for username/password authentication
  - **password** (string): Password for username/password authentication

- **log_object_paths** ([]string): Paths or NodeIDs of LogObject nodes to collect from. Default: `["Objects/ServerLog"]`
  - Supports NodeID format: `"ns=0;i=2042"` or `"i=2042"`
  - Supports browse path format: `"Objects/ServerLog"`
  - Multiple paths can be specified to collect from multiple LogObjects simultaneously
  - The receiver will attempt to resolve each path and collect logs from all discovered nodes

- **collection_interval** (duration): Interval between log collection attempts. Default: `30s`. Minimum: `1s`

- **max_records_per_call** (int): Maximum number of records to retrieve per GetRecords call. Default: `1000`. Range: `1-10000`

- **filter** (object): Log filtering options
  - **min_severity** (string): Minimum severity level to collect. Default: `Info`
    - Options: `Trace`, `Debug`, `Info`, `Warn`, `Error`, `Fatal`
  - **max_log_records** (int): Maximum total number of log records to collect. Default: `10000`

- **connection_timeout** (duration): Timeout for establishing OPC UA connection. Default: `30s`

- **request_timeout** (duration): Timeout for individual OPC UA requests. Default: `10s`

- **tls** (object): TLS/certificate configuration
  - **cert_file** (string): Path to client certificate file
  - **key_file** (string): Path to client private key file
  - **ca_file** (string): Path to CA certificate file
  - **insecure_skip_verify** (bool): Skip certificate verification (for testing only). Default: `false`

## Supported Features

- ✅ OPC UA connection with configurable security
- ✅ Multiple authentication methods (anonymous, username/password, certificate)
- ✅ Periodic log collection
- ✅ Severity level mapping (OPC UA → OpenTelemetry)
- ✅ Trace context propagation
- ✅ Resource attributes for server identification
- ⚠️ OPC UA Part 26 LogObject support (simplified implementation)

## Data Mapping

### Severity Mapping

The receiver maps OPC UA severity levels to OpenTelemetry severity numbers:

| OPC UA Range | OPC UA Level | OpenTelemetry SeverityNumber |
|--------------|--------------|------------------------------|
| 0-99         | Trace        | SeverityNumberTrace (1)      |
| 100-199      | Debug        | SeverityNumberDebug (5)      |
| 200-399      | Info         | SeverityNumberInfo (9)       |
| 400-599      | Warn         | SeverityNumberWarn (13)      |
| 600-799      | Error        | SeverityNumberError (17)     |
| 800-1000     | Fatal        | SeverityNumberFatal (21)     |

### Resource Attributes

The following resource attributes are set for all logs:

- `opcua.server.endpoint`: OPC UA server endpoint URL
- `service.name`: Service name (default: "opcua-server")
- `telemetry.sdk.name`: SDK name ("opentelemetry-collector-opcua")
- `telemetry.sdk.language`: SDK language ("go")

### Log Attributes

Per-log attributes from OPC UA:

- `opcua.source`: Log source component/module
- Custom attributes from OPC UA LogRecord

## Examples

### Basic Anonymous Connection

```yaml
receivers:
  opcua:
    endpoint: opc.tcp://localhost:4840
    collection_interval: 30s

exporters:
  debug:
    verbosity: detailed

service:
  pipelines:
    logs:
      receivers: [opcua]
      exporters: [debug]
```

### Secure Connection with Username/Password

```yaml
receivers:
  opcua:
    endpoint: opc.tcp://secure.opcua.server:4840
    security_policy: Basic256Sha256
    security_mode: SignAndEncrypt
    auth:
      type: username_password
      username: ${env:OPCUA_USERNAME}
      password: ${env:OPCUA_PASSWORD}
    collection_interval: 60s

exporters:
  otlphttp:
    endpoint: http://observability-backend:4318

service:
  pipelines:
    logs:
      receivers: [opcua]
      processors: [batch]
      exporters: [otlphttp]
```

### Certificate-Based Authentication

```yaml
receivers:
  opcua:
    endpoint: opc.tcp://industrial.server:4840
    security_policy: Basic256Sha256
    security_mode: SignAndEncrypt
    auth:
      type: certificate
    tls:
      cert_file: /etc/opcua/client-cert.pem
      key_file: /etc/opcua/client-key.pem
      ca_file: /etc/opcua/ca-cert.pem
    log_object_paths:
      - Objects/ServerLog
      - Objects/DeviceSets/PLC1/Logs
    collection_interval: 30s
    filter:
      min_severity: Warn
      max_log_records: 5000

exporters:
  debug:
    verbosity: detailed

service:
  pipelines:
    logs:
      receivers: [opcua]
      exporters: [debug]
```

## Troubleshooting

### Connection Issues

**Problem**: Failed to connect to OPC UA server

**Solutions**:
- Verify the endpoint URL is correct and starts with `opc.tcp://`
- Check network connectivity to the OPC UA server
- Ensure security policy and mode match server configuration
- Verify credentials are correct for authentication
- Check firewall rules allow connection to the OPC UA port

### Authentication Failures

**Problem**: Authentication errors when connecting

**Solutions**:
- For username/password: Verify credentials are correct
- For certificate: Ensure certificate files exist and are readable
- Check that the server accepts the configured authentication method
- Verify the user has permissions to access LogObject nodes

### No Logs Collected

**Problem**: Receiver starts but no logs appear

**Solutions**:
- Verify the OPC UA server implements Part 26 LogObject (most servers don't yet)
- Check `log_object_paths` configuration points to valid LogObject nodes
- Ensure `min_severity` filter isn't too restrictive
- Increase log level to DEBUG to see detailed collection information
- Check if the server has generated any logs in the configured time period

### Performance Issues

**Problem**: High CPU or memory usage

**Solutions**:
- Increase `collection_interval` to reduce polling frequency
- Decrease `max_records_per_call` to limit batch sizes
- Use `filter.min_severity` to collect only important logs
- Set `filter.max_log_records` to limit total log volume

## Development

### Building

```bash
cd receiver/opcua
go build
```

### Testing

```bash
# Run all tests
go test ./...

# Run with coverage
go test -cover ./...

# Run specific test
go test -run TestTransformLogs -v
```

### Building Custom Collector

```powershell
# Update builder-config.yaml to include opcua receiver
.\ocb.exe --config builder-config.yaml

# Build the collector
cd otelcol-dev
go build -o otelcol-dev.exe
```

## Limitations

- **Alpha Status**: This receiver is in alpha stage and the API may change
- **Part 26 Support**: Full OPC UA Part 26 LogObject implementation is simplified
- **Polling Only**: Currently uses polling; subscriptions not yet supported
- **GetRecords Method**: Simplified implementation; most OPC UA servers don't implement Part 26 yet

## Future Enhancements

- Full OPC UA Part 26 LogObject specification compliance
- Support for subscriptions/monitoring in addition to polling
- ContinuationPoint handling for large log sets
- Additional filtering options (by source, category, etc.)
- Metrics about receiver performance
- Support for multiple LogObject nodes simultaneously

## Contributing

Contributions are welcome! Please see the [CONTRIBUTING.md](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/main/CONTRIBUTING.md) guide.

## License

This receiver is part of the OpenTelemetry Collector and follows the same license.

## References

- [OPC UA Specification Part 26 - LogObject](https://reference.opcfoundation.org/Core/Part26/v105/docs/)
- [OpenTelemetry Collector Documentation](https://opentelemetry.io/docs/collector/)
- [gopcua - Go OPC-UA Library](https://github.com/gopcua/opcua)
- [OpenTelemetry Logs Data Model](https://opentelemetry.io/docs/specs/otel/logs/data-model/)
