# OPC UA Log Receiver

## Overview

The OPC UA Log Receiver collects log records from OPC UA servers implementing the LogObject specification (OPC UA Part 26). It converts OPC UA log records into OpenTelemetry log format, enabling observability for industrial automation systems.

**Features**: multiple authentication methods (anonymous, username/password, X.509 certificates), configurable security policies, severity filtering, trace context propagation, ContinuationPoint pagination.

Polling decodes the standard Part 26 `LogRecordsDataType` response, including
optional fields. The legacy test-server LogRecord ExtensionObject array remains supported.
Unsupported standard response types and invalid continuation points fail the scrape.

## Status

**Stability**: Beta | **Supported Pipeline Types**: logs

## Prerequisites

- OPC UA server with network connectivity
- Appropriate credentials or certificates (depending on security configuration)
- OPC UA server implementing Part 26 LogObject specification

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
      - "ns=2;s=DeviceLog"

    # Collection settings
    collection_interval: 30s
    max_records_per_call: 1000

    # Filtering options
    filter:
      min_severity: Info  # Trace, Debug, Info, Warn, Error, Fatal, Emergency
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

    # Resource attributes emitted with every log record
    resource:
      service_name: my-opcua-server   # default: opcua-server
      service_namespace: production    # optional; omitted when empty

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

- **security_policy** (string): Security policy. Default: `None`
  - Options: `None`, `Basic256`, `Basic256Sha256`, `Aes128_Sha256_RsaOaep`, `Aes256_Sha256_RsaPss`

- **security_mode** (string): Security mode. Default: `None`
  - Options: `None`, `Sign`, `SignAndEncrypt`

- **auth** (object): Authentication configuration
  - Known limitation: username and certificate user authentication are not reliably configured; see the [review](../../PART26_REVIEW.md).
  - **type** (string): Authentication type. Default: `anonymous`
    - Options: `anonymous`, `username_password`, `certificate`
  - **username** / **password** (string): Credentials for `username_password` auth
  - **cert_file** / **key_file** (string): Certificate paths for `certificate` auth

- **log_object_paths** ([]string): LogObject NodeIDs or known ServerLog aliases. Default: `["Objects/ServerLog"]`
  - `"Objects/ServerLog"` and `"Objects/Server/ServerLog"` resolve to the standard ServerLog NodeID.
  - Supports NodeID format: `"ns=0;i=19372"` or `"ns=2;s=DeviceLog"`; arbitrary browse paths are not resolved.

- **collection_interval** (duration): Interval between log collections. Default: `30s`. Minimum: `1s`

- **max_records_per_call** (int): Maximum records per GetRecords call. Default: `1000`. Range: `1–10000`

- **filter** (object): Log filtering options
  - **min_severity** (string): Minimum severity to collect. Default: `Info`
    - Options: `Trace`, `Debug`, `Info`, `Warn`, `Warning`, `Error`, `Fatal`, `Emergency`
    - Inclusive thresholds: `Trace`/`Debug` = 1, `Info` = 51, `Warn`/`Warning` = 151, `Error` = 201, `Fatal`/`Emergency` = 401.
    - Polling passes the threshold to `GetRecords`; subscription mode filters received events locally. Missing or invalid event severities are discarded with a warning.
  - **max_log_records** (int): Currently not enforced. Default: `10000`

- **connection_timeout** (duration): Timeout for establishing connection. Default: `30s`

- **request_timeout** (duration): Timeout for individual requests. Default: `10s`

- **tls** (object): TLS configuration
  - **cert_file** / **key_file** (string): Client certificate and key
  - **ca_file** (string): Currently not applied to the OPC UA client
  - **insecure_skip_verify** (bool): Currently not applied to the OPC UA client. Default: `false`

- **resource** (object): Resource attributes emitted with every log record
  - **service_name** (string): Value for `service.name`. Default: `opcua-server`
  - **service_namespace** (string): Value for `service.namespace` (omitted when empty)

The custom collector distribution also includes an optional OTel filter
processor for filtering decoded log bodies and attributes. It runs after the
receiver and therefore does not reduce OPC UA traffic. See the
[filtering example](../../README.md#log-filtering) and
[subscription limitations](../../PART26_REVIEW.md).

## Data Mapping

### Severity Mapping

The receiver maps OPC UA Part 26 §5.4 severity values to OpenTelemetry SeverityNumbers and derives severity text (OPC UA does not transmit it over the wire):

| OPC UA Range | OPC UA Level | Severity Text | OTel SeverityNumber |
|---|---|---|---|
| 1–50 | Debug | Debug | DEBUG (5) |
| 51–100 | Information | Information | INFO (9) |
| 101–150 | Notice | Notice | INFO4 (12) |
| 151–200 | Warning | Warning | WARN (13) |
| 201–250 | Error | Error | ERROR (17) |
| 251–300 | Critical | Critical | ERROR2 (18) |
| 301–400 | Alert | Alert | ERROR3 (19) |
| 401–1000 | Emergency | Emergency | FATAL (21) |

### Resource Attributes

| Attribute | Type | Description |
|---|---|---|
| `service.name` | string | Configured service name (default: `opcua-server`) |
| `service.namespace` | string | Configured service namespace (omitted if empty) |
| `server.address` | string | OPC UA server hostname |
| `server.port` | int | OPC UA server port number |

### Log Attributes

| Attribute | Type | Description |
|---|---|---|
| `opcua.source.name` | string | Log source component/module name |
| `opcua.source.namespace` | int | OPC UA namespace index of the source node |
| `opcua.source.id_type` | string | Node ID type (`Numeric`, `String`, `Guid`, `ByteString`) |
| `opcua.source.id` | string | Node ID value |
| Custom attributes | various | Additional fields from the OPC UA LogRecord (string, int, float, bool) |

Polling maps present trace and span IDs and marks the trace ID random, as described in Part 26 Annex C; the source does not provide a sampled flag.

## Troubleshooting

### Connection Issues

- Verify the endpoint URL starts with `opc.tcp://`
- Check network connectivity and firewall rules
- Ensure security policy and mode match the server configuration

### Authentication Failures

- For username/password: verify credentials are correct
- For certificate: ensure certificate files exist and are readable
- Check that the server accepts the configured authentication method

### No Logs Collected

- Verify the OPC UA server implements Part 26 LogObject
- Check `log_object_paths` points to valid LogObject nodes
- Ensure `min_severity` filter is not too restrictive

### Performance Issues

- Increase `collection_interval` to reduce polling frequency
- Decrease `max_records_per_call` to limit batch sizes
- Use `filter.min_severity` to limit volume

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

## Limitations

- **Beta Status**: Known interoperability and data-loss risks remain; see the [Part 26 review](../../PART26_REVIEW.md).
- **Part 26 Adoption**: Server support varies; compatibility has been tested with the included SDK 2.0.2 test server.

## Contributing

Contributions are welcome! Please see the [CONTRIBUTING.md](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/main/CONTRIBUTING.md) guide.

## License

Apache License 2.0.

## References

- [OPC UA Specification Part 26 - LogObject](https://reference.opcfoundation.org/Core/Part26/v105/docs/)
- [OpenTelemetry Collector Documentation](https://opentelemetry.io/docs/collector/)
- [gopcua - Go OPC-UA Library](https://github.com/gopcua/opcua)
- [OpenTelemetry Logs Data Model](https://opentelemetry.io/docs/specs/otel/logs/data-model/)
