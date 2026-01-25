# Development Environment Containers
# Dashboard is Our entry point into the system.
# To be upgrated to streamlit with actions.
SERVICES += dashboard dozzle
dashboard_RUN_DEPS := dozzle

# VNC container has emacs, defines the USER/PASSWORD
# Ollama Gatewai (openai-API) proxy to all them LLMS
# Otel Collector is required for GeminiCLI logging
SERVICES += vnc ollama-gatewai otel-collector
vnc_RUN_DEPS := dashboard ollama-gatewai otel-collector

# VS codeserver (web) using Continue plug-in Agent.
SERVICES += codeserver
codeserver_BUILD_DEPS := vnc
codeserver_RUN_DEPS := $(vnc_RUN_DEPS)

# Both vscode (vnc) and code-server (web) using Continue plug-in Agent.
SERVICES += vscode
vscode_BUILD_DEPS := vnc
vscode_RUN_DEPS := $(vnc_RUN_DEPS)

include deploy/cloud/cloud.mk
include deploy/github/github.mk
include deploy/docker/dockerhub.mk

include env/services.mk
