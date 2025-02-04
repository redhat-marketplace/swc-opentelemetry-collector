# Build stage
FROM otel/opentelemetry-collector-builder:latest AS builder

COPY --from=builder /tmp/dist/otelcol-custom /otelcol-custom

RUN otelcol-builder --config /builder-config.yaml --output /otelcol-custom

# Final image
FROM registry.access.redhat.com/ubi8/ubi-minimal:latest

COPY --from=builder /otelcol-custom /otelcol-custom

ENTRYPOINT ["/otelcol-custom"]
