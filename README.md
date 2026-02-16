# OpenTelemetry Collector OPC UA Receiver

[![Go Reference](https://pkg.go.dev/badge/github.com/bruegth/opentelemetry-collector-opcua-receiver/receiver/opcua.svg)](https://pkg.go.dev/github.com/bruegth/opentelemetry-collector-opcua-receiver/receiver/opcua)
[![Go Version](https://img.shields.io/github/go-mod/go-version/bruegth/opentelemetry-collector-opcua-receiver)](https://go.dev/)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)
[![Stability: Alpha](https://img.shields.io/badge/stability-alpha-orange.svg)](CONTRIBUTING.md)
[![Go Report Card](https://goreportcard.com/badge/github.com/bruegth/opentelemetry-collector-opcua-receiver)](https://goreportcard.com/report/github.com/bruegth/opentelemetry-collector-opcua-receiver)

An OpenTelemetry Collector receiver for collecting logs from OPC UA servers implementing the LogObject specification (OPC UA Part 26).

## Overview

This project provides a custom receiver for the OpenTelemetry Collector that enables collection of log records from OPC UA industrial automation servers. It bridges the gap between industrial OPC UA systems and modern observability platforms.

### Features

- ✅ **Multiple Authentication Methods**: Anonymous, username/password, X509 certificates
- ✅ **Security Support**: Multiple security policies (None, Basic256, Basic256Sha256, etc.)
- ✅ **Flexible Configuration**: Customizable collection intervals, filtering, and log object paths
- ✅ **Data Transformation**: Automatic conversion from OPC UA log format to OpenTelemetry
- ✅ **Trace Context Propagation**: Preserves distributed tracing context
- ✅ **Comprehensive Testing**: Unit and integration tests with high coverage

### Status

**⚠️ Alpha**: This receiver is in alpha stage. The API may change and the implementation is being actively developed.

## Quick Start

### Prerequisites

- Go 1.25.1 or later
- OpenTelemetry Collector Builder (OCB) v0.145.0 or later
- OPC UA server (optional for development/testing)

### Installation

#### Option 1: Build Custom Collector

```bash
# Clone the repository
git clone https://github.com/bruegth/opentelemetry-collector-opcua-receiver.git
cd opentelemetry-collector-opcua-receiver

# Download OpenTelemetry Collector Builder
# Windows: Download ocb.exe from https://github.com/open-telemetry/opentelemetry-collector/releases
# Linux/Mac: Download appropriate binary

# Build the collector
./ocb.exe --config builder-config.yaml

# Run the collector
cd otelcol-dev
./otelcol-dev --config ../config.yaml
```

#### Option 2: Use as Go Module

Add to your collector builder configuration or import directly:

```bash
# Install the module
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

```yaml
receivers:
  opcua:
    endpoint: opc.tcp://opcua.server.local:4840
    security_policy: Basic256Sha256
    security_mode: SignAndEncrypt
    auth:
      type: username_password
      username: opcua_user
      password: ${env:OPCUA_PASSWORD}
    log_object_paths:
      - Objects/ServerLog
      - Objects/DeviceSets/Device1/Logs
    collection_interval: 30s
    max_records_per_call: 1000
    filter:
      min_severity: Info
      max_log_records: 10000
    connection_timeout: 30s
    request_timeout: 10s

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

For complete configuration options, see [receiver/opcua/README.md](receiver/opcua/README.md).

## Development

### Project Structure

```
.
├── receiver/opcua/          # OPC UA receiver implementation
│   ├── config.go            # Configuration structure
│   ├── factory.go           # Receiver factory
│   ├── receiver.go          # Receiver lifecycle
│   ├── scraper.go           # Log collection logic
│   ├── client.go            # OPC UA client wrapper
│   ├── transformer.go       # Data transformation
│   ├── *_test.go            # Unit tests
│   ├── testserver/          # Test utilities
│   ├── metadata.yaml        # Receiver metadata
│   └── README.md            # Receiver documentation
├── builder-config.yaml      # Collector builder configuration
├── config.yaml              # Runtime configuration
├── go.work                  # Go workspace file
└── README.md                # This file
```

### Setting Up Development Environment

1. **Install Go 1.25.1+**
   ```bash
   # Verify installation
   go version
   ```

2. **Clone the repository**
   ```bash
   git clone https://github.com/bruegth/opentelemetry-collector-opcua-receiver.git
   cd opentelemetry-collector-opcua-receiver
   ```

3. **Initialize the workspace**
   ```bash
   # The go.work file is already configured
   # Install dependencies
   cd receiver/opcua
   go mod tidy
   cd ../..
   ```

4. **Download OpenTelemetry Collector Builder**
   - Windows: Download `ocb.exe` from [releases](https://github.com/open-telemetry/opentelemetry-collector/releases)
   - Linux/Mac: Download appropriate binary
   - Place in project root

### Building

#### Build the Receiver Module Only

```bash
cd receiver/opcua
go build
```

#### Build the Complete Collector

```bash
# Build using OCB
./ocb.exe --config builder-config.yaml

# Or use PowerShell script (if available)
.\build.ps1
```

The compiled collector will be in `otelcol-dev/otelcol-dev` (or `.exe` on Windows).

### Testing

#### Run Unit Tests

```bash
cd receiver/opcua
go test ./...
```

#### Run Tests with Coverage

```bash
cd receiver/opcua
go test -cover ./...
```

#### Run Tests with Verbose Output

```bash
cd receiver/opcua
go test -v ./...
```

#### Run Specific Test

```bash
cd receiver/opcua
go test -run TestConfigValidate -v
```

#### Generate Coverage Report

```bash
cd receiver/opcua
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html
```

### Local Testing with OPC UA Server

#### Using Docker (if you have an OPC UA server image)

```bash
# Start OPC UA server
docker run -d --name opcua-server -p 4840:4840 your-opcua-server-image

# Run the collector
cd otelcol-dev
./otelcol-dev --config ../config.yaml
```

#### Using Jaeger for Log Visualization

```powershell
# Start Jaeger (using existing script)
.\jaeger.ps1

# Update config.yaml to export to Jaeger if needed
```

### Code Quality

#### Run Linter

```bash
# Install golangci-lint
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Run linter
cd receiver/opcua
golangci-lint run
```

#### Format Code

```bash
cd receiver/opcua
go fmt ./...
```

#### Run Go Vet

```bash
cd receiver/opcua
go vet ./...
```

## CI/CD

This project uses GitHub Actions for continuous integration and delivery:

- **Build & Test**: Runs on every push and pull request
- **Linting**: Code quality checks
- **Coverage**: Test coverage reporting
- **Release**: Automated releases on tags

See [.github/workflows/](.github/workflows/) for workflow definitions.

## Docker

### Build Docker Image Locally

```bash
docker build -t otelcol-opcua:latest .
```

### Run in Docker

```bash
docker run -p 4317:4317 -p 4318:4318 \
  -v $(pwd)/config.yaml:/etc/otelcol/config.yaml \
  otelcol-opcua:latest
```

### Multi-Architecture Build

```bash
# Build for multiple platforms
docker buildx build --platform linux/amd64,linux/arm64 -t otelcol-opcua:latest .
```

Or use the PowerShell build script on Windows:
```powershell
.\build.ps1
```

## Contributing

Contributions are welcome! Please follow these guidelines:

1. **Fork the repository**
2. **Create a feature branch** (`git checkout -b feature/amazing-feature`)
3. **Write tests** for your changes
4. **Ensure tests pass** (`go test ./...`)
5. **Format your code** (`go fmt ./...`)
6. **Commit your changes** (`git commit -m 'Add amazing feature'`)
7. **Push to the branch** (`git push origin feature/amazing-feature`)
8. **Open a Pull Request**

### Code Style

- Follow standard Go conventions
- Use `gofmt` for formatting
- Write meaningful commit messages
- Add tests for new functionality
- Update documentation as needed

### Testing Requirements

- All new code must have unit tests
- Maintain or improve code coverage
- Integration tests for new features

## Troubleshooting

### Build Issues

**Problem**: `ocb.exe` not found

**Solution**: Download OCB from [releases](https://github.com/open-telemetry/opentelemetry-collector/releases) and place in project root.

**Problem**: Module resolution errors

**Solution**:
```bash
cd receiver/opcua
go mod tidy
go mod download
```

### Runtime Issues

**Problem**: Connection refused to OPC UA server

**Solution**: Verify endpoint URL, network connectivity, and firewall rules.

**Problem**: Authentication failures

**Solution**: Check credentials, certificate paths, and server authentication requirements.

For more troubleshooting, see [receiver/opcua/README.md](receiver/opcua/README.md#troubleshooting).

## Documentation

- **Receiver Documentation**: [receiver/opcua/README.md](receiver/opcua/README.md)
- **Configuration Reference**: [receiver/opcua/README.md#configuration](receiver/opcua/README.md#configuration)
- **OPC UA Specification Part 26**: [OPC Foundation](https://reference.opcfoundation.org/Core/Part26/v105/docs/)
- **OpenTelemetry Collector**: [Official Docs](https://opentelemetry.io/docs/collector/)
- **gopcua Library**: [GitHub](https://github.com/gopcua/opcua)

## Roadmap

- [ ] Full OPC UA Part 26 LogObject compliance
- [ ] Subscription-based log collection (in addition to polling)
- [ ] ContinuationPoint pagination for large log sets
- [ ] Enhanced filtering capabilities
- [ ] Performance metrics and monitoring
- [ ] Support for multiple concurrent LogObject nodes
- [ ] Beta stability

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
