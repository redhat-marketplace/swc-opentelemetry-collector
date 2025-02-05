# Build stage
FROM docker.io/otel/opentelemetry-collector-builder:latest AS builder

COPY builder-config.yaml /build/builder-config.yaml

RUN ocb --config=/build/builder-config.yaml

# Final image
FROM registry.access.redhat.com/ubi9/ubi-minimal:latest

COPY --from=builder /tmp/dist/otelcol-custom /otelcol-custom

ENTRYPOINT ["/otelcol-custom"]
