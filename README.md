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

### Log filtering

`filter.min_severity` applies an inclusive OPC UA severity threshold in both
collection modes. Polling sends the threshold to `GetRecords`; subscription
events are filtered by the receiver after delivery. For example, `Warning` keeps
severity 151 and above. Subscription events with missing or invalid severities
are discarded with a warning.

The distribution also includes the optional [OpenTelemetry filter processor](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/v0.165.1/processor/filterprocessor)
(alpha for logs). Add it to a logs pipeline to drop records matching an OTTL
condition; it runs after reception and does not reduce OPC UA network traffic:

```yaml
processors:
  filter:
    error_mode: propagate
    log_conditions:
      - 'log.attributes["opcua.source.name"] == "NetworkModule"'

service:
  pipelines:
    logs:
      receivers: [opcua]
      processors: [filter, batch]
      exporters: [debug]
```

This snippet extends the advanced example above and drops records from
`NetworkModule`; only fields decoded by the receiver can be filtered. It does
not change the default pipeline or resolve the known subscription limitations.

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
│   ├── filter-collector-config.yaml # Filtering integration configuration
│   ├── validate-filter.py   # Filtering output validation
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
- [x] Minimum severity in both modes and opt-in OTel log filtering
- [ ] Performance metrics
- [x] Beta stability

### Potential OPC UA metrics

**Research only:** this receiver currently emits logs, not metrics. None of the
metric candidates below, companion-model discovery, or historical reads is
implemented. The OPC UA specifications define information models; the mapping
ideas below are not a claimed OPC UA-to-OpenTelemetry standard or a conformance
claim. Availability depends on each server's model version and optional facets.

