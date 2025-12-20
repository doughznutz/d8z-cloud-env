# Development Environment Containers
# Dashboard is Our entry point into the system.
# To be upgrated to streamlit with actions.
SERVICES += dashboard dozzle
dashboard_RUN_DEPS := dozzle

# Gemini CLI with telemetry sends logs to otel-collector
SERVICES += gemini otel-collector
gemini_RUN_DEPS := otel-collector

# Ollama (openai-API) gateway to all them LLMS, along with a database to store the request/response pairs.
SERVICES += ollama
ollama_RUN_DEPS := ollamadb

# Database containers
# To be removed and replaced with MCP.
SERVICES += adminer ollamadb 
adminer_RUN_DEPS := dashboard ollamadb

# This VNC container has emacs, defines the USER/PASSWORD
# Used as a base image for the other editors.
SERVICES += vnc
vnc_RUN_DEPS := dashboard

# VS codeserver (web) using Continue plug-in Agent.
SERVICES += codeserver
codeserver_BUILD_DEPS := vnc
codeserver_RUN_DEPS := dashboard ollama otel-collector

# Both vscode (vnc) and code-server (web) using Continue plug-in Agent.
# Removed from build as unneeded. 20251025
SERVICES += vscode
vscode_BUILD_DEPS := vnc
vscode_RUN_DEPS := dashboard ollama otel-collector

include deploy/github/github.mk
include deploy/docker/dockerhub.mk

include env/services.mk
