# "base" environment container. 
# docker and github containers will be for testing once merged into base.
# These arent technically services, but are here for simplicity
SERVICES += base docker cloud
base_BUILD_DEPS :=
base_RUN_DEPS :=

SERVICES += gemini otel-collector
gemini_RUN_DEPS := otel-collector

# Development Environment Containers
# Dashboard is Our entry point into the system.
SERVICES += dashboard
dashboard_BUILD_DEPS :=
dashboard_RUN_DEPS :=

SERVICES += dozzle
dozzle_BUILD_DEPS :=
dozzle_RUN_DEPS := dashboard

# Ollama (openai-API) gateway to all them LLMS, along with a database to store the request/response pairs.
SERVICES += ollama
ollama_BUILD_DEPS :=
ollama_RUN_DEPS := ollamadb otel-collector

SERVICES += litellm
litellm_BUILD_DEPS :=
litellm_RUN_DEPS := geminidb


# Database containers
SERVICES += adminer geminidb ollamadb 
adminer_RUN_DEPS := dashboard geminidb ollamadb

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

# Unfortunately order matters here.
include github/github.mk
include docker/docker.mk
