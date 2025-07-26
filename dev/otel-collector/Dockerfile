# syntax=docker/dockerfile:1

# ==============================================================================
# Builder Stage
#
# This stage builds the custom OpenTelemetry Collector with the pgexporter,
# which is not included in the default collector image.
# ==============================================================================
FROM golang:1.22 as builder  
# Use a golang image as the builder
# It is a good practice to pin the image to a specific digest
# e.g., FROM otel/opentelemetry-collector:0.90.1@sha256:....

WORKDIR /app

# Download and install the postgresql exporter
# RUN go install github.com/open-telemetry/opentelemetry-collector-contrib/exporter/pgexporter@v0.90.1

# ==============================================================================
# Final Stage
#
# This stage creates the final, lean image by copying the built exporter
# and the configuration file into the official collector image.
# ==============================================================================
FROM otel/opentelemetry-collector:0.90.1
# It is a good practice to pin the image to a specific digest
# e.g., FROM otel/opentelemetry-collector:0.90.1@sha256:....

# Copy the custom exporter from the builder stage and the local config file
# COPY --from=builder /go/bin/pgexporter /usr/bin/
COPY config.yaml /etc/otel-collector-config.yaml