SERVICES += docker

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
	@docker compose run --rm docker build_and_push_image 

.PHONY: docker-pull
docker-pull: build-docker
	@docker compose run --rm docker pull_image 

.PHONY: docker-list-repos
docker-list-repos: build-docker
	@docker compose run --rm docker list_docker_repos

.PHONY: docker-list-images
docker-list-images: build-docker
	@docker compose run --rm docker list_repo_images

