#!/bin/bash

# This script is the entrypoint for the Docker container.

# Generate a timestamp in the format YYYYMMDD_HHMMSS
LOG_FILE_DATETIME=$(date +%Y%m%d_%H%M%S)

# Export the timestamp as an environment variable so it can be used by envsubst
export LOG_FILE_DATETIME

# Use envsubst to substitute environment variables in the template file
# and create the final configuration file.
envsubst < /etc/otel-collector-config.yaml.template > /etc/otel-collector-config.yaml

# Start the OpenTelemetry Collector with the generated configuration file.
/opt/otelcol-contrib --config=/etc/otel-collector-config.yaml