| Model | Potential observations and considerations |
| --- | --- |
| [Part 16: State Machines](https://reference.opcfoundation.org/specs/OPC-10000-16/) | Current state per machine; a bounded 0/1 series per known state is a possible custom metric. Polling can miss transitions. |
| [Part 10: Programs](https://reference.opcfoundation.org/specs/OPC-10000-10/) | Program state and, where complete lifecycle events are available, execution duration and outcome counts. Program completion is not implied by a successful control call. |
| [Part 9: Alarms and Conditions](https://reference.opcfoundation.org/specs/OPC-10000-9/) | Active or unacknowledged condition counts and condition events. Retained conditions, branches and `ConditionRefresh` must be handled; an event count is not the current active count. |
| [Part 11: Historical Access](https://reference.opcfoundation.org/specs/OPC-10000-11/), [Part 13: Aggregates](https://reference.opcfoundation.org/specs/OPC-10000-13/) | Historical values and server-calculated aggregates can support backfill or trends. Preserve quality, time windows and aggregate semantics; an average is not a histogram. |
| [Part 17: Alias Names](https://reference.opcfoundation.org/specs/OPC-10000-17/) | Potential discovery/identity aid rather than a metric by itself; aliases can refer to the same source. |
| [Part 18: Role-Based Security](https://reference.opcfoundation.org/specs/OPC-10000-18/) | Authorization and audit context, not a direct production metric. Avoid exporting identities or permissions as metric labels. |
| [Part 20: File Transfer](https://reference.opcfoundation.org/specs/OPC-10000-20/) | Potential transfer size, duration and errors when actually observed. Do not download files solely to generate metrics. |
| [Part 100: Device Integration](https://reference.opcfoundation.org/specs/OPC-10000-100/) | Device health, lifetime and identification are useful discovery and resource context; health codes require an explicit model-specific mapping. |
| [Part 110: Asset Management Basics](https://reference.opcfoundation.org/specs/OPC-10000-110/) | Asset health and maintenance state/duration, when provided. Asset identity and access to maintenance details need explicit privacy policy. |
| [Part 200: Industrial Automation](https://reference.opcfoundation.org/specs/OPC-10000-200/) | Stacklight element state, provider-defined statistics and calibration information. Statistics are not automatically monotonic counters. |
| [Part 210: Relative Spatial Location](https://reference.opcfoundation.org/specs/OPC-10000-210/), [Part 211: Global Positioning](https://reference.opcfoundation.org/specs/OPC-10000-211/) | Position and orientation observations require known coordinate frames, units and timestamps. Avoid continuously changing metric labels and consider location privacy. |
| [OPC 10030](https://reference.opcfoundation.org/specs/OPC-10030/), [10031-4 Job Control V1/V2](https://reference.opcfoundation.org/specs/OPC-10031-4/) | Equipment context and job states; duration or completed-job counts require reliable lifecycle observation. V1 and V2 have distinct models/namespaces. |
| [OPC 30270: Asset Administration Shell](https://reference.opcfoundation.org/specs/OPC-30270/) | Semantic IDs and submodels can describe assets and measurements. AAS V2/V3 metadata and model mappings must be checked separately; they are not themselves metric values. |
| [OPC 10100-1: WoT Connectivity](https://reference.opcfoundation.org/specs/OPC-10100-1/) | Discovery and metadata for observed properties; only export measurements after validating their type, unit and source. |
| [OPC 40001-1: Machinery](https://reference.opcfoundation.org/specs/OPC-40001-1/), [-2: Process Values](https://reference.opcfoundation.org/specs/OPC-40001-2/), [-3: Jobs](https://reference.opcfoundation.org/specs/OPC-40001-3/) | Machine state, process readings and job lifecycle; interpret quantities and units per model and avoid per-job metric labels. |
| [OPC 40001-4: Energy](https://reference.opcfoundation.org/specs/OPC-40001-4/), [OPC 34100: Energy Consumption Management](https://reference.opcfoundation.org/specs/OPC-34100/) | Measured consumption and power are promising where their source, units and reset behavior are known. Machinery Energy builds on ECM; avoid double-counting the same measurement. |
| [OPC 40001-101: Result Transfer](https://reference.opcfoundation.org/specs/OPC-40001-101/) | Result outcomes or transfer observations, when the server actually supplies them. Avoid result IDs and payloads as metric labels. |
| [OPC 30081: PA-DIM](https://reference.opcfoundation.org/specs/OPC-30081/) | Process measurements such as temperature, pressure and flow, with their engineering units and data quality. A process temperature is not automatically a hardware temperature. |
| [OPC 40010-1: Robotics](https://reference.opcfoundation.org/specs/OPC-40010-1/) | Motion-device, axis and controller observations; preserve units and distinguish measured values from commands or safety functions. |
| [OPC 40100-1/-2: Machine Vision](https://github.com/OPCFoundation/UA-.NETStandard/blob/master/docs/Vision.md) | System state, inspection outcomes and asset health, subject to available model facets. Avoid images and result IDs as metric labels. |
| [OPC 40501-1: Machine Tools and OPC 40505: Wireless Peripherals](https://github.com/OPCFoundation/UA-.NETStandard/blob/master/docs/MachineTool.md) | Machine/tool state and peripheral measurements. Wireless peripherals can exist independently of a machine-tool instance. |
| [OPC 40210: Geometric Measurement Systems](https://github.com/OPCFoundation/UA-.NETStandard/blob/master/docs/GMS.md) | Measurement status, characteristics and tolerances. The SDK documents an undefined mandatory Production conformance unit, so it withholds the complete Production facet; this does not mean all production observations are absent. |
| [OPC 40020-1: Cranes and Hoists](https://github.com/OPCFoundation/UA-.NETStandard/blob/master/docs/CranesHoists.md) | Load, motion-device and equipment-health observations, with correct units and quality. Monitoring must not be treated as a safety controller. |
| [OPC 40223: Pumps and Vacuum Pumps](https://reference.opcfoundation.org/specs/OPC-40223/) | Available measurements, supervision and maintenance states; respect the source's quantity, units and reset semantics. |
| [OPC 30010: AutoID](https://github.com/OPCFoundation/UA-.NETStandard/blob/master/docs/AutoID.md) | Reader health and, if observed reliably, scan/error counts. Do not put RFID tags or scanned values in metric labels. |
| [OPC 30500-1: LADS](https://github.com/OPCFoundation/UA-.NETStandard/blob/master/docs/LADS.md) | Functional-unit state, measured values and program outcomes. Sample identifiers and result contents require separate privacy and cardinality decisions. |
| [OPC 40200: Weighing Technology](https://reference.opcfoundation.org/specs/OPC-40200/), [OPC 30050: PackML](https://github.com/OPCFoundation/UA-.NETStandard/blob/master/docs/PackML.md) | Weight, scale status and PackML unit/machine state where available; PackML can also be hosted independently. Account for tare, units and nested state machines. |

#### OpenTelemetry alignment and implementation priorities

1. **State and measurements:** A first opt-in implementation could read explicit
   Part 16 finite-state-machine identifiers and bounded state sets, followed by
   selected process measurements. The `CurrentState.Id` of a finite state
   machine is a NodeId; display text is not a stable identifier. Preserve
   namespace URIs rather than assuming namespace indexes are stable, reject bad
   or inactive values, and account for missed transitions and stale series.
2. **Existing conventions where applicable:** OTel hardware conventions are
   currently marked Development. `hw.temperature` is a gauge in `Cel` and
   requires `hw.id` for an identified hardware sensor. `hw.power` is a gauge in
   `W`, while `hw.energy` is a counter in `J`; use these only for matching
   hardware measurements, not every process temperature or plant energy total.
   `hw.status` is an UpDownCounter with `hw.id`, `hw.type` and `hw.state`;
   map to it only when the observed hardware health has matching semantics,
   not for arbitrary machine or program states.
   Convert units only when their meaning is known (for example, 1 kWh =
   3,600,000 J). See [temperature](https://opentelemetry.io/docs/specs/semconv/hardware/temperature/)
   and [common hardware metrics](https://opentelemetry.io/docs/specs/semconv/hardware/common/).
3. **Events and history later:** Alarm and job counters need deduplication,
   recovery and complete lifecycle semantics. History backfill needs explicit
   timestamp, quality and duplicate handling; do not manufacture a cumulative
   counter or histogram from an unrelated aggregate.

General requirements for any future work: add an explicit metrics pipeline;
keep collection opt-in and read-only; bound discovery, subscriptions and label
cardinality; retain source timestamps and OPC UA status/quality; distinguish
missing or bad values from zero; handle counter resets; and protect asset IDs,
locations and result data. Use appropriate [OTel metric units and instrument
types](https://opentelemetry.io/docs/specs/semconv/general/metrics/) and do not
assign `host.*` or `device.*` resource attributes to remote assets without
matching their defined scope.

The current .NET 10 test server uses OPC Foundation SDK 2.0.2 and publishes a
Part 26 LogObject, not these companion models. Its installed SDK contains core
finite-state-machine types, which could seed a future integration test. The
linked SDK guides describe the upstream `master` branch; package availability,
model versions and preview status must be verified separately before using them
with SDK 2.0.2. A future test should validate real server values, bad status,
state changes, restart/reconnect behavior and the resulting OTel data without
changing the existing log pipeline.

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
