FROM alpine:3.19 AS certs
RUN apk --update add ca-certificates

FROM golang:1.25.1 AS build-stage
WORKDIR /build

# Copy go workspace files
COPY go.work go.work

# Copy receiver source code
COPY receiver/ receiver/

# Copy builder configuration
COPY builder-config.yaml builder-config.yaml

# Install OCB, generate collector source, add to workspace, then build
RUN --mount=type=cache,target=/root/.cache/go-build GO111MODULE=on go install go.opentelemetry.io/collector/cmd/builder@v0.145.0
RUN --mount=type=cache,target=/root/.cache/go-build builder --config builder-config.yaml --skip-compilation \
    && go work use ./otelcol-dev \
    && cd otelcol-dev && go build -trimpath -o otelcol-dev -ldflags="-s -w"

FROM gcr.io/distroless/base:latest

ARG USER_UID=10001
USER ${USER_UID}

COPY ./config.yaml /otelcol/collector-config.yaml
COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --chmod=755 --from=build-stage /build/otelcol-dev/otelcol-dev /otelcol/otelcol-dev

ENTRYPOINT ["/otelcol/otelcol-dev"]
CMD ["--config", "/otelcol/collector-config.yaml"]

EXPOSE 4317 4318 12001