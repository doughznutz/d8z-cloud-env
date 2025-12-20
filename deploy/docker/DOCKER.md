# Docker Environment (Docker Hub)

The `docker/` directory contains a containerized environment designed to facilitate interactions with Docker Hub. It provides a set of tools to manage Docker repositories and images programmatically.

## Components

*   **`Dockerfile`**: Builds an Ubuntu-based container image that includes `docker`, `docker-compose`, `curl`, and `jq`. These tools are used to build images and interact with the Docker Hub API.

*   **`entrypoint.sh`**: The main entrypoint for the `docker` container. It acts as a dispatcher, exposing several commands for Docker Hub operations:
    *   `check_docker_repo`
    *   `create_docker_repo`
    *   `build_and_push_image`
    *   `pull_image`
    *   `list_docker_repos`
    *   `list_repo_images`

*   **`functions.sh`**: Contains the shell functions that implement the commands from the entrypoint. These functions use `curl` to make calls to the Docker Hub v2 API for managing repositories and `docker` CLI commands for building, pushing, and pulling images.

*   **`dockerhub.mk`**: A Makefile that provides convenient shortcuts for running the Docker Hub commands via `docker compose run`. This allows for easy integration into a larger build system. Example targets include `make docker-create-repo` and `make docker-build-and-push`.

*   **`services.mk`**: A generic and reusable Makefile for managing the lifecycle of any `docker-compose` service (e.g., `build`, `up`, `stop`, `rm`).
