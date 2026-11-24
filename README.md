# OpenTelemetry Collector OPC UA Receiver

[![Go Reference](https://pkg.go.dev/badge/github.com/bruegth/opentelemetry-collector-opcua-receiver/receiver/opcua.svg)](https://pkg.go.dev/github.com/bruegth/opentelemetry-collector-opcua-receiver/receiver/opcua)
[![Go version](https://img.shields.io/github/go-mod/go-version/bruegth/opentelemetry-collector-opcua-receiver?filename=%2Freceiver%2Fopcua%2Fgo.mod)](https://go.dev/)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)
[![Stability: Beta](https://img.shields.io/badge/stability-beta-yellow.svg)](CONTRIBUTING.md)
[![CI](https://github.com/bruegth/opentelemetry-collector-opcua-receiver/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/bruegth/opentelemetry-collector-opcua-receiver/actions/workflows/ci.yml)

An OpenTelemetry Collector receiver for collecting logs from OPC UA servers implementing the LogObject specification (OPC UA Part 26).

## Overview

This project provides a custom receiver for the OpenTelemetry Collector that enables collection of log records from OPC UA industrial automation servers. It bridges the gap between industrial OPC UA systems and modern observability platforms.

**Features**: polling and subscription collection, configurable security policies, severity filtering, trace context propagation, and ContinuationPoint pagination.

### Status

**Beta**: The receiver is under active development and has known Part 26 limitations; see the [review notes](PART26_REVIEW.md) before production use.

## Quick Start

### Prerequisites

- Go 1.26.0 or later
- OpenTelemetry Collector Builder (OCB) v0.166.0
- .NET 10 SDK (for building the test server)
- OPC UA server (optional for development/testing)

### Installation

#### Option 1: Build Custom Collector

```bash
# Clone the repository
git clone https://github.com/bruegth/opentelemetry-collector-opcua-receiver.git
cd opentelemetry-collector-opcua-receiver

# Install OpenTelemetry Collector Builder
go install go.opentelemetry.io/collector/cmd/builder@v0.166.0
```

Ensure the Go binary directory (`go env GOBIN`, or `bin` under `go env GOPATH`
when `GOBIN` is empty) is on your `PATH`. Build and run from the repository root:

```bash
# Linux/macOS
# Build the collector
GOWORK=off builder --config builder-config.yaml

# Run the collector
./otelcol-dev/otelcol-dev --config config.yaml
```

```powershell
# Windows PowerShell
cmd /c "set GOWORK=off&& builder --config builder-config.yaml"
.\otelcol-dev\otelcol-dev.exe --config .\config.yaml
```

#### Option 2: Use as Go Module

Add to your collector builder configuration or import directly:

```bash
# Run from your own Go module, not this repository's root
go get github.com/bruegth/opentelemetry-collector-opcua-receiver/receiver/opcua@latest
```

```go
// Import in your code
import "github.com/bruegth/opentelemetry-collector-opcua-receiver/receiver/opcua"
```

View the package documentation on [pkg.go.dev](https://pkg.go.dev/github.com/bruegth/opentelemetry-collector-opcua-receiver/receiver/opcua).

## Configuration

### Basic Example

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

### Advanced Example

Username/password and certificate user authentication, as well as OPC UA
certificate trust settings, have known limitations. See the
[Part 26 review](PART26_REVIEW.md) before relying on them.

```yaml
receivers:
  opcua:
    endpoint: opc.tcp://opcua.server.local:4840
    security_policy: None
    security_mode: None
    auth:
      type: anonymous
    log_object_paths:
      - Objects/ServerLog
      - "ns=2;s=DeviceLog"
    collection_interval: 30s
    max_records_per_call: 1000
    filter:
      min_severity: Info
    connection_timeout: 30s
    request_timeout: 10s
    resource:
      service_name: my-opcua-server
      # service_namespace: production

exporters:
  debug:
    verbosity: detailed

processors:
  batch:

service:
  pipelines:
    logs:
      receivers: [opcua]
      processors: [batch]
      exporters: [debug]
```

For complete configuration options, see [receiver/opcua/README.md](receiver/opcua/README.md).

## Development

### Project Structure

```text
.
├── receiver/opcua/          # OPC UA receiver implementation
│   ├── config.go            # Configuration structure
│   ├── factory.go           # Receiver factory
│   ├── receiver.go          # Receiver lifecycle
│   ├── scraper.go           # Log collection logic
│   ├── client.go            # OPC UA client wrapper
│   ├── get_records.go       # GetRecords method call & parsing
│   ├── log_records_data_type.go # Standard Part 26 binary decoder
│   ├── log_record_type.go   # Legacy LogRecord ExtensionObject codec
│   ├── transformer.go       # Data transformation
│   ├── *_test.go            # Unit tests
│   ├── testdata/            # Test utilities & mock server
│   ├── metadata.yaml        # Receiver metadata
│   └── README.md            # Receiver documentation
├── testserver/              # C# OPC UA test server (Part 26)
│   ├── Program.cs           # Server entry point
│   ├── TestNodeManager.cs   # GetRecords method implementation
│   ├── LogRecordData.cs     # 10 fixed test records
│   ├── Dockerfile           # Container image
│   ├── ci-collector-config.yaml  # OTel collector config for E2E tests
│   ├── validate.sh          # E2E output validation script
│   └── expected/            # Expected test data
├── builder-config.yaml      # Collector builder configuration
├── config.yaml              # Runtime configuration
├── go.work                  # Go workspace file
├── opentelemetry-collector-opcua-receiver.slnx # Test server solution
└── README.md                # This file
```

### Setting Up Development Environment

1. **Install Go 1.26.0+**
   ```bash
   # Verify installation
   go version
   ```

2. **Clone the repository**
   ```bash
   git clone https://github.com/bruegth/opentelemetry-collector-opcua-receiver.git
   cd opentelemetry-collector-opcua-receiver
   ```

3. **Download dependencies**
   ```bash
   # The go.work file is already configured
   # Install dependencies
   cd receiver/opcua
   go mod download
   cd ../..
   ```

4. **Install OpenTelemetry Collector Builder** as described in [Quick Start](#quick-start).

### Building

#### Build the OPC UA Test Server

The .NET 10 test server uses OPC Foundation SDK 2.0.2 and returns the standard
Part 26 `LogRecordsDataType`. Responses contain at most three records per page
to exercise continuation-point handling.

```bash
dotnet build opentelemetry-collector-opcua-receiver.slnx
```

#### Build the Receiver Module Only

```bash
cd receiver/opcua
go build
```

#### Build the Complete Collector

Follow the platform-specific OCB commands in [Quick Start](#quick-start).
The compiled collector will be in `otelcol-dev/otelcol-dev` (or `.exe` on Windows).

### Testing

```bash
cd receiver/opcua

# Run all tests
go test ./...

# Run with coverage
go test -cover ./...

# Generate HTML coverage report
go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out -o coverage.html
```

### Code Quality

```bash
cd receiver/opcua
go fmt ./...
go vet ./...
golangci-lint run
```

## CI/CD

This project uses GitHub Actions for continuous integration and delivery:

- **Build & Test**: Runs on pushes to `main` and `initial`, and on pull requests targeting `main`
- **Linting**: Code quality checks
- **Coverage**: Test coverage reporting
- **E2E Test**: Runs C# OPC UA test server + OTel collector in Docker, validates file export output
- **Release**: Automated releases on tags

See [.github/workflows/](.github/workflows/) for workflow definitions.

## Docker

### Build Docker Image Locally

```bash
docker build -t otelcol-opcua:latest .
```

### Run in Docker

```bash
docker run -p 4317:4317 \
  -v "$(pwd)/config.yaml:/otelcol/collector-config.yaml:ro" \
  otelcol-opcua:latest
```

Adjust `config.yaml` before running: `localhost` inside the container refers to
the container itself, and the example Jaeger exporter needs a reachable backend.
The mounted configuration must be readable by the container's non-root user.
The sample configuration enables OTLP/gRPC on port 4317, not OTLP/HTTP on 4318.

### Multi-Architecture Build

The collector can be cross-compiled for `linux/amd64`, `linux/arm/v7`, `linux/arm64`,
`linux/ppc64le`, and `linux/s390x`. The release workflow publishes binaries
for these targets; it does not publish container images. Runtime testing on
each target architecture is still recommended.

The OCI archive build requires a Buildx builder with OCI export support, such
as the `docker-container` driver or a containerd image store.

```bash
# Build a local multi-platform OCI archive
docker buildx build --platform linux/amd64,linux/arm/v7,linux/arm64,linux/ppc64le,linux/s390x \
  --output type=oci,dest=otelcol-opcua.oci.tar -t otelcol-opcua:latest .
```

Or use the PowerShell build script on Windows to load and run the image. Loading
multiple platforms requires a Docker containerd image store:
```powershell
.\build.ps1
```

## Contributing

Contributions are welcome! Fork the repository, create a feature branch, write tests, and open a pull request. Follow standard Go conventions (`gofmt`, `go vet`, meaningful commit messages).

## Troubleshooting

- **`builder` not found**: Install OCB as shown in [Quick Start](#quick-start) and ensure the Go binary directory is on `PATH`.
- **Module errors**: Run `cd receiver/opcua && go mod tidy`.
- **Connection refused**: Verify endpoint URL, network connectivity, and firewall rules.
- **Authentication failures**: Check credentials, certificate paths, and server auth requirements.

For more detail, see [receiver/opcua/README.md](receiver/opcua/README.md#troubleshooting).

## Documentation

- **Receiver Documentation**: [receiver/opcua/README.md](receiver/opcua/README.md)
- **Configuration Reference**: [receiver/opcua/README.md#configuration](receiver/opcua/README.md#configuration)
- **OPC UA Specification Part 26**: [OPC Foundation](https://reference.opcfoundation.org/Core/Part26/v105/docs/)
- **OpenTelemetry Collector**: [Official Docs](https://opentelemetry.io/docs/collector/)
- **gopcua Library**: [GitHub](https://github.com/gopcua/opcua)

## Roadmap

- [x] OPC UA Part 26 GetRecords method call with ExtensionObject decoding
- [x] ContinuationPoint pagination for large log sets
- [x] E2E integration testing with C# OPC UA test server
- [x] Subscription-based log collection (in addition to polling)
- [ ] Enhanced filtering capabilities
- [ ] Performance metrics
- [x] Beta stability

## License

This project is licensed under the Apache License 2.0 - see the [LICENSE](LICENSE) file for details.

## Acknowledgments

- [OpenTelemetry Community](https://opentelemetry.io/)
- [OPC Foundation](https://opcfoundation.org/)
- [gopcua Contributors](https://github.com/gopcua/opcua)

## Support

- **Issues**: [GitHub Issues](https://github.com/bruegth/opentelemetry-collector-opcua-receiver/issues)
- **Discussions**: [GitHub Discussions](https://github.com/bruegth/opentelemetry-collector-opcua-receiver/discussions)
- **OpenTelemetry Slack**: [#otel-collector](https://cloud-native.slack.com/archives/C01N6P7KR6W)

## Author

**bruegth** - [GitHub Profile](https://github.com/bruegth)

---

**Note**: This is an independent project and is not officially endorsed by the OpenTelemetry project or OPC Foundation.
