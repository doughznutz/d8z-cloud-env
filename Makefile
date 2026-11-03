# Development Environment Containers
# Traefik is our entry point into the system.
SERVICES += traefik dozzle
traefik_RUN_DEPS := dozzle

# Ollama Gatewai (openai-API) proxy to all them LLMS
# Otel Collector is required for GeminiCLI logging
SERVICES += ollama-gatewai otel-collector

# Deightz - streamlined code-server (no VNC desktop), built straight from
# lscr.io/linuxserver/code-server.
SERVICES += deightz
deightz_RUN_DEPS := traefik ollama-gatewai otel-collector

include deploy/cloud/cloud.mk
include deploy/github/github.mk
include deploy/docker/dockerhub.mk

include dood/services.mk
