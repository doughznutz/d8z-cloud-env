# Environment Overview

This directory, `env/`, serves as the foundation for the project's containerized tooling. It contains several components that work together to create a consistent and manageable development environment.

## Core Components

*   **`Dockerfile`**: This is a primary, all-in-one Dockerfile that builds a comprehensive development image. It includes a wide array of tools needed for development and operations, such as `git`, `docker`, `docker-compose`, the Google Cloud SDK (`gcloud`), `curl`, `jq`, and `make`.

*   **`entrypoint.sh`**: This script is the main entrypoint for the container built from the root `Dockerfile`. It performs several crucial startup checks and configurations:
    *   `check_docker`: Verifies that the Docker socket is correctly mounted and accessible.
    *   `check_env`: Ensures a `.env` file exists, creating one from a template if needed, and prompts the user for essential variables like `PROJECT`, `GITHUB_REPO`, and `DOCKER_HUB_IMAGE`.
    *   `check_project`: Checks for the existence of the project directory and can clone it from GitHub if it's missing.

*   **`startup/`**: This subdirectory holds template or example files for project initialization.
    *   `example.env`: A template for the `.env` file, listing all the environment variables used across the different services.
    *   `example.dockerignore`: A sample `.dockerignore` file.

## Sub-Environments (MCPs)

The `env/` directory also contains specialized sub-environments, or "Managed Connection Proxy" (MCP) servers, each designed to facilitate interactions with a specific external service.

*   **`cloud/`**: Provides a containerized environment for managing Google Cloud Platform (GCP) resources.
*   **`docker/`**: Contains tools for interacting with Docker Hub to manage repositories and images.
*   **`github/`**: Includes scripts and tools for automating interactions with GitHub repositories.
