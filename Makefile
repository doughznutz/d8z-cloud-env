# Thin convenience wrapper around docker compose. The traefik gateway's
# console (gw-server) is the control plane from here on: local services,
# projects, child gateways and the Deployment jobs (GitHub, Docker Hub, GCP)
# all live there - see docker-compose.yaml.

COMPOSE := docker compose

.PHONY: up local down build logs server-check

# Root gateway (traefik + dozzle) - the entry point, http://localhost:8080
up:
	$(COMPOSE) up -d --build

# Also start the dev services directly, bypassing the console
local:
	$(COMPOSE) --profile local up -d --build

down:
	$(COMPOSE) --profile local down

build:
	$(COMPOSE) --profile local --profile deploy build

logs:
	$(COMPOSE) logs -f traefik

# Vet + build gw-server locally (no image)
server-check:
	cd dev/traefik/server && go vet ./... && go build -o /dev/null .
