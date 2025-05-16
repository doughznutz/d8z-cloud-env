# Define services incrementally
SERVICES += base
base_BUILD_DEPS :=
base_RUN_DEPS := proxy

SERVICES += dind
dind_BUILD_DEPS := base
dind_RUN_DEPS := proxy

SERVICES += github
dind_BUILD_DEPS :=
dind_RUN_DEPS :=

SERVICES += ollama
proxy_BUILD_DEPS :=
proxy_RUN_DEPS := 

SERVICES += proxy
proxy_BUILD_DEPS := base
proxy_RUN_DEPS := 

SERVICES += voideditor
voideditor_BUILD_DEPS := dind
voideditor_RUN_DEPS := base proxy ollama


include dind/docker.mk

