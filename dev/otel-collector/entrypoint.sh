#!/bin/bash

# Generate the timestamped filename
LOG_FILE_DATETIME=$(date +%Y%m%d_%H%M%S)

# Set the environment variable
export LOG_FILE_DATETIME

# Use envsubst to replace the environment variables in the template
envsubst < /etc/otel-collector-config.yaml.template > /etc/otel-collector-config.yaml
echo Hello

# Start the otel-collector
/opt/otelcol-contrib --config=/etc/otel-collector-config.yaml