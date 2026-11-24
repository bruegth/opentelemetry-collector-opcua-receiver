# Project guidance

## Structure

- `receiver/opcua` is the Go module for a beta OpenTelemetry logs receiver for OPC UA Part 26. `go.work` includes this module.
- `factory.go` selects polling (`receiver.go`, `scraper.go`) or events (`subscription_receiver.go`). `client.go` and `get_records.go` implement OPC UA access and continuation-point pagination; `log_records_data_type.go` decodes standard records and `transformer.go` maps them to OpenTelemetry logs.
- `receiver/opcua/testdata` contains mocks and record types also imported by production code. The C# server and expected records in `testserver` support Docker end-to-end tests.

## Build and test

- Run Go commands from `receiver/opcua`: `go test ./...`, `go vet ./...`, and `go build ./...`. Use focused tests when appropriate; CI also runs tests with the race detector and coverage.
- `builder-config.yaml` defines the generated collector in `otelcol-dev`; the local receiver uses a path reference. CI builds it with `GOWORK=off` using the OCB version pinned in the workflows.
- `Dockerfile` builds the generated collector in a workspace; `testserver/ci-collector-config.yaml` and `testserver/validate.sh` define the end-to-end checks. The local E2E script uses fixed Docker resource names and removes them, so check for shared resources before running it.

## Changes

- Keep OCB, Collector distribution components, Collector Go modules, Go version requirements, Docker, CI, and contributor prerequisites compatible when updating dependencies. Collector stable and pre-stable modules use different version numbers from the same release; contrib releases can lag core releases, so verify mixed-version builds and behavior.
- Preserve both collection modes and their shutdown, reconnect, pagination, and data-mapping behavior. Add focused tests for changed behavior, using the existing mocks and table-driven tests where suitable.
- Keep configuration defaults, validation, `config.schema.yaml`, `metadata.yaml`, and the receiver documentation consistent when changing configuration. Do not commit generated collector output or test artifacts.
