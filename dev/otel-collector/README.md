# OpenTelemetry Collector Configuration

This directory contains the configuration for running a custom OpenTelemetry Collector using Docker.

## Files

-   `Dockerfile`: This file defines the Docker image for the OpenTelemetry Collector. It installs the collector and sets up the necessary configuration.
-   `config.yaml.template`: This is a template for the OpenTelemetry Collector configuration. It is processed by the `entrypoint.sh` script to create the final configuration file.
-   `entrypoint.sh`: This script is the entrypoint for the Docker container. It generates a timestamped log file name and then starts the OpenTelemetry Collector.

## Usage

To build the Docker image, run the following command from this directory:

```bash
docker build -t otel-collector .
```

To run the Docker container, use the following command:

```bash
docker run -p 4317:4317 -p 4318:4318 -p 55679:55679 otel-collector
```
