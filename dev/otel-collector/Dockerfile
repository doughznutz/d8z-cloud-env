# Use the official Ubuntu image as a base
FROM ubuntu:latest

# Update and install necessary packages
# wget: for downloading files
# gettext-base: for envsubst
# ca-certificates: for HTTPS support
RUN apt-get update && apt-get install -y --no-install-recommends \
    wget \
    gettext-base \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Download and install the OpenTelemetry Collector Contrib distribution
ARG OTEL_COLLECTOR_VERSION=0.90.1
RUN wget https://github.com/open-telemetry/opentelemetry-collector-releases/releases/download/v${OTEL_COLLECTOR_VERSION}/otelcol-contrib_${OTEL_COLLECTOR_VERSION}_linux_amd64.tar.gz && \
    tar -xzf otelcol-contrib_${OTEL_COLLECTOR_VERSION}_linux_amd64.tar.gz && \
    mv otelcol-contrib /opt/otelcol-contrib && \
    rm otelcol-contrib_${OTEL_COLLECTOR_VERSION}_linux_amd64.tar.gz

# Copy the configuration template and entrypoint script
COPY config.yaml.template /etc/otel-collector-config.yaml.template
COPY entrypoint.sh /usr/local/bin/entrypoint.sh

# Make the entrypoint script executable
RUN chmod +x /usr/local/bin/entrypoint.sh

# Create a directory for logs
RUN mkdir -p /var/log/otel-collector

# Expose the default OpenTelemetry ports
# 4317: OTLP gRPC receiver
# 4318: OTLP HTTP receiver
# 55679: zPages extension
EXPOSE 4317 4318 55679

# Set the entrypoint script as the command to run when the container starts
CMD ["/usr/local/bin/entrypoint.sh"]
