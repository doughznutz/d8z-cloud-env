# Development Environment Containers
# Dashboard is Our entry point into the system.
SERVICES += dashboard dozzle
dashboard_RUN_DEPS := dozzle

# Gemini CLI with telemetry sends logs to otel-collector
SERVICES += gemini otel-collector
gemini_RUN_DEPS := otel-collector

# Ollama (openai-API) gateway to all them LLMS, along with a database to store the request/response pairs.
SERVICES += ollama
ollama_BUILD_DEPS :=
ollama_RUN_DEPS := ollamadb otel-collector

# Database containers
SERVICES += adminer ollamadb 
adminer_RUN_DEPS := dashboard ollamadb

# This VNC container has emacs, defines the USER/PASSWORD and is used as a base image for the other editors.
SERVICES += vnc
vnc_BUILD_DEPS := 
vnc_RUN_DEPS := dashboard ollama

# VS codeserver (web) using Continue plug-in Agent.
SERVICES += codeserver
codeserver_BUILD_DEPS := vnc
codeserver_RUN_DEPS := dashboard ollama

# Open Sourced version of vscode with baked-in "void" Agent.
SERVICES += voideditor
voideditor_BUILD_DEPS := vnc
voideditor_RUN_DEPS := dashboard ollama

# Both vscode (vnc) and code-server (web) using Continue plug-in Agent.
SERVICES += vscode
vscode_BUILD_DEPS := vnc
vscode_RUN_DEPS := dashboard ollama

include env/github/github.mk
include env/docker/dockerhub.mk

include env/docker/services.mk
