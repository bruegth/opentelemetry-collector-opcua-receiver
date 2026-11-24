# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased] - planned for 0.3.0
### Added
- Decode standard OPC UA Part 26 `LogRecordsDataType` responses, including optional fields and continuation pages.
- Include the optional OpenTelemetry filter processor for filtering decoded logs.
- Build collector images and release binaries for Linux ARMv7, ARM64, PPC64LE and S390X, alongside AMD64.
- Test polling, subscription severity filtering and the filter processor against the C# OPC UA server.
- Document possible future OPC UA metrics; the receiver still emits logs only.

### Changed
- Declare the logs receiver beta; known Part 26 gaps are documented in the review notes.
- Update Go to 1.26 and OpenTelemetry Collector Builder and core components to 0.166.0.
- Upgrade the C# test server to .NET 10 and OPC UA SDK 2.0.2, using a `.slnx` solution.
- Apply the configured minimum severity to subscription events; invalid or missing event severities are discarded with a warning.

### Fixed
- Correct the default ServerLog NodeID, severity thresholds and trace flags.
- Keep GetRecords parameters unchanged across continuation calls and reject mismatched security policy or mode during endpoint selection.

## [0.2.0]

### Added
- Subscription-based log collection via OPC UA `BaseLogEventType` events
  (`collection_mode: subscription`). The server pushes log records as they
  arrive instead of the collector polling on a fixed interval.
- Node-side `ConditionClassId` and `ConditionSubClassId` checks before
  subscribing; these checks do not yet implement Part 26 event filtering.
- Extended C# test server with `BaseLogEventType` event firing and
  `ConditionClassId`/`ConditionSubClassId` properties for local testing.

## [0.1.0] - 2026-02-20

### Added
- E2E Test with C# TestServer

### Fixed
- LogRecord mapping
- Remove usage of 'telemetry.sdk'

### Changed
- 'host.ip' attribute used for endpoint
- 'opcua.source' attributes usage changed

## [0.0.1] - 2026-02-17

### Added
- Initial implementation of OPC UA log receiver
- Support for multiple authentication methods (anonymous, username/password, certificate)
- Multiple security policies and modes support
- Configurable log collection intervals
- OPC UA to OpenTelemetry log transformation
- Trace context propagation
- Comprehensive unit tests
- GitHub Actions CI/CD workflows
- Docker support with multi-architecture builds
- Complete documentation and examples
- Initial alpha release
- Basic OPC UA Part 26 LogObject support
- OpenTelemetry Collector integration
- Configuration validation
- Test infrastructure

### Security
- TLS/SSL support for secure connections
- Certificate-based authentication
- Security policy enforcement

[Unreleased]: https://github.com/bruegth/opentelemetry-collector-opcua-receiver/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/bruegth/opentelemetry-collector-opcua-receiver/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/bruegth/opentelemetry-collector-opcua-receiver/compare/v0.0.1...v0.1.0
[0.0.1]: https://github.com/bruegth/opentelemetry-collector-opcua-receiver/releases/tag/v0.0.1
