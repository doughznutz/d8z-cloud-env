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

SERVICES += ollama
ollama_BUILD_DEPS :=
ollama_RUN_DEPS := 

SERVICES += proxy
proxy_BUILD_DEPS := base
proxy_RUN_DEPS := 

SERVICES += voideditor
voideditor_BUILD_DEPS := docker
voideditor_RUN_DEPS := base proxy ollama

SERVICES += vscode
vscode_BUILD_DEPS := docker
vscode_RUN_DEPS := proxy ollama


include docker/docker.mk

