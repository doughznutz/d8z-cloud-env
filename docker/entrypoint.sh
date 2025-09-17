#!/bin/bash

# Source the functions
. functions.sh

case "$1" in
    help)
        echo "Usage: $0 [command]"
        echo ""
        echo "Available commands:"
        echo "  check_docker_repo       Check if a Docker Hub repository exists"
        echo "  create_docker_repo      Create a new Docker Hub repository"
        echo "  build_and_push_image    Build and push a Docker image to Docker Hub"
        echo "  pull_image              Pull a Docker image from Docker Hub"
        echo "  list_docker_repos       List Docker Hub repositories for the user"
        echo "  list_repo_images        List images (tags) in a Docker Hub repository"
        echo "  help                    Show this help message"
        ;;
    check_docker_repo)
        check_docker_repo
        ;;
    create_docker_repo)
        create_docker_repo
        ;;
    build_and_push_image)
        if [ -z "$2" ]; then
            echo "Usage: $0 build_and_push_image <image_name>"
            exit 1
        fi
        build_and_push_image "$2"
        ;;
    pull_image)
        if [ -z "$2" ]; then
            echo "Usage: $0 pull_image <image_name>"
            exit 1
        fi
        pull_image "$2"
        ;;
    list_docker_repos)
        list_docker_repos
        ;;
    list_repo_images)
        list_repo_images
        ;;
    *)
        # Default action if no command is provided, execute the command
        exec "$@"
        ;;
esac
