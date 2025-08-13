FROM ubuntu:latest

# Update and install necessary packages
RUN apt-get update && apt-get install -y --no-install-recommends \
    wget \
    gettext-base \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Download and install the OpenTelemetry Collector
ARG OTEL_COLLECTOR_VERSION=0.90.1
RUN wget https://github.com/open-telemetry/opentelemetry-collector-releases/releases/download/v${OTEL_COLLECTOR_VERSION}/otelcol-contrib_${OTEL_COLLECTOR_VERSION}_linux_amd64.tar.gz && \
    tar -xzf otelcol-contrib_${OTEL_COLLECTOR_VERSION}_linux_amd64.tar.gz && \
    mv otelcol-contrib /opt/otelcol-contrib && \
    rm otelcol-contrib_${OTEL_COLLECTOR_VERSION}_linux_amd64.tar.gz

# Copy your configuration files and scripts
COPY config.yaml.template /etc/otel-collector-config.yaml.template
COPY entrypoint.sh /usr/local/bin/entrypoint.sh

# Make the script executable
RUN chmod +x /usr/local/bin/entrypoint.sh

# Create log directory (optional)
RUN mkdir -p /var/log/otel-collector

# Expose ports (adjust as needed based on your config)
EXPOSE 4317 4318 55679

# Set the command to run the script and then start the collector
CMD ["/usr/local/bin/entrypoint.sh"]
