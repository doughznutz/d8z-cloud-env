# Define services incrementally, then call this file
# SERVICES += base
# base_BUILD_DEPS :=
# base_RUN_DEPS := proxy

# Help
.PHONY: help
help:
	@echo "Usage:"
	@echo "  make build-<service>     Build a service with its build-time dependencies"
	@echo "  make up-<service>        Start a service with its runtime dependencies"
	@echo "  make stop-<service>      Stop a service"
	@echo "  make rm-<service>        Stop and remove a service"
	@echo "  make restart-<service>   Restart a service"
	@echo "  make logs-<service>      Follow logs for a service"
	@echo
	@echo "  make build               Build all services"
	@echo "  make up                  Start all services"
	@echo "  make stop                Stop all services"
	@echo "  make rm                  Remove all services"
	@echo "  make restart             Restart all services"
	@echo "  make logs                Show logs for all services"

# Auto-generated service targets
$(foreach service,$(SERVICES), \
  $(eval .PHONY: build-$(service)) \
  $(eval build-$(service): $(addprefix build-,$($(service)_BUILD_DEPS)); \
  /usr/bin/docker compose build $(service)) \
)
$(foreach service,$(SERVICES), \
  $(eval .PHONY: up-$(service)) \
  $(eval up-$(service): $(addprefix up-,$($(service)_RUN_DEPS)) build-$(service) ; \
  docker compose up -d $(service) --remove-orphans) \
)
$(foreach service,$(SERVICES), \
  $(eval .PHONY: stop-$(service)) \
  $(eval stop-$(service): $(addprefix stop-,$($(service)_RUN_DEPS)) ; \
  docker compose stop $(service)) \
)
$(foreach service,$(SERVICES), \
  $(eval .PHONY: rm-$(service)) \
  $(eval rm-$(service): stop-$(service) ; \
  docker compose rm -f $(service) --remove-orphans) \
)
$(foreach service,$(SERVICES), \
  $(eval .PHONY: run-$(service)) \
  $(eval run-$(service): stop-$(service) ; \
  docker compose run $(service)) \
)
$(foreach service,$(SERVICES), \
  $(eval .PHONY: restart-$(service)) \
  $(eval restart-$(service): ; \
  docker compose restart $(service)) \
)
$(foreach service,$(SERVICES), \
  $(eval .PHONY: logs-$(service)) \
  $(eval logs-$(service): ; \
  docker compose logs -f $(service)) \
)

# Multi-service targets
.PHONY: build up stop rm restart logs
build: $(addprefix build-,$(SERVICES))
up: $(addprefix up-,$(SERVICES))
stop: $(addprefix stop-,$(SERVICES))
rm: stop $(addprefix rm-,$(SERVICES))
restart: $(addprefix restart-,$(SERVICES))
logs: $(addprefix logs-,$(SERVICES))

# Docker Hub commands
.PHONY: docker-help
docker-help: build-docker
	docker compose run docker help

.PHONY: docker-check-repo
docker-check-repo: build-docker
	@docker compose run --rm docker check_docker_repo

.PHONY: docker-create-repo
docker-create-repo: build-docker
	@docker compose run --rm docker create_docker_repo

.PHONY: docker-build-and-push
docker-build-and-push: build-docker
	@if [ -z "$(IMAGE_NAME)" ]; then \
		echo "Usage: make docker-build-and-push IMAGE_NAME=<image_name>"; \
		exit 1; \
	fi
	@docker compose run --rm docker build_and_push_image $(IMAGE_NAME)

.PHONY: docker-pull
docker-pull: build-docker
	@if [ -z "$(IMAGE_NAME)" ]; then \
		echo "Usage: make docker-pull IMAGE_NAME=<image_name>"; \
		exit 1; \
	fi
	@docker compose run --rm docker pull_image $(IMAGE_NAME)

.PHONY: docker-list-repos
docker-list-repos: build-docker
	@docker compose run --rm docker list_docker_repos

.PHONY: docker-list-images
docker-list-images: build-docker
	@docker compose run --rm docker list_repo_images

