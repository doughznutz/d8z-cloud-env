# Define services incrementally
SERVICES += base
base_BUILD_DEPS :=
base_RUN_DEPS := proxy

SERVICES += docker
docker_BUILD_DEPS := base
docker_RUN_DEPS := proxy

SERVICES += github
github_BUILD_DEPS :=
github_RUN_DEPS :=

# Ollama (openai-API) gateway to all them LLMS, along with a database to store the request/response pairs.
SERVICES += ollama postgres adminer
ollama_BUILD_DEPS :=
ollama_RUN_DEPS := postgres adminer

SERVICES += proxy
proxy_BUILD_DEPS := base
proxy_RUN_DEPS := 

# Editors: VoidEditor (AI:integrated) and VSCode (AI:Continue) and VS code-Server (AI:Continue)  TODO: could combine these 2.
# Open Sourced version of vscode with baked-in "void" Agent.
SERVICES += voideditor
voideditor_BUILD_DEPS := docker
voideditor_RUN_DEPS := base proxy ollama

# Both vscode (vnc) and code-server (web) using Continue plug-in Agent.
SERVICES += vscode
vscode_BUILD_DEPS := docker
vscode_RUN_DEPS := proxy ollama


include docker/docker.mk


# Move these into github/github.mk and pick your targets better.
.PHONY: branch
branch:
	docker compose run github create_branch clean

.PHONY: pull_request
pull_request:
	docker compose run github create_pull_request $(BRANCH)
