# Define services incrementally
SERVICES += base
base_BUILD_DEPS :=
base_RUN_DEPS := dashboard

# Docker containers. Dozzle is a log viewer
SERVICES += docker dozzle
docker_BUILD_DEPS :=
dozzle_RUN_DEPS := dashboard

SERVICES += github
github_BUILD_DEPS :=
github_RUN_DEPS :=

# Ollama (openai-API) gateway to all them LLMS, along with a database to store the request/response pairs.
SERVICES += ollama
ollama_BUILD_DEPS :=
ollama_RUN_DEPS := postgres

# Our entry point into the system.
SERVICES += dashboard
dashboard_BUILD_DEPS :=
dashboard_RUN_DEPS :=

# Database containers
SERVICES += adminer postgres
adminer_RUN_DEPS := dashboard postgres

# Editors: VoidEditor (AI:integrated) and VSCode (AI:Continue) and VS code-Server (AI:Continue)  TODO: could combine these 2.
# Open Sourced version of vscode with baked-in "void" Agent.
SERVICES += voideditor
voideditor_BUILD_DEPS := base
voideditor_RUN_DEPS := dashboard ollama

# Vscode (vnc) using Continue plug-in Agent.
SERVICES += vscode
vscode_BUILD_DEPS := base
vscode_RUN_DEPS := dashboard ollama

# Both vscode (vnc) and code-server (web) using Continue plug-in Agent.
SERVICES += vscode-server
vscode_BUILD_DEPS :=
vscode_RUN_DEPS := dashboard ollama

include docker/docker.mk


# Move these into github/github.mk and pick your targets better.
.PHONY: branch
branch:
	docker compose run --build github create_branch clean

.PHONY: pull_request
pull_request:
	docker compose run github create_pull_request $(BRANCH)
